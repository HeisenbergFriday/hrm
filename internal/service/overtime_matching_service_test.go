package service

import (
	"peopleops/internal/database"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOvertimeTestDB(t *testing.T) *gorm.DB {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试DB失败: %v", err)
	}
	db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.OvertimeMatchResult{},
		&database.OvertimeSyncHistory{},
		&database.CompensatoryLeaveLedger{},
		&database.OvertimeRuleConfig{},
	)
	return db
}

func seedOvertimeSyncHistory(db *gorm.DB, match database.OvertimeMatchResult, requestID string) error {
	now := time.Now()
	return db.Create(&database.OvertimeSyncHistory{
		UserID:                   match.UserID,
		WorkDate:                 match.WorkDate,
		ApprovalID:               match.ApprovalID,
		ApprovalProcessID:        match.ApprovalProcessID,
		EffectiveOvertimeMinutes: match.EffectiveOvertimeMinutes,
		SyncRequestID:            requestID,
		SyncMode:                 "test",
		SyncedAt:                 &now,
	}).Error
}

func makeOvertimeApproval(db *gorm.DB, id uint, userID string, start, end time.Time) database.Approval {
	a := database.Approval{
		ProcessID:     "PROC-" + userID + "-" + strconv.Itoa(int(id)),
		Title:         "加班申请",
		ApplicantID:   userID,
		ApplicantName: userID,
		Status:        "completed",
		CreateTime:    start,
		FinishTime:    end,
		Content:       map[string]interface{}{"start_time": start.Format("2006-01-02 15:04:05"), "end_time": end.Format("2006-01-02 15:04:05")},
		Extension:     map[string]interface{}{"process_code": "overtime"},
	}
	a.ID = id
	db.Create(&a)
	return a
}

func makeAttendance(db *gorm.DB, userID string, checkTime time.Time, checkType string) {
	db.Create(&database.Attendance{
		UserID:    userID,
		UserName:  userID,
		CheckTime: checkTime,
		CheckType: checkType,
	})
}

func makeAttendanceWithExtension(db *gorm.DB, userID string, checkTime time.Time, checkType string, extension map[string]interface{}) {
	db.Create(&database.Attendance{
		UserID:    userID,
		UserName:  userID,
		CheckTime: checkTime,
		CheckType: checkType,
		Extension: extension,
	})
}

// 场景1：有审批有考勤，正确计算调休分钟
func TestOvertimeMatch_ValidAttendanceCreditMinutes(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 5, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 5, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 1, "u1", start, end)
	makeAttendance(db, "u1", start.Add(10*time.Minute), "上班")
	makeAttendance(db, "u1", end.Add(-10*time.Minute), "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(1); err != nil {
		t.Fatalf("匹配失败: %v", err)
	}

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 1).First(&result)
	if result.EffectiveOvertimeMinutes <= 0 {
		t.Errorf("应有有效调休分钟，实际: %d", result.EffectiveOvertimeMinutes)
	}

	compSvc := NewCompensatoryLeaveService(db)
	balance, _ := compSvc.GetBalance("u1")
	if balance.BalanceMinutes <= 0 {
		t.Error("调休余额应大于0")
	}
}

// 场景2：有审批无考勤，不计入调休
func TestOvertimeMatch_NoAttendanceNoCredit(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 6, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 6, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 2, "u2", start, end)

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(2)

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 2).First(&result)
	if result.EffectiveOvertimeMinutes != 0 {
		t.Errorf("无考勤时调休分钟应为0，实际: %d", result.EffectiveOvertimeMinutes)
	}
	if result.MatchStatus != "no_clock_record" {
		t.Errorf("匹配状态应为no_clock_record，实际: %s", result.MatchStatus)
	}
}

