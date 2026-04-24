package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
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

// 考勤组缓存（5分钟 TTL）
type attendanceGroupCache struct {
	data   []map[string]interface{}
	expiry time.Time
}

type attendanceGroupDetailCache struct {
	data   map[string]interface{}
	expiry time.Time
}

var (
	attGroupsCache    attendanceGroupCache
	attGroupsCacheMu  sync.Mutex
	attGroupDetailMap sync.Map // key: groupID(int64) → attendanceGroupDetailCache
)

func Init() error {
	appKey = os.Getenv("DINGTALK_APP_KEY")
	appSecret = os.Getenv("DINGTALK_APP_SECRET")
	corpID = os.Getenv("DINGTALK_CORP_ID")

	if appKey == "" || appSecret == "" {
		return fmt.Errorf("缂哄皯 DINGTALK_APP_KEY 鎴?DINGTALK_APP_SECRET")
	}

	logrus.Info("閽夐拤瀹㈡埛绔垵濮嬪寲瀹屾垚")
	return nil
}

// GetCorpID 杩斿洖浼佷笟 CorpId锛屼緵鍓嶇 JS-SDK 浣跨敤
func GetCorpID() string {
	return corpID
}

// ===================== Access Token =====================

// GetAccessToken 鑾峰彇浼佷笟鍐呴儴搴旂敤鐨?access_token锛堝甫缂撳瓨锛?
func GetAccessToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	// 缂撳瓨鏈夋晥
	if token != "" && time.Now().Before(tokenExp) {
		return token, nil
	}

	body := map[string]string{
		"appKey":    appKey,
		"appSecret": appSecret,
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/accessToken", body, nil)
	if err != nil {
		return "", fmt.Errorf("鑾峰彇 access_token 澶辫触: %w", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("access_token 鍝嶅簲鏍煎紡寮傚父: %v", resp)
	}

	expireIn := 7200.0
	if v, ok := resp["expireIn"].(float64); ok {
		expireIn = v
	}

	token = accessToken
	tokenExp = time.Now().Add(time.Duration(expireIn-60) * time.Second)
	logrus.Info("dingtalk access_token fetched")
	return token, nil
}

// ===================== OAuth 鐧诲綍 =====================

// GetQRLoginURL 鑾峰彇閽夐拤鎵爜鐧诲綍 URL
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

// GetUserAccessToken 鐢ㄦ巿鏉冪爜鎹㈠彇鐢ㄦ埛 token
func GetUserAccessToken(code string) (string, error) {
	body := map[string]string{
		"clientId":     appKey,
		"clientSecret": appSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", body, nil)
	if err != nil {
		return "", fmt.Errorf("鑾峰彇鐢ㄦ埛 access_token 澶辫触: %w", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("鐢ㄦ埛 access_token 鍝嶅簲寮傚父: %v", resp)
	}
	return accessToken, nil
}

// GetUserInfoByCode 閫氳繃鎺堟潈鐮佽幏鍙栫敤鎴蜂俊鎭紙鏂扮増 OAuth2锛岀敤浜庢壂鐮佺櫥褰曪級
func GetUserInfoByCode(code string) (map[string]interface{}, error) {
	// 1. 鍏堢敤 code 鎹㈠彇鐢ㄦ埛 access_token
	userToken, err := GetUserAccessToken(code)
	if err != nil {
		return nil, err
	}

	// 2. 鐢?user access_token 鑾峰彇鐢ㄦ埛淇℃伅
	headers := map[string]string{
		"x-acs-dingtalk-access-token": userToken,
	}
	resp, err := getJSON("https://api.dingtalk.com/v1.0/contact/users/me", headers)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛淇℃伅澶辫触: %w", err)
	}

	return resp, nil
}

// GetUserIDByInAppCode 浼佷笟鍐呴儴搴旂敤鍏嶇櫥锛氶€氳繃鍏嶇櫥鐮佽幏鍙栦紒涓氬唴 userid
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
		return "", fmt.Errorf("鍏嶇櫥鑾峰彇鐢ㄦ埛韬唤澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return "", fmt.Errorf("鍏嶇櫥鑾峰彇鐢ㄦ埛韬唤澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("鍏嶇櫥鍝嶅簲鏍煎紡寮傚父: %v", resp)
	}

	userid := getString(result, "userid")
	if userid == "" {
		return "", fmt.Errorf("鍏嶇櫥鏈繑鍥?userid")
	}

	return userid, nil
}

// GetUserDetailByUserID 閫氳繃 userid 鑾峰彇鐢ㄦ埛璇︾粏淇℃伅锛圕ontact.User.Read锛?
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
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛璇︽儏澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛璇︽儏澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("鐢ㄦ埛璇︽儏鏍煎紡寮傚父: %v", resp)
	}

	return result, nil
}

// ===================== 缁勭粐鏋舵瀯鍚屾 =====================

// DeptInfo 閮ㄩ棬淇℃伅
type DeptInfo struct {
	DeptID   int64  `json:"dept_id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parent_id"`
}

// UserInfo 鐢ㄦ埛淇℃伅
type UserInfo struct {
	UserID             string  `json:"userid"`
	Name               string  `json:"name"`
	Email              string  `json:"email"`
	Mobile             string  `json:"mobile"`
	DeptIDList         []int64 `json:"dept_id_list"`
	Position           string  `json:"title"`
	Avatar             string  `json:"avatar"`
	Active             bool    `json:"active"`
	HiredDate          string  `json:"hired_date"` // 入职日期，格式 YYYY-MM-DD
	PlannedRegularDate string  `json:"planned_regular_date"`
	ActualRegularDate  string  `json:"actual_regular_date"`
}

// SyncDepartments 鍚屾鎵€鏈夐儴闂?
func SyncDepartments() ([]DeptInfo, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allDepts []DeptInfo

	// Recursively fetch all departments starting from root department 1.
	if err := fetchDeptTree(accessToken, 1, &allDepts); err != nil {
		return nil, err
	}

	// Also include the root department itself.
	rootDept, err := fetchDeptDetail(accessToken, 1)
	if err == nil && rootDept != nil {
		allDepts = append([]DeptInfo{*rootDept}, allDepts...)
	}

	logrus.Infof("dingtalk sync departments complete: %d", len(allDepts))
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
		return fmt.Errorf("鑾峰彇瀛愰儴闂ㄥけ璐? %s", errmsg)
	}

	resultList, ok := resp["result"].([]interface{})
	if !ok {
		return nil // No sub departments.
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

		// Recursively fetch child departments.
		if err := fetchDeptTree(accessToken, dept.DeptID, result); err != nil {
			logrus.Warnf("鑾峰彇閮ㄩ棬 %d 鐨勫瓙閮ㄩ棬澶辫触: %v", dept.DeptID, err)
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
		return nil, fmt.Errorf("鑾峰彇閮ㄩ棬璇︽儏澶辫触")
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("閮ㄩ棬璇︽儏鏍煎紡寮傚父")
	}

	return &DeptInfo{
		DeptID:   int64(result["dept_id"].(float64)),
		Name:     getString(result, "name"),
		ParentID: int64(getFloat(result, "parent_id")),
	}, nil
}

