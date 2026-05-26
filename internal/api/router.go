package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"peopleops/internal/middleware"
	"strings"

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

	// 文件访问（公开）
	router.GET("/api/v1/files/:filename", ServeFile)

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
				departments.GET("", GetScopedDepartments)
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
				org.GET("/departments/tree", GetOrgDepartmentTree)
				org.GET("/departments/:id/history", GetOrgDepartmentHistory)

				// 员工相关
				org.GET("/overview", GetOrgOverview)
				org.GET("/employees", GetOrgEmployees)
				org.GET("/employees/:id", GetOrgEmployeeDetail)

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
				permission.PUT("/roles/:id", UpdateRole)
				// 权限管理
				permission.GET("/permissions", GetPermissions)
				// 用户角色管理
				permission.GET("/users/:user_id/roles", GetUserRoles)
				permission.POST("/users/roles/assign", AssignUserRole)
				permission.POST("/users/roles/remove", RemoveUserRole)
				permission.GET("/roles/:role_id/users", GetRoleUsers)
				// 用户权限查询
				permission.GET("/users/:user_id/permissions", GetUserPermissions)
				// 菜单权限
				permission.GET("/roles/:role_id/menu", GetMenuPermission)
				permission.POST("/roles/:role_id/menu", SaveMenuPermission)
				// 数据权限
				permission.GET("/roles/:role_id/data", GetDataPermission)
				permission.POST("/roles/:role_id/data", SaveDataPermission)
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
				employee.GET("/ledger", GetEmployeeLifecycleLedger)

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
				overtime.POST("/matches/force", ForceOvertimeMatch)
				overtime.POST("/matches/clear-rematch", ClearAndRematchOvertime)
				overtime.POST("/matches/delete", DeleteOvertimeMatchRecords)
				overtime.POST("/sync-and-match", SyncAndMatch)
				overtime.POST("/reset-manual-leave", ResetManualLeave)
				overtime.POST("/resync-overtime", ResyncOvertimeToDingTalk)
				overtime.POST("/supplementary/submit", SubmitSupplementaryClockIn)
				overtime.POST("/supplementary/approve", ApproveSupplementaryClockIn)
				overtime.GET("/supplementary/list", GetSupplementaryRequests)
				overtime.POST("/supplementary/sync-dingtalk", SyncSupplementaryFromDingTalk)
			}
			compTime := authRequired.Group("/comp-time")
			{
				compTime.GET("/balance", GetCompTimeBalance)
				compTime.POST("/manual-grant", ManualGrantCompensatoryLeave)
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

			// 文件上传
			authRequired.POST("/upload", UploadFile)

			performance := authRequired.Group("/performance")
			{
				performance.GET("/activities", GetPerformanceActivities)
				performance.POST("/activities", CreatePerformanceActivity)
				performance.GET("/activities/:activity_id", GetPerformanceActivity)
				performance.PUT("/activities/:activity_id", UpdatePerformanceActivity)

				// 活动状态流转
				performance.POST("/activities/:activity_id/start", StartPerformanceActivity)
				performance.POST("/activities/:activity_id/open-self-evaluation", OpenSelfEvaluation)
				performance.POST("/activities/:activity_id/open-manager-evaluation", OpenManagerEvaluation)
				performance.POST("/activities/:activity_id/confirm-results", ConfirmActivityResults)
				performance.POST("/activities/:activity_id/archive", ArchivePerformanceActivity)

				// 新增状态流转（9状态流）
				performance.POST("/activities/:activity_id/open-target-setting", OpenTargetSettingHandler)
				performance.POST("/activities/:activity_id/open-employee-confirmation", OpenEmployeeConfirmationHandler)
				performance.POST("/activities/:activity_id/open-manager-confirmation", OpenManagerConfirmationHandler)
				performance.POST("/activities/:activity_id/open-hr-confirmation", OpenHRConfirmationHandler)
				performance.POST("/activities/:activity_id/lock", LockPerformanceActivityHandler)
				performance.POST("/activities/:activity_id/force-lock-overdue-hr", ForceLockOverdueHRConfirmationHandler)

				// 兼容旧接口
				performance.POST("/activities/:activity_id/publish", PublishPerformanceActivity)
				performance.POST("/activities/:activity_id/close", ClosePerformanceActivity)

				performance.PUT("/activities/:activity_id/distribution-rules", PutDistributionRules)
				performance.GET("/activities/:activity_id/distribution-rules", GetDistributionRules)
				performance.GET("/activities/:activity_id/result-summary", GetPerformanceResultSummary)
				performance.GET("/activities/:activity_id/distribution-check", GetPerformanceDistributionCheck)
				performance.GET("/activities/:activity_id/realtime-distribution-check", GetRealtimeDistributionCheck)

				performance.POST("/activities/:activity_id/refresh-participants", RefreshPerformanceParticipants)
				performance.GET("/activities/:activity_id/participants", GetPerformanceParticipants)
				performance.GET("/participants/:participant_id", GetParticipant)

				// 评分（旧路径，兼容）
				performance.POST("/participants/:participant_id/self-evaluation", SubmitSelfEvaluation)
				performance.POST("/participants/:participant_id/manager-evaluation", SubmitManagerEvaluation)
				// 评分（新路径，带钉钉审批同步）
				performance.POST("/reviews/:participant_id/self-evaluation", SubmitReviewSelfEvaluation)
				performance.POST("/reviews/:participant_id/manager-evaluation", SubmitReviewManagerEvaluation)
				performance.POST("/goal-reviews/:participant_id/self-evaluation", SubmitGoalSelfEvaluationHandler)
				performance.POST("/goal-reviews/:participant_id/manager-evaluation", SubmitGoalManagerEvaluationHandler)
				performance.POST("/goal-reviews/:participant_id/bonus-penalty", SetBonusPenaltyScoreHandler)
				performance.POST("/auto-score", AutoScoreGoalRecordsHandler)
				performance.POST("/activities/:activity_id/batch-manager-evaluations", BatchSubmitManagerEvaluation)

				performance.POST("/participants/:participant_id/adjust-final-level", AdjustFinalLevel)
				performance.POST("/participants/:participant_id/confirm-result", ConfirmResult)
				// 三级确认流程
				performance.POST("/participants/:participant_id/confirm-employee", ConfirmEmployeeResultHandler)
				performance.POST("/participants/:participant_id/confirm-manager", ConfirmManagerResultHandler)
				performance.POST("/participants/:participant_id/confirm-hr", ConfirmHRResultHandler)
				performance.POST("/participants/:participant_id/trigger-interview", TriggerPerformanceInterview)

				performance.GET("/participants/:participant_id/versions", GetParticipantVersions)
				performance.GET("/participants/:participant_id/relationship-change-logs", GetParticipantRelationshipChangeLogs)
				performance.GET("/activities/:activity_id/relationship-change-logs", GetActivityRelationshipChangeLogs)
				performance.POST("/activities/:activity_id/batch-confirm-results", BatchConfirmResults)
				performance.POST("/activities/:activity_id/batch-confirm", BatchConfirmResults)
				performance.POST("/activities/:activity_id/send-self-eval-reminder", SendSelfEvalReminder)
				performance.POST("/activities/:activity_id/send-manager-eval-reminder", SendManagerEvalReminder)
				performance.POST("/activities/:activity_id/send-hr-confirm-reminder", SendHRConfirmReminder)
				performance.PUT("/activities/:activity_id/finance", SetCompanyFinanceHandler)
				performance.GET("/activities/:activity_id/finance", GetCompanyFinanceHandler)
				performance.GET("/activities/:activity_id/pending-hr-confirm", GetPendingHRConfirmHandler)
				performance.PUT("/activities/:activity_id/hr-confirm-deadline", SetHRConfirmDeadlineHandler)
				performance.GET("/activities/:activity_id/hr-confirm-deadline-status", GetHRConfirmDeadlineStatusHandler)

				// 指标库管理
				performance.GET("/indicator-libraries", GetIndicatorLibraries)
				performance.POST("/indicator-libraries", CreateIndicatorLibrary)
				performance.GET("/indicator-libraries/:id", GetIndicatorLibrary)
				performance.PUT("/indicator-libraries/:id", UpdateIndicatorLibrary)
				performance.POST("/indicator-libraries/:id/archive", ArchiveIndicatorLibrary)
				performance.GET("/indicator-libraries/department/:department_id", GetIndicatorLibrariesByDepartment)
				performance.POST("/indicator-libraries/inherit", InheritIndicatorLibrary)

				// 指标项管理
				performance.GET("/indicator-items", GetIndicatorItems)
				performance.POST("/indicator-items", CreateIndicatorItem)
				performance.PUT("/indicator-items/:id", UpdateIndicatorItem)
				performance.DELETE("/indicator-items/:id", DeleteIndicatorItem)
				performance.GET("/indicator-items/search", SearchIndicatorItems)

				// 模板管理（兼容旧接口）
				performance.GET("/templates", GetPerformanceTemplates)
				performance.POST("/templates", CreatePerformanceTemplate)
				performance.GET("/templates/:id", GetPerformanceTemplate)
				performance.PUT("/templates/:id", UpdatePerformanceTemplate)

				// 目标记录管理
				performance.GET("/goal-records/:participant_id", GetGoalRecords)
				performance.POST("/goal-records/:participant_id", BatchSaveGoalRecords)
				performance.POST("/goal-records/:participant_id/submit", SubmitGoalApprovalHandler)
				performance.POST("/goal-records/:participant_id/approve", ApproveGoalRecords)
				performance.POST("/goal-records/:participant_id/reject", RejectGoalRecords)
				performance.GET("/goal-records/:participant_id/manager-goals", GetManagerGoals)
				performance.GET("/goal-records/:participant_id/suggestions", GetGoalSuggestions)
				performance.POST("/activities/:activity_id/batch-assign-goals", BatchAssignGoals)

				// 加减分
				performance.POST("/participants/:participant_id/bonus-penalty", SetBonusPenaltyScoreHandler)
			}
		}
	}

	registerFrontendRoutes(router)

	return router
}

func registerFrontendRoutes(router *gin.Engine) {
	distDir := filepath.Join("frontend", "dist")
	indexFile := filepath.Join(distDir, "index.html")

	router.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if strings.HasPrefix(requestPath, "/api/") {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "API route not found",
			})
			return
		}

		if _, err := os.Stat(indexFile); err != nil {
			c.String(http.StatusServiceUnavailable, "frontend build not found at %s, please run npm run build in D:\\ai项目\\frontend", indexFile)
			return
		}

		cleanPath := strings.TrimPrefix(path.Clean(requestPath), "/")
		if cleanPath != "" && cleanPath != "." {
			if strings.HasPrefix(cleanPath, "..") {
				c.Status(http.StatusNotFound)
				return
			}

			candidate := filepath.Join(distDir, filepath.FromSlash(cleanPath))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				c.File(candidate)
				return
			}
		}

		c.File(indexFile)
	})
}
