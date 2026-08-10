package service

import (
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestApprovalResyncDoesNotDuplicateOvertimeMatchOrCompensatoryCredit(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.OvertimeRuleConfig{},
		&database.OvertimeMatchResult{},
		&database.CompensatoryLeaveLedger{},
		&database.OvertimeSyncHistory{},
		&database.OvertimeSupplementaryRequest{},
	); err != nil {
		t.Fatalf("migrate overtime tables: %v", err)
	}

	createTime := time.Date(2026, 8, 5, 9, 0, 0, 0, time.Local)
	finishTime := createTime.Add(time.Hour)
	approvals := []database.Approval{
		{
			OrgID: "org-a", ProcessID: "overtime-approved", Title: "加班审批", ApplicantID: "user-1", ApplicantName: "员工甲",
			Status: "COMPLETED", CreateTime: createTime, FinishTime: finishTime,
			Content:   map[string]interface{}{"加班开始时间": "2026-08-05 18:00:00", "加班结束时间": "2026-08-05 21:00:00"},
			Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-OVERTIME"},
		},
		{
			OrgID: "org-a", ProcessID: "leave-approved", Title: "年假审批", ApplicantID: "user-2", ApplicantName: "员工乙",
			Status: "COMPLETED", CreateTime: createTime, FinishTime: finishTime,
			Content:   map[string]interface{}{"加班开始时间": "2026-08-05 18:00:00", "加班结束时间": "2026-08-05 21:00:00"},
			Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-LEAVE"},
		},
		{
			OrgID: "org-a", ProcessID: "overtime-refused", Title: "加班审批", ApplicantID: "user-3", ApplicantName: "员工丙",
			Status: "COMPLETED", CreateTime: createTime, FinishTime: finishTime,
			Content:   map[string]interface{}{"加班开始时间": "2026-08-05 18:00:00", "加班结束时间": "2026-08-05 21:00:00"},
			Extension: map[string]interface{}{"result": "refuse", "process_code": "PROC-OVERTIME"},
		},
	}
	if err := db.Create(&approvals).Error; err != nil {
		t.Fatalf("create approvals: %v", err)
	}
	attendances := []database.Attendance{
		{OrgID: "org-a", UserID: "user-1", UserName: "员工甲", CheckTime: time.Date(2026, 8, 5, 18, 0, 0, 0, time.Local), CheckType: "OnDuty"},
		{OrgID: "org-a", UserID: "user-1", UserName: "员工甲", CheckTime: time.Date(2026, 8, 5, 21, 0, 0, 0, time.Local), CheckType: "OffDuty"},
	}
	if err := db.Create(&attendances).Error; err != nil {
		t.Fatalf("create attendances: %v", err)
	}

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	for i := 0; i < 2; i++ {
		if err := svc.MatchApprovedOvertime("2026-08-05", "2026-08-05"); err != nil {
			t.Fatalf("match run %d: %v", i+1, err)
		}
	}

	var matchCount int64
	if err := db.Model(&database.OvertimeMatchResult{}).Where("org_id = ?", "org-a").Count(&matchCount).Error; err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if matchCount != 1 {
		t.Fatalf("match count = %d, want 1", matchCount)
	}
	var ledgerCount int64
	if err := db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND ledger_type = ?", "org-a", "credit").Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count ledgers: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("credit ledger count = %d, want 1", ledgerCount)
	}
	var ledger database.CompensatoryLeaveLedger
	if err := db.Where("org_id = ?", "org-a").First(&ledger).Error; err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	if ledger.UserID != "user-1" || ledger.CreditMinutes != 180 {
		t.Fatalf("ledger = %#v", ledger)
	}
}
