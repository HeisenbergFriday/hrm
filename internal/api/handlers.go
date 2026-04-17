package api

import (
	"fmt"
	"net/http"
	"os"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// 分页响应结构
type PagedResponse struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
}

// HealthCheck 健康检查
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"status": "ok"},
	})
}

// generateToken 生成 JWT token
func generateToken(userID, userName string) (string, time.Time, error) {
	expiresAt := time.Now().Add(24 * time.Hour)
	claims := &middleware.Claims{
		UserID:   userID,
		UserName: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(secret))
	return tokenString, expiresAt, err
}

// Login 登录
func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "用户名和密码不能为空",
		})
		return
	}

	// 用 user_id 或 email 查找用户
	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByUserID(req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "用户名或密码错误",
		})
		return
	}

	// 校验密码
	if !database.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "用户名或密码错误",
		})
		return
	}

	// 生成 JWT token
	tokenString, expiresAt, err := generateToken(fmt.Sprintf("%d", user.ID), user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成令牌失败",
		})
		return
	}

	// 写入 LoginLog
	database.DB.Create(&database.LoginLog{
		UserID:      fmt.Sprintf("%d", user.ID),
		UserName:    user.Name,
		LoginType:   "local",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token": tokenString,
			"user": gin.H{
				"id":            user.ID,
				"user_id":       user.UserID,
				"name":          user.Name,
				"email":         user.Email,
				"mobile":        user.Mobile,
				"department_id": user.DepartmentID,
				"position":      user.Position,
				"avatar":        user.Avatar,
				"status":        user.Status,
			},
			"expires_at": expiresAt,
		},
	})
}

// GetUsers 获取用户列表
func GetUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	userService := service.NewUserService(database.DB)
	users, total, err := userService.GetUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取用户列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    PagedResponse{Items: users, Total: total},
	})
}

// GetUser 获取用户详情
func GetUser(c *gin.Context) {
	id := c.Param("id")

	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"user": user},
	})
}

