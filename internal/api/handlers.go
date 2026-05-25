package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
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

func applyDingTalkProfileFields(profile *database.EmployeeProfile, user dingtalk.UserInfo, status string) {
	profile.WorkEmail = user.Email
	profile.ProfileStatus = status
	if user.HiredDate != "" {
		profile.EntryDate = user.HiredDate
	}
	if user.PlannedRegularDate != "" {
		profile.PlannedRegularDate = user.PlannedRegularDate
	}
	if user.ActualRegularDate != "" {
		profile.ActualRegularDate = user.ActualRegularDate
		profile.ProbationEndDate = user.ActualRegularDate
	}
}

// HealthCheck 健康检查

func resolveOrgScope(c *gin.Context) (*service.OrgDataScope, error) {
	currentUserID, _ := c.Get("userID")
	orgService := service.NewOrgService(database.DB)
	return orgService.ResolveScopeForUser(fmt.Sprint(currentUserID))
}

func respondOrgAccessDenied(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "当前账号无权访问该组织数据",
	})
}

func dingtalkDepartmentsToOrgSyncItems(depts []dingtalk.DeptInfo) []service.OrgDepartmentSyncItem {
	items := make([]service.OrgDepartmentSyncItem, 0, len(depts))
	for _, d := range depts {
		items = append(items, service.OrgDepartmentSyncItem{
			DepartmentID: fmt.Sprintf("%d", d.DeptID),
			Name:         d.Name,
			ParentID:     fmt.Sprintf("%d", d.ParentID),
		})
	}
	return items
}

func createOperationAuditLog(c *gin.Context, operation, resource string, details map[string]interface{}) {
	userID := fmt.Sprint(c.GetString("userID"))
	if userID == "" {
		if value, ok := c.Get("userID"); ok {
			userID = fmt.Sprint(value)
		}
	}

	userName := strings.TrimSpace(c.GetString("userName"))
	if userName == "" {
		if value, ok := c.Get("userName"); ok {
			userName = fmt.Sprint(value)
		}
	}
	if userID == "" {
		userID = "system"
	}
	if userName == "" {
		userName = "system"
	}

	auditService := service.NewAuditService(database.DB)
	_ = auditService.CreateLog(&database.OperationLog{
		UserID:    userID,
		UserName:  userName,
		Operation: operation,
		Resource:  resource,
		IP:        c.ClientIP(),
		Details:   details,
	})
}

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
	employeeService := service.NewEmployeeService(database.DB)
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

		profile, profileErr := employeeService.GetProfileByUserID(u.UserID)
		if profileErr != nil {
			profile := &database.EmployeeProfile{
				UserID:     u.UserID,
				EmployeeID: u.UserID,
			}
			applyDingTalkProfileFields(profile, u, status)
			employeeService.CreateProfile(profile)
		} else {
			applyDingTalkProfileFields(profile, u, status)
			employeeService.UpdateProfile(profile)
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
	corpID := dingtalk.GetCorpID()
	appHomeURL := resolveDingTalkAppHomeURL(c)
	redirectURI := resolveDingTalkRedirectURI(c)
	missingConfig := []string{}
	if corpID == "" {
		missingConfig = append(missingConfig, "DINGTALK_CORP_ID")
	}
	log.Printf("[dingtalk/config] host=%s app_home_url=%s redirect_uri=%s missing=%v", c.Request.Host, appHomeURL, redirectURI, missingConfig)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"corp_id":      corpID,
			"client_id":    os.Getenv("DINGTALK_APP_KEY"),
			"redirect_uri": redirectURI,
			"app_home_url": appHomeURL,
			"missing":      missingConfig,
		},
	})
}

// DingTalkQRLoginStart 閽夐拤鎵爜鐧诲綍寮€濮?
func DingTalkQRLoginStart(c *gin.Context) {
	state := "test_state"
	redirectURI := resolveDingTalkRedirectURI(c)
	log.Printf("[dingtalk/qr/start] host=%s forwarded_host=%s redirect_uri=%s ua=%s", c.Request.Host, c.GetHeader("X-Forwarded-Host"), redirectURI, c.GetHeader("User-Agent"))

	qrCodeURL, err := dingtalk.GetQRCodeWithRedirect(state, redirectURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get qrcode failed",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"qr_code_url":  qrCodeURL,
			"state":        state,
			"redirect_uri": redirectURI,
		},
	})
}

