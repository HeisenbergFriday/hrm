package dingtalk

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func installGroupRobotTestOrganization(t *testing.T, extension map[string]interface{}) {
	t.Helper()
	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open("file:group-robot-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Organization{}); err != nil {
		t.Fatalf("migrate organization: %v", err)
	}
	if err := db.Create(&database.Organization{
		OrgID:          "org-a",
		Name:           "组织A",
		CorpID:         "corp-a-" + t.Name(),
		DingTalkAppKey: "app-key-a",
		DingTalkSecret: "app-secret-a",
		Status:         "active",
		Extension:      extension,
	}).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })
	clearTokenCacheForTest(t)
}

func TestSendGroupScheduleMarkdownForOrgAccepted(t *testing.T) {
	installGroupRobotTestOrganization(t, map[string]interface{}{"dingtalk_robot_code": "robot-a"})
	var groupBody map[string]interface{}
	stubDingTalkHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/oauth2/accessToken"):
			return jsonResponse(http.StatusOK, `{"accessToken":"token-a","expireIn":7200}`), nil
		case strings.Contains(req.URL.Path, "/robot/groupMessages/send"):
			if got := req.Header.Get("x-acs-dingtalk-access-token"); got != "token-a" {
				t.Fatalf("access token header = %q", got)
			}
			if err := json.NewDecoder(req.Body).Decode(&groupBody); err != nil {
				t.Fatalf("decode group body: %v", err)
			}
			return jsonResponse(http.StatusOK, `{"processQueryKey":"request-accepted"}`), nil
		default:
			t.Fatalf("unexpected URL: %s", req.URL.String())
			return nil, nil
		}
	})

	accepted, err := SendGroupScheduleMarkdownForOrg("org-a", "cid-group-a", "七月作息表", "请查收", "https://hr.example/api/v1/week-schedule/group-image?token=hidden")
	if err != nil {
		t.Fatalf("send group message: %v", err)
	}
	if accepted == nil || accepted.RequestID != "request-accepted" {
		t.Fatalf("acceptance = %#v", accepted)
	}
	if groupBody["robotCode"] != "robot-a" || groupBody["openConversationId"] != "cid-group-a" || groupBody["msgKey"] != "sampleMarkdown" {
		t.Fatalf("group body = %#v", groupBody)
	}
	param, _ := groupBody["msgParam"].(string)
	if !strings.Contains(param, "https://hr.example/") || strings.Contains(param, "media_id") {
		t.Fatalf("msgParam must use HTTPS image URL, got %s", param)
	}
}

func TestSendGroupScheduleMarkdownForOrgUsesOrgAppKeyAsRobotCode(t *testing.T) {
	installGroupRobotTestOrganization(t, nil)
	stubDingTalkHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/oauth2/accessToken") {
			return jsonResponse(http.StatusOK, `{"accessToken":"token-a","expireIn":7200}`), nil
		}
		var body map[string]interface{}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body["robotCode"] != "app-key-a" {
			t.Fatalf("robotCode = %#v, want organization app key", body["robotCode"])
		}
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	if _, err := SendGroupScheduleMarkdownForOrg("org-a", "cid", "t", "c", "https://hr.example/image"); err != nil {
		t.Fatal(err)
	}
}

func TestSendGroupScheduleMarkdownForOrgMapsRobotNotInGroup(t *testing.T) {
	installGroupRobotTestOrganization(t, nil)
	stubDingTalkHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/oauth2/accessToken") {
			return jsonResponse(http.StatusOK, `{"accessToken":"token-a","expireIn":7200}`), nil
		}
		return jsonResponse(http.StatusOK, `{"code":"robotNotInConversation","message":"robot is not in conversation"}`), nil
	})
	_, err := SendGroupScheduleMarkdownForOrg("org-a", "invalid-cid", "t", "c", "https://hr.example/image")
	if SyncErrorCode(err) != ErrorCodeGroupUnavailable || SyncErrorSafeMessage(err) != "群聊无效或机器人未加入该群" {
		t.Fatalf("err=%v code=%s safe=%s", err, SyncErrorCode(err), SyncErrorSafeMessage(err))
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "temporary timeout token=secret-value" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

var _ net.Error = timeoutNetError{}

func TestSendGroupScheduleMarkdownForOrgMapsTimeoutAndRedactsSecrets(t *testing.T) {
	installGroupRobotTestOrganization(t, nil)
	stubDingTalkHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/oauth2/accessToken") {
			return jsonResponse(http.StatusOK, `{"accessToken":"token-a","expireIn":7200}`), nil
		}
		return nil, timeoutNetError{}
	})
	_, err := SendGroupScheduleMarkdownForOrg("org-a", "cid", "t", "c", "https://hr.example/image")
	if SyncErrorCode(err) != ErrorCodeNetworkFailed {
		t.Fatalf("code=%s err=%v", SyncErrorCode(err), err)
	}
	if summary := SafeErrorSummary(err); strings.Contains(summary, "secret-value") || strings.Contains(summary, "token=") {
		t.Fatalf("unsafe summary: %s", summary)
	}
}

func TestSendGroupScheduleMarkdownForOrgRejectsMissingConfigAndNonHTTPSImage(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })
	if _, err := SendGroupScheduleMarkdownForOrg("org-missing", "cid", "t", "c", "https://hr.example/image"); SyncErrorCode(err) != ErrorCodeConfigMissing {
		t.Fatalf("missing config err=%v code=%s", err, SyncErrorCode(err))
	}
	if _, err := SendGroupScheduleMarkdownForOrg("org-a", "cid", "t", "c", "http://hr.example/image"); SyncErrorCode(err) != ErrorCodeConfigMissing {
		t.Fatalf("non-HTTPS err=%v code=%s", err, SyncErrorCode(err))
	}
}

func TestGroupMessageRejectionDoesNotExposeRawResponseBody(t *testing.T) {
	err := classifyGroupMessageRejection("Denied", `secret="private" webhook=https://example.invalid/token`)
	if err == nil {
		t.Fatal("expected error")
	}
	if summary := SafeErrorSummary(err); strings.Contains(summary, "private") || strings.Contains(summary, "example.invalid") {
		t.Fatalf("unsafe summary: %s", summary)
	}
}