// UpdateUser 更新用户信息
func UpdateUser(c *gin.Context) {
	id := c.Param("id")

	var updateData struct {
		Extension map[string]interface{} `json:"extension"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	user.Extension = updateData.Extension
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新用户失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"user": user},
	})
}

// GetDepartments 获取部门列表
func GetDepartments(c *gin.Context) {
	departmentService := service.NewDepartmentService(database.DB)
	departments, err := departmentService.GetAllDepartments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"departments": departments},
	})
}

// GetDepartment 获取部门详情
func GetDepartment(c *gin.Context) {
	id := c.Param("id")

	departmentService := service.NewDepartmentService(database.DB)
	department, err := departmentService.GetDepartmentByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "部门不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"department": department},
	})
}

// SyncUsers 同步用户
func SyncUsers(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	// 从钉钉拉取用户
	users, err := dingtalk.SyncUsers()
	if err != nil {
		syncService.UpdateSyncStatus("users", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步用户失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	userService := service.NewUserService(database.DB)
	count := 0
	for _, u := range users {
		deptID := ""
		if len(u.DeptIDList) > 0 {
			deptID = fmt.Sprintf("%d", u.DeptIDList[0])
		}
		status := "active"
		if !u.Active {
			status = "inactive"
		}

		existing, err := userService.GetUserByUserID(u.UserID)
		if err != nil {
			// 新建
			newUser := &database.User{
				UserID:       u.UserID,
				Name:         u.Name,
				Email:        u.Email,
				Mobile:       u.Mobile,
				DepartmentID: deptID,
				Position:     u.Position,
				Avatar:       u.Avatar,
				Status:       status,
			}
			userService.CreateUser(newUser)
		} else {
			// 更新
			existing.Name = u.Name
			existing.Email = u.Email
			existing.Mobile = u.Mobile
			existing.DepartmentID = deptID
			existing.Position = u.Position
			existing.Avatar = u.Avatar
			existing.Status = status
			userService.UpdateUser(existing)
		}
		count++
	}

	syncService.UpdateSyncStatus("users", "success", fmt.Sprintf("同步 %d 个用户", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"count": count},
	})
}

// SyncDepartments 同步部门
func SyncDepartments(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	// 从钉钉拉取部门
	depts, err := dingtalk.SyncDepartments()
	if err != nil {
		syncService.UpdateSyncStatus("departments", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步部门失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	deptService := service.NewDepartmentService(database.DB)
	count := 0
	for _, d := range depts {
		deptID := fmt.Sprintf("%d", d.DeptID)
		parentID := fmt.Sprintf("%d", d.ParentID)

		existing, err := deptService.GetDepartmentByDepartmentID(deptID)
		if err != nil {
			// 新建
			newDept := &database.Department{
				DepartmentID: deptID,
				Name:         d.Name,
				ParentID:     parentID,
			}
			deptService.CreateDepartment(newDept)
		} else {
			// 更新
			existing.Name = d.Name
			existing.ParentID = parentID
			deptService.UpdateDepartment(existing)
		}
		count++
	}

	syncService.UpdateSyncStatus("departments", "success", fmt.Sprintf("同步 %d 个部门", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"count": count},
	})
}

// GetDingTalkConfig 返回钉钉前端配置（corpId 等），供 JS-SDK 初始化
func GetDingTalkConfig(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"corp_id":   dingtalk.GetCorpID(),
			"client_id": os.Getenv("DINGTALK_APP_KEY"),
		},
	})
}

// DingTalkQRLoginStart 钉钉扫码登录开始
func DingTalkQRLoginStart(c *gin.Context) {
	// 生成state参数
	state := "test_state"

	// 获取钉钉扫码登录二维码
	qrCodeURL, err := dingtalk.GetQRCode(state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取二维码失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"qr_code_url": qrCodeURL,
			"state":       state,
		},
	})
}

// DingTalkInAppLogin 钉钉内免登
func DingTalkInAppLogin(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	// 1. 通过免登码获取企业内 userid
	userid, err := dingtalk.GetUserIDByInAppCode(req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "钉钉免登失败: " + err.Error(),
		})
		return
	}

	// 2. 通过 userid 获取用户详细信息（Contact.User.Read）
	userDetail, err := dingtalk.GetUserDetailByUserID(userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取用户详情失败: " + err.Error(),
		})
		return
	}

	name, _ := userDetail["name"].(string)
	email, _ := userDetail["email"].(string)
	mobile, _ := userDetail["mobile"].(string)
	avatar, _ := userDetail["avatar"].(string)
	position, _ := userDetail["title"].(string)

	// 3. 查找或创建本地用户（用钉钉 userid 匹配）
	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByUserID(userid)
	if err != nil {
		// 用户不存在，自动创建
		deptID := "1"
		if deptList, ok := userDetail["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
			if id, ok := deptList[0].(float64); ok {
				deptID = fmt.Sprintf("%d", int64(id))
			}
		}

		newUser := &database.User{
			UserID: userid, Name: name, Email: email,
			Mobile: mobile, Avatar: avatar, Position: position,
			DepartmentID: deptID, Status: "active",
		}
		userService.CreateUser(newUser)
		user = newUser
	}

	tokenString, expiresAt, err := generateToken(fmt.Sprintf("%d", user.ID), user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成令牌失败",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		UserID: fmt.Sprintf("%d", user.ID), UserName: user.Name,
		LoginType: "dingtalk_in_app", LoginStatus: "success",
		IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token": tokenString,
			"user": gin.H{
				"id": user.ID, "user_id": user.UserID,
				"name": user.Name, "email": user.Email,
				"mobile": user.Mobile, "department_id": user.DepartmentID,
				"position": user.Position, "avatar": user.Avatar,
				"status": user.Status,
			},
			"expires_at": expiresAt,
		},
	})
}

// DingTalkCallback 钉钉回调
func DingTalkCallback(c *gin.Context) {
	code := c.Query("authCode")
	if code == "" {
		code = c.Query("code")
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "缺少授权码参数",
		})
		return
	}

	// 通过code获取用户信息
	userInfo, err := dingtalk.GetUserInfoByCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取用户信息失败: " + err.Error(),
		})
		return
	}

	// 查找或创建本地用户
	dtUserID := ""
	if v, ok := userInfo["unionId"].(string); ok {
		dtUserID = v
	} else if v, ok := userInfo["nick"].(string); ok {
		dtUserID = v
	}

	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByUserID(dtUserID)
	if err != nil {
		// 用户不存在，自动创建
		nick, _ := userInfo["nick"].(string)
		email, _ := userInfo["email"].(string)
		mobile, _ := userInfo["mobile"].(string)
		avatarUrl, _ := userInfo["avatarUrl"].(string)

		newUser := &database.User{
			UserID:       dtUserID,
			Name:         nick,
			Email:        email,
			Mobile:       mobile,
			Avatar:       avatarUrl,
			DepartmentID: "1",
			Status:       "active",
		}
		if createErr := userService.CreateUser(newUser); createErr != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "创建用户失败",
			})
			return
		}
		user = newUser
	}

	// 生成 JWT
	tokenString, expiresAt, err := generateToken(fmt.Sprintf("%d", user.ID), user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成令牌失败",
		})
		return
	}

	// 写入登录日志
	database.DB.Create(&database.LoginLog{
		UserID:      fmt.Sprintf("%d", user.ID),
		UserName:    user.Name,
		LoginType:   "dingtalk_qr",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token": tokenString,
			"user": gin.H{
				"id":            user.ID,
				"user_id":       user.UserID,
				"name":          user.Name,
				"email":         user.Email,
				"mobile":        user.Mobile,
				"department_id": user.DepartmentID,
				"position":      user.Position,
				"avatar":        user.Avatar,
				"status":        user.Status,
			},
			"expires_at": expiresAt,
		},
	})
}

// Logout 登出
func Logout(c *gin.Context) {
	// 记录登出日志
	userID, _ := c.Get("userID")
	userName, _ := c.Get("userName")
	if uid, ok := userID.(string); ok {
		uname, _ := userName.(string)
		database.DB.Create(&database.OperationLog{
			UserID:    uid,
			UserName:  uname,
			Operation: "登出",
			Resource:  "系统",
			IP:        c.ClientIP(),
		})
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
	})
}

// GetCurrentUser 获取当前用户信息
func GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "未登录",
		})
		return
	}

	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByID(userID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
		})
		return
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"user": gin.H{
				"id":            user.ID,
				"user_id":       user.UserID,
				"name":          user.Name,
				"email":         user.Email,
				"mobile":        user.Mobile,
				"department_id": user.DepartmentID,
				"position":      user.Position,
				"avatar":        user.Avatar,
				"status":        user.Status,
			},
		},
	})
}

// GetSyncStatus 获取同步状态
func GetSyncStatus(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)
	statuses, err := syncService.GetAllSyncStatus()
	if err != nil {
		// 没有同步记录时返回空状态
		c.JSON(200, Response{
			Code:    200,
			Message: "success",
			Data: gin.H{
				"status": gin.H{
					"departments": gin.H{"last_sync_time": nil, "status": "never"},
					"users":       gin.H{"last_sync_time": nil, "status": "never"},
				},
			},
		})
		return
	}

	result := gin.H{}
	for _, s := range statuses {
		result[s.Type] = gin.H{
			"last_sync_time": s.LastSyncTime,
			"status":         s.Status,
			"message":        s.Message,
		}
	}
	// 确保 departments 和 users 总存在
	if _, ok := result["departments"]; !ok {
		result["departments"] = gin.H{"last_sync_time": nil, "status": "never"}
	}
	if _, ok := result["users"]; !ok {
		result["users"] = gin.H{"last_sync_time": nil, "status": "never"}
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
		Data:    gin.H{"status": result},
	})
}

// GetDepartmentTree 获取部门树
func GetDepartmentTree(c *gin.Context) {
	departmentService := service.NewDepartmentService(database.DB)
	departments, err := departmentService.GetAllDepartments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门列表失败",
		})
		return
	}

	// 构建树形结构
	type TreeNode struct {
		ID       string      `json:"id"`
		Name     string      `json:"name"`
		ParentID string      `json:"parent_id"`
		Children []*TreeNode `json:"children"`
	}

	nodeMap := make(map[string]*TreeNode)
	var roots []*TreeNode

	for _, dept := range departments {
		node := &TreeNode{
			ID:       dept.DepartmentID,
			Name:     dept.Name,
			ParentID: dept.ParentID,
			Children: []*TreeNode{},
		}
		nodeMap[dept.DepartmentID] = node
	}

	for _, node := range nodeMap {
		if parent, ok := nodeMap[node.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"tree": roots},
	})
}

// GetEmployees 获取员工列表
func GetEmployees(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")

	userService := service.NewUserService(database.DB)

	var users []database.User
	var total int64
	var err error

	if departmentID != "" {
		users, total, err = userService.GetUsersByDepartment(departmentID, page, pageSize)
	} else {
		users, total, err = userService.GetUsers(page, pageSize)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": users,
			"total": total,
		},
	})
}

// GetEmployee 获取员工详情
func GetEmployee(c *gin.Context) {
	id := c.Param("id")

	userService := service.NewUserService(database.DB)
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "员工不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"employee": user},
	})
}

// SyncOrgData 同步组织数据
func SyncOrgData(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	// 同步部门
	depts, deptErr := dingtalk.SyncDepartments()
	deptCount := 0
	deptStatus := "success"
	if deptErr != nil {
		deptStatus = "failed"
	} else {
		deptService := service.NewDepartmentService(database.DB)
		for _, d := range depts {
			deptID := fmt.Sprintf("%d", d.DeptID)
			parentID := fmt.Sprintf("%d", d.ParentID)
			existing, err := deptService.GetDepartmentByDepartmentID(deptID)
			if err != nil {
				deptService.CreateDepartment(&database.Department{
					DepartmentID: deptID, Name: d.Name, ParentID: parentID,
				})
			} else {
				existing.Name = d.Name
				existing.ParentID = parentID
				deptService.UpdateDepartment(existing)
			}
			deptCount++
		}
		syncService.UpdateSyncStatus("departments", "success", fmt.Sprintf("同步 %d 个部门", deptCount))
	}

	// 同步用户
	users, userErr := dingtalk.SyncUsers()
	userCount := 0
	userStatus := "success"
	if userErr != nil {
		userStatus = "failed"
	} else {
		userService := service.NewUserService(database.DB)
		employeeService := service.NewEmployeeService(database.DB)
		for _, u := range users {
			deptID := ""
			if len(u.DeptIDList) > 0 {
				deptID = fmt.Sprintf("%d", u.DeptIDList[0])
			}
			status := "active"
			if !u.Active {
				status = "inactive"
			}
			existing, err := userService.GetUserByUserID(u.UserID)
			if err != nil {
				userService.CreateUser(&database.User{
					UserID: u.UserID, Name: u.Name, Email: u.Email,
					Mobile: u.Mobile, DepartmentID: deptID,
					Position: u.Position, Avatar: u.Avatar, Status: status,
				})
				// 同时创建员工档案
				employeeService.CreateProfile(&database.EmployeeProfile{
					UserID:        u.UserID,
					EmployeeID:    u.UserID, // 使用钉钉UserID作为员工工号
					WorkEmail:     u.Email,
					ProfileStatus: status,
				})
			} else {
				existing.Name = u.Name
				existing.Email = u.Email
				existing.Mobile = u.Mobile
				existing.DepartmentID = deptID
				existing.Position = u.Position
				existing.Avatar = u.Avatar
				existing.Status = status
				userService.UpdateUser(existing)
				// 检查是否存在员工档案
				profile, profileErr := employeeService.GetProfileByID(u.UserID)
				if profileErr != nil {
					// 创建员工档案
					employeeService.CreateProfile(&database.EmployeeProfile{
						UserID:        u.UserID,
						EmployeeID:    u.UserID, // 使用钉钉UserID作为员工工号
						WorkEmail:     u.Email,
						ProfileStatus: status,
					})
				} else {
					// 更新员工档案
					profile.WorkEmail = u.Email
					profile.ProfileStatus = status
					employeeService.UpdateProfile(profile)
				}
			}
			userCount++
		}
		syncService.UpdateSyncStatus("users", "success", fmt.Sprintf("同步 %d 个用户", userCount))
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"sync_status": gin.H{
				"departments": gin.H{"count": deptCount, "status": deptStatus},
				"employees":   gin.H{"count": userCount, "status": userStatus},
				"sync_time":   time.Now(),
			},
		},
	})
}

// GetAttendanceRecords 获取考勤记录列表
func GetAttendanceRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"user_id":       c.Query("user_id"),
		"department_id": c.Query("department_id"),
		"start_date":    c.Query("start_date"),
		"end_date":      c.Query("end_date"),
	}

	attendanceService := service.NewAttendanceService(database.DB)
	records, total, err := attendanceService.GetRecords(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": records,
			"total": total,
		},
	})
}

// GetAttendanceStats 获取考勤统计
func GetAttendanceStats(c *gin.Context) {
	filters := map[string]string{
		"start_date":    c.Query("start_date"),
		"end_date":      c.Query("end_date"),
		"department_id": c.Query("department_id"),
	}

	attendanceService := service.NewAttendanceService(database.DB)
	stats, err := attendanceService.GetStats(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    stats,
	})
}

// SyncAttendance 同步考勤数据
func SyncAttendance(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	c.ShouldBindJSON(&req)

	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
	}

	syncService := service.NewSyncService(database.DB)

	// 获取所有用户的钉钉 UserID
	var users []database.User
	database.DB.Select("user_id, name").Find(&users)

	var userIDs []string
	userNameMap := make(map[string]string)
	for _, u := range users {
		if u.UserID != "" && u.UserID != "admin" {
			userIDs = append(userIDs, u.UserID)
			userNameMap[u.UserID] = u.Name
		}
	}

	if len(userIDs) == 0 {
		syncService.UpdateSyncStatus("attendance", "success", "没有需要同步的用户")
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"sync_status": gin.H{"count": 0, "status": "success", "sync_time": time.Now()},
			},
		})
		return
	}

	records, err := dingtalk.GetAttendance(userIDs, req.StartDate, req.EndDate)
	if err != nil {
		syncService.UpdateSyncStatus("attendance", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步考勤失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	count := 0
	for _, r := range records {
		checkType := "上班"
		if r.CheckType == "OffDuty" {
			checkType = "下班"
		}
		checkTime, _ := time.Parse("2006-01-02 15:04:05", r.UserCheckTime)

		record := &database.Attendance{
			UserID:    r.UserID,
			UserName:  userNameMap[r.UserID],
			CheckTime: checkTime,
			CheckType: checkType,
			Location:  r.LocationResult,
			Extension: map[string]interface{}{
				"time_result":     r.TimeResult,
				"location_result": r.LocationResult,
			},
		}
		if r.TimeResult == "Late" || r.TimeResult == "Early" || r.TimeResult == "NotSigned" {
			abnormalType := "迟到"
			if r.TimeResult == "Early" {
				abnormalType = "早退"
			} else if r.TimeResult == "NotSigned" {
				abnormalType = "缺勤"
			}
			record.Extension["abnormal_type"] = abnormalType
		}

		database.DB.Create(record)
		count++
	}

	syncService.UpdateSyncStatus("attendance", "success", fmt.Sprintf("同步 %d 条考勤记录", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"sync_status": gin.H{
				"count":      count,
				"status":     "success",
				"sync_time":  time.Now(),
				"start_date": req.StartDate,
				"end_date":   req.EndDate,
			},
		},
	})
}

// ExportAttendance 导出考勤数据
func ExportAttendance(c *gin.Context) {
	var req struct {
		StartDate    string `json:"start_date" binding:"required"`
		EndDate      string `json:"end_date" binding:"required"`
		UserID       string `json:"user_id"`
		DepartmentID string `json:"department_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误：开始日期和结束日期不能为空",
		})
		return
	}

	// 获取当前用户信息
	userID, _ := c.Get("userID")
	userName, _ := c.Get("userName")
	uid, _ := userID.(string)
	uname, _ := userName.(string)

	fileName := fmt.Sprintf("attendance_%s_%s.xlsx", req.StartDate, req.EndDate)
	export := &database.AttendanceExport{
		UserID:    uid,
		UserName:  uname,
		FileName:  fileName,
		Status:    "pending",
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	attendanceService := service.NewAttendanceService(database.DB)
	if err := attendanceService.CreateExport(export); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建导出任务失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"export_id":    export.ID,
			"file_name":    export.FileName,
			"record_count": 0,
			"status":       export.Status,
			"created_at":   export.CreatedAt,
		},
	})
}

