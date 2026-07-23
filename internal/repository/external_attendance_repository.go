package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// External Doris source table names (read-only).
const (
	ExternalSourceAttendanceTable = "dwd_dingtalk_user_attendance_info_di"
	// External column spelling is deparment (not department).
	ExternalSourceDepartmentTable = "dwd_dingtalk_user_deparment_relation_di"
)

// AttendanceSelectColumns is the fixed white-list used by Doris SELECT.
// Tests assert this list includes user_attendance_info_json and excludes invalid aliases.
var AttendanceSelectColumns = []string{
	"user_id",
	"corp_id",
	"corp_name",
	"work_date",
	"record_id",
	"check_type",
	"user_check_time",
	"plan_check_time",
	"plan_id",
	"procInst_id",
	"source_type",
	"time_result",
	"location_result",
	"group_id",
	"user_address",
	"approve_list",
	"check_record_list",
	"attendance_result_list",
	"class_setting_info",
	"user_attendance_info_json",
	"db_update_time",
}

// ErrExternalSyncLocked means another job holds the org/source lock.
var ErrExternalSyncLocked = errors.New("external sync lock held")

// ExternalAttendanceRow is a white-listed Doris attendance projection.
type ExternalAttendanceRow struct {
	UserID               string
	CorpID               string
	CorpName             string
	WorkDate             string
	RecordID             string
	CheckType            string
	UserCheckTime        sql.NullTime
	PlanCheckTime        sql.NullTime
	PlanID               string
	ProcInstID           string
	SourceType           string
	TimeResult           string
	LocationResult       string
	GroupID              string
	UserAddress          string
	ApproveListJSON      string
	CheckRecordListJSON  string
	AttendanceResultJSON string
	ClassSettingInfoJSON string
	UserAttendanceJSON   string
	DBUpdateTime         time.Time
	// PageTieKey is a non-empty stable key for pagination within the same db_update_time.
	PageTieKey string
}

// ExternalDepartmentRow is a white-listed Doris department-relation projection.
type ExternalDepartmentRow struct {
	UserID                   string
	UserName                 string
	CorpName                 string
	Title                    string
	WorkPlace                string
	DepartmentID             string
	DepartmentName           string
	DepartmentLevel          string
	FirstLevelDepartmentID   string
	FirstLevelDepartmentName string
	DBUpdateTime             time.Time
	PageTieKey               string
}

// ExternalAttendanceSourceRepository reads Doris only (SELECT).
type ExternalAttendanceSourceRepository struct {
	db           *sql.DB
	queryTimeout time.Duration
}

func NewExternalAttendanceSourceRepository(db *sql.DB, queryTimeout time.Duration) *ExternalAttendanceSourceRepository {
	if queryTimeout <= 0 {
		queryTimeout = 30 * time.Second
	}
	return &ExternalAttendanceSourceRepository{db: db, queryTimeout: queryTimeout}
}