// DingTalkInAppLogin 閽夐拤鍐呭厤鐧?
func DingTalkInAppLogin(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}
	log.Printf("[dingtalk/in-app] host=%s has_code=%t ua=%s", c.Request.Host, strings.TrimSpace(req.Code) != "", c.GetHeader("User-Agent"))

	userid, err := dingtalk.GetUserIDByInAppCode(req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "dingtalk in-app login failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] resolved_userid=%s", userid)

	userDetail, err := dingtalk.GetUserDetailByUserID(userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user detail failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] user_detail=%v", userDetail)

	name, _ := userDetail["name"].(string)
	email, _ := userDetail["email"].(string)
	mobile, _ := userDetail["mobile"].(string)
	avatar, _ := userDetail["avatar"].(string)
	position, _ := userDetail["title"].(string)
	deptID := "1"
	if deptList, ok := userDetail["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
		if id, ok := deptList[0].(float64); ok {
			deptID = fmt.Sprintf("%d", int64(id))
		}
	}

	userService := service.NewUserService(database.DB)
	user, err := findLocalUserByDingTalkIdentity(userService, userid)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}

		user, err = findLocalUserByContact(userService, email, mobile)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondDingTalkUserNotSynced(c, "dingtalk_in_app", userid, name)
				return
			}
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.DepartmentID = deptID
	user.Status = "active"
	if err := assignUserEmailSafely(userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafely(userService, user, mobile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user mobile failed: " + err.Error(),
		})
		return
	}
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user failed: " + err.Error(),
		})
		return
	}

	tokenString, expiresAt, err := generateToken(fmt.Sprintf("%d", user.ID), user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		UserID:      fmt.Sprintf("%d", user.ID),
		UserName:    user.Name,
		LoginType:   "dingtalk_in_app",
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

// DingTalkCallback 閽夐拤鍥炶皟
func DingTalkCallback(c *gin.Context) {
	code := c.Query("authCode")
	if code == "" {
		code = c.Query("code")
	}
	log.Printf("[dingtalk/callback] host=%s raw_query=%s has_code=%t ua=%s", c.Request.Host, c.Request.URL.RawQuery, code != "", c.GetHeader("User-Agent"))

	if code == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "missing auth code",
		})
		return
	}

	userInfo, err := dingtalk.GetUserInfoByCode(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user info failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/callback] user_info=%v", userInfo)

	associatedUserID := getStringByKeys(userInfo, "associated_user_id", "associatedUserId", "userid", "userId")
	unionID := getStringByKeys(userInfo, "unionId", "unionid", "union_id")
	openID := getStringByKeys(userInfo, "openId", "openid", "open_id")
	dtUserID := associatedUserID
	if dtUserID == "" && unionID != "" {
		resolvedUserID, resolveErr := dingtalk.GetUserIDByUnionID(unionID)
		if resolveErr == nil {
			dtUserID = resolvedUserID
		} else {
			log.Printf("[dingtalk/callback] resolve unionid failed: union_id=%s err=%v", unionID, resolveErr)
		}
	}

	if dtUserID == "" && openID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "missing dingtalk user identity",
		})
		return
	}

	var name, email, mobile, avatar, position string
	deptID := "1"
	if dtUserID != "" {
		userDetail, detailErr := dingtalk.GetUserDetailByUserID(dtUserID)
		if detailErr == nil {
			name, _ = userDetail["name"].(string)
			email, _ = userDetail["email"].(string)
			mobile, _ = userDetail["mobile"].(string)
			avatar, _ = userDetail["avatar"].(string)
			position, _ = userDetail["title"].(string)
			if deptList, ok := userDetail["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
				if id, ok := deptList[0].(float64); ok {
					deptID = fmt.Sprintf("%d", int64(id))
				}
			}
		}
	}
	if name == "" {
		name, _ = userInfo["nick"].(string)
	}
	if email == "" {
		email, _ = userInfo["email"].(string)
	}
	if mobile == "" {
		mobile, _ = userInfo["mobile"].(string)
	}
	if avatar == "" {
		avatar, _ = userInfo["avatarUrl"].(string)
	}

	userService := service.NewUserService(database.DB)
	user, err := findLocalUserByDingTalkIdentity(userService, dtUserID, associatedUserID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}

		user, err = findLocalUserByContact(userService, email, mobile)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				identityForLog := dtUserID
				if identityForLog == "" {
					identityForLog = openID
				}
				if identityForLog == "" {
					identityForLog = unionID
				}
				respondDingTalkUserNotSynced(c, "dingtalk_qr", identityForLog, name)
				return
			}
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.DepartmentID = deptID
	user.Status = "active"
	if err := assignUserEmailSafely(userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafely(userService, user, mobile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user mobile failed: " + err.Error(),
		})
		return
	}
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user failed: " + err.Error(),
		})
		return
	}

	tokenString, expiresAt, err := generateToken(fmt.Sprintf("%d", user.ID), user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

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

func respondDingTalkUserNotSynced(c *gin.Context, loginType, userID, userName string) {
	database.DB.Create(&database.LoginLog{
		UserID:      userID,
		UserName:    userName,
		LoginType:   loginType,
		LoginStatus: "failed",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		ErrorMsg:    "user not synced",
	})

	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "dingtalk user not synced, please sync org data first",
	})
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.Split(forwardedProto, ",")[0]
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return dingtalk.GetAppHomeURL()
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func getStringByKeys(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findLocalUserByDingTalkIdentity(userService *service.UserService, candidates ...string) (*database.User, error) {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		user, err := userService.GetUserByUserID(candidate)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func findLocalUserByContact(userService *service.UserService, email, mobile string) (*database.User, error) {
	email = strings.TrimSpace(email)
	if email != "" {
		user, err := userService.GetUserByEmail(email)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	mobile = strings.TrimSpace(mobile)
	if mobile != "" {
		user, err := userService.GetUserByMobile(mobile)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func assignUserEmailSafely(userService *service.UserService, user *database.User, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}

	existing, err := userService.GetUserByEmail(email)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip email update for user_id=%s because email=%s already belongs to user_id=%s", user.UserID, email, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Email = email
	return nil
}

func assignUserMobileSafely(userService *service.UserService, user *database.User, mobile string) error {
	mobile = strings.TrimSpace(mobile)
	if mobile == "" {
		return nil
	}

	existing, err := userService.GetUserByMobile(mobile)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip mobile update for user_id=%s because mobile=%s already belongs to user_id=%s", user.UserID, mobile, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Mobile = mobile
	return nil
}

func resolveDingTalkAppHomeURL(c *gin.Context) string {
	if configured := dingtalk.GetConfiguredAppHomeURL(); configured != "" {
		return configured
	}
	return requestBaseURL(c)
}

func resolveDingTalkRedirectURI(c *gin.Context) string {
	if configured := dingtalk.GetConfiguredRedirectURI(); configured != "" {
		return configured
	}
	return resolveDingTalkAppHomeURL(c) + "/callback"
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

func GetOrgOverview(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgService(database.DB)
	overview, err := orgService.GetOverview(scope, c.Query("department_id"))
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织概览失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"overview": overview},
	})
}

func GetScopedDepartments(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgService(database.DB)
	departments, err := orgService.GetVisibleDepartments(scope)
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
		Data: gin.H{
			"departments": departments,
			"scope":       scope,
		},
	})
}

