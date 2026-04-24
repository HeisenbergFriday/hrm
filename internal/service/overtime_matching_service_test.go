package service

import (
	"peopleops/internal/database"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOvertimeTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试DB失败: %v", err)
	}
	db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.OvertimeMatchResult{},
		&database.CompensatoryLeaveLedger{},
		&database.OvertimeRuleConfig{},
	)
	return db
}

func makeOvertimeApproval(db *gorm.DB, id uint, userID string, start, end time.Time) database.Approval {
	a := database.Approval{
		ProcessID:     "PROC-" + userID,
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
	if result.QualifiedMinutes <= 0 {
		t.Errorf("应有有效调休分钟，实际: %d", result.QualifiedMinutes)
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
	if result.QualifiedMinutes != 0 {
		t.Errorf("无考勤时调休分钟应为0，实际: %d", result.QualifiedMinutes)
	}
}

// 场景3：考勤时长短于审批时长，使用考勤时长
func TestOvertimeMatch_AttendanceShorterThanApproval(t *testing.T) {
	db := setupOvertimeTestDB(t)
	start := time.Date(2026, 4, 7, 18, 0, 0, 0, time.Local)
	end := time.Date(2026, 4, 7, 22, 0, 0, 0, time.Local) // 审批4小时
	makeOvertimeApproval(db, 3, "u3", start, end)
	// 考勤只打了2小时
	makeAttendance(db, "u3", start, "上班")
	makeAttendance(db, "u3", start.Add(2*time.Hour), "下班")

	svc := NewOvertimeMatchingService(db)
	svc.MatchApproval(3)

	var result database.OvertimeMatchResult
	db.Where("approval_id = ?", 3).First(&result)
	if result.QualifiedMinutes != 120 {
		t.Errorf("应使用考勤时长120分钟，实际: %d", result.QualifiedMinutes)
	}
	if result.MatchStatus != "partial" {
		t.Errorf("匹配状态应为partial，实际: %s", result.MatchStatus)
	}
}

// 场景4：重复执行匹配是幂等的
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