// SyncUsers 鍚屾鎸囧畾閮ㄩ棬鐨勬墍鏈夌敤鎴?
func SyncUsers() ([]UserInfo, error) {
	depts, err := SyncDepartments()
	if err != nil {
		return nil, fmt.Errorf("鍚屾鐢ㄦ埛鍓嶈幏鍙栭儴闂ㄥけ璐? %w", err)
	}
	return SyncUsersWithDepts(depts)
}

// SyncUsersWithDepts 浣跨敤宸叉湁閮ㄩ棬鍒楄〃鍚屾鎵€鏈夌敤鎴凤紝閬垮厤閲嶅璋冪敤 SyncDepartments
func SyncUsersWithDepts(depts []DeptInfo) ([]UserInfo, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	userMap := make(map[string]UserInfo) // 鍘婚噸

	for _, dept := range depts {
		users, err := fetchDeptUsers(accessToken, dept.DeptID)
		if err != nil {
			logrus.Warnf("鑾峰彇閮ㄩ棬 %d(%s) 鐢ㄦ埛澶辫触: %v", dept.DeptID, dept.Name, err)
			continue
		}
		for _, u := range users {
			userMap[u.UserID] = u
		}
	}

	if err := enrichUsersWithHRMRegularDates(accessToken, userMap); err != nil {
		logrus.Warnf("dingtalk hrm regular date sync skipped: %v", err)
	}

	var allUsers []UserInfo
	for _, u := range userMap {
		allUsers = append(allUsers, u)
	}

	logrus.Infof("dingtalk sync users complete: %d", len(allUsers))
	return allUsers, nil
}

type hrmRegularDates struct {
	Planned string
	Actual  string
}

func enrichUsersWithHRMRegularDates(accessToken string, users map[string]UserInfo) error {
	if len(users) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(users))
	for userID := range users {
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}

	for start := 0; start < len(userIDs); start += 50 {
		end := start + 50
		if end > len(userIDs) {
			end = len(userIDs)
		}
		dates, err := fetchHRMRegularDates(accessToken, userIDs[start:end])
		if err != nil {
			return err
		}
		for userID, regularDates := range dates {
			user, ok := users[userID]
			if !ok {
				continue
			}
			user.PlannedRegularDate = regularDates.Planned
			user.ActualRegularDate = regularDates.Actual
			users[userID] = user
		}
	}

	return nil
}

func fetchHRMRegularDates(accessToken string, userIDs []string) (map[string]hrmRegularDates, error) {
	result := make(map[string]hrmRegularDates)
	if len(userIDs) == 0 {
		return result, nil
	}

	body := map[string]interface{}{
		"agentid":           getDingTalkAgentID(),
		"userid_list":       strings.Join(userIDs, ","),
		"field_filter_list": "sys01-planRegularTime,sys01-regularTime",
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/smartwork/hrm/employee/v2/list?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return result, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		if errmsg == "" {
			errmsg = fmt.Sprintf("unknown errcode %.0f", errcode)
		}
		return result, fmt.Errorf("fetch hrm regular dates failed: %s", errmsg)
	}

	items, ok := resp["result"].([]interface{})
	if !ok {
		return result, nil
	}
	for _, item := range items {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		userID := getString(record, "userid")
		if userID == "" {
			continue
		}

		var regularDates hrmRegularDates
		fields, ok := record["field_data_list"].([]interface{})
		if !ok {
			continue
		}
		for _, field := range fields {
			fieldMap, ok := field.(map[string]interface{})
			if !ok {
				continue
			}
			value := extractHRMFieldValue(fieldMap)
			switch getString(fieldMap, "field_code") {
			case "sys01-planRegularTime":
				regularDates.Planned = value
			case "sys01-regularTime":
				regularDates.Actual = value
			}
		}
		result[userID] = regularDates
	}

	return result, nil
}

func getDingTalkAgentID() int64 {
	raw := strings.TrimSpace(os.Getenv("DINGTALK_AGENT_ID"))
	if raw == "" {
		return 1
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 1
	}
	return id
}