func GetOrgDepartmentTree(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgService(database.DB)
	tree, err := orgService.GetDepartmentTree(scope)
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门树失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"tree":  tree,
			"scope": scope,
		},
	})
}

func GetOrgDepartmentHistory(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	orgService := service.NewOrgService(database.DB)
	logs, err := orgService.GetDepartmentHistory(scope, c.Param("id"), limit)
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门变更历史失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": logs,
			"total": len(logs),
		},
	})
}

func GetOrgEmployees(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgService(database.DB)
	users, total, err := orgService.ListEmployees(scope, page, pageSize, service.OrgEmployeeFilters{
		DepartmentID: c.Query("department_id"),
		Search:       c.Query("search"),
		Status:       c.Query("status"),
	})
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": users,
			"total": total,
			"scope": scope,
		},
	})
}

func GetOrgEmployeeDetail(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgService(database.DB)
	detail, err := orgService.GetEmployeeAggregate(scope, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrgAccessDenied):
			respondOrgAccessDenied(c)
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "员工不存在",
			})
		default:
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取员工详情失败",
				Data:    gin.H{"error": err.Error()},
			})
		}
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"detail": detail},
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
		users, total, err = userService.GetSyncedEmployeesByDepartment(departmentID, page, pageSize)
	} else {
		users, total, err = userService.GetSyncedEmployees(page, pageSize)
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

	// 一并返回员工档案（按 user_id 查），避免前端再发请求
	employeeService := service.NewEmployeeService(database.DB)
	profile, _ := employeeService.GetProfileByUserID(user.UserID)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"employee": user, "profile": profile},
	})
}

