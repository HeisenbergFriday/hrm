package dingtalk

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stubDingTalkHTTPClient(t *testing.T, transport roundTripFunc) {
	t.Helper()
	original := dingTalkHTTPClient
	dingTalkHTTPClient = &http.Client{Transport: transport, Timeout: time.Second}
	t.Cleanup(func() { dingTalkHTTPClient = original })
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func clearTokenCacheForTest(t *testing.T) {
	t.Helper()
	tokenMu.Lock()
	original := tokenByOrg
	tokenByOrg = make(map[string]accessTokenCacheEntry)
	tokenMu.Unlock()
	t.Cleanup(func() {
		tokenMu.Lock()
		tokenByOrg = original
		tokenMu.Unlock()
	})
}

func TestGetAccessTokenFailureUsesSafeCode(t *testing.T) {
	clearTokenCacheForTest(t)
	stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"expireIn":7200}`), nil
	})

	_, err := getAccessTokenWithConfig(Config{OrgID: "org-a", AppKey: "key-a", AppSecret: "secret-a"})
	if SyncErrorCode(err) != ErrorCodeTokenFailed {
		t.Fatalf("error code = %q, want %q; err=%v", SyncErrorCode(err), ErrorCodeTokenFailed, err)
	}
	if strings.Contains(err.Error(), "secret-a") {
		t.Fatalf("token error leaked secret: %v", err)
	}
}

func TestFetchDeptTreePermissionDenied(t *testing.T) {
	stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errcode":60011,"errmsg":"没有通讯录权限"}`), nil
	})
	var departments []DeptInfo
	err := fetchDeptTree("safe-token", 1, &departments)
	if SyncErrorCode(err) != ErrorCodePermissionDenied {
		t.Fatalf("error code = %q, want permission denied; err=%v", SyncErrorCode(err), err)
	}
}

func TestDingTalkHTTPNetworkErrorIsRedactedForStandardAndLogrusOutput(t *testing.T) {
	const leakedToken = "token-must-not-leak"
	stubDingTalkHTTPClient(t, func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed " + req.URL.String() + " Authorization=Bearer-secret Cookie=session-secret DSN=user:db-pass@tcp(db:3306)/peopleops")
	})

	_, err := postJSONOAPI("https://oapi.dingtalk.test/path?access_token="+leakedToken, map[string]interface{}{})
	if SyncErrorCode(err) != ErrorCodeNetworkFailed {
		t.Fatalf("error code = %q, want network failed; err=%v", SyncErrorCode(err), err)
	}

	var logs bytes.Buffer
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(&logs)
	t.Cleanup(func() { logrus.SetOutput(originalOutput) })
	logrus.Warnf("org sync request failed: %s", safeDingTalkErrorForLog(err))

	visible := err.Error() + logs.String()
	for _, secret := range []string{leakedToken, "Bearer-secret", "session-secret", "db-pass"} {
		if strings.Contains(visible, secret) {
			t.Fatalf("diagnostic output leaked %q: %s", secret, visible)
		}
	}
}

func TestFetchDeptTreeRejectsMissingOrInvalidResult(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"errcode":0}`},
		{name: "wrong type", body: `{"errcode":0,"result":{"dept_id":2}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, test.body), nil
			})
			var departments []DeptInfo
			err := fetchDeptTree("safe-token", 1, &departments)
			if SyncErrorCode(err) != ErrorCodeResponseInvalid {
				t.Fatalf("error code = %q, want response invalid; err=%v", SyncErrorCode(err), err)
			}
		})
	}
}

func TestSyncDepartmentsForConfigFailsWhenRootDetailFails(t *testing.T) {
	clearTokenCacheForTest(t)
	cfg := AppConfig{OrgID: "org-a", AppKey: "key-a", AppSecret: "secret-a"}
	cacheKey := configFromAppConfig(cfg).cacheKey()
	tokenMu.Lock()
	tokenByOrg[cacheKey] = accessTokenCacheEntry{token: "safe-token", expiry: time.Now().Add(time.Hour)}
	tokenMu.Unlock()

	stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errcode":60011,"errmsg":"permission denied"}`), nil
	})
	departments, err := SyncDepartmentsForConfig(cfg)
	if err == nil || departments != nil {
		t.Fatalf("root detail failure must abort sync: departments=%v err=%v", departments, err)
	}
	if SyncErrorCode(err) != ErrorCodePermissionDenied {
		t.Fatalf("error code = %q, want permission denied", SyncErrorCode(err))
	}
}

