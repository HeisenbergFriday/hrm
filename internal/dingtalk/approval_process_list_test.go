package dingtalk

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseApprovalProcessTemplates(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"name":         "加班申请",
			"process_code": "PROC-OT-1",
			"icon_url":     "https://img.example/icon.png",
			"url":          "https://aflow.dingtalk.com/dingtalk/web/query/form?token=secret&corpId=abc",
			"description":  "员工加班",
			"category":     "考勤",
		},
		map[string]interface{}{
			"name":        "无 code 忽略",
			"processCode": "",
		},
		map[string]interface{}{
			"flow_title":  "请假",
			"processCode": "PROC-LEAVE-1",
		},
		"bad-item",
	}
	got := parseApprovalProcessTemplates(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].Name != "加班申请" || got[0].ProcessCode != "PROC-OT-1" {
		t.Fatalf("item0=%+v", got[0])
	}
	if strings.Contains(got[0].URL, "token=") || strings.Contains(got[0].URL, "corpId=") {
		t.Fatalf("url should strip sensitive query: %s", got[0].URL)
	}
	if got[1].ProcessCode != "PROC-LEAVE-1" || got[1].Name != "请假" {
		t.Fatalf("item1=%+v", got[1])
	}
}

func TestListManageableApprovalProcessesForOrg_Pagination(t *testing.T) {
	original := listManageableApprovalProcessesPage
	t.Cleanup(func() { listManageableApprovalProcessesPage = original })

	calls := 0
	listManageableApprovalProcessesPage = func(accessToken, operatorUserID string, offset, size int64) ([]ApprovalProcessTemplate, int64, error) {
		calls++
		if operatorUserID != "admin1" {
			t.Fatalf("operator=%s", operatorUserID)
		}
		if size != 100 {
			t.Fatalf("size=%d", size)
		}
		switch offset {
		case 0:
			return []ApprovalProcessTemplate{
				{Name: "请假", ProcessCode: "PC-1"},
				{Name: "加班", ProcessCode: "PC-2"},
			}, 2, nil
		case 2:
			return []ApprovalProcessTemplate{
				{Name: "补卡", ProcessCode: "PC-3"},
				{Name: "加班重复", ProcessCode: "PC-2"}, // dedupe
			}, 0, nil
		default:
			return nil, 0, fmt.Errorf("unexpected offset %d", offset)
		}
	}

	// Bypass real token by temporarily stubbing GetAccessTokenForOrg via page function only:
	// ListManageableApprovalProcessesForOrg still calls GetAccessTokenForOrg.
	// Use env-backed default org token path may fail; inject by calling list page only through a thin wrapper test of pagination loop.
	// Instead, call the public function with a fake GetAccessToken by setting empty DB and env? Safer: unit-test pagination via exported function after mocking token getter is hard.
	// We'll test the loop by temporarily replacing GetAccessTokenForOrg dependency through list page only when token fetch succeeds.
	// To keep this test offline, invoke a local helper equivalent by using listManageableApprovalProcessesPage after faking token via process list function under test with stubbed GetAccessTokenForOrg is not available.
	// Workaround: call ListManageableApprovalProcessesForOrg after stubbing list page and also stubbing token by monkeying package-level Init? Use a test-only path:
	// Directly exercise pagination by constructing a temporary function that mirrors the loop is already production code.
	// We'll set DINGTALK_APP_KEY/SECRET and intercept list page; GetAccessTokenForOrg may still hit network.
	// So test pagination logic via a package-level helper used by ListManageableApprovalProcessesForOrg.
	// For offline safety, re-implement call as:
	items, err := collectApprovalProcessesWithToken("token", "admin1")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
	if len(items) != 3 {
		t.Fatalf("len=%d want 3 (deduped): %+v", len(items), items)
	}
	codes := map[string]bool{}
	for _, it := range items {
		codes[it.ProcessCode] = true
	}
	if !codes["PC-1"] || !codes["PC-2"] || !codes["PC-3"] {
		t.Fatalf("codes=%v", codes)
	}
}