// GetAttendanceExports 获取导出记录列表
func GetAttendanceExports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	attendanceService := service.NewAttendanceService(database.DB)
	exports, total, err := attendanceService.GetExports(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取导出记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": exports,
			"total": total,
		},
	})
}

// GetLastSyncTime 获取最近同步时间
func GetLastSyncTime(c *gin.Context) {
	attendanceService := service.NewAttendanceService(database.DB)
	status, err := attendanceService.GetLastSyncTime()
	if err != nil {
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"attendance": gin.H{
					"last_sync_time": nil,
					"status":         "never",
					"record_count":   0,
				},
			},
		})
		return
	}

	var count int64
	database.DB.Model(&database.Attendance{}).Count(&count)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"attendance": gin.H{
				"last_sync_time": status.LastSyncTime,
				"status":         status.Status,
				"record_count":   count,
			},
		},
	})
}

// GetApprovalTemplates 获取审批模板列表
func GetApprovalTemplates(c *gin.Context) {
	approvalService := service.NewApprovalService(database.DB)
	templates, total, err := approvalService.GetTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审批模板失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": templates,
			"total": total,
		},
	})
}

// GetApprovalInstances 获取审批实例列表
func GetApprovalInstances(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"status":       c.Query("status"),
		"template_id":  c.Query("template_id"),
		"applicant_id": c.Query("applicant_id"),
		"start_date":   c.Query("start_date"),
		"end_date":     c.Query("end_date"),
	}

	approvalService := service.NewApprovalService(database.DB)
	instances, total, err := approvalService.GetInstances(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审批实例失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": instances,
			"total": total,
		},
	})
}