// SyncOrgData 同步组织数据
func SyncOrgData(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	// 同步部门
	depts, deptErr := dingtalk.SyncDepartments()
	deptCount := 0
	deptStatus := "success"
	deptErrMsg := ""
	if deptErr != nil {
		deptStatus = "failed"
		deptErrMsg = deptErr.Error()
		log.Printf("[SyncOrgData] 部门同步失败: %v", deptErr)
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

	// 同步用户（复用已有部门列表，避免重复调用 SyncDepartments）
	users, userErr := dingtalk.SyncUsersWithDepts(depts)
	userCount := 0
	userStatus := "success"
	userErrMsg := ""
	if userErr != nil {
		userStatus = "failed"
		userErrMsg = userErr.Error()
		log.Printf("[SyncOrgData] 用户同步失败: %v", userErr)
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
				profile := &database.EmployeeProfile{
					UserID:     u.UserID,
					EmployeeID: u.UserID,
				}
				applyDingTalkProfileFields(profile, u, status)
				employeeService.CreateProfile(profile)
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
				profile, profileErr := employeeService.GetProfileByUserID(u.UserID)
				if profileErr != nil {
					// 创建员工档案
					profile := &database.EmployeeProfile{
						UserID:     u.UserID,
						EmployeeID: u.UserID,
					}
					applyDingTalkProfileFields(profile, u, status)
					employeeService.CreateProfile(profile)
				} else {
					// 更新员工档案：始终同步入职日期（若钉钉有值则覆盖）
					applyDingTalkProfileFields(profile, u, status)
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
				"departments": gin.H{"count": deptCount, "status": deptStatus, "error": deptErrMsg},
				"employees":   gin.H{"count": userCount, "status": userStatus, "error": userErrMsg},
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
		Force     bool   `json:"force"` // true 时先删除该范围内旧记录再重新拉取
	}
	c.ShouldBindJSON(&req)

	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
	}

	if req.Force {
		cst := time.FixedZone("CST", 8*3600)
		start, err1 := time.ParseInLocation("2006-01-02", req.StartDate, cst)
		end, err2 := time.ParseInLocation("2006-01-02", req.EndDate, cst)
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "日期格式错误"})
			return
		}
		end = end.AddDate(0, 0, 1) // 包含 end 当天
		if err := database.DB.Where("check_time >= ? AND check_time < ?", start, end).Delete(&database.Attendance{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "清理旧记录失败: " + err.Error()})
			return
		}
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
		checkTime, _ := time.ParseInLocation("2006-01-02 15:04:05", r.UserCheckTime, time.FixedZone("CST", 8*3600))

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

		if err := service.NewAttendanceService(database.DB).SaveRecord(record); err != nil {
			syncService.UpdateSyncStatus("attendance", "failed", err.Error())
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鍚屾鑰冨嫟澶辫触: " + err.Error(),
			})
			return
		}
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
func GetEmployeeLifecycleLedger(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filters := map[string]string{
		"department_id": c.Query("department_id"),
		"status":        c.Query("status"),
		"keyword":       strings.TrimSpace(c.Query("keyword")),
	}

	employeeService := service.NewEmployeeService(database.DB)
	items, total, err := employeeService.GetLifecycleLedger(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取入转调离台账失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items, "total": total},
	})
}

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

