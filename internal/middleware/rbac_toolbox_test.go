package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newToolboxTestContext(t *testing.T, perms []string, menus []string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("userID", "u1")
	c.Set("orgID", "orgA")
	authCtx := &AuthContext{
		OrgID:         "orgA",
		UserID:        "u1",
		RawUserID:     "u1",
		PermissionSet: map[string]struct{}{},
		MenuKeySet:    map[string]struct{}{},
	}
	for _, p := range perms {
		authCtx.PermissionSet[p] = struct{}{}
	}
	for _, m := range menus {
		authCtx.MenuKeySet[m] = struct{}{}
	}
	c.Set(authContextKey, authCtx)
	return c, w
}

func runMiddleware(t *testing.T, mw gin.HandlerFunc, c *gin.Context) bool {
	t.Helper()
	mw(c)
	return !c.IsAborted()
}

func TestToolboxRBAC_MenuOnlyCannotOperate(t *testing.T) {
	c, w := newToolboxTestContext(t, nil, []string{"menu:attendance-toolbox"})
	ok := runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("menu-only should be 403, ok=%v code=%d", ok, w.Code)
	}
}

func TestToolboxRBAC_OperateCanOperate(t *testing.T) {
	c, _ := newToolboxTestContext(t, []string{"attendance_toolbox_operate"}, nil)
	if !runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c) {
		t.Fatal("operate should pass")
	}
}

func TestToolboxRBAC_OperateCannotDingtalkSync(t *testing.T) {
	c, w := newToolboxTestContext(t, []string{"attendance_toolbox_operate"}, nil)
	ok := runMiddleware(t, RequirePermission("attendance_toolbox_dingtalk_sync", "attendance_manage"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("operate-only must not sync, ok=%v code=%d", ok, w.Code)
	}
}

func TestToolboxRBAC_SyncOnlyCannotOperate(t *testing.T) {
	c, w := newToolboxTestContext(t, []string{"attendance_toolbox_dingtalk_sync"}, nil)
	ok := runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("sync-only must not operate, ok=%v code=%d", ok, w.Code)
	}
}

func TestToolboxRBAC_QuickRequiresBoth(t *testing.T) {
	// missing sync
	c, w := newToolboxTestContext(t, []string{"attendance_toolbox_operate"}, nil)
	ok := runMiddleware(t, RequireAllPermissions("attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("quick missing sync must 403, ok=%v code=%d", ok, w.Code)
	}
	// missing operate
	c2, w2 := newToolboxTestContext(t, []string{"attendance_toolbox_dingtalk_sync"}, nil)
	ok2 := runMiddleware(t, RequireAllPermissions("attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"), c2)
	if ok2 || w2.Code != http.StatusForbidden {
		t.Fatalf("quick missing operate must 403, ok=%v code=%d", ok2, w2.Code)
	}
	// both present
	c3, _ := newToolboxTestContext(t, []string{"attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"}, nil)
	if !runMiddleware(t, RequireAllPermissions("attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"), c3) {
		t.Fatal("quick with both perms should pass")
	}
}

func TestToolboxRBAC_AttendanceManageCompat(t *testing.T) {
	// single-permission routes
	c, _ := newToolboxTestContext(t, []string{"attendance_manage"}, nil)
	if !runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c) {
		t.Fatal("attendance_manage should authorize operate route")
	}
	c2, _ := newToolboxTestContext(t, []string{"attendance_manage"}, nil)
	if !runMiddleware(t, RequirePermission("attendance_toolbox_dingtalk_sync", "attendance_manage"), c2) {
		t.Fatal("attendance_manage should authorize sync route")
	}
	// quick (AND) — attendance_manage compat inside RequireAllPermissions
	c3, _ := newToolboxTestContext(t, []string{"attendance_manage"}, nil)
	if !runMiddleware(t, RequireAllPermissions("attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"), c3) {
		t.Fatal("attendance_manage should authorize quick workflow")
	}
}

func TestToolboxRBAC_RulesEditRoute(t *testing.T) {
	c, w := newToolboxTestContext(t, []string{"attendance_toolbox_operate"}, nil)
	ok := runMiddleware(t, RequirePermission("attendance_toolbox_rules_edit", "attendance_manage"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("operate-only must not edit rules, ok=%v code=%d", ok, w.Code)
	}
	c2, _ := newToolboxTestContext(t, []string{"attendance_toolbox_rules_edit"}, nil)
	if !runMiddleware(t, RequirePermission("attendance_toolbox_rules_edit", "attendance_manage"), c2) {
		t.Fatal("rules_edit should pass")
	}
}

func TestToolboxRBAC_OperateWithRulesStillNeedsRulesEditOnRoute(t *testing.T) {
	// Route middleware for rules endpoints requires rules_edit; operate alone is insufficient.
	c, w := newToolboxTestContext(t, []string{"attendance_toolbox_operate"}, nil)
	ok := runMiddleware(t, RequirePermission("attendance_toolbox_rules_edit", "attendance_manage"), c)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("operate without rules_edit must 403 on rules route")
	}
	// rules_edit + operate (typical custom-rule overtime user)
	c2, _ := newToolboxTestContext(t, []string{"attendance_toolbox_operate", "attendance_toolbox_rules_edit"}, nil)
	if !runMiddleware(t, RequirePermission("attendance_toolbox_rules_edit", "attendance_manage"), c2) {
		t.Fatal("rules_edit+operate should pass rules route")
	}
	if !runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c2) {
		t.Fatal("rules_edit+operate should also pass operate route")
	}
}

func TestToolboxRBAC_MenuCannotDownloadOrSync(t *testing.T) {
	c, w := newToolboxTestContext(t, nil, []string{"menu:attendance-toolbox"})
	if runMiddleware(t, RequirePermission("attendance_toolbox_operate", "attendance_manage"), c) || w.Code != http.StatusForbidden {
		t.Fatal("menu-only cannot download/operate")
	}
	c2, w2 := newToolboxTestContext(t, nil, []string{"menu:attendance-toolbox"})
	if runMiddleware(t, RequirePermission("attendance_toolbox_dingtalk_sync", "attendance_manage"), c2) || w2.Code != http.StatusForbidden {
		t.Fatal("menu-only cannot dingtalk sync")
	}
}
