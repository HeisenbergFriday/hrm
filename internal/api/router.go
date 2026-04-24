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

			// 大小周管理模块
			weekSchedule := authRequired.Group("/week-schedule")
			{
				weekSchedule.GET("/rules", GetWeekScheduleRules)
				weekSchedule.POST("/rules", CreateWeekScheduleRule)
				weekSchedule.POST("/rules/batch", BatchSetWeekScheduleRules)
				weekSchedule.PUT("/rules/:id", UpdateWeekScheduleRule)
				weekSchedule.DELETE("/rules/:id", DeleteWeekScheduleRule)

				weekSchedule.GET("/shifts", GetDingTalkShifts)
				weekSchedule.POST("/shifts", CreateDingTalkShift)
				weekSchedule.GET("/debug/attendance-groups", DebugAttendanceGroups)

				weekSchedule.GET("/calendar", GetWeekCalendar)

				weekSchedule.POST("/overrides", SetWeekOverride)
				weekSchedule.DELETE("/overrides/:id", DeleteWeekOverride)

				weekSchedule.POST("/sync/to-dingtalk", SyncWeekToDingTalk)
				weekSchedule.POST("/sync/from-dingtalk", SyncWeekFromDingTalk)
				weekSchedule.GET("/sync/logs", GetWeekSyncLogs)

				// 法定节假日
				weekSchedule.GET("/holidays", GetHolidays)
				weekSchedule.POST("/holidays", CreateHoliday)
				weekSchedule.POST("/holidays/batch", BatchCreateHolidays)
				weekSchedule.POST("/holidays/sync/from-juhe", SyncHolidaysFromJuhe)
				weekSchedule.DELETE("/holidays/:id", DeleteHoliday)
			}

			// 年假模块
			leave := authRequired.Group("/leave")
			{
				leave.GET("/eligibility", GetLeaveEligibility)
				leave.POST("/eligibility/recalculate", RecalculateLeaveEligibility)
				leave.GET("/grants", GetLeaveGrants)
				leave.POST("/grants/run-quarter", RunQuarterGrant)
				leave.POST("/grants/regrant", RegrantLeave)
				leave.POST("/grants/sync-to-dingtalk", SyncGrantsToDingTalk)
				leave.GET("/vacation-types", ListVacationTypes)
				leave.POST("/consume", ConsumeAnnualLeave)
				leave.GET("/consume-log", GetConsumeLog)
			}

			// 加班与调休模块
			overtime := authRequired.Group("/overtime")
			{
				overtime.GET("/matches", GetOvertimeMatches)
				overtime.POST("/matches/run", RunOvertimeMatch)
			}
			compTime := authRequired.Group("/comp-time")
			{
				compTime.GET("/balance", GetCompTimeBalance)
			}

			// 员工下班时间配置
			shiftConfig := authRequired.Group("/shift-config")
			{
				shiftConfig.GET("/list", GetShiftConfigs)
				shiftConfig.GET("/catalogs", GetShiftCatalogs)
				shiftConfig.POST("/preview", PreviewShiftConfigs)
				shiftConfig.POST("/set", SetShiftConfigs)
				shiftConfig.POST("/apply", ApplyShiftConfigs)
				shiftConfig.DELETE("/:user_id", DeleteShiftConfig)
				shiftConfig.POST("/get-or-create-shift", GetOrCreateCustomShift)
			}
		}
	}

	return router
}