// 场景3：考勤时长短于审批时长，使用考勤时长（扣除休息时间）
func TestOvertimeMatch_AttendanceShorterThanApproval(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 7, 9, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 7, 18, 30, 0, 0, time.Local) // 审批9.5小时
	makeOvertimeApproval(db, 3, "u3", start, end)
	// 考勤打了8.5小时（达到休息扣除阈值）
	makeAttendance(db, "u3", start, "上班")
	makeAttendance(db, "u3", start.Add(8*time.Hour+30*time.Minute), "下班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(3)

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 3).First(&result)
	// 8.5小时（510分钟）减去30分钟休息时间，应该是480分钟
	if result.EffectiveOvertimeMinutes != 480 {
		t.Errorf("应使用考勤时长480分钟（510-30），实际: %d", result.EffectiveOvertimeMinutes)
	}
	if result.MatchStatus != "matched" {
		t.Errorf("匹配状态应为matched，实际: %s", result.MatchStatus)
	}
}

func TestOvertimeMatch_FiltersInvalidClockRecords(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 11, 9, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 11, 18, 30, 0, 0, time.Local)
	makeOvertimeApproval(db, 7, "u7", start, end)

	makeAttendanceWithExtension(db, "u7", start, "上班", map[string]interface{}{"sourceType": "USER", "isLegal": "Y"})
	makeAttendanceWithExtension(db, "u7", start.Add(4*time.Hour), "打卡", map[string]interface{}{"sourceType": "USER", "invalidRecordMsg": "需要二次确认"})
	makeAttendanceWithExtension(db, "u7", start.Add(8*time.Hour+30*time.Minute), "下班", map[string]interface{}{"sourceType": "USER", "isLegal": "Y"})

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(7); err != nil {
		t.Fatalf("匹配失败: %v", err)
	}

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 7).First(&result)
	if result.ActualClockSpanMinutes != 510 {
		t.Fatalf("应忽略无效打卡并按首末有效打卡计算510分钟，实际: %d", result.ActualClockSpanMinutes)
	}
	if result.EffectiveOvertimeMinutes != 480 {
		t.Fatalf("有效调休应为480分钟，实际: %d", result.EffectiveOvertimeMinutes)
	}
}

// 场景6：只有一次打卡，不计入调休
func TestOvertimeMatch_OnlyOneClockRecord(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 10, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 10, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 6, "u6", start, end)
	// 只打了一次卡
	makeAttendance(db, "u6", start, "上班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(6)

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 6).First(&result)
	if result.EffectiveOvertimeMinutes != 0 {
		t.Errorf("只有一次打卡时调休分钟应为0，实际: %d", result.EffectiveOvertimeMinutes)
	}
	if result.MatchStatus != "insufficient_clock_record" {
		t.Errorf("匹配状态应为insufficient_clock_record，实际: %s", result.MatchStatus)
	}
}

func TestOvertimeMatch_DuplicateApprovalsSameUserDateOnlyCreditOnce(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 12, 9, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 12, 18, 30, 0, 0, time.Local)
	makeOvertimeApproval(db, 8, "u8", start, end)
	makeOvertimeApproval(db, 9, "u8", start.Add(time.Hour), end.Add(time.Hour))
	makeAttendance(db, "u8", start, "上班")
	makeAttendance(db, "u8", start.Add(8*time.Hour+30*time.Minute), "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(8); err != nil {
		t.Fatalf("首次匹配失败: %v", err)
	}
	if err := svc.MatchApproval(9); err != nil {
		t.Fatalf("重复日期匹配失败: %v", err)
	}

	var matchCount int64
	db.Model(&database.OvertimeMatchResult{}).Where("user_id = ? AND work_date = ?", "u8", "2026-04-12").Count(&matchCount)
	if matchCount != 1 {
		t.Fatalf("同一员工同一天应只生成1条匹配记录，实际: %d", matchCount)
	}

	var ledgerCount int64
	db.Model(&database.CompensatoryLeaveLedger{}).Where("user_id = ? AND ledger_type = ?", "u8", "credit").Count(&ledgerCount)
	if ledgerCount != 1 {
		t.Fatalf("同一员工同一天调休应只增加1次，实际: %d", ledgerCount)
	}
}