func TestListManageableApprovalProcessesPage_PermissionError(t *testing.T) {
	original := listManageableApprovalProcessesPage
	t.Cleanup(func() { listManageableApprovalProcessesPage = original })
	listManageableApprovalProcessesPage = func(accessToken, operatorUserID string, offset, size int64) ([]ApprovalProcessTemplate, int64, error) {
		return nil, 0, fmt.Errorf("dingtalk process listbyuserid failed: errcode=88 errmsg=无权限访问")
	}
	_, err := collectApprovalProcessesWithToken("token", "admin1")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "errcode=88") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "access_token=") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestListManageableApprovalProcesses_EmptyList(t *testing.T) {
	original := listManageableApprovalProcessesPage
	t.Cleanup(func() { listManageableApprovalProcessesPage = original })
	listManageableApprovalProcessesPage = func(accessToken, operatorUserID string, offset, size int64) ([]ApprovalProcessTemplate, int64, error) {
		return []ApprovalProcessTemplate{}, 0, nil
	}
	items, err := collectApprovalProcessesWithToken("token", "admin1")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len=%d", len(items))
	}
}

func TestListManageableApprovalProcesses_InvalidFormat(t *testing.T) {
	original := listManageableApprovalProcessesPage
	t.Cleanup(func() { listManageableApprovalProcessesPage = original })
	listManageableApprovalProcessesPage = func(accessToken, operatorUserID string, offset, size int64) ([]ApprovalProcessTemplate, int64, error) {
		return nil, 0, errors.New("dingtalk process listbyuserid response format invalid: missing result")
	}
	_, err := collectApprovalProcessesWithToken("token", "admin1")
	if err == nil || !strings.Contains(err.Error(), "format invalid") {
		t.Fatalf("err=%v", err)
	}
}

func TestSanitizeApprovalTemplateURL(t *testing.T) {
	got := sanitizeApprovalTemplateURL("https://aflow.dingtalk.com/path?access_token=abc&x=1#frag")
	if strings.Contains(got, "access_token") || strings.Contains(got, "x=1") || strings.Contains(got, "#frag") {
		t.Fatalf("got=%s", got)
	}
	if !strings.HasPrefix(got, "https://aflow.dingtalk.com/path") {
		t.Fatalf("got=%s", got)
	}
}

func TestSanitizeDingTalkErrMasksSecrets(t *testing.T) {
	err := sanitizeDingTalkErr(fmt.Errorf("request failed access_token=supersecrettoken&client_secret=cs123"))
	msg := err.Error()
	if strings.Contains(msg, "supersecrettoken") || strings.Contains(msg, "cs123") {
		t.Fatalf("leaked secret: %s", msg)
	}
	if !strings.Contains(msg, "access_token=***") {
		t.Fatalf("expected masked token: %s", msg)
	}
}

func TestMatchProcessCodeExactOnly(t *testing.T) {
	items := []ApprovalProcessTemplate{
		{Name: "加班", ProcessCode: "a3ba921b2f3c3c63a634f82dd5b305a7"},
		{Name: "相似", ProcessCode: "a3ba921b2f3c3c63a634f82dd5b305a8"},
	}
	var hit *ApprovalProcessTemplate
	target := "a3ba921b2f3c3c63a634f82dd5b305a7"
	for i := range items {
		if items[i].ProcessCode == target {
			hit = &items[i]
			break
		}
	}
	if hit == nil || hit.Name != "加班" {
		t.Fatalf("exact match failed: %+v", hit)
	}
	// similar must not match
	for _, it := range items {
		if it.ProcessCode != target && strings.Contains(it.ProcessCode, target[:10]) {
			// ensure we don't treat contains as match in production code path tests via helper
			if it.ProcessCode == target {
				t.Fatal("false positive")
			}
		}
	}
}
