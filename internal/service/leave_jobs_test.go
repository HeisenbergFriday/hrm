package service

import (
	"testing"

	"peopleops/internal/database"
)

func TestIsAnnualLeaveApprovalConsumable(t *testing.T) {
	tests := []struct {
		name   string
		status string
		result string
		want   bool
	}{
		{name: "COMPLETED+agree", status: "COMPLETED", result: "agree", want: true},
		{name: "completed+agree", status: "completed", result: "agree", want: true},
		{name: "COMPLETED+refuse", status: "COMPLETED", result: "refuse", want: false},
		{name: "COMPLETED+approved-alias", status: "COMPLETED", result: "approved", want: false},
		{name: "COMPLETED+pass-alias", status: "COMPLETED", result: "pass", want: false},
		{name: "COMPLETED+success-alias", status: "COMPLETED", result: "success", want: false},
		{name: "running", status: "RUNNING", result: "agree", want: false},
		{name: "completed+empty-result", status: "completed", result: "", want: false},
		{name: "completed+拒绝", status: "COMPLETED", result: "拒绝", want: false},
		{name: "completed+通过", status: "COMPLETED", result: "通过", want: false},
		{name: "completed+unknown", status: "COMPLETED", result: "redirect", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnnualLeaveApprovalConsumable(tt.status, tt.result); got != tt.want {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestApprovalResultFromExtension(t *testing.T) {
	if got := approvalResultFromExtension(map[string]interface{}{"result": "agree"}); got != "agree" {
		t.Fatalf("got=%s", got)
	}
	if got := approvalResultFromExtension(nil); got != "" {
		t.Fatalf("got=%s", got)
	}
}

func TestConsumeAnnualLeaveApprovalsForOrgIsStatusSafeAndIdempotent(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.AnnualLeaveGrant{}, &database.AnnualLeaveConsumeLog{}, &database.AnnualLeaveConsumeRequest{}); err != nil {
		t.Fatalf("migrate leave tables: %v", err)
	}
	grant := database.AnnualLeaveGrant{
		OrgID: "org-a", UserID: "user-1", Year: 2026, Quarter: 3,
		GrantedDays: 5, RemainingDays: 5, GrantType: "normal",
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	approvals := []database.Approval{
		{OrgID: "org-a", ProcessID: "leave-approved", Title: "年假审批", ApplicantID: "user-1", Status: "COMPLETED", Content: map[string]interface{}{"天数": "1", "开始日期": "2026-07-10", "结束日期": "2026-07-10"}, Extension: map[string]interface{}{"result": "agree"}},
		{OrgID: "org-a", ProcessID: "leave-refused", Title: "年假审批", ApplicantID: "user-1", Status: "COMPLETED", Content: map[string]interface{}{"天数": "2"}, Extension: map[string]interface{}{"result": "refuse"}},
		{OrgID: "org-a", ProcessID: "leave-terminated", Title: "年假审批", ApplicantID: "user-1", Status: "TERMINATED", Content: map[string]interface{}{"天数": "2"}, Extension: map[string]interface{}{"result": "agree"}},
		{OrgID: "org-a", ProcessID: "leave-canceled", Title: "年假审批", ApplicantID: "user-1", Status: "CANCELED", Content: map[string]interface{}{"天数": "2"}, Extension: map[string]interface{}{"result": "agree"}},
	}

	scheduler := &LeaveJobScheduler{db: db}
	scheduler.consumeAnnualLeaveApprovalsForOrg("org-a", approvals)
	scheduler.consumeAnnualLeaveApprovalsForOrg("org-a", approvals)

	var saved database.AnnualLeaveGrant
	if err := db.First(&saved, grant.ID).Error; err != nil {
		t.Fatalf("reload grant: %v", err)
	}
	if saved.UsedDays != 1 || saved.RemainingDays != 4 {
		t.Fatalf("grant used=%v remaining=%v, want 1/4", saved.UsedDays, saved.RemainingDays)
	}
	var logs int64
	if err := db.Model(&database.AnnualLeaveConsumeLog{}).Where("org_id = ?", "org-a").Count(&logs).Error; err != nil {
		t.Fatalf("count consume logs: %v", err)
	}
	if logs != 1 {
		t.Fatalf("consume log count = %d, want 1", logs)
	}
}
