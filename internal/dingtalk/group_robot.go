package dingtalk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	ErrorCodeGroupUnavailable = "DINGTALK_GROUP_UNAVAILABLE"
	ErrorCodeGroupRejected    = "DINGTALK_GROUP_REJECTED"
)

// GroupMessageAcceptance means DingTalk accepted the asynchronous group-message
// request. It is deliberately not named "success" because delivery is not yet
// confirmed by this API.
type GroupMessageAcceptance struct {
	RequestID string
}

// SendGroupScheduleMarkdownForOrg submits text plus an HTTPS image to a group
// through the enterprise-internal robot API. Credentials and RobotCode are
// resolved exclusively from the current organization configuration.
func SendGroupScheduleMarkdownForOrg(orgID, openConversationID, title, content, imageURL string) (*GroupMessageAcceptance, error) {
	orgID = strings.TrimSpace(orgID)
	openConversationID = strings.TrimSpace(openConversationID)
	if orgID == "" {
		return nil, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺失", errors.New("orgID is empty"))
	}
	if openConversationID == "" {
		return nil, newSyncError(ErrorCodeGroupUnavailable, "群聊无效或机器人未加入该群", errors.New("openConversationId is empty"))
	}
	parsedURL, err := url.Parse(strings.TrimSpace(imageURL))
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return nil, newSyncError(ErrorCodeConfigMissing, "作息表临时图片地址必须使用 HTTPS", errors.New("invalid HTTPS image URL"))
	}

	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return nil, err
	}
	cfg = cfg.normalized()
	robotCode := strings.TrimSpace(cfg.RobotCode)
	if robotCode == "" {
		return nil, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺少机器人编码", fmt.Errorf("missing robot code for org %s", cfg.OrgID))
	}
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = "作息时间表"
	}
	content = strings.TrimSpace(content)
	if content == "" {
		content = title + "，请查收。"
	}
	msgParamBytes, err := json.Marshal(map[string]string{
		"title": title,
		"text":  content + "\n\n![作息时间表](" + parsedURL.String() + ")",
	})
	if err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉请求数据格式异常", err)
	}

	resp, err := postJSON("https://api.dingtalk.com/v1.0/robot/groupMessages/send", map[string]interface{}{
		"robotCode":          robotCode,
		"openConversationId": openConversationID,
		"msgKey":             "sampleMarkdown",
		"msgParam":           string(msgParamBytes),
	}, map[string]string{"x-acs-dingtalk-access-token": accessToken})
	if err != nil {
		return nil, err
	}
	if err := groupMessageResponseError(resp); err != nil {
		return nil, err
	}

	requestID := firstNonEmpty(
		getString(resp, "requestId"),
		getString(resp, "request_id"),
		getString(resp, "processQueryKey"),
		getString(resp, "taskId"),
	)
	return &GroupMessageAcceptance{RequestID: requestID}, nil
}

func groupMessageResponseError(resp map[string]interface{}) error {
	if resp == nil {
		return newSyncError(ErrorCodeResponseInvalid, "钉钉接口返回异常", errors.New("empty group message response"))
	}
	if success, ok := resp["success"].(bool); ok && !success {
		return classifyGroupMessageRejection(getString(resp, "code"), getString(resp, "message"))
	}
	code := strings.TrimSpace(getString(resp, "code"))
	if code != "" && code != "0" && !strings.EqualFold(code, "ok") {
		return classifyGroupMessageRejection(code, getString(resp, "message"))
	}
	return nil
}

func classifyGroupMessageRejection(code, message string) error {
	detail := strings.TrimSpace(code + " " + message)
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "conversation") || strings.Contains(lower, "robot") ||
		strings.Contains(lower, "not in") || strings.Contains(lower, "not exist") ||
		strings.Contains(message, "不在群") || strings.Contains(message, "群不存在") || strings.Contains(message, "群聊") {
		return newSyncError(ErrorCodeGroupUnavailable, "群聊无效或机器人未加入该群", errors.New(detail))
	}
	return newSyncError(ErrorCodeGroupRejected, "钉钉拒绝了群消息请求", errors.New(detail))
}

// SafeErrorSummary returns a bounded, redacted diagnostic suitable for local
// logs. Callers should prefer SyncErrorSafeMessage for user-facing text.
func SafeErrorSummary(err error) string {
	return safeDingTalkErrorForLog(err)
}