// ===================== 大小周管理 =====================

// GetWeekScheduleRules 获取所有大小周规则
func GetWeekScheduleRules(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
	rules, err := svc.GetAllRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取规则列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": rules},
	})
}

// CreateWeekScheduleRule 创建大小周规则
func CreateWeekScheduleRule(c *gin.Context) {
	var rule database.WeekScheduleRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.CreateRule(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建规则失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"rule": rule},
	})
}

// UpdateWeekScheduleRule 更新大小周规则
func UpdateWeekScheduleRule(c *gin.Context) {
	idStr := c.Param("id")
	svc := service.NewWeekScheduleService(database.DB)

	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	existing, err := svc.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "规则不存在",
		})
		return
	}

	var input database.WeekScheduleRule
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if input.ScopeType != "" {
		existing.ScopeType = input.ScopeType
	}
	if input.ScopeID != "" || input.ScopeType == "company" {
		existing.ScopeID = input.ScopeID
	}
	if input.ScopeName != "" {
		existing.ScopeName = input.ScopeName
	}
	if input.BaseDate != "" {
		existing.BaseDate = input.BaseDate
	}
	if input.Pattern != "" {
		existing.Pattern = input.Pattern
	}
	if input.Status != "" {
		existing.Status = input.Status
	}

	if err := svc.UpdateRule(existing); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新规则失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"rule": existing},
	})
}

// DeleteWeekScheduleRule 删除大小周规则
func DeleteWeekScheduleRule(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除规则失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// BatchSetWeekScheduleRules 批量为员工设置大小周规则
func BatchSetWeekScheduleRules(c *gin.Context) {
	var input service.BatchSetUserRulesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	if len(input.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请选择至少一个员工",
		})
		return
	}

	if input.BaseDate == "" || input.Pattern == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "base_date 和 pattern 不能为空",
		})
		return
	}

	if input.ConflictMode == "" {
		input.ConflictMode = "skip"
	}

	var users []database.User
	if err := database.DB.Where("user_id IN ?", input.UserIDs).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "查询用户信息失败",
		})
		return
	}

	userMap := make(map[string]database.User, len(users))
	for _, u := range users {
		userMap[u.UserID] = u
	}

	svc := service.NewWeekScheduleService(database.DB)
	result, err := svc.BatchSetUserRules(&input, userMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "批量设置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// GetDingTalkShifts 获取钉钉班次列表
func GetDingTalkShifts(c *gin.Context) {
	shifts, err := dingtalk.GetShiftList()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取班次列表失败: " + err.Error(),
		})
		return
	}

	type ShiftItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	var items []ShiftItem
	for _, shift := range shifts {
		if idVal, ok := shift["id"].(float64); ok && int64(idVal) > 0 {
			name, _ := shift["name"].(string)
			items = append(items, ShiftItem{
				ID:   int64(idVal),
				Name: name,
			})
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

// DebugAttendanceGroups 返回所有考勤组及其班次详情，用于诊断休息班次 ID
func DebugAttendanceGroups(c *gin.Context) {
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "未配置 DINGTALK_ADMIN_USER_ID",
		})
		return
	}

	groups, err := dingtalk.GetAttendanceGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤组失败: " + err.Error(),
		})
		return
	}

	shifts, _ := dingtalk.GetShiftList()
	shiftNameMap := make(map[int64]string, len(shifts))
	for _, s := range shifts {
		if id, ok := s["id"].(float64); ok && id > 0 {
			name, _ := s["name"].(string)
			shiftNameMap[int64(id)] = name
		}
	}

	type GroupInfo struct {
		GroupID   interface{} `json:"group_id"`
		GroupName interface{} `json:"group_name"`
		GroupType interface{} `json:"group_type"`
		ShiftIDs  []int64     `json:"shift_ids"`
		Shifts    []gin.H     `json:"shifts"`
		RawKeys   []string    `json:"raw_keys"`
	}

	result := make([]GroupInfo, 0, len(groups))
	for _, g := range groups {
		gid, _ := g["group_id"].(float64)
		info := GroupInfo{
			GroupID:   g["group_id"],
			GroupName: g["group_name"],
			GroupType: g["group_type"],
			RawKeys:   make([]string, 0, len(g)),
		}
		for k := range g {
			info.RawKeys = append(info.RawKeys, k)
		}

		detail, detailErr := dingtalk.GetAttendanceGroup(opUserID, int64(gid))
		if detailErr == nil {
			shiftIDs := dingtalk.CollectAttendanceGroupShiftIDs(detail)
			info.ShiftIDs = make([]int64, 0, len(shiftIDs))
			info.Shifts = make([]gin.H, 0, len(shiftIDs))
			for sid := range shiftIDs {
				info.ShiftIDs = append(info.ShiftIDs, sid)
				info.Shifts = append(info.Shifts, gin.H{
					"shift_id":   sid,
					"shift_name": shiftNameMap[sid],
				})
			}
			restID := dingtalk.GetAttendanceGroupRestClassID(detail)
			info.RawKeys = append(info.RawKeys, fmt.Sprintf("detected_rest_shift_id=%d", restID))
		}
		result = append(result, info)
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"groups": result, "all_shifts": shifts},
	})
}

