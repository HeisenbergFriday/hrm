package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"peopleops/internal/config"
	"peopleops/internal/dingtalk"
)

func mask(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "<empty>"
	}
	if len(s) <= 8 {
		return s[:1] + "***"
	}
	return s[:4] + "***" + s[len(s)-2:]
}

func main() {
	if err := os.Chdir(`D:\AITEAM\HR`); err != nil {
		fatal(err)
	}
	_ = config.Load()
	if err := dingtalk.Init(); err != nil {
		// Init may warn; continue if token still works
		fmt.Printf("dingtalk.Init warn: %v\n", err)
	}

	orgID := strings.TrimSpace(os.Getenv("DINGTALK_STREAM_ORG_ID"))
	if orgID == "" {
		orgID = "default"
	}
	admin := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if admin == "" {
		fatal(fmt.Errorf("DINGTALK_ADMIN_USER_ID missing"))
	}
	fmt.Printf("org_id=%s admin=%s\n", orgID, mask(admin))

	mode := "list"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "list":
		listTemplates(orgID, admin)
	case "create":
		processCode := ""
		if len(os.Args) > 2 {
			processCode = os.Args[2]
		}
		createInstance(orgID, admin, processCode)
	default:
		fatal(fmt.Errorf("unknown mode %s", mode))
	}
}

func listTemplates(orgID, admin string) {
	items, err := dingtalk.ListManageableApprovalProcessesForOrg(orgID, admin)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("template_count=%d\n", len(items))
	for i, it := range items {
		code := strings.TrimSpace(it.ProcessCode)
		name := strings.TrimSpace(it.Name)
		safe := isSafeTemplate(name, code)
		fmt.Printf("%02d safe=%t name=%s code=%s category=%s\n", i+1, safe, name, code, strings.TrimSpace(it.Category))
	}
}

func isSafeTemplate(name, code string) bool {
	n := strings.ToLower(name + " " + code)
	// Prefer non-attendance business templates for first test.
	badKeywords := []string{"请假", "加班", "补卡", "出差", "leave", "overtime", "correction", "attendance"}
	for _, k := range badKeywords {
		if strings.Contains(n, strings.ToLower(k)) {
			return false
		}
	}
	goodKeywords := []string{"外出", "联调", "测试", "通用", "报销", "用章", "用印", "采购", "stream"}
	for _, k := range goodKeywords {
		if strings.Contains(n, strings.ToLower(k)) {
			return true
		}
	}
	// Unknown templates are not preferred automatically.
	return false
}

func createInstance(orgID, admin, processCode string) {
	processCode = strings.TrimSpace(processCode)
	if processCode == "" {
		items, err := dingtalk.ListManageableApprovalProcessesForOrg(orgID, admin)
		if err != nil {
			fatal(err)
		}
		// Prefer 外出, then any safe template.
		for _, it := range items {
			if strings.Contains(it.Name, "外出") {
				processCode = it.ProcessCode
				fmt.Printf("selected_name=%s selected_code=%s\n", it.Name, processCode)
				break
			}
		}
		if processCode == "" {
			for _, it := range items {
				if isSafeTemplate(it.Name, it.ProcessCode) {
					processCode = it.ProcessCode
					fmt.Printf("selected_name=%s selected_code=%s\n", it.Name, processCode)
					break
				}
			}
		}
		if processCode == "" {
			fatal(fmt.Errorf("no safe template found; pass process_code explicitly"))
		}
	} else {
		fmt.Printf("selected_code=%s\n", processCode)
	}

	token, err := dingtalk.GetAccessTokenForOrg(orgID)
	if err != nil {
		fatal(err)
	}

	// New workflow API: create process instance.
	// Keep form values minimal and generic.
	payload := map[string]interface{}{
		"processCode":      processCode,
		"originatorUserId": admin,
		"deptId":           -1,
		"microappAgentId":  0,
		"approvers":        []map[string]interface{}{{"actionType": "NONE", "userIds": []string{admin}}},
		"formComponentValues": []map[string]interface{}{
			{"name": "测试说明", "value": "Stream联调测试-请拒绝"},
			{"name": "说明", "value": "Stream联调测试-请拒绝"},
			{"name": "事由", "value": "Stream联调测试-请拒绝"},
		},
	}
	// Try new API first.
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.dingtalk.com/v1.0/workflow/processInstances", bytes.NewReader(body))
	if err != nil {
		fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close new API response body: %v\n", err)
		}
	}()
	raw, _ := io.ReadAll(resp.Body)
	fmt.Printf("new_api_http=%d\n", resp.StatusCode)
	// Do not dump full raw if it may include form; print only keys/instance id.
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if id := firstString(m, "instanceId", "processInstanceId"); id != "" {
		fmt.Printf("created_instance=%s\n", mask(id))
		return
	}
	// Fallback old oapi
	oldPayload := map[string]interface{}{
		"process_code":       processCode,
		"originator_user_id": admin,
		"dept_id":            -1,
		"approvers":          admin,
		"form_component_values": []map[string]interface{}{
			{"name": "测试说明", "value": "Stream联调测试-请拒绝"},
			{"name": "说明", "value": "Stream联调测试-请拒绝"},
			{"name": "事由", "value": "Stream联调测试-请拒绝"},
		},
	}
	oldBody, _ := json.Marshal(oldPayload)
	oldURL := fmt.Sprintf("https://oapi.dingtalk.com/topapi/processinstance/create?access_token=%s", token)
	oldReq, err := http.NewRequest(http.MethodPost, oldURL, bytes.NewReader(oldBody))
	if err != nil {
		fatal(err)
	}
	oldReq.Header.Set("Content-Type", "application/json")
	oldResp, err := client.Do(oldReq)
	if err != nil {
		fatal(err)
	}
	defer func() {
		if err := oldResp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close old API response body: %v\n", err)
		}
	}()
	oldRaw, _ := io.ReadAll(oldResp.Body)
	fmt.Printf("old_api_http=%d\n", oldResp.StatusCode)
	var om map[string]interface{}
	_ = json.Unmarshal(oldRaw, &om)
	if id := firstString(om, "process_instance_id"); id != "" {
		fmt.Printf("created_instance=%s\n", mask(id))
		if ec, ok := om["errcode"].(float64); ok {
			fmt.Printf("errcode=%d errmsg=%v\n", int(ec), om["errmsg"])
		}
		return
	}
	// Print sanitized error only
	fmt.Printf("create_failed new_keys=%v old_keys=%v\n", keysOf(m), keysOf(om))
	if msg := firstString(m, "message", "msg", "errmsg"); msg != "" {
		fmt.Printf("new_api_msg=%s\n", msg)
	}
	if msg := firstString(om, "errmsg", "message"); msg != "" {
		fmt.Printf("old_api_msg=%s\n", msg)
	}
	if code := firstString(m, "code"); code != "" {
		fmt.Printf("new_api_code=%s\n", code)
	}
	if ec, ok := om["errcode"].(float64); ok {
		fmt.Printf("old_api_errcode=%d\n", int(ec))
	}
	os.Exit(2)
}

func firstString(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if result, ok := m["result"].(map[string]interface{}); ok {
			if v, ok := result[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func keysOf(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(2)
}