func TestNonDefaultOrganizationMissingAgentIDDoesNotUseGlobalFallback(t *testing.T) {
	t.Setenv("DINGTALK_AGENT_ID", "999")
	agentID, err := requireDingTalkAgentID(Config{OrgID: "org-a"})
	if agentID != 0 || SyncErrorCode(err) != ErrorCodeConfigMissing {
		t.Fatalf("agentID=%d code=%q, want fail-closed config error", agentID, SyncErrorCode(err))
	}
}

func TestFetchDeptUsersRejectsIncompleteResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing result", body: `{"errcode":0}`},
		{name: "missing list", body: `{"errcode":0,"result":{"has_more":false}}`},
		{name: "missing has more", body: `{"errcode":0,"result":{"list":[]}}`},
		{name: "unchanged cursor", body: `{"errcode":0,"result":{"list":[],"has_more":true,"next_cursor":0}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, test.body), nil
			})
			users, err := fetchDeptUsers("safe-token", 1)
			if err == nil || users != nil {
				t.Fatalf("incomplete employee response must fail closed: users=%#v err=%v", users, err)
			}
			if SyncErrorCode(err) != ErrorCodeResponseInvalid {
				t.Fatalf("error code = %q, want %q", SyncErrorCode(err), ErrorCodeResponseInvalid)
			}
		})
	}
}

func TestFetchDeptUsersNormalizesHiredDate(t *testing.T) {
	tests := []struct {
		name      string
		rawValue  string
		wantValue string
	}{
		{name: "millisecond number", rawValue: `1731254400000`, wantValue: "2024-11-11"},
		{name: "millisecond string", rawValue: `"1731254400000"`, wantValue: "2024-11-11"},
		{name: "date string", rawValue: `"2024-11-11"`, wantValue: "2024-11-11"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
				body := `{"errcode":0,"result":{"list":[{"userid":"employee-1","name":"员工","active":true,"dept_id_list":[1],"hired_date":` + test.rawValue + `}],"has_more":false}}`
				return jsonResponse(http.StatusOK, body), nil
			})

			users, err := fetchDeptUsers("safe-token", 1)
			if err != nil {
				t.Fatalf("fetchDeptUsers() error = %v", err)
			}
			if len(users) != 1 || users[0].HiredDate != test.wantValue {
				t.Fatalf("users = %#v, want hired date %q", users, test.wantValue)
			}
		})
	}
}

func TestSyncUsersWithDeptsForConfigRequiresEveryDepartmentSource(t *testing.T) {
	clearTokenCacheForTest(t)
	cfg := AppConfig{OrgID: "org-a", AppKey: "key-a", AppSecret: "secret-a"}
	cacheKey := configFromAppConfig(cfg).cacheKey()
	tokenMu.Lock()
	tokenByOrg[cacheKey] = accessTokenCacheEntry{token: "safe-token", expiry: time.Now().Add(time.Hour)}
	tokenMu.Unlock()

	requestCount := 0
	stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
		requestCount++
		if requestCount == 1 {
			return jsonResponse(http.StatusOK, `{"errcode":0,"result":{"list":[{"userid":"employee-1","name":"员工","active":true,"dept_id_list":[1]}],"has_more":false}}`), nil
		}
		return jsonResponse(http.StatusOK, `{"errcode":60011,"errmsg":"permission denied"}`), nil
	})

	users, err := SyncUsersWithDeptsForConfig(cfg, []DeptInfo{{DeptID: 1, Name: "总部"}, {DeptID: 2, Name: "受限部门"}})
	if err == nil || users != nil {
		t.Fatalf("partial department source must not be returned as complete: users=%#v err=%v", users, err)
	}
	if SyncErrorCode(err) != ErrorCodeUserSourceIncomplete {
		t.Fatalf("error code = %q, want %q", SyncErrorCode(err), ErrorCodeUserSourceIncomplete)
	}
}

func TestSyncUsersWithDeptsForConfigRejectsEmptyCompleteSource(t *testing.T) {
	clearTokenCacheForTest(t)
	cfg := AppConfig{OrgID: "org-a", AppKey: "key-a", AppSecret: "secret-a"}
	cacheKey := configFromAppConfig(cfg).cacheKey()
	tokenMu.Lock()
	tokenByOrg[cacheKey] = accessTokenCacheEntry{token: "safe-token", expiry: time.Now().Add(time.Hour)}
	tokenMu.Unlock()
	stubDingTalkHTTPClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"errcode":0,"result":{"list":[],"has_more":false}}`), nil
	})

	users, err := SyncUsersWithDeptsForConfig(cfg, []DeptInfo{{DeptID: 1, Name: "总部"}})
	if err == nil || users != nil || SyncErrorCode(err) != ErrorCodeUserSourceIncomplete {
		t.Fatalf("empty source must fail closed: users=%#v code=%q err=%v", users, SyncErrorCode(err), err)
	}
}