// GetApproval 获取审批详情
func GetApproval(c *gin.Context) {
	id := c.Param("id")

	approvalService := service.NewApprovalService(database.DB)
	approval, err := approvalService.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "审批不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"approval": approval},
	})
}

// SyncApproval 同步审批数据
func SyncApproval(c *gin.Context) {
	var req struct {
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		ProcessCode string `json:"process_code"`
	}
	c.ShouldBindJSON(&req)

	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
	}

	syncService := service.NewSyncService(database.DB)

	if req.ProcessCode == "" {
		// 没有指定审批模板代码，只更新同步状态
		syncService.UpdateSyncStatus("approvals", "success", "请指定 process_code 以同步具体审批流程")
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"sync_status": gin.H{
					"count":   0,
					"status":  "success",
					"message": "请在请求中提供 process_code 参数",
				},
			},
		})
		return
	}

	instances, err := dingtalk.GetApprovals(req.ProcessCode, req.StartDate, req.EndDate)
	if err != nil {
		syncService.UpdateSyncStatus("approvals", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步审批失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	count := 0
	for _, inst := range instances {
		createTime, _ := time.Parse("2006-01-02 15:04:05", inst.CreateTime)
		finishTime, _ := time.Parse("2006-01-02 15:04:05", inst.FinishTime)

		// 将 form_component_values 转为 content map
		content := make(map[string]interface{})
		for _, fv := range inst.FormValues {
			name, _ := fv["name"].(string)
			value, _ := fv["value"].(string)
			if name != "" {
				content[name] = value
			}
		}

		approval := &database.Approval{
			ProcessID:     inst.ProcessInstanceID,
			Title:         inst.Title,
			ApplicantID:   inst.OriginatorUserID,
			ApplicantName: inst.OriginatorUserID,
			Status:        inst.Status,
			CreateTime:    createTime,
			FinishTime:    finishTime,
			Content:       content,
			Extension: map[string]interface{}{
				"result":       inst.Result,
				"process_code": req.ProcessCode,
			},
		}

		// Upsert by process_id
		var existing database.Approval
		if err := database.DB.Where("process_id = ?", inst.ProcessInstanceID).First(&existing).Error; err != nil {
			database.DB.Create(approval)
		} else {
			existing.Status = inst.Status
			existing.FinishTime = finishTime
			existing.Content = content
			database.DB.Save(&existing)
		}
		count++
	}

	syncService.UpdateSyncStatus("approvals", "success", fmt.Sprintf("同步 %d 个审批实例", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"sync_status": gin.H{
				"count":      count,
				"status":     "success",
				"sync_time":  time.Now(),
				"start_date": req.StartDate,
				"end_date":   req.EndDate,
			},
		},
	})
}

