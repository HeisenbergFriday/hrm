package service

import (
	"testing"

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
