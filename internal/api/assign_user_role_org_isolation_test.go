package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAssignRoleIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:api-assign-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Role{},
		&database.UserRole{},
		&database.Permission{},
		&database.RolePermission{},
		&database.MenuPermission{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func otherAssignOrg(org string) string {
	switch org {
	case "default":
		return "muteng"
	case "muteng":
		return "xiaotie"
	default:
		return "default"
	}
}

func seedAPITestUser(t *testing.T, db *gorm.DB, org, userID, name string) {
	t.Helper()
	u := &database.User{
		OrgID:          org,
		UserID:         userID,
		DingTalkUserID: "dt-" + org + "-" + userID,
		Name:           name,
		Status:         "active",
		Email:          userID + "@" + org + ".test",
		Mobile:         "m-" + org + "-" + userID,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user %s/%s: %v", org, userID, err)
	}
}

func seedAssignRoleFixture(t *testing.T, db *gorm.DB, org string) (userID string, localRoleID, foreignRoleID uint) {
	t.Helper()
	userID = "api-user-" + org
	foreign := otherAssignOrg(org)
	seedAPITestUser(t, db, org, userID, "API User")
	local := database.Role{OrgID: org, Name: "api-local-" + org}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("seed local role: %v", err)
	}
	foreignRole := database.Role{OrgID: foreign, Name: "api-foreign-" + foreign}
	if err := db.Create(&foreignRole).Error; err != nil {
		t.Fatalf("seed foreign role: %v", err)
	}
	return userID, local.ID, foreignRole.ID
}

// TestAssignUserRole_HTTPOrgIsolation verifies handler-level isolation for three orgs:
// same-org success, cross-org role 404 without write, missing role 404.
func TestAssignUserRole_HTTPOrgIsolation(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openAssignRoleIsolationDB(t)
			userID, localRoleID, foreignRoleID := seedAssignRoleFixture(t, db, org)

			original := database.DB
			database.DB = db
			t.Cleanup(func() { database.DB = original })

			// 1) same-org success
			body := fmt.Sprintf(`{"user_id":%q,"role_id":%d}`, userID, localRoleID)
			c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/permission/users/roles/assign", body, org)
			AssignUserRole(c)
			if recorder.Code != http.StatusOK {
				t.Fatalf("same-org status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			var localBindings int64
			if err := db.Model(&database.UserRole{}).
				Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, localRoleID).
				Count(&localBindings).Error; err != nil {
				t.Fatalf("count local: %v", err)
			}
			if localBindings != 1 {
				t.Fatalf("local bindings = %d, want 1", localBindings)
			}

			// 2) cross-org role → 404, no foreign write, previous binding kept
			body = fmt.Sprintf(`{"user_id":%q,"role_id":%d}`, userID, foreignRoleID)
			c, recorder = newSecurityCtx(t, http.MethodPost, "/api/v1/permission/users/roles/assign", body, org)
			AssignUserRole(c)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("cross-org status = %d body=%s, want 404", recorder.Code, recorder.Body.String())
			}
			var foreignBindings int64
			if err := db.Model(&database.UserRole{}).
				Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, foreignRoleID).
				Count(&foreignBindings).Error; err != nil {
				t.Fatalf("count foreign: %v", err)
			}
			if foreignBindings != 0 {
				t.Fatalf("foreign role was written via HTTP")
			}
			if err := db.Model(&database.UserRole{}).
				Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, localRoleID).
				Count(&localBindings).Error; err != nil {
				t.Fatalf("recount local: %v", err)
			}
			if localBindings != 1 {
				t.Fatalf("local binding lost after cross-org attempt, count=%d", localBindings)
			}

			// 3) missing role → 404
			body = fmt.Sprintf(`{"user_id":%q,"role_id":%d}`, userID, 999999)
			c, recorder = newSecurityCtx(t, http.MethodPost, "/api/v1/permission/users/roles/assign", body, org)
			AssignUserRole(c)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("missing role status = %d body=%s, want 404", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestGetUserRoles_HTTPDoesNotLeakCrossOrgRole ensures query path filters poisoned bindings.
func TestGetUserRoles_HTTPDoesNotLeakCrossOrgRole(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openAssignRoleIsolationDB(t)
			userID, localRoleID, foreignRoleID := seedAssignRoleFixture(t, db, org)
			if err := db.Create(&database.UserRole{OrgID: org, UserID: userID, RoleID: localRoleID}).Error; err != nil {
				t.Fatalf("seed local binding: %v", err)
			}
			poisonUser := "poison-api-" + org
			seedAPITestUser(t, db, org, poisonUser, "Poison")
			if err := db.Create(&database.UserRole{OrgID: org, UserID: poisonUser, RoleID: foreignRoleID}).Error; err != nil {
				t.Fatalf("seed poison binding: %v", err)
			}

			original := database.DB
			database.DB = db
			t.Cleanup(func() { database.DB = original })

			// Normal user sees only local role.
			c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/permission/users/"+userID+"/roles", "", org)
			c.Params = gin.Params{{Key: "user_id", Value: userID}}
			GetUserRoles(c)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			roles := extractRolesFromResponse(t, recorder.Body.Bytes())
			if len(roles) != 1 {
				t.Fatalf("roles len = %d, want 1; body=%s", len(roles), recorder.Body.String())
			}
			if id, _ := roles[0]["id"].(float64); uint(id) != localRoleID {
				t.Fatalf("role id = %v, want %d", roles[0]["id"], localRoleID)
			}
			if gotOrg, _ := roles[0]["org_id"].(string); gotOrg != org {
				t.Fatalf("role org_id = %q, want %q", gotOrg, org)
			}

			// Poisoned user must not receive foreign role definition.
			c, recorder = newSecurityCtx(t, http.MethodGet, "/api/v1/permission/users/"+poisonUser+"/roles", "", org)
			c.Params = gin.Params{{Key: "user_id", Value: poisonUser}}
			GetUserRoles(c)
			if recorder.Code != http.StatusOK {
				t.Fatalf("poison status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			poisonRoles := extractRolesFromResponse(t, recorder.Body.Bytes())
			if len(poisonRoles) != 0 {
				t.Fatalf("poisoned cross-org role leaked: %#v", poisonRoles)
			}
		})
	}
}

func extractRolesFromResponse(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, string(body))
	}
	data, _ := raw["data"].(map[string]any)
	if data == nil {
		return nil
	}
	items, _ := data["roles"].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