// BuildAttendancePageTieKey returns a non-empty stable pagination key.
// Prefer record_id when present; otherwise hash composite semantic fields.
func BuildAttendancePageTieKey(userID, workDate, checkType string, userCheckTime sql.NullTime, sourceType, planID, procInstID, recordID string) string {
	if rid := strings.TrimSpace(recordID); rid != "" {
		return "r:" + rid
	}
	uct := ""
	if userCheckTime.Valid {
		uct = userCheckTime.Time.UTC().Format(time.RFC3339Nano)
	}
	return "h:" + hashParts(
		strings.TrimSpace(userID),
		strings.TrimSpace(workDate),
		strings.TrimSpace(checkType),
		uct,
		strings.TrimSpace(sourceType),
		strings.TrimSpace(planID),
		strings.TrimSpace(procInstID),
	)
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// attendancePageTieKeySQL is Doris-side expression for stable pagination.
// Prefer record_id; otherwise SHA2 of semantic fields. Must stay in SQL (not Go over-fetch).
const attendancePageTieKeySQL = `
CASE
  WHEN record_id IS NOT NULL AND CAST(record_id AS CHAR) <> ''
    THEN CONCAT('r:', CAST(record_id AS CHAR))
  ELSE CONCAT(
    'h:',
    SHA2(
      CONCAT_WS(
        '|',
        IFNULL(CAST(user_id AS CHAR), ''),
        IFNULL(CAST(work_date AS CHAR), ''),
        IFNULL(CAST(check_type AS CHAR), ''),
        IFNULL(CAST(user_check_time AS CHAR), ''),
        IFNULL(CAST(source_type AS CHAR), ''),
        IFNULL(CAST(plan_id AS CHAR), ''),
        IFNULL(CAST(procInst_id AS CHAR), '')
      ),
      256
    )
  )
END`

// ListAttendanceSince pages attendance with SQL-side page_tie_key (no Go over-fetch).
// Cursor: (db_update_time, page_tie_key) strict binary comparison.
func (r *ExternalAttendanceSourceRepository) ListAttendanceSince(
	ctx context.Context,
	corpID string,
	from time.Time,
	afterKey string,
	limit int,
) ([]ExternalAttendanceRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("external attendance source db unavailable")
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	// White-list columns only; page_tie_key computed in SQL so large same-timestamp
	// batches (10k+ rows) paginate correctly without missing data.
	q := fmt.Sprintf(`
SELECT
  user_id,
  corp_id,
  corp_name,
  work_date,
  record_id,
  check_type,
  user_check_time,
  plan_check_time,
  plan_id,
  proc_inst_id,
  source_type,
  time_result,
  location_result,
  group_id,
  user_address,
  approve_list,
  check_record_list,
  attendance_result_list,
  class_setting_info,
  user_attendance_info_json,
  db_update_time,
  page_tie_key
FROM (
  SELECT
    CAST(user_id AS CHAR) AS user_id,
    CAST(corp_id AS CHAR) AS corp_id,
    CAST(IFNULL(corp_name, '') AS CHAR) AS corp_name,
    CAST(IFNULL(work_date, '') AS CHAR) AS work_date,
    CAST(IFNULL(record_id, '') AS CHAR) AS record_id,
    CAST(IFNULL(check_type, '') AS CHAR) AS check_type,
    user_check_time,
    plan_check_time,
    CAST(IFNULL(plan_id, '') AS CHAR) AS plan_id,
    CAST(IFNULL(procInst_id, '') AS CHAR) AS proc_inst_id,
    CAST(IFNULL(source_type, '') AS CHAR) AS source_type,
    CAST(IFNULL(time_result, '') AS CHAR) AS time_result,
    CAST(IFNULL(location_result, '') AS CHAR) AS location_result,
    CAST(IFNULL(group_id, '') AS CHAR) AS group_id,
    CAST(IFNULL(user_address, '') AS CHAR) AS user_address,
    CAST(IFNULL(approve_list, '') AS CHAR) AS approve_list,
    CAST(IFNULL(check_record_list, '') AS CHAR) AS check_record_list,
    CAST(IFNULL(attendance_result_list, '') AS CHAR) AS attendance_result_list,
    CAST(IFNULL(class_setting_info, '') AS CHAR) AS class_setting_info,
    CAST(IFNULL(user_attendance_info_json, '') AS CHAR) AS user_attendance_info_json,
    db_update_time,
    `+attendancePageTieKeySQL+` AS page_tie_key
  FROM dwd.dwd_dingtalk_user_attendance_info_di
  WHERE corp_id = ?
) source
WHERE db_update_time > ?
   OR (db_update_time = ? AND page_tie_key > ?)
ORDER BY db_update_time ASC, page_tie_key ASC
LIMIT %d`, limit)

	rows, err := r.db.QueryContext(ctx, q, corpID, from, from, afterKey)
	if err != nil {
		q2 := strings.Replace(q, "FROM dwd.dwd_dingtalk_user_attendance_info_di", "FROM dwd_dingtalk_user_attendance_info_di", 1)
		rows, err = r.db.QueryContext(ctx, q2, corpID, from, from, afterKey)
		if err != nil {
			return nil, fmt.Errorf("query external attendance: %w", err)
		}
	}
	defer func() { _ = rows.Close() }()

	out := make([]ExternalAttendanceRow, 0, limit)
	for rows.Next() {
		var row ExternalAttendanceRow
		if err := rows.Scan(
			&row.UserID,
			&row.CorpID,
			&row.CorpName,
			&row.WorkDate,
			&row.RecordID,
			&row.CheckType,
			&row.UserCheckTime,
			&row.PlanCheckTime,
			&row.PlanID,
			&row.ProcInstID,
			&row.SourceType,
			&row.TimeResult,
			&row.LocationResult,
			&row.GroupID,
			&row.UserAddress,
			&row.ApproveListJSON,
			&row.CheckRecordListJSON,
			&row.AttendanceResultJSON,
			&row.ClassSettingInfoJSON,
			&row.UserAttendanceJSON,
			&row.DBUpdateTime,
			&row.PageTieKey,
		); err != nil {
			return nil, fmt.Errorf("scan external attendance: %w", err)
		}
		if strings.TrimSpace(row.PageTieKey) == "" {
			// Defensive: never advance on empty key
			row.PageTieKey = BuildAttendancePageTieKey(
				row.UserID, row.WorkDate, row.CheckType, row.UserCheckTime,
				row.SourceType, row.PlanID, row.ProcInstID, row.RecordID,
			)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MaxAttendanceUpdateTime returns max(db_update_time) for corp.
func (r *ExternalAttendanceSourceRepository) MaxAttendanceUpdateTime(ctx context.Context, corpID string) (*time.Time, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("external attendance source db unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT MAX(db_update_time)
FROM dwd.dwd_dingtalk_user_attendance_info_di
WHERE corp_id = ?`, corpID).Scan(&t)
	if err != nil {
		err = r.db.QueryRowContext(ctx, `
SELECT MAX(db_update_time)
FROM dwd_dingtalk_user_attendance_info_di
WHERE corp_id = ?`, corpID).Scan(&t)
	}
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

// ListDepartmentsSince pages department-relation rows by corp_name.
func (r *ExternalAttendanceSourceRepository) ListDepartmentsSince(
	ctx context.Context,
	corpName string,
	from time.Time,
	afterKey string,
	limit int,
) ([]ExternalDepartmentRow, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("external attendance source db unavailable")
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	q := fmt.Sprintf(`
SELECT
  CAST(user_id AS CHAR) AS user_id,
  CAST(IFNULL(user_name, '') AS CHAR) AS user_name,
  CAST(IFNULL(corp_name, '') AS CHAR) AS corp_name,
  CAST(IFNULL(title, '') AS CHAR) AS title,
  CAST(IFNULL(work_place, '') AS CHAR) AS work_place,
  CAST(IFNULL(deparment_id, '') AS CHAR) AS department_id,
  CAST(IFNULL(deparment_name, '') AS CHAR) AS department_name,
  CAST(IFNULL(deparment_level, '') AS CHAR) AS department_level,
  CAST(IFNULL(first_level_deparment_id, '') AS CHAR) AS first_level_department_id,
  CAST(IFNULL(first_level_deparment_name, '') AS CHAR) AS first_level_department_name,
  db_update_time
FROM dwd.dwd_dingtalk_user_deparment_relation_di
WHERE corp_name = ?
  AND (
    db_update_time > ?
    OR (db_update_time = ? AND CONCAT(CAST(user_id AS CHAR), ':', CAST(IFNULL(deparment_id, '') AS CHAR)) > ?)
  )
ORDER BY db_update_time ASC, CAST(user_id AS CHAR) ASC, CAST(IFNULL(deparment_id, '') AS CHAR) ASC
LIMIT %d`, limit)

	rows, err := r.db.QueryContext(ctx, q, corpName, from, from, afterKey)
	if err != nil {
		q2 := strings.Replace(q, "FROM dwd.dwd_dingtalk_user_deparment_relation_di", "FROM dwd_dingtalk_user_deparment_relation_di", 1)
		rows, err = r.db.QueryContext(ctx, q2, corpName, from, from, afterKey)
		if err != nil {
			return nil, fmt.Errorf("query external departments: %w", err)
		}
	}
	defer func() { _ = rows.Close() }()

	out := make([]ExternalDepartmentRow, 0, limit)
	for rows.Next() {
		var row ExternalDepartmentRow
		if err := rows.Scan(
			&row.UserID,
			&row.UserName,
			&row.CorpName,
			&row.Title,
			&row.WorkPlace,
			&row.DepartmentID,
			&row.DepartmentName,
			&row.DepartmentLevel,
			&row.FirstLevelDepartmentID,
			&row.FirstLevelDepartmentName,
			&row.DBUpdateTime,
		); err != nil {
			return nil, fmt.Errorf("scan external department: %w", err)
		}
		row.PageTieKey = strings.TrimSpace(row.UserID) + ":" + strings.TrimSpace(row.DepartmentID)
		out = append(out, row)
	}
	return out, rows.Err()
}

// MaxDepartmentUpdateTime returns max(db_update_time) for corp_name.
func (r *ExternalAttendanceSourceRepository) MaxDepartmentUpdateTime(ctx context.Context, corpName string) (*time.Time, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("external attendance source db unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT MAX(db_update_time)
FROM dwd.dwd_dingtalk_user_deparment_relation_di
WHERE corp_name = ?`, corpName).Scan(&t)
	if err != nil {
		err = r.db.QueryRowContext(ctx, `
SELECT MAX(db_update_time)
FROM dwd_dingtalk_user_deparment_relation_di
WHERE corp_name = ?`, corpName).Scan(&t)
	}
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}

// ExternalAttendanceLocalRepository writes staging + local business tables (scoped by org).
type ExternalAttendanceLocalRepository struct {
	db    *gorm.DB
	orgID string
}

// ExternalAttendanceUserProfile is the lightweight roster data used by the
// daily attendance result view. One user may belong to multiple departments;
// the first active relation is used as the table summary, while filtering is
// performed against all active relations.
type ExternalAttendanceUserProfile struct {
	LocalUserID    string `json:"local_user_id"`
	UserName       string `json:"user_name"`
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
}

func NewExternalAttendanceLocalRepository(db *gorm.DB, orgID string) *ExternalAttendanceLocalRepository {
	return &ExternalAttendanceLocalRepository{
		db:    db,
		orgID: database.NormalizeOrganizationID(orgID),
	}
}

func (r *ExternalAttendanceLocalRepository) scoped() *gorm.DB {
	return r.db.Where("org_id = ?", r.orgID)
}

func (r *ExternalAttendanceLocalRepository) GetCursor(sourceTable string) (*database.ExternalSyncCursor, error) {
	var cur database.ExternalSyncCursor
	err := r.scoped().Where("source_table = ?", sourceTable).First(&cur).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

func (r *ExternalAttendanceLocalRepository) SaveCursor(cur *database.ExternalSyncCursor) error {
	if cur == nil {
		return fmt.Errorf("cursor is nil")
	}
	cur.OrgID = r.orgID
	var existing database.ExternalSyncCursor
	err := r.scoped().Where("source_table = ?", cur.SourceTable).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(cur).Error
	}
	if err != nil {
		return err
	}
	cur.ID = existing.ID
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"cursor_time":        cur.CursorTime,
		"cursor_tie_key":     cur.CursorTieKey,
		"last_success_at":    cur.LastSuccessAt,
		"last_error_summary": cur.LastErrorSummary,
	}).Error
}

func (r *ExternalAttendanceLocalRepository) CreateJob(job *database.ExternalSyncJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	job.OrgID = r.orgID
	return r.db.Create(job).Error
}

// HasRunningJob reports whether a non-terminal sync job exists for this org/source.
func (r *ExternalAttendanceLocalRepository) HasRunningJob(source string) (bool, error) {
	q := r.scoped().Model(&database.ExternalSyncJob{}).Where("status = ?", "running")
	if source = strings.TrimSpace(source); source != "" && source != "all" {
		q = q.Where("source = ?", source)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ExternalAttendanceLocalRepository) UpdateJob(job *database.ExternalSyncJob) error {
	if job == nil || job.ID == 0 {
		return fmt.Errorf("job is invalid")
	}
	return r.db.Model(&database.ExternalSyncJob{}).
		Where("id = ? AND org_id = ?", job.ID, r.orgID).
		Updates(map[string]interface{}{
			"status":        job.Status,
			"finished_at":   job.FinishedAt,
			"cursor_from":   job.CursorFrom,
			"cursor_to":     job.CursorTo,
			"inserted":      job.Inserted,
			"updated":       job.Updated,
			"skipped":       job.Skipped,
			"failed":        job.Failed,
			"error_summary": job.ErrorSummary,
		}).Error
}

func (r *ExternalAttendanceLocalRepository) GetJob(id uint) (*database.ExternalSyncJob, error) {
	var job database.ExternalSyncJob
	if err := r.scoped().Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *ExternalAttendanceLocalRepository) ListJobs(page, pageSize int) ([]database.ExternalSyncJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	q := r.scoped().Model(&database.ExternalSyncJob{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var jobs []database.ExternalSyncJob
	err := q.Order("started_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&jobs).Error
	return jobs, total, err
}

// ListAttendanceRawForDaily returns the staging rows used to build daily
// attendance results. The caller enforces a bounded date range.
func (r *ExternalAttendanceLocalRepository) ListAttendanceRawForDaily(startDate, endDate, userID, departmentID string) ([]database.ExternalAttendanceRaw, error) {
	if r.db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	q := r.scoped().Model(&database.ExternalAttendanceRaw{}).
		Where("work_date >= ? AND work_date <= ?", startDate, endDate)

	if userID = strings.TrimSpace(userID); userID != "" {
		rawUserID := userID
		if i := strings.LastIndex(rawUserID, ":"); i >= 0 {
			rawUserID = rawUserID[i+1:]
		}
		q = q.Where("(local_user_id = ? OR external_user_id = ?)", userID, rawUserID)
	}
	if departmentID = strings.TrimSpace(departmentID); departmentID != "" {
		q = q.Where(`local_user_id IN (
			SELECT user_id FROM user_department_relations
			WHERE org_id = ? AND department_id = ? AND is_active = ?
		)`, r.orgID, departmentID, true)
	}

	var rows []database.ExternalAttendanceRaw
	err := q.Order("work_date DESC, local_user_id ASC, source_updated_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *ExternalAttendanceLocalRepository) ListApproveLinksBySourceRowKeys(sourceRowKeys []string) ([]database.ExternalAttendanceApproveLink, error) {
	if r.db == nil || len(sourceRowKeys) == 0 {
		return []database.ExternalAttendanceApproveLink{}, nil
	}
	var links []database.ExternalAttendanceApproveLink
	err := r.scoped().Where("source_row_key IN ?", sourceRowKeys).
		Order("begin_time ASC, id ASC").Find(&links).Error
	return links, err
}

func (r *ExternalAttendanceLocalRepository) LoadAttendanceUserProfiles(localUserIDs []string) (map[string]ExternalAttendanceUserProfile, error) {
	profiles := make(map[string]ExternalAttendanceUserProfile, len(localUserIDs))
	if r.db == nil || len(localUserIDs) == 0 {
		return profiles, nil
	}

	var users []database.User
	if err := r.db.Where("org_id = ? AND user_id IN ? AND deleted_at IS NULL", r.orgID, localUserIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		profiles[user.UserID] = ExternalAttendanceUserProfile{
			LocalUserID:  user.UserID,
			UserName:     strings.TrimSpace(user.Name),
			DepartmentID: strings.TrimSpace(user.DepartmentID),
		}
	}

	var rawProfiles []database.ExternalUserDepartmentRaw
	if err := r.db.Where("org_id = ? AND local_user_id IN ?", r.orgID, localUserIDs).
		Order("source_updated_at DESC, id DESC").Find(&rawProfiles).Error; err != nil {
		return nil, err
	}
	for _, row := range rawProfiles {
		profile := profiles[row.LocalUserID]
		profile.LocalUserID = row.LocalUserID
		if profile.UserName == "" {
			profile.UserName = strings.TrimSpace(row.UserName)
		}
		if profile.DepartmentID == "" {
			profile.DepartmentID = strings.TrimSpace(row.DepartmentID)
		}
		if profile.DepartmentName == "" {
			profile.DepartmentName = strings.TrimSpace(row.DepartmentName)
		}
		profiles[row.LocalUserID] = profile
	}

	var relations []database.UserDepartmentRelation
	if err := r.db.Where("org_id = ? AND user_id IN ? AND is_active = ?", r.orgID, localUserIDs, true).
		Order("id ASC").Find(&relations).Error; err != nil {
		return nil, err
	}
	for _, rel := range relations {
		profile := profiles[rel.UserID]
		profile.LocalUserID = rel.UserID
		if profile.DepartmentID == "" {
			profile.DepartmentID = strings.TrimSpace(rel.DepartmentID)
		}
		if profile.DepartmentName == "" {
			profile.DepartmentName = strings.TrimSpace(rel.DepartmentName)
		}
		profiles[rel.UserID] = profile
	}

	return profiles, nil
}

// ExternalSyncLockScope is the single serial lock key per org for all external sync jobs.
// attendance/department/all all compete for this same key (DB unique on org_id+scope_key).
const ExternalSyncLockScope = "external-attendance"

// AcquireSyncLock acquires a DB-level mutex for the org.
// scope argument is ignored for locking (kept for call-site compatibility); all sources share one lock.
// Correctness relies on unique index (org_id, scope_key) + INSERT, not Count-then-Insert.
func (r *ExternalAttendanceLocalRepository) AcquireSyncLock(scope, owner string, ttl time.Duration) error {
	if r.db == nil {
		return fmt.Errorf("db unavailable")
	}
	_ = scope
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	now := time.Now()
	expires := now.Add(ttl)

	// Cleanup expired locks for this org/key so a crashed owner can be taken over.
	if err := r.db.Where("org_id = ? AND scope_key = ? AND expires_at < ?", r.orgID, ExternalSyncLockScope, now).
		Delete(&database.ExternalSyncLock{}).Error; err != nil {
		return err
	}

	lock := &database.ExternalSyncLock{
		OrgID:     r.orgID,
		ScopeKey:  ExternalSyncLockScope,
		Owner:     owner,
		ExpiresAt: expires,
	}
	if err := r.db.Create(lock).Error; err != nil {
		// unique conflict => another instance holds the lock
		if isExternalSyncDuplicateKey(err) {
			return ErrExternalSyncLocked
		}
		return err
	}
	return nil
}

func isExternalSyncDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "duplicate") ||
		strings.Contains(lower, "unique constraint") ||
		strings.Contains(lower, "1062")
}

// ReleaseSyncLock releases a previously acquired lock owned by owner.
func (r *ExternalAttendanceLocalRepository) ReleaseSyncLock(scope, owner string) error {
	if r.db == nil {
		return nil
	}
	_ = scope
	return r.db.Where("org_id = ? AND scope_key = ? AND owner = ?", r.orgID, ExternalSyncLockScope, owner).
		Delete(&database.ExternalSyncLock{}).Error
}

// UpsertAttendanceRaw inserts or updates staging only when incoming source_updated_at is newer/equal.
func (r *ExternalAttendanceLocalRepository) UpsertAttendanceRaw(raw *database.ExternalAttendanceRaw) (inserted, updated, skipped bool, err error) {
	if raw == nil {
		return false, false, false, fmt.Errorf("raw is nil")
	}
	raw.OrgID = r.orgID
	var existing database.ExternalAttendanceRaw
	err = r.scoped().Where("source_row_key = ?", raw.SourceRowKey).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := r.db.Create(raw).Error; err != nil {
			return false, false, false, err
		}
		return true, false, false, nil
	}
	if err != nil {
		return false, false, false, err
	}
	if raw.SourceUpdatedAt.Before(existing.SourceUpdatedAt) {
		return false, false, true, nil
	}
	raw.ID = existing.ID
	raw.CreatedAt = existing.CreatedAt
	if err := r.db.Save(raw).Error; err != nil {
		return false, false, false, err
	}
	return false, true, false, nil
}

// BuildApproveItemKey builds a non-null unique key for approve_list items.
// Delegates to database.BuildExternalApproveItemKey so runtime and schema migration stay aligned.
func BuildApproveItemKey(procInstID, tagName, bizType, subType, begin, end, duration, durationUnit string) string {
	return database.BuildExternalApproveItemKey(procInstID, tagName, bizType, subType, begin, end, duration, durationUnit)
}

func (r *ExternalAttendanceLocalRepository) UpsertApproveLink(link *database.ExternalAttendanceApproveLink) error {
	if link == nil {
		return fmt.Errorf("link is nil")
	}
	link.OrgID = r.orgID
	if strings.TrimSpace(link.ItemKey) == "" {
		begin := ""
		end := ""
		if link.BeginTime != nil {
			begin = link.BeginTime.UTC().Format(time.RFC3339Nano)
		}
		if link.EndTime != nil {
			end = link.EndTime.UTC().Format(time.RFC3339Nano)
		}
		link.ItemKey = BuildApproveItemKey(
			link.ProcInstID, link.TagName, link.BizType, link.SubType,
			begin, end, link.Duration, link.DurationUnit,
		)
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "org_id"},
			{Name: "source_row_key"},
			{Name: "item_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"proc_inst_id", "tag_name", "biz_type", "sub_type",
			"begin_time", "end_time", "duration", "duration_unit",
			"gmt_finished", "raw_item_json", "updated_at",
		}),
	}).Create(link).Error
}

func (r *ExternalAttendanceLocalRepository) UpsertDepartmentRaw(raw *database.ExternalUserDepartmentRaw) (inserted, updated, skipped bool, err error) {
	if raw == nil {
		return false, false, false, fmt.Errorf("raw is nil")
	}
	raw.OrgID = r.orgID
	var existing database.ExternalUserDepartmentRaw
	err = r.scoped().Where("source_row_key = ?", raw.SourceRowKey).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := r.db.Create(raw).Error; err != nil {
			return false, false, false, err
		}
		return true, false, false, nil
	}
	if err != nil {
		return false, false, false, err
	}
	if raw.SourceUpdatedAt.Before(existing.SourceUpdatedAt) {
		return false, false, true, nil
	}
	raw.ID = existing.ID
	raw.CreatedAt = existing.CreatedAt
	if err := r.db.Save(raw).Error; err != nil {
		return false, false, false, err
	}
	return false, true, false, nil
}

