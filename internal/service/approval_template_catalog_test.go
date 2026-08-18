package service

import (
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

func TestApprovalServiceCompletesTemplateCatalogFromOrganizationConfig(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.ApprovalTemplate{}); err != nil {
		t.Fatalf("migrate approval templates: %v", err)
	}
	stored := []database.ApprovalTemplate{
		{OrgID: "org-a", TemplateID: "PROC-STORED", Name: "已同步模板", Category: "other", Status: "active"},
		{OrgID: "org-b", TemplateID: "PROC-FOREIGN", Name: "外组织模板", Category: "other", Status: "active"},
	}
	if err := db.Create(&stored).Error; err != nil {
		t.Fatalf("create templates: %v", err)
	}

	svc := NewApprovalServiceWithOrgID(db, "org-a")
	svc.configForOrg = func(orgID string) (dingtalk.Config, error) {
		if orgID != "org-a" {
			t.Fatalf("config org = %q, want org-a", orgID)
		}
		return dingtalk.Config{OrgID: orgID, ProcessCodes: map[string]string{
			"leave":  "PROC-LEAVE",
			"custom": "PROC-STORED",
		}}, nil
	}

	templates, total, err := svc.GetTemplates()
	if err != nil {
		t.Fatalf("GetTemplates() error = %v", err)
	}
	if total != 2 || len(templates) != 2 {
		t.Fatalf("templates total=%d items=%#v, want two org-a templates", total, templates)
	}
	byCode := make(map[string]database.ApprovalTemplate, len(templates))
	for _, template := range templates {
		byCode[template.TemplateID] = template
	}
	if byCode["PROC-STORED"].Name != "已同步模板" {
		t.Fatalf("persisted template was not preserved: %#v", byCode["PROC-STORED"])
	}
	configured := byCode["PROC-LEAVE"]
	if configured.Name != "请假审批" || configured.Category != "leave" || configured.Status != "active" || configured.OrgID != "org-a" {
		t.Fatalf("configured template = %#v", configured)
	}
	if _, leaked := byCode["PROC-FOREIGN"]; leaked {
		t.Fatal("foreign organization template leaked into catalog")
	}
}

func TestApprovalStatsUsesConfiguredTemplateNameWhenTemplateTableIsEmpty(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.Approval{}, &database.ApprovalTemplate{}); err != nil {
		t.Fatalf("migrate approval tables: %v", err)
	}
	if err := db.Create(&database.Approval{
		OrgID: "org-a", ProcessID: "approval-1", Title: "张三提交的请假", ApplicantID: "u1", ApplicantName: "张三",
		Status: "COMPLETED", CreateTime: time.Now().In(ApprovalSyncLocation()),
		Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-LEAVE"},
	}).Error; err != nil {
		t.Fatalf("create approval: %v", err)
	}

	svc := NewApprovalStatsServiceWithOrgID(db, "org-a")
	svc.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a", ProcessCodes: map[string]string{"leave": "PROC-LEAVE"}}, nil
	}
	stats, err := svc.Get(map[string]string{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(stats.TemplateStats) != 1 || stats.TemplateStats[0].TemplateName != "请假审批" {
		t.Fatalf("template stats = %#v", stats.TemplateStats)
	}
}