func extractHRMFieldValue(field map[string]interface{}) string {
	values, ok := field["field_value_list"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range values {
		valueMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		value := normalizeDingTalkDate(stringValue(valueMap["value"]))
		if value != "" {
			return value
		}
		value = normalizeDingTalkDate(stringValue(valueMap["label"]))
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		if value == 0 {
			return ""
		}
		return strconv.FormatInt(int64(value), 10)
	case int64:
		if value == 0 {
			return ""
		}
		return strconv.FormatInt(value, 10)
	case int:
		if value == 0 {
			return ""
		}
		return strconv.Itoa(value)
	default:
		return ""
	}
}

func normalizeDingTalkDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= len("2006-01-02") && value[4] == '-' && value[7] == '-' {
		return value[:10]
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil && ts > 0 {
		if ts > 1_000_000_000_000 {
			return time.UnixMilli(ts).Format("2006-01-02")
		}
		if ts > 1_000_000_000 {
			return time.Unix(ts, 0).Format("2006-01-02")
		}
	}
	return value
}

type VacationType struct {
	LeaveCode     string  `json:"leave_code"`
	LeaveName     string  `json:"leave_name"`
	LeaveViewUnit string  `json:"leave_view_unit"`
	HoursInPerDay float64 `json:"hours_in_per_day"`
}

func ListVacationTypes(opUserID string) ([]VacationType, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/type/list?access_token=%s", accessToken),
		map[string]interface{}{
			"vacation_source": "all",
			"op_userid":       opUserID,
		},
	)
	if err != nil {
		return nil, err
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return nil, fmt.Errorf("list vacation types failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	items, ok := resp["result"].([]interface{})
	if !ok {
		return nil, nil
	}
	result := make([]VacationType, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, VacationType{
			LeaveCode:     getString(m, "leave_code"),
			LeaveName:     getString(m, "leave_name"),
			LeaveViewUnit: getString(m, "leave_view_unit"),
			HoursInPerDay: getFloat(m, "hours_in_per_day"),
		})
	}
	return result, nil
}

func UpdateAnnualLeaveQuota(userID string, year int, days float64, reason string) error {
	if days <= 0 {
		return nil
	}
	opUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if opUserID == "" {
		return fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}

	leaveCode, hoursPerDay, err := resolveAnnualLeaveType(opUserID)
	if err != nil {
		return err
	}
	if hoursPerDay <= 0 {
		hoursPerDay = getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	}

	accessToken, err := GetAccessToken()
	if err != nil {
		return err
	}

	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.Local)
	// 钉钉单位：1/100 天（100 = 1天，250 = 2.5天）
	quotaPerDay := int64(math.Round(days * 100))
	quotaPerHour := int64(math.Round(days * hoursPerDay * 100))

	logrus.Infof("[leave-sync] UpdateAnnualLeaveQuota userID=%s year=%d days=%.2f leaveCode=%s quotaPerDay=%d",
		userID, year, days, leaveCode, quotaPerDay)

	updateBody := map[string]interface{}{
		"op_userid": opUserID,
		"leave_quotas": []map[string]interface{}{
			{
				"userid":             userID,
				"leave_code":         leaveCode,
				"quota_num_per_day":  quotaPerDay,
				"quota_num_per_hour": quotaPerHour,
				"quota_cycle":        strconv.Itoa(year),
				"start_time":         start.UnixMilli(),
				"end_time":           end.UnixMilli(),
				"reason":             reason,
			},
		},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/quota/update?access_token=%s", accessToken),
		updateBody,
	)
	if err != nil {
		return err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode == 0 {
		return nil
	}

	logrus.Warnf("[leave-sync] quota/update errcode=%.0f errmsg=%s", errcode, dingTalkErrorMessage(resp, errcode))

	// errcode 880015：用户在钉钉中尚无此假期配额记录。
	// 新版 API 两步走：先 GET 初始化记录，再 POST 写入实际配额。
	if errcode == 880015 {
		newAPIHeaders := map[string]string{"x-acs-dingtalk-access-token": accessToken}

		// 步骤 1：初始化配额记录（若记录已存在则幂等，不会重置）
		initURL := fmt.Sprintf(
			"https://api.dingtalk.com/v1.0/attendance/leaves/initializations/balances?opUserId=%s&userId=%s&leaveCode=%s",
			url.QueryEscape(opUserID), url.QueryEscape(userID), url.QueryEscape(leaveCode),
		)
		if _, initErr := getJSON(initURL, newAPIHeaders); initErr != nil {
			return fmt.Errorf("initialize annual leave quota record failed: %w", initErr)
		}

		// 步骤 2：写入实际配额（JSON 数组 body，opUserId 放 query）
		setURL := fmt.Sprintf(
			"https://api.dingtalk.com/v1.0/attendance/leaves/quota?opUserId=%s",
			url.QueryEscape(opUserID),
		)
		setBody := []map[string]interface{}{{
			"userId":          userID,
			"leaveCode":       leaveCode,
			"quotaNumPerDay":  quotaPerDay,
			"quotaNumPerHour": quotaPerHour,
			"startTime":       start.UnixMilli(),
			"endTime":         end.UnixMilli(),
			"quotaCycle":      strconv.Itoa(year),
			"reason":          reason,
		}}
		if _, setErr := postJSON(setURL, setBody, newAPIHeaders); setErr != nil {
			return fmt.Errorf("set annual leave quota after init failed: %w", setErr)
		}

		logrus.Infof("[leave-sync] new API init+set success userID=%s year=%d days=%.2f", userID, year, days)
		return nil
	}

	return fmt.Errorf("update annual leave quota failed: errcode=%.0f %s", errcode, dingTalkErrorMessage(resp, errcode))
}

type cachedLeaveType struct {
	leaveCode   string
	hoursPerDay float64
	expiry      time.Time
}

var (
	annualLeaveTypeCache   cachedLeaveType
	annualLeaveTypeCacheMu sync.Mutex
)

func resolveAnnualLeaveType(opUserID string) (string, float64, error) {
	if code := strings.TrimSpace(os.Getenv("DINGTALK_ANNUAL_LEAVE_CODE")); code != "" {
		return code, getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8), nil
	}

	annualLeaveTypeCacheMu.Lock()
	defer annualLeaveTypeCacheMu.Unlock()

	if annualLeaveTypeCache.leaveCode != "" && time.Now().Before(annualLeaveTypeCache.expiry) {
		return annualLeaveTypeCache.leaveCode, annualLeaveTypeCache.hoursPerDay, nil
	}

	leaveName := strings.TrimSpace(os.Getenv("DINGTALK_ANNUAL_LEAVE_NAME"))
	if leaveName == "" {
		leaveName = "年假"
	}
	types, err := ListVacationTypes(opUserID)
	if err != nil {
		return "", 0, err
	}
	for _, item := range types {
		if item.LeaveName == leaveName {
			annualLeaveTypeCache = cachedLeaveType{
				leaveCode:   item.LeaveCode,
				hoursPerDay: item.HoursInPerDay,
				expiry:      time.Now().Add(time.Hour),
			}
			return item.LeaveCode, item.HoursInPerDay, nil
		}
	}
	return "", 0, fmt.Errorf("annual leave type %q not found in DingTalk; set DINGTALK_ANNUAL_LEAVE_CODE", leaveName)
}

func getEnvFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func dingTalkErrorMessage(resp map[string]interface{}, errcode float64) string {
	parts := make([]string, 0, 3)
	if errmsg := strings.TrimSpace(getString(resp, "errmsg")); errmsg != "" {
		parts = append(parts, errmsg)
	}
	if subMsg := strings.TrimSpace(getString(resp, "sub_msg")); subMsg != "" {
		parts = append(parts, subMsg)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("unknown errcode %.0f", errcode)
	}
	return strings.Join(parts, "; ")
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
			if errmsg == "" {
				errmsg = fmt.Sprintf("unknown errcode %.0f", errcode)
			}
			return nil, fmt.Errorf("鑾峰彇閮ㄩ棬鐢ㄦ埛澶辫触: %s", errmsg)
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

			// hired_date 是毫秒时间戳，转成 YYYY-MM-DD
			if ts, ok := m["hired_date"].(float64); ok && ts > 0 {
				user.HiredDate = time.UnixMilli(int64(ts)).Format("2006-01-02")
			}

			// 澶勭悊绌?email 鐨勬儏鍐碉紝鐢熸垚鍞竴 email
			if user.Email == "" {
				user.Email = user.UserID + "@dingtalk.com"
			}

			// 澶勭悊绌?mobile 鐨勬儏鍐碉紝鐢熸垚鍞竴 mobile
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

// ===================== 鑰冨嫟鍚屾 =====================

// AttendanceRecord 鑰冨嫟璁板綍
type AttendanceRecord struct {
	UserID         string `json:"userId"`
	CheckType      string `json:"checkType"` // OnDuty / OffDuty
	UserCheckTime  string `json:"userCheckTime"`
	LocationResult string `json:"locationResult"` // Normal / Outside
	TimeResult     string `json:"timeResult"`     // Normal / Late / Early
}

// GetAttendance 鑾峰彇鑰冨嫟鏁版嵁
func GetAttendance(userIDs []string, startDate, endDate string) ([]AttendanceRecord, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allRecords []AttendanceRecord

	// DingTalk allows at most 50 users per request.
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
				logrus.Warnf("鑾峰彇鑰冨嫟璁板綍澶辫触: %s", errmsg)
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

	logrus.Infof("dingtalk sync attendance complete: %d", len(allRecords))
	return allRecords, nil
}

// ===================== 瀹℃壒鍚屾 =====================

// ApprovalInstance 瀹℃壒瀹炰緥
type ApprovalInstance struct {
	ProcessInstanceID string                   `json:"process_instance_id"`
	Title             string                   `json:"title"`
	Status            string                   `json:"status"`
	Result            string                   `json:"result"`
	CreateTime        string                   `json:"create_time"`
	FinishTime        string                   `json:"finish_time"`
	OriginatorUserID  string                   `json:"originator_userid"`
	FormValues        []map[string]interface{} `json:"form_component_values"`
}

// GetApprovals 鑾峰彇瀹℃壒瀹炰緥鍒楄〃
func GetApprovals(processCode, startDate, endDate string) ([]ApprovalInstance, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	// 瑙ｆ瀽鏃ユ湡涓烘绉掓椂闂存埑
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
			return nil, fmt.Errorf("鑾峰彇瀹℃壒瀹炰緥 ID 澶辫触: %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		idList, ok := result["list"].([]interface{})
		if !ok || len(idList) == 0 {
			break
		}

		// 閫愪釜鑾峰彇瀹℃壒璇︽儏
		for _, id := range idList {
			instanceID, ok := id.(string)
			if !ok {
				continue
			}
			instance, err := getApprovalDetail(accessToken, instanceID)
			if err != nil {
				logrus.Warnf("鑾峰彇瀹℃壒瀹炰緥 %s 璇︽儏澶辫触: %v", instanceID, err)
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

	logrus.Infof("dingtalk sync approvals complete: %d", len(allInstances))
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
		return nil, fmt.Errorf("瀹℃壒璇︽儏鏍煎紡寮傚父")
	}

	instance := &ApprovalInstance{
		ProcessInstanceID: instanceID,
		Title:             getString(pi, "title"),
		Status:            getString(pi, "status"),
		Result:            getString(pi, "result"),
		CreateTime:        getString(pi, "create_time"),
		FinishTime:        getString(pi, "finish_time"),
		OriginatorUserID:  getString(pi, "originator_userid"),
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

// ===================== HTTP 宸ュ叿 =====================

// postJSON 鍙戦€?POST 璇锋眰鍒版柊鐗?API锛坅pi.dingtalk.com锛?
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
		return nil, fmt.Errorf("JSON 瑙ｆ瀽澶辫触: %s", string(data))
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DingTalk API HTTP %d: %s", resp.StatusCode, string(data))
	}

	return result, nil
}

// postJSONOAPI 鍙戦€?POST 璇锋眰鍒版棫鐗?API锛坥api.dingtalk.com锛?
func postJSONOAPI(url string, body interface{}) (map[string]interface{}, error) {
	return postJSON(url, body, nil)
}

// getJSON 鍙戦€?GET 璇锋眰
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
		return nil, fmt.Errorf("JSON 瑙ｆ瀽澶辫触: %s", string(data))
	}

	return result, nil
}

// ===================== 宸ュ叿鍑芥暟 =====================

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

// ===================== 澶у皬鍛ㄦ帓鐝?API =====================

// GetAttendanceGroups 获取企业考勤组列表（带5分钟本地缓存）
func GetAttendanceGroups() ([]map[string]interface{}, error) {
	attGroupsCacheMu.Lock()
	if attGroupsCache.data != nil && time.Now().Before(attGroupsCache.expiry) {
		cached := attGroupsCache.data
		attGroupsCacheMu.Unlock()
		return cached, nil
	}
	attGroupsCacheMu.Unlock()

	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allGroups []map[string]interface{}
	offset := 0

	for {
		body := map[string]interface{}{
			"offset": offset,
			"size":   10,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/getsimplegroups?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %w", err)
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		groups, ok := result["groups"].([]interface{})
		if !ok || len(groups) == 0 {
			break
		}

		for _, g := range groups {
			if gm, ok := g.(map[string]interface{}); ok {
				allGroups = append(allGroups, gm)
			}
		}

		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		offset += 10
	}

	logrus.Infof("get attendance groups complete: %d", len(allGroups))
	attGroupsCacheMu.Lock()
	attGroupsCache = attendanceGroupCache{data: allGroups, expiry: time.Now().Add(5 * time.Minute)}
	attGroupsCacheMu.Unlock()
	return allGroups, nil
}

// GetAttendanceGroup 鏌ヨ鍗曚釜鑰冨嫟缁勮鎯?
func GetAttendanceGroup(opUserID string, groupID int64) (map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"group_id":   groupID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/query?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("query attendance group failed: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		if errmsg == "" {
			errmsg = fmt.Sprintf("unknown errcode %.0f", errcode)
		}
		return nil, fmt.Errorf("query attendance group failed: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("query attendance group failed: invalid result payload")
	}
	attGroupDetailMap.Store(groupID, attendanceGroupDetailCache{data: result, expiry: time.Now().Add(5 * time.Minute)})
	return result, nil
}

func AttendanceGroupHasShift(group map[string]interface{}, shiftID int64) bool {
	if shiftID <= 0 || len(group) == 0 {
		return false
	}

	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) == shiftID {
					return true
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) == shiftID {
				return true
			}
		}
	}

	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if classID, ok := item["class_id"].(float64); ok && int64(classID) == shiftID {
					return true
				}
			}
		}
	}

	return false
}

func FindAnyAttendanceGroupShiftID(group map[string]interface{}) int64 {
	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					return int64(id)
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				return int64(id)
			}
		}
	}

	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if classID, ok := item["class_id"].(float64); ok && int64(classID) > 0 {
					return int64(classID)
				}
			}
		}
	}

	return 0
}

func CollectAttendanceGroupShiftIDs(group map[string]interface{}) map[int64]struct{} {
	ids := make(map[int64]struct{})

	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					ids[int64(id)] = struct{}{}
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				ids[int64(id)] = struct{}{}
			}
		}
	}

	return ids
}