// GetRoles 获取角色列表
func GetRoles(c *gin.Context) {
	permService := service.NewPermissionService(database.DB)
	roles, total, err := permService.GetRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取角色列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": roles,
			"total": total,
		},
	})
}

// CreateRole 创建角色
func CreateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	role := &database.Role{
		Name:        req.Name,
		Description: req.Description,
	}

	permService := service.NewPermissionService(database.DB)
	if err := permService.CreateRole(role); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建角色失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"role": role},
	})
}

// GetPermissions 获取权限列表
func GetPermissions(c *gin.Context) {
	permService := service.NewPermissionService(database.DB)
	permissions, total, err := permService.GetPermissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取权限列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": permissions,
			"total": total,
		},
	})
}

// GetAuditLogs 获取审计日志
func GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"user_id":    c.Query("user_id"),
		"start_date": c.Query("start_date"),
		"end_date":   c.Query("end_date"),
	}

	auditService := service.NewAuditService(database.DB)
	logs, total, err := auditService.GetLogs(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审计日志失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": logs,
			"total": total,
		},
	})
}

// GetJobs 获取任务列表
func GetJobs(c *gin.Context) {
	// 任务列表基于同步状态表动态生成
	syncService := service.NewSyncService(database.DB)

	jobs := []gin.H{
		{"id": "1", "name": "同步用户数据", "description": "从钉钉同步用户数据", "type": "sync_users", "status": "idle"},
		{"id": "2", "name": "同步部门数据", "description": "从钉钉同步部门数据", "type": "sync_departments", "status": "idle"},
		{"id": "3", "name": "同步考勤数据", "description": "从钉钉同步考勤数据", "type": "sync_attendance", "status": "idle"},
	}

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	for i, job := range jobs {
		syncType := typeMap[job["id"].(string)]
		if status, err := syncService.GetSyncStatus(syncType); err == nil {
			jobs[i]["last_run_time"] = status.LastSyncTime
			jobs[i]["status"] = status.Status
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": jobs,
			"total": len(jobs),
		},
	})
}