func TestOvertimeMatch_QueryRangeUsesApprovalStartDate(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 24, 9, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 24, 18, 30, 0, 0, time.Local)
	approval := makeOvertimeApproval(db, 10, "u10", start, end)
	db.Model(&database.Approval{}).Where("id = ?", approval.ID).Update("create_time", time.Date(2026, 3, 31, 10, 0, 0, 0, time.Local))
	makeAttendance(db, "u10", start, "上班")
	makeAttendance(db, "u10", start.Add(8*time.Hour+30*time.Minute), "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApprovedOvertime("2026-04-01", "2026-04-30"); err != nil {
		t.Fatalf("批量匹配失败: %v", err)
	}

	var count int64
	db.Model(&database.OvertimeMatchResult{}).Where("approval_id = ?", 10).Count(&count)
	if count != 1 {
		t.Fatalf("应按审批加班开始日期匹配到4月审批，实际匹配记录数: %d", count)
	}
}

// 场景7：打卡时间在加班窗口外，不计入有效加班
func TestOvertimeMatch_ClockOutsideOvertimeWindow_Invalid(t *testing.T) {
	db := setupOvertimeTestDB(t)
	// 加班审批：18:00-21:00
	start := time.Date(2026, 4, 15, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 15, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 11, "u11", start, end)
	// 员工只有正常上下班打卡（9:00上班，18:00下班），未在加班期间打卡
	makeAttendance(db, "u11", time.Date(2026, 4, 15, 9, 0, 0, 0, time.Local), "上班")
	makeAttendance(db, "u11", time.Date(2026, 4, 15, 18, 0, 0, 0, time.Local), "下班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(11)

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 11).First(&result)
	if result.EffectiveOvertimeMinutes != 0 {
		t.Errorf("加班窗口内打卡不足2次，调休分钟应为0，实际: %d", result.EffectiveOvertimeMinutes)
	}
	// 9:00 不在窗口[16:00,23:00]内，18:00 在窗口内，共1条 → insufficient
	if result.MatchStatus != "insufficient_clock_record" {
		t.Errorf("匹配状态应为insufficient_clock_record，实际: %s", result.MatchStatus)
	}
}

// 场景8：打卡时间在加班窗口内（需求示例：17:55上班，21:10下班），计入有效加班
func TestOvertimeMatch_ClockInsideOvertimeWindow_Valid(t *testing.T) {
	db := setupOvertimeTestDB(t)
	// 加班审批：18:00-21:00
	start := time.Date(2026, 4, 16, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 16, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 12, "u12", start, end)
	// 员工17:55打卡，21:10离开
	makeAttendance(db, "u12", time.Date(2026, 4, 16, 17, 55, 0, 0, time.Local), "上班")
	makeAttendance(db, "u12", time.Date(2026, 4, 16, 21, 10, 0, 0, time.Local), "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(12); err != nil {
		t.Fatalf("匹配失败: %v", err)
	}

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 12).First(&result)
	if result.EffectiveOvertimeMinutes <= 0 {
		t.Errorf("应有有效调休分钟，实际: %d", result.EffectiveOvertimeMinutes)
	}
	if result.MatchStatus != "matched" {
		t.Errorf("匹配状态应为matched，实际: %s", result.MatchStatus)
	}
}