// findRestShiftFromShiftIDs resolves the rest shift ID for rotation groups.
// It reads shift_ids from the group, calls GetShiftList, and finds the shift named "休息".
func findRestShiftFromShiftIDs(group map[string]interface{}) int64 {
	// Collect all shift IDs listed in the group
	groupShiftSet := CollectAttendanceGroupShiftIDs(group)
	if len(groupShiftSet) == 0 {
		return 0
	}
	groupShiftIDList := make([]string, 0, len(groupShiftSet))
	for gsid := range groupShiftSet {
		groupShiftIDList = append(groupShiftIDList, fmt.Sprintf("%d", gsid))
	}
	logrus.Infof("findRestShiftFromShiftIDs: group shift_ids=%v", groupShiftIDList)

	shifts, err := GetShiftList()
	if err != nil {
		logrus.Warnf("findRestShiftFromShiftIDs: GetShiftList failed: %v", err)
		return 0
	}

	var names []string
	for _, shift := range shifts {
		id, _ := shift["id"].(float64)
		if id <= 0 {
			continue
		}
		shiftID := int64(id)
		name, _ := shift["name"].(string)
		names = append(names, fmt.Sprintf("%d=%s", shiftID, name))
		if _, inGroup := groupShiftSet[shiftID]; !inGroup {
			continue
		}
		logrus.Infof("findRestShiftFromShiftIDs: group shift %d=%q all_fields=%v", shiftID, name, shift)
		// Match rest shift by name or by is_rest field
		if strings.Contains(name, "休") {
			logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d (%s) in group", shiftID, name)
			return shiftID
		}
		for _, key := range []string{"isRest", "is_rest", "isrest"} {
			switch val := shift[key].(type) {
			case string:
				if val == "Y" || val == "y" || val == "true" || val == "1" {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			case bool:
				if val {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			case float64:
				if val == 1 {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			}
		}
	}
	logrus.Warnf("findRestShiftFromShiftIDs: no rest shift found in group shift_ids; all shifts: %v", names)
	return 0
}

// FindRestClassID 从考勤组数据中找到休息班次 ID
// 钉钉排班制考勤组中，休息日必须与专属的休息班次 ID（不能用工作班次，也不能省略）
func FindRestClassID(group map[string]interface{}) int64 {
	// isRestClass checks a class map for rest indicators across different DingTalk API versions
	isRestClass := func(cls map[string]interface{}) bool {
		for _, key := range []string{"isrest", "isRestDay", "isRest", "is_rest"} {
			v, ok := cls[key]
			if !ok {
				continue
			}
			switch val := v.(type) {
			case string:
				if val == "Y" || val == "y" || val == "true" || val == "1" {
					return true
				}
			case bool:
				if val {
					return true
				}
			case float64:
				if val == 1 {
					return true
				}
			}
		}
		return false
	}

	getClassID := func(cls map[string]interface{}) int64 {
		for _, key := range []string{"id", "classId", "class_id", "classID"} {
			if id, ok := cls[key].(float64); ok && id > 0 {
				return int64(id)
			}
		}
		return 0
	}

	// Try both "classes" and "class_list" (field name varies by group type)
	for _, fieldName := range []string{"classes", "class_list"} {
		classes, ok := group[fieldName].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range classes {
			cls, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if isRestClass(cls) {
				if id := getClassID(cls); id > 0 {
					return id
				}
			}
		}
	}

	knownShiftIDs := CollectAttendanceGroupShiftIDs(group)
	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				className, _ := item["class_name"].(string)
				if strings.Contains(className, "休") {
					if classID, ok := item["class_id"].(float64); ok && int64(classID) > 0 {
						logrus.Infof("FindRestClassID: found rest class %d (%s) from cycle_schedules", int64(classID), className)
						return int64(classID)
					}
				}
			}
		}

		classCounts := make(map[int64]int)
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				classID, ok := item["class_id"].(float64)
				if !ok || int64(classID) <= 0 {
					continue
				}
				id := int64(classID)
				if _, known := knownShiftIDs[id]; known {
					continue
				}
				classCounts[id]++
			}
		}

		var selectedID int64
		var selectedCount int
		for id, count := range classCounts {
			if count > selectedCount {
				selectedID = id
				selectedCount = count
			}
		}
		if selectedID > 0 {
			logrus.Infof("FindRestClassID: inferred rest class %d from cycle_schedules", selectedID)
			return selectedID
		}
	}

	// Log full group keys to aid debugging when rest class is still not found
	keys := make([]string, 0, len(group))
	for k := range group {
		keys = append(keys, k)
	}
	logrus.Warnf("FindRestClassID: rest class not found; group top-level keys: %v", keys)
	return 0
}

func GetAttendanceGroupRestClassID(group map[string]interface{}) int64 {
	restClassID := FindRestClassID(group)
	if restClassID == 0 {
		restClassID = findRestShiftFromShiftIDs(group)
	}
	if restClassID == 0 {
		logrus.Warnf("GetAttendanceGroupRestClassID: rest class still missing; debug=%s", summarizeAttendanceGroupRestDebug(group))
	}
	return restClassID
}

func resolveScheduleAsyncRestShiftID(opUserID string, groupID int64) int64 {
	const dingTalkRestShiftSentinel int64 = 1

	group, err := GetAttendanceGroup(opUserID, groupID)
	if err != nil {
		logrus.Warnf("resolveScheduleAsyncRestShiftID: get group detail failed for %d: %v; fallback=%d", groupID, err, dingTalkRestShiftSentinel)
		return dingTalkRestShiftSentinel
	}

	restShiftID := GetAttendanceGroupRestClassID(group)
	if restShiftID > 0 {
		return restShiftID
	}

	logrus.Infof("resolveScheduleAsyncRestShiftID: group %d uses DingTalk rest sentinel %d", groupID, dingTalkRestShiftSentinel)
	return dingTalkRestShiftSentinel
}

func summarizeAttendanceGroupRestDebug(group map[string]interface{}) string {
	shiftRefs := make([]string, 0)
	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					shiftRefs = append(shiftRefs, fmt.Sprintf("%d", int64(id)))
				}
			}
		}
	}
	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				shiftRefs = append(shiftRefs, fmt.Sprintf("%d", int64(id)))
			}
		}
	}

	cycleItems := make([]string, 0)
	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for cycleIdx, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for itemIdx, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				classID := int64(0)
				if v, ok := item["class_id"].(float64); ok {
					classID = int64(v)
				}
				className, _ := item["class_name"].(string)
				cycleItems = append(cycleItems, fmt.Sprintf("cycle[%d].item[%d]=%d:%s", cycleIdx, itemIdx, classID, className))
			}
		}
	}

	return fmt.Sprintf("group=%v(%v) shift_ids=%v cycle_items=%v",
		group["group_name"], group["group_id"], shiftRefs, cycleItems)
}

