package repository

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"peopleops/internal/database"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// In-memory event-log backend exercising real DingTalkEventRepository code paths
// (insert conflict, transaction, SELECT FOR UPDATE, CAS Updates + RowsAffected).

const eventLogDriverName = "peopleops_event_log_mem_mysql"

var (
	eventLogDriverOnce sync.Once
	eventLogBackends   sync.Map // dsn -> *eventLogBackend
)

type eventLogRow struct {
	ID                int64
	OrgID             string
	EventID           string
	EventType         string
	ProcessInstanceID string
	ProcessCode       string
	ChangeType        string
	Result            string
	Status            string
	ErrorMessage      string
	EventBornTime     int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type eventLogBackend struct {
	mu     sync.Mutex
	seq    int64
	byKey  map[string]*eventLogRow // org|event
	locked map[string]struct{}
	now    time.Time
}

func newEventLogBackend(now time.Time) *eventLogBackend {
	return &eventLogBackend{
		byKey:  make(map[string]*eventLogRow),
		locked: make(map[string]struct{}),
		now:    now,
	}
}

func (b *eventLogBackend) key(orgID, eventID string) string {
	return orgID + "|" + eventID
}

type eventLogDriver struct{}

func (eventLogDriver) Open(name string) (driver.Conn, error) {
	v, ok := eventLogBackends.Load(name)
	if !ok {
		return nil, fmt.Errorf("backend %s not found", name)
	}
	return &eventLogConn{b: v.(*eventLogBackend)}, nil
}

type eventLogConn struct {
	b  *eventLogBackend
	tx *eventLogTx
}

func (c *eventLogConn) Prepare(query string) (driver.Stmt, error) {
	return &eventLogStmt{c: c, query: query}, nil
}
func (c *eventLogConn) Close() error { return nil }
func (c *eventLogConn) Begin() (driver.Tx, error) {
	c.b.mu.Lock()
	c.tx = &eventLogTx{b: c.b, conn: c}
	return c.tx, nil
}
func (c *eventLogConn) BeginTx(_ any, _ driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}

type eventLogTx struct {
	b    *eventLogBackend
	conn *eventLogConn
}

func (t *eventLogTx) Commit() error {
	t.b.mu.Unlock()
	t.conn.tx = nil
	return nil
}
func (t *eventLogTx) Rollback() error {
	t.b.mu.Unlock()
	t.conn.tx = nil
	return nil
}

type eventLogStmt struct {
	c     *eventLogConn
	query string
}

func (s *eventLogStmt) Close() error  { return nil }
func (s *eventLogStmt) NumInput() int { return -1 }

func (s *eventLogStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.exec(named(args))
}
func (s *eventLogStmt) ExecContext(_ any, args []driver.NamedValue) (driver.Result, error) {
	return s.exec(args)
}
func (s *eventLogStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.queryRows(named(args))
}
func (s *eventLogStmt) QueryContext(_ any, args []driver.NamedValue) (driver.Rows, error) {
	return s.queryRows(args)
}

func named(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

func sanitizeArgs(args []driver.NamedValue) ([]driver.NamedValue, error) {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = a
		if v, ok := a.Value.(map[string]interface{}); ok {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			out[i].Value = b
		}
	}
	return out, nil
}

func (s *eventLogStmt) exec(args []driver.NamedValue) (driver.Result, error) {
	args, err := sanitizeArgs(args)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(s.query)
	b := s.c.b
	inTx := s.c.tx != nil
	if !inTx {
		b.mu.Lock()
		defer b.mu.Unlock()
	}

	switch {
	case strings.Contains(q, "insert into"):
		// GORM Create with OnConflict DoNothing.
		// Extract fields from args loosely by position from common GORM order is fragile;
		// parse named values from SQL placeholders by scanning SET-like INSERT column list.
		orgID, eventID, status, eventType, processInstanceID, processCode, changeType, result, born := extractInsertFields(s.query, args)
		if orgID == "" || eventID == "" {
			return driver.RowsAffected(0), fmt.Errorf("insert missing org/event")
		}
		k := b.key(orgID, eventID)
		if _, exists := b.byKey[k]; exists {
			// ON CONFLICT DO NOTHING
			return driver.RowsAffected(0), nil
		}
		b.seq++
		now := b.now
		row := &eventLogRow{
			ID:                b.seq,
			OrgID:             orgID,
			EventID:           eventID,
			EventType:         eventType,
			ProcessInstanceID: processInstanceID,
			ProcessCode:       processCode,
			ChangeType:        changeType,
			Result:            result,
			Status:            status,
			EventBornTime:     born,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if row.Status == "" {
			row.Status = DingTalkEventStatusProcessing
		}
		b.byKey[k] = row
		return eventLogResult{lastID: row.ID, rows: 1}, nil

	case strings.HasPrefix(strings.TrimSpace(q), "update"):
		// CAS update: WHERE id=? AND status IN (?) [AND updated_at <= ?]
		id, statuses, staleBefore, fields := extractUpdate(s.query, args)
		var target *eventLogRow
		for _, row := range b.byKey {
			if row.ID == id {
				target = row
				break
			}
		}
		if target == nil {
			return driver.RowsAffected(0), nil
		}
		statusOK := false
		for _, st := range statuses {
			if target.Status == st {
				statusOK = true
				break
			}
		}
		if !statusOK {
			return driver.RowsAffected(0), nil
		}
		if staleBefore != nil && target.UpdatedAt.After(*staleBefore) {
			return driver.RowsAffected(0), nil
		}
		if v, ok := fields["status"]; ok {
			target.Status = fmt.Sprint(v)
		}
		if v, ok := fields["error_message"]; ok {
			target.ErrorMessage = fmt.Sprint(v)
		}
		if v, ok := fields["event_type"]; ok {
			target.EventType = fmt.Sprint(v)
		}
		if v, ok := fields["process_instance_id"]; ok {
			target.ProcessInstanceID = fmt.Sprint(v)
		}
		if v, ok := fields["process_code"]; ok {
			target.ProcessCode = fmt.Sprint(v)
		}
		if v, ok := fields["change_type"]; ok {
			target.ChangeType = fmt.Sprint(v)
		}
		if v, ok := fields["result"]; ok {
			target.Result = fmt.Sprint(v)
		}
		if v, ok := fields["updated_at"]; ok {
			if t, ok2 := v.(time.Time); ok2 {
				target.UpdatedAt = t
			} else {
				target.UpdatedAt = b.now
			}
		} else {
			target.UpdatedAt = b.now
		}
		return driver.RowsAffected(1), nil
	default:
		return driver.RowsAffected(0), nil
	}
}

func (s *eventLogStmt) queryRows(args []driver.NamedValue) (driver.Rows, error) {
	args, err := sanitizeArgs(args)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(s.query)
	b := s.c.b
	inTx := s.c.tx != nil
	if !inTx {
		b.mu.Lock()
		defer b.mu.Unlock()
	}

	// First by org_id + event_id
	if strings.Contains(q, "from `ding_talk_event_logs`") || strings.Contains(q, "from ding_talk_event_logs") || strings.Contains(q, "ding_talk_event_logs") {
		orgID, eventID := "", ""
		// GORM typically binds org then event
		if len(args) >= 2 {
			orgID = fmt.Sprint(args[0].Value)
			eventID = fmt.Sprint(args[1].Value)
		}
		row := b.byKey[b.key(orgID, eventID)]
		if row == nil {
			return &eventLogRows{columns: eventLogColumns(), index: 0}, nil
		}
		return &eventLogRows{
			columns: eventLogColumns(),
			rows:    [][]driver.Value{eventLogValues(row)},
		}, nil
	}
	return &eventLogRows{columns: []string{"1"}, rows: [][]driver.Value{{int64(1)}}}, nil
}

func eventLogColumns() []string {
	return []string{
		"id", "org_id", "event_id", "event_type", "process_instance_id", "process_code",
		"change_type", "result", "status", "error_message", "event_born_time",
		"payload_summary", "created_at", "updated_at",
	}
}

func eventLogValues(row *eventLogRow) []driver.Value {
	return []driver.Value{
		row.ID, row.OrgID, row.EventID, row.EventType, row.ProcessInstanceID, row.ProcessCode,
		row.ChangeType, row.Result, row.Status, row.ErrorMessage, row.EventBornTime,
		nil, row.CreatedAt, row.UpdatedAt,
	}
}

type eventLogRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *eventLogRows) Columns() []string { return r.columns }
func (r *eventLogRows) Close() error      { return nil }
func (r *eventLogRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

type eventLogResult struct {
	lastID int64
	rows   int64
}

func (r eventLogResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r eventLogResult) RowsAffected() (int64, error) { return r.rows, nil }

func extractInsertFields(query string, args []driver.NamedValue) (orgID, eventID, status, eventType, processInstanceID, processCode, changeType, result string, born int64) {
	// Prefer scanning column list and aligning args.
	lower := strings.ToLower(query)
	colsPart := ""
	if i := strings.Index(lower, "("); i >= 0 {
		if j := strings.Index(lower[i:], ")"); j >= 0 {
			colsPart = lower[i+1 : i+j]
		}
	}
	cols := splitSQLIdentList(colsPart)
	vals := make(map[string]interface{})
	for i, col := range cols {
		if i >= len(args) {
			break
		}
		vals[strings.Trim(col, "` ")] = args[i].Value
	}
	orgID = asString(vals["org_id"])
	eventID = asString(vals["event_id"])
	status = asString(vals["status"])
	eventType = asString(vals["event_type"])
	processInstanceID = asString(vals["process_instance_id"])
	processCode = asString(vals["process_code"])
	changeType = asString(vals["change_type"])
	result = asString(vals["result"])
	if v, ok := vals["event_born_time"]; ok {
		switch t := v.(type) {
		case int64:
			born = t
		case int:
			born = int64(t)
		}
	}
	return
}

func extractUpdate(query string, args []driver.NamedValue) (id int64, statuses []string, staleBefore *time.Time, fields map[string]interface{}) {
	fields = map[string]interface{}{}
	lower := strings.ToLower(query)
	// Collect SET assignments roughly by scanning `col`=? patterns in order.
	setCols := []string{}
	rest := lower
	for {
		i := strings.Index(rest, "`")
		if i < 0 {
			break
		}
		rest = rest[i+1:]
		j := strings.Index(rest, "`")
		if j < 0 {
			break
		}
		col := rest[:j]
		rest = rest[j+1:]
		if strings.HasPrefix(strings.TrimSpace(rest), "=") {
			setCols = append(setCols, col)
		}
	}
	// WHERE id = ? usually last before status IN
	// We'll map args sequentially to set cols, remaining to where.
	idx := 0
	for _, col := range setCols {
		if idx >= len(args) {
			break
		}
		// Skip where-like columns if they appear after SET incorrectly.
		if col == "id" {
			continue
		}
		fields[col] = args[idx].Value
		idx++
	}
	// Remaining args: id, status(es), optional stale time.
	if idx < len(args) {
		switch v := args[idx].Value.(type) {
		case int64:
			id = v
		case int:
			id = int64(v)
		case uint64:
			id = int64(v)
		}
		idx++
	}
	// status IN (?) may be one arg or expanded
	for idx < len(args) {
		v := args[idx].Value
		idx++
		switch t := v.(type) {
		case string:
			// could be status or later
			if t == DingTalkEventStatusProcessing || t == DingTalkEventStatusFailed || t == DingTalkEventStatusSuccess || t == DingTalkEventStatusSkipped {
				statuses = append(statuses, t)
				continue
			}
			// unknown string may still be status
			if len(statuses) == 0 {
				statuses = append(statuses, t)
			}
		case time.Time:
			tt := t
			staleBefore = &tt
		case []string:
			statuses = append(statuses, t...)
		}
	}
	if len(statuses) == 0 {
		// default: if SET status=processing reclaim, allow failed/processing from fields context
		statuses = []string{DingTalkEventStatusProcessing, DingTalkEventStatusFailed}
	}
	return
}

func splitSQLIdentList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func openEventLogRepo(t *testing.T, now time.Time) (*DingTalkEventRepository, *eventLogBackend) {
	t.Helper()
	eventLogDriverOnce.Do(func() {
		sql.Register(eventLogDriverName, eventLogDriver{})
	})
	backend := newEventLogBackend(now)
	dsn := fmt.Sprintf("event-log-%s-%d", t.Name(), time.Now().UnixNano())
	eventLogBackends.Store(dsn, backend)
	t.Cleanup(func() { eventLogBackends.Delete(dsn) })

	sqlDB, err := sql.Open(eventLogDriverName, dsn)
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	// Ensure GORM uses table name matching our stub expectations.
	_ = db.Table("ding_talk_event_logs")

	repo := NewDingTalkEventRepositoryWithOrgID(db, "org-a")
	repo.nowFn = func() time.Time { return backend.now }
	return repo, backend
}

func sampleLog(eventID string) *database.DingTalkEventLog {
	return &database.DingTalkEventLog{
		OrgID:             "org-a",
		EventID:           eventID,
		EventType:         "bpms_instance_change",
		ProcessInstanceID: "pi-1",
		ProcessCode:       "PC",
		ChangeType:        "start",
		Status:            DingTalkEventStatusProcessing,
	}
}

func TestDingTalkEventRepo_FirstClaimSuccess(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	log := sampleLog("evt-1")
	if err := repo.TryBeginProcessing(log); err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if log.ID == 0 {
		// ID may be filled by GORM from LastInsertId; also accept backend presence.
		if _, ok := backend.byKey[backend.key("org-a", "evt-1")]; !ok {
			t.Fatal("row not inserted")
		}
	}
	row := backend.byKey[backend.key("org-a", "evt-1")]
	if row == nil || row.Status != DingTalkEventStatusProcessing {
		t.Fatalf("row=%+v", row)
	}
}

func TestDingTalkEventRepo_AlreadyProcessedIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	backend.seq = 1
	backend.byKey[backend.key("org-a", "evt-ok")] = &eventLogRow{
		ID: 1, OrgID: "org-a", EventID: "evt-ok", Status: DingTalkEventStatusSuccess,
		CreatedAt: now, UpdatedAt: now,
	}
	err := repo.TryBeginProcessing(sampleLog("evt-ok"))
	if !errors.Is(err, ErrDingTalkEventAlreadyProcessed) {
		t.Fatalf("err=%v want AlreadyProcessed", err)
	}
}

func TestDingTalkEventRepo_FreshProcessingInProgress(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	backend.seq = 1
	backend.byKey[backend.key("org-a", "evt-busy")] = &eventLogRow{
		ID: 1, OrgID: "org-a", EventID: "evt-busy", Status: DingTalkEventStatusProcessing,
		CreatedAt: now, UpdatedAt: now.Add(-1 * time.Minute),
	}
	err := repo.TryBeginProcessing(sampleLog("evt-busy"))
	if !errors.Is(err, ErrDingTalkEventInProgress) {
		t.Fatalf("err=%v want InProgress", err)
	}
}

func TestDingTalkEventRepo_FailedRetryable(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	backend.seq = 1
	backend.byKey[backend.key("org-a", "evt-fail")] = &eventLogRow{
		ID: 1, OrgID: "org-a", EventID: "evt-fail", Status: DingTalkEventStatusFailed,
		ErrorMessage: "boom", CreatedAt: now, UpdatedAt: now.Add(-10 * time.Minute),
	}
	if err := repo.TryBeginProcessing(sampleLog("evt-fail")); err != nil {
		t.Fatalf("reclaim failed event: %v", err)
	}
	row := backend.byKey[backend.key("org-a", "evt-fail")]
	if row.Status != DingTalkEventStatusProcessing {
		t.Fatalf("status=%s", row.Status)
	}
	if row.ErrorMessage != "" {
		t.Fatalf("error should be cleared, got %q", row.ErrorMessage)
	}
}

func TestDingTalkEventRepo_StaleProcessingReclaim(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	backend.seq = 1
	backend.byKey[backend.key("org-a", "evt-stale")] = &eventLogRow{
		ID: 1, OrgID: "org-a", EventID: "evt-stale", Status: DingTalkEventStatusProcessing,
		CreatedAt: now.Add(-20 * time.Minute), UpdatedAt: now.Add(-10 * time.Minute),
	}
	if err := repo.TryBeginProcessing(sampleLog("evt-stale")); err != nil {
		t.Fatalf("reclaim stale: %v", err)
	}
	row := backend.byKey[backend.key("org-a", "evt-stale")]
	if row.Status != DingTalkEventStatusProcessing {
		t.Fatalf("status=%s", row.Status)
	}
	if !row.UpdatedAt.Equal(now) && row.UpdatedAt.Before(now.Add(-time.Second)) {
		// updated_at should be refreshed by reclaim
		t.Fatalf("updated_at not refreshed: %v", row.UpdatedAt)
	}
}

func TestDingTalkEventRepo_ConcurrentReclaimOnlyOneWins(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repo, backend := openEventLogRepo(t, now)
	backend.seq = 1
	backend.byKey[backend.key("org-a", "evt-race")] = &eventLogRow{
		ID: 1, OrgID: "org-a", EventID: "evt-race", Status: DingTalkEventStatusProcessing,
		CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-20 * time.Minute),
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		success  int
		inProg   int
		otherErr int
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.TryBeginProcessing(sampleLog("evt-race"))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, ErrDingTalkEventInProgress):
				inProg++
			default:
				otherErr++
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("success=%d want 1 (inProg=%d other=%d)", success, inProg, otherErr)
	}
	if success+inProg+otherErr != 8 {
		t.Fatalf("unexpected counts success=%d inProg=%d other=%d", success, inProg, otherErr)
	}
}
