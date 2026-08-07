package service

import (
	"fmt"
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestApprovalStatsAggregatesBeyondTenThousandAndClassifiesTerminalStates(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.Approval{}, &database.ApprovalTemplate{}); err != nil {
		t.Fatalf("migrate approval stats tables: %v", err)
	}
	if err := db.Create(&database.ApprovalTemplate{OrgID: "org-a", TemplateID: "PROC-A", Name: "通用审批"}).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	now := time.Date(2026, 8, 5, 9, 0, 0, 0, ApprovalSyncLocation())
	approvals := make([]database.Approval, 0, 10005)
	for index := 0; index < 10001; index++ {
		approvals = append(approvals, database.Approval{
			OrgID: "org-a", ProcessID: fmt.Sprintf("completed-%05d", index), Title: "通用审批",
			ApplicantID: "u1", ApplicantName: "员工甲", Status: "COMPLETED", CreateTime: now,
			Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-A"},
		})
	}
	for index, status := range []string{"REFUSE", "RUNNING", "TERMINATED", "CANCELED"} {
		approvals = append(approvals, database.Approval{
			OrgID: "org-a", ProcessID: fmt.Sprintf("terminal-%d", index), Title: "通用审批",
			ApplicantID: "u1", ApplicantName: "员工甲", Status: status, CreateTime: now,
			Extension: map[string]interface{}{"process_code": "PROC-A"},
		})
	}
	if err := db.CreateInBatches(approvals, 500).Error; err != nil {
		t.Fatalf("create approvals: %v", err)
	}
	stats, err := NewApprovalStatsServiceWithOrgID(db, "org-a").Get(map[string]string{})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.Summary.Total != 10005 || stats.Summary.Completed != 10001 || stats.Summary.Refused != 1 || stats.Summary.Running != 1 || stats.Summary.Terminated != 1 || stats.Summary.Canceled != 1 {
		t.Fatalf("summary = %#v", stats.Summary)
	}
	if len(stats.TemplateStats) != 1 || stats.TemplateStats[0].Total != 10005 {
		t.Fatalf("template stats = %#v", stats.TemplateStats)
	}
	foreign := database.Approval{
		OrgID: "org-b", ProcessID: "foreign", Title: "通用审批", ApplicantID: "u1", ApplicantName: "外组织",
		Status: "COMPLETED", CreateTime: now, Extension: map[string]interface{}{"process_code": "PROC-A"},
	}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatalf("create foreign approval: %v", err)
	}
	stats, err = NewApprovalStatsServiceWithOrgID(db, "org-a").Get(map[string]string{})
	if err != nil || stats.Summary.Total != 10005 {
		t.Fatalf("tenant-isolated stats = %#v err=%v", stats.Summary, err)
	}
}