// RunJob 运行任务
func RunJob(c *gin.Context) {
	id := c.Param("id")

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	syncType, ok := typeMap[id]
	if !ok {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "任务不存在",
		})
		return
	}

	syncService := service.NewSyncService(database.DB)
	syncService.UpdateSyncStatus(syncType, "success", "手动执行任务")

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"job": gin.H{
				"id":         id,
				"status":     "completed",
				"start_time": time.Now(),
			},
		},
	})
}

// 员工档案中心接口

// GetEmployeeProfiles 获取员工档案列表
func GetEmployeeProfiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"department_id": c.Query("department_id"),
		"status":        c.Query("status"),
	}

	employeeService := service.NewEmployeeService(database.DB)
	profiles, total, err := employeeService.GetProfiles(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": profiles,
			"total": total,
		},
	})
}

// GetEmployeeProfile 获取员工档案详情
func GetEmployeeProfile(c *gin.Context) {
	id := c.Param("id")

	employeeService := service.NewEmployeeService(database.DB)
	profile, err := employeeService.GetProfileByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "档案不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// CreateEmployeeProfile 创建员工档案
func CreateEmployeeProfile(c *gin.Context) {
	var profile database.EmployeeProfile

	if err := c.ShouldBindJSON(&profile); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if profile.ProfileStatus == "" {
		profile.ProfileStatus = "active"
	}

	employeeService := service.NewEmployeeService(database.DB)
	if err := employeeService.CreateProfile(&profile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// UpdateEmployeeProfile 更新员工档案
func UpdateEmployeeProfile(c *gin.Context) {
	id := c.Param("id")

	employeeService := service.NewEmployeeService(database.DB)
	profile, err := employeeService.GetProfileByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "档案不存在",
		})
		return
	}

	if err := c.ShouldBindJSON(profile); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if err := employeeService.UpdateProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// GetTransfers 获取调动记录列表
func GetTransfers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")

	employeeService := service.NewEmployeeService(database.DB)
	transfers, total, err := employeeService.GetTransfers(page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取调动记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": transfers, "total": total},
	})
}

