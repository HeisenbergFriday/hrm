package api

import (
	"peopleops/internal/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	router := gin.Default()

	// 配置CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 健康检查
	router.GET("/health", HealthCheck)

	// API v1 路由组
	v1 := router.Group("/api/v1")
	{
		// 认证相关
		auth := v1.Group("/auth")
		{
			auth.POST("/login", Login)
			auth.POST("/logout", Logout)
			auth.GET("/me", middleware.JWTAuth(), GetCurrentUser)

			// 钉钉登录相关
			dingtalk := auth.Group("/dingtalk")
			{
				dingtalk.GET("/qr/start", DingTalkQRLoginStart)
				dingtalk.POST("/in-app", DingTalkInAppLogin)
				dingtalk.GET("/callback", DingTalkCallback)
				dingtalk.GET("/config", GetDingTalkConfig)
			}
		}

		// 需要认证的路由
		authRequired := v1.Group("/")
		authRequired.Use(middleware.JWTAuth())
		{
			// 用户相关
			users := authRequired.Group("/users")
			{
				users.GET("", GetUsers)
				users.GET("/:id", GetUser)
				users.PUT("/:id", UpdateUser)
			}

			// 部门相关
			departments := authRequired.Group("/departments")
			{
				departments.GET("", GetDepartments)
				departments.GET("/:id", GetDepartment)
			}

			// 同步相关
			sync := authRequired.Group("/sync")
			{
				sync.POST("/departments", SyncDepartments)
				sync.POST("/users", SyncUsers)
				sync.GET("/status", GetSyncStatus)
			}

			// 组织与员工模块
			org := authRequired.Group("/org")
			{
				// 部门树
				org.GET("/departments/tree", GetDepartmentTree)

				// 员工相关
				org.GET("/employees", GetEmployees)
				org.GET("/employees/:id", GetEmployee)

				// 同步
				org.POST("/sync", SyncOrgData)
			}

			// 考勤模块
			attendance := authRequired.Group("/attendance")
			{
				attendance.GET("/records", GetAttendanceRecords)
				attendance.GET("/stats", GetAttendanceStats)
				attendance.POST("/sync", SyncAttendance)
				attendance.POST("/export", ExportAttendance)
				attendance.GET("/exports", GetAttendanceExports)
				attendance.GET("/last-sync", GetLastSyncTime)
			}

			// 审批模块
			approvals := authRequired.Group("/approvals")
			{
				approvals.GET("/templates", GetApprovalTemplates)
				approvals.GET("/instances", GetApprovalInstances)
				approvals.GET("/:id", GetApproval)
				approvals.POST("/sync", SyncApproval)
			}

			// 权限管理模块
			permission := authRequired.Group("/permission")
			{
				// 角色管理
				permission.GET("/roles", GetRoles)
				permission.POST("/roles", CreateRole)
				// 权限管理
				permission.GET("/permissions", GetPermissions)
			}

			// 审计日志模块
			audit := authRequired.Group("/audit")
			{
				audit.GET("/logs", GetAuditLogs)
			}

			// 任务中心模块
			jobs := authRequired.Group("/jobs")
			{
				jobs.GET("", GetJobs)
				jobs.POST("/:id/run", RunJob)
			}

			// 员工档案中心模块
			employee := authRequired.Group("/employee")
			{
				// 员工档案
				employee.GET("/profiles", GetEmployeeProfiles)
				employee.GET("/profiles/:id", GetEmployeeProfile)
				employee.POST("/profiles", CreateEmployeeProfile)
				employee.PUT("/profiles/:id", UpdateEmployeeProfile)

				// 人事流程
				employee.GET("/transfers", GetTransfers)
				employee.POST("/transfers", CreateTransfer)
				employee.GET("/resignations", GetResignations)
				employee.POST("/resignations", CreateResignation)
				employee.GET("/onboardings", GetOnboardings)
				employee.POST("/onboardings", CreateOnboarding)
			}

			// 人才分析模块
			talent := authRequired.Group("/talent")
			{
				talent.GET("/analysis", GetTalentAnalysisList)
				talent.GET("/analysis/:id", GetTalentAnalysisDetail)
				talent.POST("/analysis", CreateTalentAnalysis)
			}
		}
	}

	return router
}