func (r *ExternalAttendanceLocalRepository) UpsertDepartmentRelation(rel *database.UserDepartmentRelation) error {
	if rel == nil {
		return fmt.Errorf("relation is nil")
	}
	rel.OrgID = r.orgID
	rel.IsActive = true
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "org_id"},
			{Name: "user_id"},
			{Name: "department_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"external_user_id", "department_name", "department_level",
			"first_level_department_id", "first_level_department_name",
			"title", "work_place", "is_active", "source_updated_at",
			"last_seen_snapshot_id", "last_seen_at", "updated_at",
		}),
	}).Create(rel).Error
}

// DeactivateMissingDepartmentRelations soft-deactivates relations not seen in a completed full snapshot.
func (r *ExternalAttendanceLocalRepository) DeactivateMissingDepartmentRelations(snapshotID string, seenAt time.Time) (int64, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return 0, fmt.Errorf("snapshot_id required for department deactivation")
	}
	res := r.db.Model(&database.UserDepartmentRelation{}).
		Where("org_id = ? AND is_active = ? AND (last_seen_snapshot_id IS NULL OR last_seen_snapshot_id <> ?)", r.orgID, true, snapshotID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"updated_at": seenAt,
		})
	return res.RowsAffected, res.Error
}

// LookupUserName returns a local user name for localUserID within the org, if any.
func (r *ExternalAttendanceLocalRepository) LookupUserName(localUserID string) string {
	if r.db == nil || strings.TrimSpace(localUserID) == "" {
		return ""
	}
	var user database.User
	// Try scoped user_id first, then ding_talk_user_id / raw external suffix.
	err := r.db.Where("org_id = ? AND user_id = ?", r.orgID, localUserID).First(&user).Error
	if err == nil && strings.TrimSpace(user.Name) != "" {
		return user.Name
	}
	raw := localUserID
	if i := strings.LastIndex(localUserID, ":"); i >= 0 {
		raw = localUserID[i+1:]
	}
	if err := r.db.Where("org_id = ? AND (user_id = ? OR ding_talk_user_id = ?)", r.orgID, raw, raw).First(&user).Error; err == nil {
		return strings.TrimSpace(user.Name)
	}
	return ""
}