func GetShiftList() ([]map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allShifts []map[string]interface{}
	cursor := 0

	for {
		body := map[string]interface{}{
			"op_user_id": "",
			"cursor":     cursor,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/shift/list?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇鐝鍒楄〃澶辫触: %w", err)
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("鑾峰彇鐝鍒楄〃澶辫触: %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		shifts, ok := result["result"].([]interface{})
		if !ok || len(shifts) == 0 {
			break
		}

		for _, s := range shifts {
			if sm, ok := s.(map[string]interface{}); ok {
				allShifts = append(allShifts, sm)
			}
		}

		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		nextCursor, _ := result["cursor"].(float64)
		cursor = int(nextCursor)
	}

	logrus.Infof("get shifts complete: %d", len(allShifts))
	return allShifts, nil
}

// FindShiftByName 浠庣彮娆″垪琛ㄤ腑鎸夊悕绉版煡鎵?
func FindShiftByName(shifts []map[string]interface{}, name string) (int64, bool) {
	for _, shift := range shifts {
		if getString(shift, "name") == name {
			if id := int64(getFloat(shift, "id")); id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}

// CreateShift 鍦ㄩ拤閽夊垱寤烘柊鐝
func CreateShift(opUserID string, shiftName string, checkInTime string, checkOutTime string) (int64, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return 0, err
	}

	checkInAt, err := formatShiftCheckTime(checkInTime, false)
	if err != nil {
		return 0, fmt.Errorf("娑撳﹦褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
	}
	checkOutAt, err := formatShiftCheckTime(checkOutTime, false)
	if err != nil {
		return 0, fmt.Errorf("娑撳褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
	}

	across := 0
	if !checkOutAt.After(checkInAt) {
		across = 1
		checkOutAt, err = formatShiftCheckTime(checkOutTime, true)
		if err != nil {
			return 0, fmt.Errorf("娑撳褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
		}
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"shift": map[string]interface{}{
			"name": shiftName,
			"sections": []map[string]interface{}{
				{
					"times": []map[string]interface{}{
						{
							"check_type": "OnDuty",
							"check_time": checkInAt.Format("2006-01-02 15:04:05"),
							"across":     0,
							"free_check": false,
						},
						{
							"check_type": "OffDuty",
							"check_time": checkOutAt.Format("2006-01-02 15:04:05"),
							"across":     across,
							"free_check": false,
						},
					},
				},
			},
		},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/shift/add?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return 0, fmt.Errorf("鍒涘缓鐝澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return 0, fmt.Errorf("鍒涘缓鐝澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("鍒涘缓鐝鍝嶅簲鏍煎紡寮傚父")
	}

	shiftID := int64(getFloat(result, "id"))
	if shiftID == 0 {
		return 0, fmt.Errorf("鍒涘缓鐝鏈繑鍥炴湁鏁圛D")
	}

	logrus.Infof("鍒涘缓鐝鎴愬姛: name=%s, id=%d", shiftName, shiftID)
	return shiftID, nil
}

// GetUserScheduleList 鑾峰彇鐢ㄦ埛鏌愭鏃堕棿鐨勬帓鐝?
func formatShiftCheckTime(timeText string, nextDay bool) (time.Time, error) {
	timeText = strings.TrimSpace(timeText)
	layouts := []string{"15:04", "15:04:05"}
	baseDate := "2020-12-02"
	if nextDay {
		baseDate = "2020-12-03"
	}

	var lastErr error
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, timeText, time.Local)
		if err != nil {
			lastErr = err
			continue
		}
		dateTime := fmt.Sprintf("%s %02d:%02d:%02d", baseDate, t.Hour(), t.Minute(), t.Second())
		return time.ParseInLocation("2006-01-02 15:04:05", dateTime, time.Local)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unsupported time format")
	}
	return time.Time{}, lastErr
}

func GetUserScheduleList(userID string, workDateFrom, workDateTo string) ([]map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	// 瑙ｆ瀽鏃ユ湡鑼冨洿
	startDate, err := time.Parse("2006-01-02", workDateFrom)
	if err != nil {
		return nil, fmt.Errorf("寮€濮嬫棩鏈熸牸寮忛敊璇? %w", err)
	}

	endDate, err := time.Parse("2006-01-02", workDateTo)
	if err != nil {
		return nil, fmt.Errorf("缁撴潫鏃ユ湡鏍煎紡閿欒: %w", err)
	}

	var allSchedules []map[string]interface{}

	// 閬嶅巻姣忎竴澶╋紝璋冪敤API鏌ヨ
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")

		body := map[string]interface{}{
			"user_id":   userID,
			"work_date": dateStr,
		}

		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/schedule/listbyday?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			logrus.Warnf("鑾峰彇 %s 鎺掔彮澶辫触: %v", dateStr, err)
			continue
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			logrus.Warnf("鑾峰彇 %s 鎺掔彮澶辫触: %s", dateStr, errmsg)
			continue
		}

		result, ok := resp["result"].([]interface{})
		if !ok {
			continue
		}

		for _, s := range result {
			if sm, ok := s.(map[string]interface{}); ok {
				allSchedules = append(allSchedules, sm)
			}
		}
	}

	return allSchedules, nil
}

// GetScheduleListBatchByDay 鎵归噺鑾峰彇澶氫釜鐢ㄦ埛鏌愬ぉ鐨勬帓鐝?// 浣跨敤 /topapi/attendance/schedule/listbyusers 鎺ュ彛锛屾敮鎸佷竴娆℃煡璇㈠涓敤鎴?
func GetScheduleListBatchByDay(userIDs []string, workDate string) ([]map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	t, err := time.Parse("2006-01-02", workDate)
	if err != nil {
		return nil, fmt.Errorf("鏃ユ湡鏍煎紡閿欒: %w", err)
	}
	dayMs := t.UnixMilli()

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		return nil, fmt.Errorf("鏈厤缃?DINGTALK_ADMIN_USER_ID 鐜鍙橀噺")
	}

	body := map[string]interface{}{
		"op_user_id":     opUserID,
		"userids":        strings.Join(userIDs, ","),
		"from_date_time": dayMs,
		"to_date_time":   dayMs,
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/schedule/listbyusers?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("鎵归噺鑾峰彇 %s 鎺掔彮澶辫触: %w", workDate, err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("鎵归噺鑾峰彇 %s 鎺掔彮澶辫触: %s", workDate, errmsg)
	}

	result, ok := resp["result"].([]interface{})
	if !ok {
		return nil, nil
	}

	var schedules []map[string]interface{}
	for _, s := range result {
		if sm, ok := s.(map[string]interface{}); ok {
			schedules = append(schedules, sm)
		}
	}
	return schedules, nil
}

// GetHolidaysFromDingTalk 浠庨拤閽夎幏鍙栬妭鍋囨棩鏁版嵁
func GetHolidaysFromDingTalk(userID string, startDate, endDate string) (map[string]string, error) {
	schedules, err := GetUserScheduleList(userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鎺掔彮鏁版嵁澶辫触: %w", err)
	}

	holidays := make(map[string]string)

	for _, schedule := range schedules {
		// 鑾峰彇宸ヤ綔鏃ユ湡
		workDate, ok := schedule["work_date"].(string)
		if !ok {
			continue
		}

		// 检查是否为休息日
		isRest, ok := schedule["is_rest"].(string)
		if !ok {
			continue
		}

		if isRest == "Y" {
			// 从排班数据中提取节假日名称
			holidays[workDate] = "节假日"
		}
	}

	return holidays, nil
}

// FindScheduleGroupID 浠庤€冨嫟缁勫垪琛ㄤ腑鎵惧埌绗竴涓帓鐝埗鎴栬疆鐝埗鑰冨嫟缁?ID
func FindScheduleGroupID(groups []map[string]interface{}) (int64, error) {
	if len(groups) == 0 {
		return 0, fmt.Errorf("没有找到考勤组")
	}

	preferredGroupID := strings.TrimSpace(os.Getenv("DINGTALK_ATTENDANCE_GROUP_ID"))
	preferredGroupName := strings.TrimSpace(os.Getenv("DINGTALK_ATTENDANCE_GROUP_NAME"))

	if preferredGroupID != "" {
		expectedID, err := strconv.ParseInt(preferredGroupID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("DINGTALK_ATTENDANCE_GROUP_ID 閺嶇厧绱￠柨娆掝嚖: %w", err)
		}
		for _, group := range groups {
			gid, ok := group["group_id"].(float64)
			if !ok || int64(gid) != expectedID {
				continue
			}
			groupType, _ := group["type"].(string)
			groupName, _ := group["group_name"].(string)
			if groupType != "SCHEDULE" && groupType != "TURN" {
				return 0, fmt.Errorf("指定考勤组 %s(%d) 类型为 %s，不支持排班操作", groupName, expectedID, groupType)
			}
			logrus.Infof("娴ｈ法鏁ら幐鍥х暰閼板啫瀚熺紒? %s, 缁鐎? %s, ID: %d", groupName, groupType, expectedID)
			return expectedID, nil
		}
		return 0, fmt.Errorf("閹稿洤鐣鹃懓鍐ㄥ珶缂? ID=%d 閺堫亜婀崣顖滄暏閼板啫瀚熺紒鍕灙鐞涖劋鑵戦幍鎯у煂", expectedID)
	}

	if preferredGroupName != "" {
		for _, group := range groups {
			groupType, _ := group["type"].(string)
			groupName, _ := group["group_name"].(string)
			if !strings.EqualFold(strings.TrimSpace(groupName), preferredGroupName) {
				continue
			}
			if groupType != "SCHEDULE" && groupType != "TURN" {
				return 0, fmt.Errorf("指定考勤组 %s 类型为 %s，不支持排班操作", groupName, groupType)
			}
			if gid, ok := group["group_id"].(float64); ok {
				logrus.Infof("娴ｈ法鏁ら幐鍥х暰閼板啫瀚熺紒? %s, 缁鐎? %s, ID: %v", groupName, groupType, gid)
				return int64(gid), nil
			}
		}
		return 0, fmt.Errorf("閹稿洤鐣鹃懓鍐ㄥ珶缂? %s 閺堫亜婀崣顖滄暏閼板啫瀚熺紒鍕灙鐞涖劋鑵戦幍鎯у煂", preferredGroupName)
	}

	type eligibleGroup struct {
		id   int64
		name string
		kind string
	}

	eligibleGroups := make([]eligibleGroup, 0)
	for _, group := range groups {
		groupType, ok := group["type"].(string)
		groupName, _ := group["group_name"].(string)
		logrus.Infof("鑰冨嫟缁? %s, 绫诲瀷: %s", groupName, groupType)
		if !ok || (groupType != "SCHEDULE" && groupType != "TURN") {
			continue
		}
		gid, ok := group["group_id"].(float64)
		if !ok {
			continue
		}
		eligibleGroups = append(eligibleGroups, eligibleGroup{
			id:   int64(gid),
			name: groupName,
			kind: groupType,
		})
	}

	if len(eligibleGroups) == 1 {
		group := eligibleGroups[0]
		logrus.Infof("浣跨敤鑰冨嫟缁? %s, 绫诲瀷: %s, ID: %d", group.name, group.kind, group.id)
		return group.id, nil
	}
	if len(eligibleGroups) > 1 {
		names := make([]string, 0, len(eligibleGroups))
		for _, group := range eligibleGroups {
			names = append(names, fmt.Sprintf("%s(%d)", group.name, group.id))
		}
		return 0, fmt.Errorf("found %d eligible attendance groups: %s; set DINGTALK_ATTENDANCE_GROUP_ID or DINGTALK_ATTENDANCE_GROUP_NAME before syncing", len(eligibleGroups), strings.Join(names, ", "))
	}

	for _, group := range groups {
		groupType, ok := group["type"].(string)
		if ok && groupType == "FIXED" {
			return 0, fmt.Errorf("考勤组为固定班制，无法排班，请创建排班制或轮班制考勤组")
		}
	}
	return 0, fmt.Errorf("没有找到可用考勤组，请确保已创建排班制或轮班制考勤组")
}

// SetAttendanceScheduleWithGroup 浣跨敤棰勮幏鍙栫殑鑰冨嫟缁?ID 璁剧疆鐢ㄦ埛鏌愬ぉ鐨勬帓鐝?
func SetAttendanceScheduleWithGroup(opUserID string, userID string, workDate string, shiftID int64, groupID int64) error {
	accessToken, err := GetAccessToken()
	if err != nil {
		return err
	}

	t, err := time.Parse("2006-01-02", workDate)
	if err != nil {
		return fmt.Errorf("鏃ユ湡鏍煎紡閿欒: %w", err)
	}

	scheduleItem := map[string]interface{}{
		"userid":    userID,
		"work_date": t.UnixMilli(),
		"is_rest":   shiftID == 0,
		"shift_id":  shiftID,
	}
	if shiftID == 0 {
		scheduleItem["shift_id"] = resolveScheduleAsyncRestShiftID(opUserID, groupID)
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"group_id":   groupID,
		"schedules":  []map[string]interface{}{scheduleItem},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/schedule/async?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return fmt.Errorf("璁剧疆鎺掔彮澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("璁剧疆鎺掔彮澶辫触: %s", errmsg)
	}

	logrus.Infof("璁剧疆鎺掔彮鎴愬姛: 鐢ㄦ埛=%s, 鏃ユ湡=%s, 鐝ID=%d", userID, workDate, shiftID)
	return nil
}

// ScheduleItem 鎺掔彮鏉＄洰
type ScheduleItem struct {
	UserID   string
	WorkDate string // "2006-01-02"
	ShiftID  int64  // 0 = 浼戞伅
}

// ValidationResult 校验结果
type ValidationResult struct {
	Valid   bool              `json:"valid"`
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"` // userID -> error message
}

// ValidateScheduleItems 校验排班项目
func ValidateScheduleItems(opUserID string, items []ScheduleItem, groupID int64) *ValidationResult {
	result := &ValidationResult{
		Valid:   true,
		Message: "",
		Errors:  make(map[string]string),
	}

	// 1. 校验考勤组是否存在且有效
	group, err := GetAttendanceGroup(opUserID, groupID)
	if err != nil {
		result.Valid = false
		result.Message = "考勤组不存在或无效"
		return result
	}

	// 2. 收集考勤组中的员工
	groupDetail, err := GetAttendanceGroup(opUserID, groupID)
	if err != nil {
		result.Valid = false
		result.Message = "无法获取考勤组详情"
		return result
	}

	// 提取考勤组中的员工ID
	employeeIDs := make(map[string]bool)
	if members, ok := groupDetail["userids"].(map[string]interface{}); ok {
		if userIDs, ok := members["string"].([]interface{}); ok {
			for _, uid := range userIDs {
				if userID, ok := uid.(string); ok {
					employeeIDs[userID] = true
				}
			}
		}
	}

	// 3. 提取考勤组中的班次
	groupShiftIDs := CollectAttendanceGroupShiftIDs(group)

	// 4. 校验每个项目
	now := time.Now()
	for _, item := range items {
		// 校验员工是否在考勤组中
		if !employeeIDs[item.UserID] {
			result.Errors[item.UserID] = "员工不在考勤组中"
			result.Valid = false
			continue
		}

		// 校验日期是否在允许范围内（不能是过去的日期）
		workDate, err := time.Parse("2006-01-02", item.WorkDate)
		if err != nil {
			result.Errors[item.UserID] = "日期格式错误"
			result.Valid = false
			continue
		}

		if workDate.Before(now.AddDate(0, 0, -1)) {
			result.Errors[item.UserID] = "不能修改过去的日期"
			result.Valid = false
			continue
		}

		// 校验班次是否在考勤组中
		if item.ShiftID > 0 && !containsShiftID(groupShiftIDs, item.ShiftID) {
			result.Errors[item.UserID] = "班次不在考勤组中"
			result.Valid = false
			continue
		}
	}

	if !result.Valid && result.Message == "" {
		result.Message = "部分项目校验失败"
	}

	return result
}

// containsShiftID 检查班次ID是否在集合中
func containsShiftID(shiftIDs map[int64]struct{}, shiftID int64) bool {
	_, ok := shiftIDs[shiftID]
	return ok
}

// BatchSetAttendanceSchedule 鎵归噺璁剧疆鎺掔彮锛屽皢澶氭潯鎺掔彮鎵撳寘鍒板崟娆?API 璇锋眰
func BatchSetAttendanceSchedule(opUserID string, items []ScheduleItem, groupID int64) (successCount int, failedItems []ScheduleItem, err error) {
	if len(items) == 0 {
		return 0, nil, nil
	}

	// 前置校验
	validationResult := ValidateScheduleItems(opUserID, items, groupID)
	if !validationResult.Valid {
		// 收集失败的项目
		failedMap := make(map[string]bool)
		for userID := range validationResult.Errors {
			failedMap[userID] = true
		}

		for _, item := range items {
			if failedMap[item.UserID] {
				failedItems = append(failedItems, item)
			}
		}

		return 0, failedItems, fmt.Errorf(validationResult.Message)
	}

	accessToken, err := GetAccessToken()
	if err != nil {
		return 0, items, err
	}

	restShiftID := resolveScheduleAsyncRestShiftID(opUserID, groupID)

	// Split: work items (ShiftID>0) MUST be in a separate batch from rest items (ShiftID==0).
	// DingTalk rejects the entire batch if any schedule entry is missing shift_id.
	var workItems, restItems []ScheduleItem
	for _, item := range items {
		if item.ShiftID > 0 {
			workItems = append(workItems, item)
		} else {
			restItems = append(restItems, item)
		}
	}

	const batchSize = 200
	var batchErrors []string

	pushBatch := func(chunk []ScheduleItem, makeSchedule func(ScheduleItem, int64) map[string]interface{}) {
		schedules := make([]map[string]interface{}, 0, len(chunk))
		var parseFailItems []ScheduleItem
		for _, item := range chunk {
			t, parseErr := time.Parse("2006-01-02", item.WorkDate)
			if parseErr != nil {
				parseFailItems = append(parseFailItems, item)
				continue
			}
			schedules = append(schedules, makeSchedule(item, t.UnixMilli()))
		}
		if len(parseFailItems) > 0 {
			failedItems = append(failedItems, parseFailItems...)
		}
		if len(schedules) == 0 {
			return
		}
		body := map[string]interface{}{"op_user_id": opUserID, "group_id": groupID, "schedules": schedules}
		resp, postErr := postJSONOAPI(fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/schedule/async?access_token=%s", accessToken), body)
		if postErr != nil {
			failedItems = append(failedItems, chunk...)
			batchErrors = append(batchErrors, "request failed: "+postErr.Error())
			return
		}
		if errcode, _ := resp["errcode"].(float64); errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			failedItems = append(failedItems, chunk...)
			batchErrors = append(batchErrors, "api error: "+errmsg)
			return
		}
		successCount += len(schedules)
		logrus.Infof("batch set attendance schedule success: %d items", len(schedules))
	}

	// Push work items
	for i := 0; i < len(workItems); i += batchSize {
		end := i + batchSize
		if end > len(workItems) {
			end = len(workItems)
		}
		pushBatch(workItems[i:end], func(item ScheduleItem, ts int64) map[string]interface{} {
			return map[string]interface{}{
				"userid":    item.UserID,
				"work_date": ts,
				"is_rest":   false,
				"shift_id":  item.ShiftID,
			}
		})
	}

	// Push rest items with is_rest=true and shift_id=0 in a separate batch.
	// DingTalk batch API requires shift_id to be present (even 0) for all items.
	for i := 0; i < len(restItems); i += batchSize {
		end := i + batchSize
		if end > len(restItems) {
			end = len(restItems)
		}
		pushBatch(restItems[i:end], func(item ScheduleItem, ts int64) map[string]interface{} {
			return map[string]interface{}{
				"userid":    item.UserID,
				"work_date": ts,
				"is_rest":   true,
				"shift_id":  restShiftID,
			}
		})
	}

	if len(batchErrors) > 0 {
		return successCount, failedItems, fmt.Errorf(strings.Join(batchErrors, "; "))
	}
	return successCount, failedItems, nil
}

// SetAttendanceSchedule 璁剧疆鐢ㄦ埛鏌愬ぉ鐨勬帓鐝紙鍚戝悗鍏煎锛屽唴閮ㄤ細鏌ヨ鑰冨嫟缁勶級
// shiftID > 0 琛ㄧず涓婄彮锛堜娇鐢ㄨ鐝锛夛紝shiftID == 0 琛ㄧず浼戞伅
func SetAttendanceSchedule(opUserID string, userID string, workDate string, shiftID int64) error {
	groups, err := GetAttendanceGroups()
	if err != nil {
		return fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %w", err)
	}
	groupID, err := FindScheduleGroupID(groups)
	if err != nil {
		return err
	}
	return SetAttendanceScheduleWithGroup(opUserID, userID, workDate, shiftID, groupID)
}
