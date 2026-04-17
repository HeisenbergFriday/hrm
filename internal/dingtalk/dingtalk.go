package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	appKey    string
	appSecret string
	corpID    string
	token     string
	tokenExp  time.Time
	tokenMu   sync.Mutex
)

func Init() error {
	appKey = os.Getenv("DINGTALK_APP_KEY")
	appSecret = os.Getenv("DINGTALK_APP_SECRET")
	corpID = os.Getenv("DINGTALK_CORP_ID")

	if appKey == "" || appSecret == "" {
		return fmt.Errorf("缺少 DINGTALK_APP_KEY 或 DINGTALK_APP_SECRET")
	}

	logrus.Info("钉钉客户端初始化完成")
	return nil
}

// GetCorpID 返回企业 CorpId，供前端 JS-SDK 使用
func GetCorpID() string {
	return corpID
}

// ===================== Access Token =====================

// GetAccessToken 获取企业内部应用的 access_token（带缓存）
func GetAccessToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	// 缓存有效
	if token != "" && time.Now().Before(tokenExp) {
		return token, nil
	}

	body := map[string]string{
		"appKey":    appKey,
		"appSecret": appSecret,
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/accessToken", body, nil)
	if err != nil {
		return "", fmt.Errorf("获取 access_token 失败: %w", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("access_token 响应格式异常: %v", resp)
	}

	expireIn := 7200.0
	if v, ok := resp["expireIn"].(float64); ok {
		expireIn = v
	}

	token = accessToken
	tokenExp = time.Now().Add(time.Duration(expireIn-60) * time.Second) // 提前60秒过期
	logrus.Info("钉钉 access_token 获取成功")
	return token, nil
}

// ===================== OAuth 登录 =====================

// GetQRLoginURL 获取钉钉扫码登录 URL
func GetQRCode(state string) (string, error) {
	redirectURI := os.Getenv("DINGTALK_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "http://localhost:3000/callback"
	}

	loginURL := fmt.Sprintf(
		"https://login.dingtalk.com/oauth2/auth?redirect_uri=%s&response_type=code&client_id=%s&scope=openid%%20corpid&state=%s&prompt=consent",
		url.QueryEscape(redirectURI),
		appKey,
		state,
	)
	return loginURL, nil
}

// GetUserAccessToken 用授权码换取用户 token
func GetUserAccessToken(code string) (string, error) {
	body := map[string]string{
		"clientId":     appKey,
		"clientSecret": appSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", body, nil)
	if err != nil {
		return "", fmt.Errorf("获取用户 access_token 失败: %w", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("用户 access_token 响应异常: %v", resp)
	}
	return accessToken, nil
}

// GetUserInfoByCode 通过授权码获取用户信息（新版 OAuth2，用于扫码登录）
func GetUserInfoByCode(code string) (map[string]interface{}, error) {
	// 1. 先用 code 换取用户 access_token
	userToken, err := GetUserAccessToken(code)
	if err != nil {
		return nil, err
	}

	// 2. 用 user access_token 获取用户信息
	headers := map[string]string{
		"x-acs-dingtalk-access-token": userToken,
	}
	resp, err := getJSON("https://api.dingtalk.com/v1.0/contact/users/me", headers)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	return resp, nil
}

// GetUserIDByInAppCode 企业内部应用免登：通过免登码获取企业内 userid
func GetUserIDByInAppCode(code string) (string, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return "", err
	}

	body := map[string]interface{}{
		"code": code,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/getuserinfo?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return "", fmt.Errorf("免登获取用户身份失败: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return "", fmt.Errorf("免登获取用户身份失败: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("免登响应格式异常: %v", resp)
	}

	userid := getString(result, "userid")
	if userid == "" {
		return "", fmt.Errorf("免登未返回 userid")
	}

	return userid, nil
}

// GetUserDetailByUserID 通过 userid 获取用户详细信息（Contact.User.Read）
func GetUserDetailByUserID(userid string) (map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"userid": userid,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("获取用户详情失败: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("获取用户详情失败: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("用户详情格式异常: %v", resp)
	}

	return result, nil
}

// ===================== 组织架构同步 =====================

// DeptInfo 部门信息
type DeptInfo struct {
	DeptID   int64  `json:"dept_id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parent_id"`
}

// UserInfo 用户信息
type UserInfo struct {
	UserID       string `json:"userid"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Mobile       string `json:"mobile"`
	DeptIDList   []int64 `json:"dept_id_list"`
	Position     string `json:"title"`
	Avatar       string `json:"avatar"`
	Active       bool   `json:"active"`
}

// SyncDepartments 同步所有部门
func SyncDepartments() ([]DeptInfo, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allDepts []DeptInfo

	// 递归获取所有部门，从根部门 1 开始
	if err := fetchDeptTree(accessToken, 1, &allDepts); err != nil {
		return nil, err
	}

	// 也把根部门自身加入
	rootDept, err := fetchDeptDetail(accessToken, 1)
	if err == nil && rootDept != nil {
		allDepts = append([]DeptInfo{*rootDept}, allDepts...)
	}

	logrus.Infof("钉钉同步部门完成，共 %d 个部门", len(allDepts))
	return allDepts, nil
}

func fetchDeptTree(accessToken string, parentID int64, result *[]DeptInfo) error {
	body := map[string]interface{}{
		"dept_id": parentID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("获取子部门失败: %s", errmsg)
	}

	resultList, ok := resp["result"].([]interface{})
	if !ok {
		return nil // 没有子部门
	}

	for _, item := range resultList {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		dept := DeptInfo{
			DeptID:   int64(m["dept_id"].(float64)),
			Name:     getString(m, "name"),
			ParentID: int64(m["parent_id"].(float64)),
		}
		*result = append(*result, dept)

		// 递归获取子部门
		if err := fetchDeptTree(accessToken, dept.DeptID, result); err != nil {
			logrus.Warnf("获取部门 %d 的子部门失败: %v", dept.DeptID, err)
		}
	}

	return nil
}

func fetchDeptDetail(accessToken string, deptID int64) (*DeptInfo, error) {
	body := map[string]interface{}{
		"dept_id": deptID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		return nil, fmt.Errorf("获取部门详情失败")
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("部门详情格式异常")
	}

	return &DeptInfo{
		DeptID:   int64(result["dept_id"].(float64)),
		Name:     getString(result, "name"),
		ParentID: int64(getFloat(result, "parent_id")),
	}, nil
}

// SyncUsers 同步指定部门的所有用户
func SyncUsers() ([]UserInfo, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	// 先获取所有部门
	depts, err := SyncDepartments()
	if err != nil {
		return nil, fmt.Errorf("同步用户前获取部门失败: %w", err)
	}

	userMap := make(map[string]UserInfo) // 去重

	for _, dept := range depts {
		users, err := fetchDeptUsers(accessToken, dept.DeptID)
		if err != nil {
			logrus.Warnf("获取部门 %d(%s) 用户失败: %v", dept.DeptID, dept.Name, err)
			continue
		}
		for _, u := range users {
			userMap[u.UserID] = u
		}
	}

	var allUsers []UserInfo
	for _, u := range userMap {
		allUsers = append(allUsers, u)
	}

	logrus.Infof("钉钉同步用户完成，共 %d 个用户", len(allUsers))
	return allUsers, nil
}

func fetchDeptUsers(accessToken string, deptID int64) ([]UserInfo, error) {
	var allUsers []UserInfo
	cursor := 0

	for {
		body := map[string]interface{}{
			"dept_id": deptID,
			"cursor":  cursor,
			"size":    100,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/list?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, err
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("获取部门用户失败: %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		list, ok := result["list"].([]interface{})
		if !ok {
			break
		}

		for _, item := range list {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			user := UserInfo{
				UserID:   getString(m, "userid"),
				Name:     getString(m, "name"),
				Email:    getString(m, "email"),
				Mobile:   getString(m, "mobile"),
				Position: getString(m, "title"),
				Avatar:   getString(m, "avatar"),
				Active:   getBool(m, "active"),
			}
			
			// 处理空 email 的情况，生成唯一 email
			if user.Email == "" {
				user.Email = user.UserID + "@dingtalk.com"
			}
			
			// 处理空 mobile 的情况，生成唯一 mobile
			if user.Mobile == "" {
				user.Mobile = "10000000000"
			}
			if deptList, ok := m["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
				for _, d := range deptList {
					if id, ok := d.(float64); ok {
						user.DeptIDList = append(user.DeptIDList, int64(id))
					}
				}
			}
			allUsers = append(allUsers, user)
		}

		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		nextCursor, _ := result["next_cursor"].(float64)
		cursor = int(nextCursor)
	}

	return allUsers, nil
}

// ===================== 考勤同步 =====================

// AttendanceRecord 考勤记录
type AttendanceRecord struct {
	UserID       string    `json:"userId"`
	CheckType    string    `json:"checkType"`    // OnDuty / OffDuty
	UserCheckTime string   `json:"userCheckTime"`
	LocationResult string  `json:"locationResult"` // Normal / Outside 等
	TimeResult   string    `json:"timeResult"`     // Normal / Late / Early 等
}

// GetAttendance 获取考勤数据
func GetAttendance(userIDs []string, startDate, endDate string) ([]AttendanceRecord, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allRecords []AttendanceRecord

	// 钉钉每次最多传 50 个用户
	for i := 0; i < len(userIDs); i += 50 {
		end := i + 50
		if end > len(userIDs) {
			end = len(userIDs)
		}
		batch := userIDs[i:end]

		offset := 0
		for {
			body := map[string]interface{}{
				"workDateFrom": startDate + " 00:00:00",
				"workDateTo":   endDate + " 23:59:59",
				"userIdList":   batch,
				"offset":       offset,
				"limit":        50,
			}
			resp, err := postJSONOAPI(
				fmt.Sprintf("https://oapi.dingtalk.com/attendance/list?access_token=%s", accessToken),
				body,
			)
			if err != nil {
				return nil, err
			}

			errcode, _ := resp["errcode"].(float64)
			if errcode != 0 {
				errmsg, _ := resp["errmsg"].(string)
				logrus.Warnf("获取考勤记录失败: %s", errmsg)
				break
			}

			recordList, ok := resp["recordresult"].([]interface{})
			if !ok || len(recordList) == 0 {
				break
			}

			for _, item := range recordList {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				record := AttendanceRecord{
					UserID:         getString(m, "userId"),
					CheckType:      getString(m, "checkType"),
					UserCheckTime:  getString(m, "userCheckTime"),
					LocationResult: getString(m, "locationResult"),
					TimeResult:     getString(m, "timeResult"),
				}
				allRecords = append(allRecords, record)
			}

			hasMore, _ := resp["hasMore"].(bool)
			if !hasMore {
				break
			}
			offset += 50
		}
	}

	logrus.Infof("钉钉同步考勤完成，共 %d 条记录", len(allRecords))
	return allRecords, nil
}

// ===================== 审批同步 =====================

// ApprovalInstance 审批实例
type ApprovalInstance struct {
	ProcessInstanceID string `json:"process_instance_id"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Result           string `json:"result"`
	CreateTime       string `json:"create_time"`
	FinishTime       string `json:"finish_time"`
	OriginatorUserID string `json:"originator_userid"`
	FormValues       []map[string]interface{} `json:"form_component_values"`
}

// GetApprovals 获取审批实例列表
func GetApprovals(processCode, startDate, endDate string) ([]ApprovalInstance, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	// 解析日期为毫秒时间戳
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)

	var allInstances []ApprovalInstance
	cursor := 0

	for {
		body := map[string]interface{}{
			"process_code": processCode,
			"start_time":   start.UnixMilli(),
			"end_time":     end.AddDate(0, 0, 1).UnixMilli(),
			"size":         20,
			"cursor":       cursor,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/processinstance/listids?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, err
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("获取审批实例 ID 失败: %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		idList, ok := result["list"].([]interface{})
		if !ok || len(idList) == 0 {
			break
		}

		// 逐个获取审批详情
		for _, id := range idList {
			instanceID, ok := id.(string)
			if !ok {
				continue
			}
			instance, err := getApprovalDetail(accessToken, instanceID)
			if err != nil {
				logrus.Warnf("获取审批实例 %s 详情失败: %v", instanceID, err)
				continue
			}
			allInstances = append(allInstances, *instance)
		}

		nextCursor, _ := result["next_cursor"].(float64)
		if nextCursor == 0 {
			break
		}
		cursor = int(nextCursor)
	}

	logrus.Infof("钉钉同步审批完成，共 %d 个实例", len(allInstances))
	return allInstances, nil
}

func getApprovalDetail(accessToken, instanceID string) (*ApprovalInstance, error) {
	body := map[string]interface{}{
		"process_instance_id": instanceID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/processinstance/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("%s", errmsg)
	}

	pi, ok := resp["process_instance"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("审批详情格式异常")
	}

	instance := &ApprovalInstance{
		ProcessInstanceID: instanceID,
		Title:            getString(pi, "title"),
		Status:           getString(pi, "status"),
		Result:           getString(pi, "result"),
		CreateTime:       getString(pi, "create_time"),
		FinishTime:       getString(pi, "finish_time"),
		OriginatorUserID: getString(pi, "originator_userid"),
	}

	if formValues, ok := pi["form_component_values"].([]interface{}); ok {
		for _, fv := range formValues {
			if m, ok := fv.(map[string]interface{}); ok {
				instance.FormValues = append(instance.FormValues, m)
			}
		}
	}

	return instance, nil
}

// ===================== HTTP 工具 =====================

// postJSON 发送 POST 请求到新版 API（api.dingtalk.com）
func postJSON(url string, body interface{}, headers map[string]string) (map[string]interface{}, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %s", string(data))
	}

	return result, nil
}

// postJSONOAPI 发送 POST 请求到旧版 API（oapi.dingtalk.com）
func postJSONOAPI(url string, body interface{}) (map[string]interface{}, error) {
	return postJSON(url, body, nil)
}

// getJSON 发送 GET 请求
func getJSON(url string, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %s", string(data))
	}

	return result, nil
}

// ===================== 工具函数 =====================

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
