package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	// 直接使用配置值
	appKey := "dingbfvbm4tots1gdnus"
	appSecret := "590JYvmSxrZCjqZY_XUXU_krsSlLHJhj107hA5EtA1La3cgrMoJhHQhaHf9dvIHg"

	if appKey == "" || appSecret == "" {
		fmt.Println("请设置 DINGTALK_APP_KEY 和 DINGTALK_APP_SECRET 环境变量")
		return
	}

	// 1. 获取 access_token
	token, err := getAccessToken(appKey, appSecret)
	if err != nil {
		fmt.Printf("获取 access_token 失败: %v\n", err)
		return
	}

	fmt.Printf("获取到 access_token: %s\n", token)

	// 2. 测试获取部门列表（使用新版 API）
	fmt.Println("=== 测试新版 API 获取部门列表 ===")
	depts, err := getDepartmentsNew(token)
	if err != nil {
		fmt.Printf("获取部门列表失败: %v\n", err)
	} else {
		fmt.Printf("获取到 %d 个部门\n", len(depts))
		for _, dept := range depts {
			fmt.Printf("部门 ID: %d, 名称: %s, 父部门 ID: %d\n", dept.DeptID, dept.Name, dept.ParentID)
		}
	}

	// 3. 测试获取根部门详情
	fmt.Println("\n=== 测试获取根部门详情 ===")
	rootDept, err := getDeptDetail(token, 1)
	if err != nil {
		fmt.Printf("获取根部门详情失败: %v\n", err)
	} else {
		fmt.Printf("根部门 ID: %d, 名称: %s, 父部门 ID: %d\n", rootDept.DeptID, rootDept.Name, rootDept.ParentID)
	}

	// 4. 测试获取通讯录权限范围
	fmt.Println("\n=== 测试获取通讯录权限范围 ===")
	scope, err := getContactScope(token)
	if err != nil {
		fmt.Printf("获取通讯录权限范围失败: %v\n", err)
	} else {
		fmt.Printf("通讯录权限范围: %s\n", scope)
	}
}

type AccessTokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpireIn    int    `json:"expireIn"`
	ErrCode     int    `json:"errcode,omitempty"`
	ErrMsg      string `json:"errmsg,omitempty"`
}

type DeptInfo struct {
	DeptID   int64  `json:"dept_id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parent_id"`
}

type DepartmentListResponse struct {
	Result  []DeptInfo `json:"result"`
	ErrCode int        `json:"errcode"`
	ErrMsg  string     `json:"errmsg"`
}

func getAccessToken(appKey, appSecret string) (string, error) {
	url := "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	data := fmt.Sprintf(`{"appKey": "%s", "appSecret": "%s"}`, appKey, appSecret)

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	fmt.Printf("获取 access_token 响应: %s\n", string(body))

	var tokenResp AccessTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.ErrCode != 0 {
		return "", fmt.Errorf("获取 access_token 失败: %s", tokenResp.ErrMsg)
	}

	return tokenResp.AccessToken, nil
}

func getDepartments(token string) ([]DeptInfo, error) {
	url := fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=%s", token)
	data := `{"dept_id": 1}`

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var deptResp DepartmentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&deptResp); err != nil {
		return nil, err
	}

	if deptResp.ErrCode != 0 {
		return nil, fmt.Errorf("获取部门列表失败: %s", deptResp.ErrMsg)
	}

	return deptResp.Result, nil
}

func getSubDepartments(token string, deptID int64) ([]DeptInfo, error) {
	url := fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=%s", token)
	data := fmt.Sprintf(`{"dept_id": %d}`, deptID)

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fmt.Printf("获取子部门响应: %s\n", string(body))

	var deptResp DepartmentListResponse
	if err := json.Unmarshal(body, &deptResp); err != nil {
		return nil, err
	}

	if deptResp.ErrCode != 0 {
		return nil, fmt.Errorf("获取子部门失败: %s", deptResp.ErrMsg)
	}

	return deptResp.Result, nil
}

func getDepartmentsNew(token string) ([]DeptInfo, error) {
	url := fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=%s", token)
	data := `{"dept_id": 1, "language": "zh_CN"}`

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fmt.Printf("获取部门列表响应: %s\n", string(body))

	var deptResp DepartmentListResponse
	if err := json.Unmarshal(body, &deptResp); err != nil {
		return nil, err
	}

	if deptResp.ErrCode != 0 {
		return nil, fmt.Errorf("获取部门列表失败: %s", deptResp.ErrMsg)
	}

	return deptResp.Result, nil
}

func getDeptDetail(token string, deptID int64) (*DeptInfo, error) {
	url := fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/get?access_token=%s", token)
	data := fmt.Sprintf(`{"dept_id": %d, "language": "zh_CN"}`, deptID)

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fmt.Printf("获取部门详情响应: %s\n", string(body))

	type DeptDetailResponse struct {
		Result  DeptInfo `json:"result"`
		ErrCode int      `json:"errcode"`
		ErrMsg  string   `json:"errmsg"`
	}

	var deptResp DeptDetailResponse
	if err := json.Unmarshal(body, &deptResp); err != nil {
		return nil, err
	}

	if deptResp.ErrCode != 0 {
		return nil, fmt.Errorf("获取部门详情失败: %s", deptResp.ErrMsg)
	}

	return &deptResp.Result, nil
}

func getContactScope(token string) (string, error) {
	url := fmt.Sprintf("https://api.dingtalk.com/v1.0/contact/scopes?access_token=%s", token)

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	fmt.Printf("获取通讯录权限范围响应: %s\n", string(body))

	type ScopeResponse struct {
		Scope struct {
			UserScope  string `json:"userScope"`
			DeptScope  string `json:"deptScope"`
			IsAllScope bool   `json:"isAllScope"`
		} `json:"scope"`
		ErrCode int    `json:"errcode,omitempty"`
		ErrMsg  string `json:"errmsg,omitempty"`
	}

	var scopeResp ScopeResponse
	if err := json.Unmarshal(body, &scopeResp); err != nil {
		return "", err
	}

	if scopeResp.ErrCode != 0 {
		return "", fmt.Errorf("获取通讯录权限范围失败: %s", scopeResp.ErrMsg)
	}

	return fmt.Sprintf("isAllScope: %v, userScope: %s, deptScope: %s", scopeResp.Scope.IsAllScope, scopeResp.Scope.UserScope, scopeResp.Scope.DeptScope), nil
}