// 场景9：审批提交时间与准备加班时间语义分离验证
// 用户15号提交审批（15号当天批准），准备加班时间为16号18:00-21:00
// ApprovalStartTime 应为15号提交时间，OvertimeStartTime 应为16号18:00
// WorkDate 应按准备加班日期（16号）确定
func TestOvertimeMatch_ApprovalAndOvertimeTimeFieldsSeparated(t *testing.T) {
	db := setupOvertimeTestDB(t)

	overtimeStart := time.Date(2026, 4, 16, 18, 0, 0, 0, time.Local)
	overtimeEnd := time.Date(2026, 4, 16, 21, 0, 0, 0, time.Local)
	submitTime := time.Date(2026, 4, 15, 10, 0, 0, 0, time.Local)
	approveTime := time.Date(2026, 4, 15, 14, 0, 0, 0, time.Local)

	approval := database.Approval{
		ProcessID:     "PROC-u13-1",
		Title:         "加班申请",
		ApplicantID:   "u13",
		ApplicantName: "u13",
		Status:        "completed",
		CreateTime:    submitTime,
		FinishTime:    approveTime,
		Content:       map[string]interface{}{"start_time": overtimeStart.Format("2006-01-02 15:04:05"), "end_time": overtimeEnd.Format("2006-01-02 15:04:05")},
		Extension:     map[string]interface{}{"process_code": "overtime"},
	}
	approval.ID = 13
	db.Create(&approval)

	makeAttendance(db, "u13", time.Date(2026, 4, 16, 17, 55, 0, 0, time.Local), "上班")
	makeAttendance(db, "u13", time.Date(2026, 4, 16, 21, 10, 0, 0, time.Local), "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(13); err != nil {
		t.Fatalf("匹配失败: %v", err)
	}

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 13).First(&result)

	if result.WorkDate != "2026-04-16" {
		t.Errorf("WorkDate应为准备加班日期2026-04-16，实际: %s", result.WorkDate)
	}
	if !result.ApprovalStartTime.Equal(submitTime) {
		t.Errorf("ApprovalStartTime应为提交时间%v，实际: %v", submitTime, result.ApprovalStartTime)
	}
	if !result.ApprovalEndTime.Equal(approveTime) {
		t.Errorf("ApprovalEndTime应为审批通过时间%v，实际: %v", approveTime, result.ApprovalEndTime)
	}
	if !result.OvertimeStartTime.Equal(overtimeStart) {
		t.Errorf("OvertimeStartTime应为准备加班开始时间%v，实际: %v", overtimeStart, result.OvertimeStartTime)
	}
	if !result.OvertimeEndTime.Equal(overtimeEnd) {
		t.Errorf("OvertimeEndTime应为准备加班结束时间%v，实际: %v", overtimeEnd, result.OvertimeEndTime)
	}
	if result.OvertimeDurationMinutes != 180 {
		t.Errorf("OvertimeDurationMinutes应为180，实际: %d", result.OvertimeDurationMinutes)
	}
	if result.ApprovalDurationMinutes != 240 {
		t.Errorf("ApprovalDurationMinutes应为240（14:00-10:00），实际: %d", result.ApprovalDurationMinutes)
	}
	if result.MatchStatus != "matched" {
		t.Errorf("匹配状态应为matched，实际: %s", result.MatchStatus)
	}
}

func TestOvertimeMatch_Idempotent(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 8, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 8, 20, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 4, "u4", start, end)
	makeAttendance(db, "u4", start, "上班")
	makeAttendance(db, "u4", end, "下班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(4)
	svc.MatchApproval(4)

	var count int64
	db.Model(&database.OvertimeMatchResult{}).Where("approval_id = ?", 4).Count(&count)
	if count != 1 {
		t.Errorf("重复执行应只有1条匹配记录，实际: %d", count)
	}

	var ledgerCount int64
	db.Model(&database.CompensatoryLeaveLedger{}).Where("source_match_id > 0").Count(&ledgerCount)
	if ledgerCount != 1 {
		t.Errorf("调休台账应只有1条credit记录，实际: %d", ledgerCount)
	}
}

// 场景5：审批撤销后调休正确回滚
func TestOvertimeMatch_RollbackOnCancel(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 9, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 9, 20, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 5, "u5", start, end)
	makeAttendance(db, "u5", start, "上班")
	makeAttendance(db, "u5", end, "下班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(5)

	compSvc := NewCompensatoryLeaveService(db)
	balanceBefore, _ := compSvc.GetBalance("u5")

	svc.RollbackApprovalMatch(5)

	balanceAfter, _ := compSvc.GetBalance("u5")
	if balanceAfter.BalanceMinutes >= balanceBefore.BalanceMinutes {
		t.Errorf("回滚后余额应减少，回滚前: %d, 回滚后: %d", balanceBefore.BalanceMinutes, balanceAfter.BalanceMinutes)
	}
}