// CreateDingTalkShift 在钉钉创建新班次
func CreateDingTalkShift(c *gin.Context) {
	var input struct {
		Name         string `json:"name" binding:"required"`
		CheckInTime  string `json:"check_in_time" binding:"required"`
		CheckOutTime string `json:"check_out_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "未配置 DINGTALK_ADMIN_USER_ID",
		})
		return
	}

	shiftID, err := dingtalk.CreateShift(opUserID, input.Name, input.CheckInTime, input.CheckOutTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"id": shiftID, "name": input.Name},
	})
}
func GetWeekCalendar(c *gin.Context) {
	userID := c.Query("user_id")
	departmentID := c.Query("department_id")
	weeksStr := c.DefaultQuery("weeks", "8")
	startDate := c.Query("start_date")

	var weeks int
	fmt.Sscanf(weeksStr, "%d", &weeks)
	if weeks <= 0 {
		weeks = 8
	}

	svc := service.NewWeekScheduleService(database.DB)
	calendar, err := svc.GetWeekCalendar(userID, departmentID, weeks, startDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取日历失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": calendar},
	})
}

// SetWeekOverride 手动设置某周为大周/小周
func SetWeekOverride(c *gin.Context) {
	var override database.WeekScheduleOverride
	if err := c.ShouldBindJSON(&override); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.SetOverride(&override); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "设置覆盖失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"override": override},
	})
}

// DeleteWeekOverride 取消手动覆盖
func DeleteWeekOverride(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.DeleteOverride(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除覆盖失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// SyncWeekToDingTalk 将大小周配置推送到钉钉
func SyncWeekToDingTalk(c *gin.Context) {
	var input struct {
		Weeks int `json:"weeks"`
	}
	c.ShouldBindJSON(&input)
	if input.Weeks <= 0 {
		input.Weeks = 4
	}

	svc := service.NewWeekScheduleService(database.DB)
	result, err := svc.SyncToDingTalk(input.Weeks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步到钉钉失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// SyncWeekFromDingTalk 从钉钉拉取大小周配置
func SyncWeekFromDingTalk(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
	result, err := svc.SyncFromDingTalkConservative()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "从钉钉同步失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// GetWeekSyncLogs 获取大小周同步日志
func GetWeekSyncLogs(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	var page, pageSize int
	fmt.Sscanf(pageStr, "%d", &page)
	fmt.Sscanf(pageSizeStr, "%d", &pageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	svc := service.NewWeekScheduleService(database.DB)
	logs, total, err := svc.GetSyncLogs(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取同步日志失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: PagedResponse{
			Items: logs,
			Total: total,
		},
	})
}

// ===================== 法定节假日管理 =====================

// GetHolidays 获取节假日列表（按年）
func GetHolidays(c *gin.Context) {
	yearStr := c.DefaultQuery("year", fmt.Sprintf("%d", time.Now().Year()))
	var year int
	fmt.Sscanf(yearStr, "%d", &year)
	if year <= 0 {
		year = time.Now().Year()
	}

	svc := service.NewWeekScheduleService(database.DB)
	holidays, err := svc.GetHolidaysByYear(year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取节假日列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": holidays, "year": year},
	})
}

// CreateHoliday 创建单个节假日
func CreateHoliday(c *gin.Context) {
	var holiday database.StatutoryHoliday
	if err := c.ShouldBindJSON(&holiday); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.CreateHoliday(&holiday); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建节假日失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"holiday": holiday},
	})
}

// BatchCreateHolidays 批量创建节假日
func BatchCreateHolidays(c *gin.Context) {
	var input struct {
		Holidays []database.StatutoryHoliday `json:"holidays"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	created, err := svc.BatchCreateHolidays(input.Holidays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "批量创建失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"created": created, "total": len(input.Holidays)},
	})
}

