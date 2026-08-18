package repository

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestApprovalFindAllFiltersByProcessCode(t *testing.T) {
	dsn := fmt.Sprintf("file:approval-template-filter-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(&database.Approval{}); err != nil {
		t.Fatalf("migrate approvals: %v", err)
	}
	now := time.Now()
	approvals := []database.Approval{
		{OrgID: "org-a", ProcessID: "a-1", Title: "请假审批", ApplicantID: "u1", ApplicantName: "甲", Status: "RUNNING", CreateTime: now, Extension: map[string]interface{}{"process_code": "PROC-A"}},
		{OrgID: "org-a", ProcessID: "b-1", Title: "加班审批", ApplicantID: "u2", ApplicantName: "乙", Status: "RUNNING", CreateTime: now, Extension: map[string]interface{}{"process_code": "PROC-B"}},
	}
	if err := db.Create(&approvals).Error; err != nil {
		t.Fatalf("create approvals: %v", err)
	}

	items, total, err := NewApprovalRepositoryWithOrgID(db, "org-a").FindAll(1, 10, map[string]string{"template_id": "PROC-A"})
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ProcessID != "a-1" {
		t.Fatalf("filtered approvals total=%d items=%#v", total, items)
	}
}

func TestMergeApprovalExtensionAppliesPatchWithoutDroppingExistingFields(t *testing.T) {
	base := map[string]interface{}{
		"local_match_ref": "match-1",
		"result":          "RUNNING",
	}
	patch := map[string]interface{}{
		"result":       "agree",
		"process_code": "PROC-1",
	}

	merged := mergeApprovalExtension(base, patch)
	if merged["local_match_ref"] != "match-1" {
		t.Fatalf("local field was dropped: %#v", merged)
	}
	if merged["result"] != "agree" {
		t.Fatalf("patched result = %#v, want agree", merged["result"])
	}
	if merged["process_code"] != "PROC-1" {
		t.Fatalf("process_code = %#v, want PROC-1", merged["process_code"])
	}
	if base["result"] != "RUNNING" {
		t.Fatalf("base map was mutated: %#v", base)
	}
}

func TestApprovalUpsertLookupUsesOrgAndProcessID(t *testing.T) {
	matched := false
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			normalized := strings.ToLower(query)
			if !strings.Contains(normalized, "org_id = ?") || !strings.Contains(normalized, "process_id = ?") {
				return false
			}
			if len(args) < 2 || args[0].Value != "org-a" || args[1].Value != "process-1" {
				return false
			}
			matched = true
			return true
		},
		columns: []string{
			"id", "org_id", "process_id", "title", "applicant_id", "applicant_name",
			"status", "create_time", "finish_time", "content", "extension", "created_at", "updated_at", "deleted_at",
		},
		rows: [][]driver.Value{{
			uint64(1), "org-a", "process-1", "old title", "u1", "User One",
			"RUNNING", now, now, `{}`, `{"local_match_ref":"match-1"}`, now, now, nil,
		}},
	})
	repo := NewApprovalRepositoryWithOrgID(db, "org-a")

	err := repo.UpsertByOrgProcessID(&database.Approval{
		OrgID:     "org-a",
		ProcessID: "process-1",
		Status:    "COMPLETED",
		Extension: map[string]interface{}{"result": "agree"},
	})
	if err != nil {
		t.Fatalf("UpsertByOrgProcessID() error = %v", err)
	}
	if !matched {
		t.Fatal("approval lookup did not include both org_id and process_id")
	}
}

func TestApprovalUpsertRejectsCrossOrgRecord(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewApprovalRepositoryWithOrgID(db, "org-a")

	err := repo.UpsertByOrgProcessID(&database.Approval{OrgID: "org-b", ProcessID: "process-1"})
	if err != ErrOrgMismatch {
		t.Fatalf("err = %v, want ErrOrgMismatch", err)
	}
}

func TestApprovalUpsertUpdatesStreamRecordWithoutCrossOrgDuplication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:approval-upsert-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Approval{}); err != nil {
		t.Fatalf("migrate approvals: %v", err)
	}
	existing := database.Approval{
		OrgID: "org-a", ProcessID: "stream-instance-1", Title: "加班审批", ApplicantID: "u1", ApplicantName: "员工甲",
		Status: "RUNNING", CreateTime: time.Now(),
		Extension: map[string]interface{}{"source": "dingtalk_stream", "stream_event_id": "event-1"},
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create stream approval: %v", err)
	}

	repo := NewApprovalRepositoryWithOrgID(db, "org-a")
	if err := repo.UpsertByOrgProcessID(&database.Approval{
		OrgID: "org-a", ProcessID: "stream-instance-1", ApplicantID: "u1", ApplicantName: "u1", Status: "COMPLETED", FinishTime: time.Now(),
		Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-OVERTIME", "source": "dingtalk_sync"},
	}); err != nil {
		t.Fatalf("upsert full-sync approval: %v", err)
	}
	if err := NewApprovalRepositoryWithOrgID(db, "org-b").UpsertByOrgProcessID(&database.Approval{
		OrgID: "org-b", ProcessID: "stream-instance-1", Title: "补卡审批", ApplicantID: "u2", ApplicantName: "员工乙", Status: "COMPLETED", CreateTime: time.Now(),
	}); err != nil {
		t.Fatalf("create same process id in another org: %v", err)
	}

	var orgACount int64
	if err := db.Model(&database.Approval{}).Where("org_id = ? AND process_id = ?", "org-a", "stream-instance-1").Count(&orgACount).Error; err != nil {
		t.Fatalf("count org-a approval: %v", err)
	}
	if orgACount != 1 {
		t.Fatalf("org-a approval count = %d, want 1", orgACount)
	}
	var updated database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-a", "stream-instance-1").First(&updated).Error; err != nil {
		t.Fatalf("load updated approval: %v", err)
	}
	if updated.ID != existing.ID || updated.Status != "COMPLETED" {
		t.Fatalf("updated approval = %#v", updated)
	}
	if updated.ApplicantName != "员工甲" {
		t.Fatalf("fallback user_id overwrote real applicant name: %q", updated.ApplicantName)
	}
	if updated.Extension["stream_event_id"] != "event-1" || updated.Extension["result"] != "agree" || updated.Extension["process_code"] != "PROC-OVERTIME" || updated.Extension["source"] != "dingtalk_sync" {
		t.Fatalf("updated extension = %#v", updated.Extension)
	}
}