func TestOvertimeMatch_ClearAndRematchDoesNotDoubleCredit(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 17, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 17, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 14, "u14", start, end)
	makeAttendance(db, "u14", start, "上班")
	makeAttendance(db, "u14", end, "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(14); err != nil {
		t.Fatalf("first match failed: %v", err)
	}

	compSvc := NewCompensatoryLeaveService(db)
	before, err := compSvc.GetBalance("u14")
	if err != nil {
		t.Fatalf("query balance before rematch failed: %v", err)
	}

	if err := svc.ClearAndRematch("u14", "2026-04-17", "2026-04-17"); err != nil {
		t.Fatalf("clear and rematch failed: %v", err)
	}

	after, err := compSvc.GetBalance("u14")
	if err != nil {
		t.Fatalf("query balance after rematch failed: %v", err)
	}
	if after.BalanceMinutes != before.BalanceMinutes {
		t.Fatalf("expected clear-rematch to preserve credited minutes, before=%d after=%d", before.BalanceMinutes, after.BalanceMinutes)
	}

	var creditCount int64
	if err := db.Model(&database.CompensatoryLeaveLedger{}).
		Where("user_id = ? AND ledger_type = ?", "u14", "credit").
		Count(&creditCount).Error; err != nil {
		t.Fatalf("count credit ledgers failed: %v", err)
	}
	if creditCount != 2 {
		t.Fatalf("expected one original credit and one rematch credit, got %d", creditCount)
	}

	var rollbackCount int64
	if err := db.Model(&database.CompensatoryLeaveLedger{}).
		Where("user_id = ? AND ledger_type = ?", "u14", "rollback").
		Count(&rollbackCount).Error; err != nil {
		t.Fatalf("count rollback ledgers failed: %v", err)
	}
	if rollbackCount != 1 {
		t.Fatalf("expected one rollback ledger, got %d", rollbackCount)
	}
}

func TestOvertimeMatch_ClearAndRematchKeepsPreviouslySyncedRecord(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 19, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 19, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 16, "u16", start, end)
	makeAttendance(db, "u16", start, "上班")
	makeAttendance(db, "u16", end, "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(16); err != nil {
		t.Fatalf("first match failed: %v", err)
	}

	var initial database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 16).First(&initial).Error; err != nil {
		t.Fatalf("query initial match failed: %v", err)
	}
	if err := db.Model(&database.OvertimeMatchResult{}).
		Where("id = ?", initial.ID).
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     "success",
			"dingtalk_sync_request_id": "request-keep",
			"match_status":             "synced",
		}).Error; err != nil {
		t.Fatalf("seed sync status failed: %v", err)
	}
	if err := seedOvertimeSyncHistory(db, initial, "request-keep"); err != nil {
		t.Fatalf("seed sync history failed: %v", err)
	}

	if err := svc.ClearAndRematch("u16", "2026-04-19", "2026-04-19"); err != nil {
		t.Fatalf("clear and rematch failed: %v", err)
	}

	var after database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 16).First(&after).Error; err != nil {
		t.Fatalf("query rematched record failed: %v", err)
	}
	if after.DingtalkSyncStatus != "success" {
		t.Fatalf("expected rematched record to preserve success sync status, got %s", after.DingtalkSyncStatus)
	}
	if after.DingtalkSyncRequestID != "request-keep" {
		t.Fatalf("expected rematched record to preserve request id, got %s", after.DingtalkSyncRequestID)
	}
	if after.MatchStatus != "synced" {
		t.Fatalf("expected rematched record to stay synced, got %s", after.MatchStatus)
	}
}