// CreateTransfer 创建调动记录
func CreateTransfer(c *gin.Context) {
	var transfer database.EmployeeTransfer
	if err := c.ShouldBindJSON(&transfer); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if transfer.Status == "" {
		transfer.Status = "pending"
	}

	employeeService := service.NewEmployeeService(database.DB)
	if err := employeeService.CreateTransfer(&transfer); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建调动记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"transfer": transfer},
	})
}

// GetResignations 获取离职记录列表
func GetResignations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")

	employeeService := service.NewEmployeeService(database.DB)
	resignations, total, err := employeeService.GetResignations(page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取离职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": resignations, "total": total},
	})
}

// CreateResignation 创建离职记录
func CreateResignation(c *gin.Context) {
	var resignation database.EmployeeResignation
	if err := c.ShouldBindJSON(&resignation); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if resignation.Status == "" {
		resignation.Status = "pending"
	}

	employeeService := service.NewEmployeeService(database.DB)
	if err := employeeService.CreateResignation(&resignation); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建离职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"resignation": resignation},
	})
}

// GetOnboardings 获取入职记录列表
func GetOnboardings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	status := c.Query("status")

	employeeService := service.NewEmployeeService(database.DB)
	onboardings, total, err := employeeService.GetOnboardings(page, pageSize, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取入职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": onboardings, "total": total},
	})
}

