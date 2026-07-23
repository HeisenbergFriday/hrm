package dingtalk

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ApprovalProcessTemplate is a manageable DingTalk OA process form summary.
type ApprovalProcessTemplate struct {
	Name        string `json:"name"`
	ProcessCode string `json:"process_code"`
	IconURL     string `json:"icon_url,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// ApprovalProcessSchema is a readonly form schema lookup by process_code.
type ApprovalProcessSchema struct {
	Name     string `json:"name,omitempty"`
	FormUUID string `json:"form_uuid,omitempty"`
	Status   string `json:"status,omitempty"`
	RawHint  string `json:"raw_hint,omitempty"`
}

// listManageableApprovalProcessesPage is injectable for offline unit tests.
var listManageableApprovalProcessesPage = listManageableApprovalProcessesPageImpl

// ListManageableApprovalProcessesForOrg lists OA templates manageable by operatorUserID.
// Uses topapi/process/listbyuserid with pagination and process_code dedupe.
func ListManageableApprovalProcessesForOrg(orgID, operatorUserID string) ([]ApprovalProcessTemplate, error) {
	operatorUserID = strings.TrimSpace(operatorUserID)
	if operatorUserID == "" {
		return nil, fmt.Errorf("operator user_id is required")
	}
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, sanitizeDingTalkErr(err)
	}
	return collectApprovalProcessesWithToken(accessToken, operatorUserID)
}

// collectApprovalProcessesWithToken paginates listManageableApprovalProcessesPage and
// dedupes by process_code. Token fetch is separate so unit tests can stay offline.
func collectApprovalProcessesWithToken(accessToken, operatorUserID string) ([]ApprovalProcessTemplate, error) {
	const pageSize = 100
	offset := int64(0)
	seen := make(map[string]struct{})
	all := make([]ApprovalProcessTemplate, 0, 32)
	for page := 0; page < 100; page++ {
		items, nextCursor, err := listManageableApprovalProcessesPage(accessToken, operatorUserID, offset, pageSize)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			code := strings.TrimSpace(item.ProcessCode)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			all = append(all, item)
		}
		if nextCursor <= 0 || nextCursor == offset || len(items) == 0 {
			break
		}
		offset = nextCursor
	}
	return all, nil
}

// GetApprovalProcessSchemaForOrg fetches form schema by processCode (readonly).
// Endpoint: GET /v1.0/workflow/forms/schemas/processCodes
func GetApprovalProcessSchemaForOrg(orgID, processCode string) (*ApprovalProcessSchema, error) {
	processCode = strings.TrimSpace(processCode)
	if processCode == "" {
		return nil, fmt.Errorf("process_code is required")
	}
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, sanitizeDingTalkErr(err)
	}
	endpoint := "https://api.dingtalk.com/v1.0/workflow/forms/schemas/processCodes?processCode=" + url.QueryEscape(processCode)
	resp, err := getJSON(endpoint, map[string]string{
		"x-acs-dingtalk-access-token": accessToken,
	})
	if err != nil {
		return nil, sanitizeDingTalkErr(err)
	}
	if code, ok := resp["code"].(string); ok && code != "" && !strings.EqualFold(code, "OK") {
		msg, _ := resp["message"].(string)
		return nil, sanitizeDingTalkErr(fmt.Errorf("dingtalk process schema failed: code=%s message=%s", code, msg))
	}
	// OAPI-style fallback
	if errcode, ok := resp["errcode"].(float64); ok && errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, sanitizeDingTalkErr(fmt.Errorf("dingtalk process schema failed: errcode=%v errmsg=%s", errcode, errmsg))
	}

	result := resp
	if nested, ok := resp["result"].(map[string]interface{}); ok {
		result = nested
	}
	schema := &ApprovalProcessSchema{
		Name:     firstString(result, "name", "formName", "title"),
		FormUUID: firstString(result, "formUuid", "form_uuid", "uuid"),
		Status:   firstString(result, "status", "formStatus"),
	}
	if schema.Name == "" && schema.FormUUID == "" {
		schema.RawHint = "schema response missing name/form_uuid"
	}
	return schema, nil
}

func listManageableApprovalProcessesPageImpl(accessToken, operatorUserID string, offset, size int64) ([]ApprovalProcessTemplate, int64, error) {
	if size <= 0 {
		size = 100
	}
	body := map[string]interface{}{
		"userid": operatorUserID,
		"offset": offset,
		"size":   size,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/process/listbyuserid?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, 0, sanitizeDingTalkErr(fmt.Errorf("dingtalk process listbyuserid failed: %w", err))
	}
	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, 0, sanitizeDingTalkErr(fmt.Errorf("dingtalk process listbyuserid failed: errcode=%v errmsg=%s", errcode, errmsg))
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		// some tenants return process_list at top level
		if list, ok := resp["process_list"].([]interface{}); ok {
			return parseApprovalProcessTemplates(list), 0, nil
		}
		return nil, 0, fmt.Errorf("dingtalk process listbyuserid response format invalid: missing result")
	}
	rawList, _ := result["process_list"].([]interface{})
	if rawList == nil {
		rawList, _ = result["list"].([]interface{})
	}
	nextCursor := int64(0)
	switch v := result["next_cursor"].(type) {
	case float64:
		nextCursor = int64(v)
	case int64:
		nextCursor = v
	case int:
		nextCursor = int64(v)
	}
	return parseApprovalProcessTemplates(rawList), nextCursor, nil
}

func parseApprovalProcessTemplates(raw []interface{}) []ApprovalProcessTemplate {
	out := make([]ApprovalProcessTemplate, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code := firstString(m, "process_code", "processCode")
		if strings.TrimSpace(code) == "" {
			continue
		}
		name := firstString(m, "name", "flow_title", "flowTitle", "title")
		out = append(out, ApprovalProcessTemplate{
			Name:        name,
			ProcessCode: strings.TrimSpace(code),
			IconURL:     firstString(m, "icon_url", "iconUrl"),
			URL:         sanitizeApprovalTemplateURL(firstString(m, "url", "form_url", "formUrl")),
			Description: firstString(m, "description", "desc"),
			Category:    firstString(m, "category", "group_name", "groupName"),
		})
	}
	return out
}

func sanitizeApprovalTemplateURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		// strip query/fragment best-effort
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			return raw[:i]
		}
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

var (
	reAccessTokenSecret = regexp.MustCompile(`(?i)(access_token|client_secret|appsecret|app_secret|secret)=([^&\s]+)`)
)

func sanitizeDingTalkErr(err error) error {
	if err == nil {
		return nil
	}
	msg := reAccessTokenSecret.ReplaceAllString(err.Error(), "${1}=***")
	return fmt.Errorf("%s", msg)
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			case fmt.Stringer:
				if s := strings.TrimSpace(t.String()); s != "" {
					return s
				}
			}
		}
	}
	return ""
}