func TestOvertimeMatch_ClearAndRematchChangedMinutesKeepsHistoricalSync(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 20, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 20, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 17, "u17", start, end)
	makeAttendance(db, "u17", start, "上班")
	makeAttendance(db, "u17", end, "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(17); err != nil {
		t.Fatalf("first match failed: %v", err)
	}

	var initial database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 17).First(&initial).Error; err != nil {
		t.Fatalf("query initial match failed: %v", err)
	}
	if err := db.Model(&database.OvertimeMatchResult{}).
		Where("id = ?", initial.ID).
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     "success",
			"dingtalk_sync_request_id": "request-old",
			"match_status":             "synced",
		}).Error; err != nil {
		t.Fatalf("seed sync status failed: %v", err)
	}
	if err := seedOvertimeSyncHistory(db, initial, "request-old"); err != nil {
		t.Fatalf("seed sync history failed: %v", err)
	}
	if err := db.Model(&database.Attendance{}).
		Where("user_id = ? AND check_type = ?", "u17", "下班").
		Update("check_time", end.Add(-30*time.Minute)).Error; err != nil {
		t.Fatalf("update attendance failed: %v", err)
	}

	if err := svc.ClearAndRematch("u17", "2026-04-20", "2026-04-20"); err != nil {
		t.Fatalf("clear and rematch failed: %v", err)
	}

	var after database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 17).First(&after).Error; err != nil {
		t.Fatalf("query rematched record failed: %v", err)
	}
	if after.EffectiveOvertimeMinutes == initial.EffectiveOvertimeMinutes {
		t.Fatalf("expected rematched minutes to change, still %d", after.EffectiveOvertimeMinutes)
	}
	if after.DingtalkSyncStatus != "success" {
		t.Fatalf("expected changed rematch to preserve historical sync, got %s", after.DingtalkSyncStatus)
	}
	if after.DingtalkSyncRequestID != "request-old" {
		t.Fatalf("expected changed rematch to keep old request id, got %s", after.DingtalkSyncRequestID)
	}
	if after.MatchStatus != "synced" {
		t.Fatalf("expected changed rematch to stay synced without pushing again, got %s", after.MatchStatus)
	}
}

func TestOvertimeMatch_RunMatchSkipsHistoricallySyncedRecordAfterDelete(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 21, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 21, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 18, "u18", start, end)
	makeAttendance(db, "u18", start, "上班")
	makeAttendance(db, "u18", end, "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(18); err != nil {
		t.Fatalf("first match failed: %v", err)
	}

	var initial database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 18).First(&initial).Error; err != nil {
		t.Fatalf("query initial match failed: %v", err)
	}
	if err := db.Model(&database.OvertimeMatchResult{}).
		Where("id = ?", initial.ID).
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     "success",
			"dingtalk_sync_request_id": "request-history",
			"match_status":             "synced",
		}).Error; err != nil {
		t.Fatalf("seed sync status failed: %v", err)
	}
	if err := seedOvertimeSyncHistory(db, initial, "request-history"); err != nil {
		t.Fatalf("seed sync history failed: %v", err)
	}

	if _, err := svc.DeleteMatchRecords("u18", "2026-04-21", "2026-04-21"); err != nil {
		t.Fatalf("delete match records failed: %v", err)
	}
	if err := svc.MatchApproval(18); err != nil {
		t.Fatalf("match after delete failed: %v", err)
	}

	var after database.OvertimeMatchResult
	if err := db.Where("approval_id = ?", 18).First(&after).Error; err != nil {
		t.Fatalf("query rematched record failed: %v", err)
	}
	if after.DingtalkSyncStatus != "success" {
		t.Fatalf("expected historical sync to be preserved after delete, got %s", after.DingtalkSyncStatus)
	}
	if after.DingtalkSyncRequestID != "request-history" {
		t.Fatalf("expected preserved request id after delete, got %s", after.DingtalkSyncRequestID)
	}
	if after.MatchStatus != "synced" {
		t.Fatalf("expected match to stay synced after delete+rerun, got %s", after.MatchStatus)
	}
}

func TestOvertimeMatch_DeleteMatchRecordsRollsBackBalance(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 18, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 18, 21, 0, 0, 0, time.Local)
	makeOvertimeApproval(db, 15, "u15", start, end)
	makeAttendance(db, "u15", start, "上班")
	makeAttendance(db, "u15", end, "下班")

	svc := NewOvertimeMatchingService(db)
	if err := svc.MatchApproval(15); err != nil {
		t.Fatalf("first match failed: %v", err)
	}

	deleted, err := svc.DeleteMatchRecords("u15", "2026-04-18", "2026-04-18")
	if err != nil {
		t.Fatalf("delete match records failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected to delete 1 match record, got %d", deleted)
	}

	compSvc := NewCompensatoryLeaveService(db)
	balance, err := compSvc.GetBalance("u15")
	if err != nil {
		t.Fatalf("query balance failed: %v", err)
	}
	if balance.BalanceMinutes != 0 {
		t.Fatalf("expected delete to roll back credited minutes, got %d", balance.BalanceMinutes)
	}
}
