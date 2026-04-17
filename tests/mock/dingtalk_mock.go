package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"
)

// DingTalkMock 钉钉API模拟服务
type DingTalkMock struct {
	Server *httptest.Server
}

// NewDingTalkMock 创建钉钉API模拟服务
func NewDingTalkMock() *DingTalkMock {
	handler := http.NewServeMux()

	// 模拟获取用户信息
	handler.HandleFunc("/user/get", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		
		// 模拟不同场景
		switch code {
		case "success":
			resp := map[string]interface{}{
				"errcode": 0,
				"errmsg":  "success",
				"userid":  "user123",
				"name":    "张三",
				"email":   "zhangsan@example.com",
				"mobile":  "13800138000",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case "fail":
			resp := map[string]interface{}{
				"errcode": 400,
				"errmsg":  "invalid code",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(resp)
		case "timeout":
			time.Sleep(5 * time.Second)
			resp := map[string]interface{}{
				"errcode": 0,
				"errmsg":  "success",
				"userid":  "user123",
				"name":    "张三",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case "empty":
			resp := map[string]interface{}{
				"errcode": 0,
				"errmsg":  "success",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		default:
			resp := map[string]interface{}{
				"errcode": 0,
				"errmsg":  "success",
				"userid":  "user123",
				"name":    "张三",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	})

	// 模拟获取部门列表
	handler.HandleFunc("/department/list", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"errcode": 0,
			"errmsg":  "success",
			"department": []map[string]interface{}{
				{
					"id":       1,
					"name":     "技术部",
					"parentid": 0,
					"order":    1,
				},
				{
					"id":       2,
					"name":     "市场部",
					"parentid": 0,
					"order":    2,
				},
				{
					"id":       3,
					"name":     "产品部",
					"parentid": 0,
					"order":    3,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 模拟获取员工列表
	handler.HandleFunc("/user/list", func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("department_id")

		resp := map[string]interface{}{
			"errcode": 0,
			"errmsg":  "success",
			"userlist": []map[string]interface{}{
				{
					"userid":     "user123",
					"name":       "张三",
					"email":      "zhangsan@example.com",
					"mobile":     "13800138000",
					"department": []int{1},
					"position":   "工程师",
				},
				{
					"userid":     "user456",
					"name":       "李四",
					"email":      "lisi@example.com",
					"mobile":     "13900139000",
					"department": []int{1},
					"position":   "产品经理",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 模拟获取考勤记录
	handler.HandleFunc("/attendance/list", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"errcode": 0,
			"errmsg":  "success",
			"recordresult": []map[string]interface{}{
				{
					"userid":     "user123",
					"checkTime":  "2024-01-01 09:00:00",
					"checkType":  "OnDuty",
					"locationResult": "正常",
				},
				{
					"userid":     "user123",
					"checkTime":  "2024-01-01 18:00:00",
					"checkType":  "OffDuty",
					"locationResult": "正常",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 模拟获取审批列表
	handler.HandleFunc("/topapi/processinstance/list", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"errcode": 0,
			"errmsg":  "success",
			"result": map[string]interface{}{
				"list": []map[string]interface{}{
					{
						"process_instance_id": "process123",
						"title":               "请假申请",
						"applicant_userid":    "user123",
						"status":              "COMPLETED",
						"create_time":         1672531200000,
						"finish_time":         1672538400000,
					},
					{
						"process_instance_id": "process456",
						"title":               "报销申请",
						"applicant_userid":    "user456",
						"status":              "IN_PROGRESS",
						"create_time":         1672617600000,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	return &DingTalkMock{Server: server}
}

// Close 关闭模拟服务
func (m *DingTalkMock) Close() {
	m.Server.Close()
}

// GetURL 获取模拟服务的URL
func (m *DingTalkMock) GetURL() string {
	return m.Server.URL
}