// CreateOnboarding 创建入职记录
func CreateOnboarding(c *gin.Context) {
	var onboarding database.EmployeeOnboarding
	if err := c.ShouldBindJSON(&onboarding); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if onboarding.Status == "" {
		onboarding.Status = "pending"
	}

	employeeService := service.NewEmployeeService(database.DB)
	if err := employeeService.CreateOnboarding(&onboarding); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建入职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"onboarding": onboarding},
	})
}

// GetTalentAnalysisList 获取人才分析列表
func GetTalentAnalysisList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")

	talentService := service.NewTalentService(database.DB)
	analyses, total, err := talentService.GetList(page, pageSize, departmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取人才分析失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": analyses,
			"total": total,
		},
	})
}

// GetTalentAnalysisDetail 获取人才分析详情
func GetTalentAnalysisDetail(c *gin.Context) {
	id := c.Param("id")

	talentService := service.NewTalentService(database.DB)
	analysis, err := talentService.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "分析记录不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"analysis": analysis},
	})
}

// CreateTalentAnalysis 创建人才分析
func CreateTalentAnalysis(c *gin.Context) {
	var analysis database.TalentAnalysis
	if err := c.ShouldBindJSON(&analysis); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	talentService := service.NewTalentService(database.DB)
	if err := talentService.Create(&analysis); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建分析记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"analysis": analysis},
	})
}