// UpsertBusinessAttendance merges external punch into local attendances without wiping existing fields.
func (r *ExternalAttendanceLocalRepository) UpsertBusinessAttendance(att *database.Attendance) error {
	if att == nil {
		return fmt.Errorf("attendance is nil")
	}
	att.OrgID = r.orgID
	var existing database.Attendance
	err := r.db.Where(
		"org_id = ? AND user_id = ? AND check_time = ? AND check_type = ?",
		r.orgID, att.UserID, att.CheckTime, att.CheckType,
	).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if strings.TrimSpace(att.UserName) == "" {
			att.UserName = r.LookupUserName(att.UserID)
		}
		if strings.TrimSpace(att.UserName) == "" {
			att.UserName = att.UserID
		}
		return r.db.Create(att).Error
	}
	if err != nil {
		return err
	}

	// Preserve non-empty existing fields; merge Extension.
	if strings.TrimSpace(att.UserName) == "" {
		att.UserName = existing.UserName
	}
	if strings.TrimSpace(att.Location) == "" {
		att.Location = existing.Location
	}
	att.Extension = mergeExtension(existing.Extension, att.Extension)
	att.ID = existing.ID
	att.CreatedAt = existing.CreatedAt
	att.DeletedAt = existing.DeletedAt
	update := &database.Attendance{
		UserName:  att.UserName,
		Location:  att.Location,
		Extension: att.Extension,
		UpdatedAt: time.Now(),
	}
	return r.db.Model(&existing).
		Select("user_name", "location", "extension", "updated_at").
		Updates(update).Error
}

func mergeExtension(base, patch map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func (r *ExternalAttendanceLocalRepository) ListCursors() ([]database.ExternalSyncCursor, error) {
	var list []database.ExternalSyncCursor
	err := r.scoped().Find(&list).Error
	return list, err
}

func (r *ExternalAttendanceLocalRepository) LatestJob() (*database.ExternalSyncJob, error) {
	var job database.ExternalSyncJob
	err := r.scoped().Order("started_at DESC").First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}