// TestApprovalFindAllDateFilterUsesUTC8Location 断言 FindAll 的 start_date/end_date 使用 UTC+8 时区解析，
// 避免在 TZ=UTC 环境下日期偏移一天导致查询结果不正确。
func TestApprovalFindAllDateFilterUsesUTC8Location(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:approval-tz-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Approval{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	loc := dingtalk.ApprovalBusinessLocation()
	// Create an approval at 2026-08-05 10:00 UTC+8
	approvalTime := time.Date(2026, 8, 5, 10, 0, 0, 0, loc)
	if err := db.Create(&database.Approval{
		OrgID: "org-a", ProcessID: "tz-filter-1", Title: "测试", ApplicantID: "u1",
		ApplicantName: "用户", Status: "COMPLETED", CreateTime: approvalTime,
	}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	repo := NewApprovalRepositoryWithOrgID(db, "org-a")

	// Filter start_date=2026-08-05 should match (record at 10:00 UTC+8 >= 00:00 UTC+8)
	results, total, err := repo.FindAll(1, 10, map[string]string{"start_date": "2026-08-05"})
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("start_date=2026-08-05: total=%d len=%d, want 1 (record at 10:00 UTC+8)", total, len(results))
	}

	// Filter end_date=2026-08-04 should NOT match (record at 2026-08-05 > 2026-08-04 + 1 day)
	_, total, err = repo.FindAll(1, 10, map[string]string{"end_date": "2026-08-04"})
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 0 {
		t.Fatalf("end_date=2026-08-04: total=%d, want 0 (record is 2026-08-05)", total)
	}

	// Filter end_date=2026-08-05 should match
	_, total, err = repo.FindAll(1, 10, map[string]string{"end_date": "2026-08-05"})
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 1 {
		t.Fatalf("end_date=2026-08-05: total=%d, want 1", total)
	}
}

// TestApprovalFindAllTitleFilterGeneratesLike 断言 FindAll 在传入 title 过滤时，
// 生成 SQL 中包含 title LIKE 子句，且 org_id 过滤仍存在。
func TestApprovalFindAllTitleFilterGeneratesLike(t *testing.T) {
	db := newDryRunGORM(t)

	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	var sql string
	var vars []interface{}
	name := "approval-title-sql-" + t.Name()
	_ = session.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
		vars = append([]interface{}{}, tx.Statement.Vars...)
	})
	t.Cleanup(func() {
		_ = session.Callback().Query().Remove(name)
	})

	scoped := NewApprovalRepositoryWithOrgID(session, "muteng")
	_, _, _ = scoped.FindAll(1, 10, map[string]string{"title": "请假"})

	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "title like") {
		t.Fatalf("expected 'title LIKE ?' in SQL, got %s", sql)
	}
	if !strings.Contains(lower, "org_id = ?") {
		t.Fatalf("expected org_id filter to remain in SQL, got %s", sql)
	}
	found := false
	for _, v := range vars {
		if s, ok := v.(string); ok && s == "%请假%" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %%请假%% in vars, got %#v", vars)
	}
}

// TestApprovalFindAllEmptyTitleDoesNotFilter 断言 title 为空时不附加 LIKE 条件，
// 避免误把整张表过滤成空。
func TestApprovalFindAllEmptyTitleDoesNotFilter(t *testing.T) {
	db := newDryRunGORM(t)

	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	var sql string
	name := "approval-title-empty-" + t.Name()
	_ = session.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
	})
	t.Cleanup(func() {
		_ = session.Callback().Query().Remove(name)
	})

	scoped := NewApprovalRepositoryWithOrgID(session, "muteng")
	_, _, _ = scoped.FindAll(1, 10, map[string]string{"title": ""})

	lower := strings.ToLower(sql)
	if strings.Contains(lower, "title like") {
		t.Fatalf("title LIKE should not appear when title empty, got %s", sql)
	}
	if !strings.Contains(lower, "org_id = ?") {
		t.Fatalf("expected org_id filter to remain in SQL, got %s", sql)
	}
}