// SyncHolidaysFromJuhe 从聚合数据API同步节假日
func SyncHolidaysFromJuhe(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
	created, err := svc.SyncHolidaysFromJuhe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "从聚合数据同步节假日失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"created": created},
	})
}

// DeleteHoliday 删除节假日
func DeleteHoliday(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.DeleteHoliday(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除节假日失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// ===================== 员工下班时间配置 =====================

// GetShiftConfigs 获取所有员工的下班时间配置（含默认 18:30 的员工）
func GetShiftConfigs(c *gin.Context) {
	svc := service.NewShiftConfigService(database.DB)
	items, err := svc.GetAllWithUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取配置失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

// SetShiftConfigs 批量/单个设置员工下班时间（仅写本地 DB，不调用钉钉 API）
func GetShiftCatalogs(c *gin.Context) {
	svc := service.NewShiftConfigService(database.DB)
	items, err := svc.ListShiftCatalogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "????????????: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

func PreviewShiftConfigs(c *gin.Context) {
	var input service.PreviewShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(database.DB)
	result, err := svc.Preview(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "预览失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

func SetShiftConfigs(c *gin.Context) {
	var input service.SetShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(database.DB)
	count, err := svc.SetConfigs(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "设置失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"updated": count},
	})
}

// DeleteShiftConfig 删除员工自定义下班时间（恢复默认 18:30）
func ApplyShiftConfigs(c *gin.Context) {
	var input service.ApplyShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "??????: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(database.DB)
	result, err := svc.ApplyAndSync(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "???????????: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: result.Message,
		Data:    result,
	})
}

func DeleteShiftConfig(c *gin.Context) {
	userID := c.Param("user_id")
	svc := service.NewShiftConfigService(database.DB)
	if err := svc.DeleteConfig(userID); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// GetOrCreateCustomShift 查找或创建钉钉班次，返回班次 ID
func GetOrCreateCustomShift(c *gin.Context) {
	var input struct {
		Name     string `json:"name" binding:"required"`
		CheckIn  string `json:"check_in" binding:"required"`
		CheckOut string `json:"check_out" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(database.DB)
	shiftID, err := svc.GetOrCreateShift(input.Name, input.CheckIn, input.CheckOut)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取/创建班次失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"shift_id": shiftID},
	})
}

// UploadFile 文件上传
func UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请选择要上传的文件",
		})
		return
	}

	// 限制文件大小 (10MB)
	if file.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "文件大小不能超过10MB",
		})
		return
	}

	// 检查上传目录
	uploadDir := "uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建上传目录失败",
		})
		return
	}

	// 生成唯一文件名
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".bin"
	}
	filename := fmt.Sprintf("%s%s", time.Now().Format("20060102150405"), ext)
	filePath := filepath.Join(uploadDir, filename)

	// 保存文件
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存文件失败",
		})
		return
	}

	// 返回文件URL
	fileURL := fmt.Sprintf("/api/v1/files/%s", filename)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "上传成功",
		Data: gin.H{
			"url":  fileURL,
			"name": file.Filename,
			"size": file.Size,
		},
	})
}

// ServeFile 提供文件访问
func ServeFile(c *gin.Context) {
	filename := c.Param("filename")
	// 防止路径穿越
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的文件名",
		})
		return
	}

	filePath := filepath.Join("uploads", filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "文件不存在",
		})
		return
	}

	c.File(filePath)
}
