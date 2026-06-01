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

	allowOrigins, allowOriginFunc := resolveCORSConfig()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowOriginFunc:  allowOriginFunc,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	router.GET("/health", HealthCheck)
	router.GET("/api/v1/files/:filename", middleware.JWTAuthWithQuery(), ServeFile)

	v1 := router.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/logout", Logout)
			auth.GET("/me", middleware.JWTAuth(), GetCurrentUser)

			dingtalk := auth.Group("/dingtalk")
			{
				dingtalk.GET("/qr/start", DingTalkQRLoginStart)
				dingtalk.POST("/in-app", DingTalkInAppLogin)
				dingtalk.GET("/callback", DingTalkCallback)
				dingtalk.GET("/config", GetDingTalkConfig)
			}
		}

		authRequired := v1.Group("/")
		authRequired.Use(middleware.JWTAuth())
		{
			orgReadMenus := []string{
				"menu:organization-dashboard",
				"menu:department-tree",
				"menu:employees",
				"menu:employee-profile",
				"menu:employee-flow",
				"menu:talent-analysis",
				"menu:attendance",
				"menu:attendance-stats",
				"menu:attendance-export",
				"menu:leave-overtime",
				"menu:performance-overview",
			}
			attendanceReadMenus := []string{
				"menu:attendance",
				"menu:attendance-stats",
				"menu:attendance-export",
			}
			employeeReadMenus := []string{
				"menu:employee-profile",
				"menu:employee-flow",
				"menu:employees",
			}

			users := authRequired.Group("/users")
			{
				users.GET("", middleware.RequirePermissionOrMenu(
					[]string{"user_manage", "permission_manage", "attendance_manage", "org:read"},
					append(append([]string{}, orgReadMenus...), "menu:permission"),
				), GetUsers)
				users.GET("/:id", middleware.RequirePermissionOrMenu(
					[]string{"user_manage", "permission_manage", "org:read"},
					append(append([]string{}, orgReadMenus...), "menu:permission"),
				), GetUser)
				users.PUT("/:id", middleware.RequirePermission("user_manage"), UpdateUser)
			}

			departments := authRequired.Group("/departments")
			{
				departments.GET("", GetScopedDepartments)
				departments.GET("/:id", GetDepartment)
			}

			sync := authRequired.Group("/sync")
			{
				sync.POST("/departments", middleware.RequirePermission("attendance_manage"), SyncDepartments)
				sync.POST("/users", middleware.RequirePermission("attendance_manage"), SyncUsers)
				sync.GET("/status", middleware.RequirePermissionOrMenu(
					[]string{"attendance_manage"},
					[]string{"menu:sync-log", "menu:sync-jobs", "menu:setting"},
				), GetSyncStatus)
			}

			org := authRequired.Group("/org")
			{
				org.GET("/departments/tree", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage", "permission_manage"}, orgReadMenus), GetOrgDepartmentTree)
				org.GET("/departments/:id/history", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage"}, orgReadMenus), GetOrgDepartmentHistory)
				org.GET("/overview", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage"}, orgReadMenus), GetOrgOverview)

				org.GET("/employees", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage"}, orgReadMenus), GetOrgEmployees)
				org.GET("/employees/:id", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage"}, orgReadMenus), GetOrgEmployeeDetail)

				org.POST("/sync", middleware.RequirePermission("attendance_manage"), SyncOrgData)
			}

			attendance := authRequired.Group("/attendance")
			{
				attendance.GET("/records", middleware.RequirePermissionOrMenu([]string{"attendance_manage"}, attendanceReadMenus), GetAttendanceRecords)
				attendance.GET("/stats", middleware.RequirePermissionOrMenu([]string{"attendance_manage"}, attendanceReadMenus), GetAttendanceStats)
				attendance.POST("/sync", middleware.RequirePermission("attendance_manage"), SyncAttendance)
				attendance.POST("/export", middleware.RequirePermission("attendance_manage"), ExportAttendance)
				attendance.GET("/exports", middleware.RequirePermissionOrMenu([]string{"attendance_manage"}, []string{"menu:attendance-export"}), GetAttendanceExports)
				attendance.GET("/last-sync", middleware.RequirePermissionOrMenu([]string{"attendance_manage"}, attendanceReadMenus), GetLastSyncTime)
			}

			// 閻庡厜鍓濇竟鎺懳熼垾铏仴
			// 审批模块：页面读取由菜单权限控制，同步由操作权限控制。
			approvals := authRequired.Group("/approvals")
			{
				approvals.POST("/sync", middleware.RequirePermission("approval:sync"), SyncApproval)
				approvals.GET("/templates", middleware.RequireMenuPermission("menu:approval-templates", "menu:approval-stats"), GetApprovalTemplates)
				approvals.GET("/instances", middleware.RequireMenuPermission("menu:approval-instances"), GetApprovalInstances)
				approvals.GET("/:id", middleware.RequireMenuPermission("menu:approval-instances"), GetApproval)
			}

			permissionRead := middleware.RequirePermissionOrMenu(
				[]string{"permission_manage"},
				[]string{"menu:permission"},
			)
			permission := authRequired.Group("/permission")
			{
				permission.GET("/roles", permissionRead, GetRoles)
				permission.POST("/roles", middleware.RequirePermission("permission_manage"), CreateRole)
				permission.GET("/permissions", permissionRead, GetPermissions)
				permission.GET("/users/:user_id/roles", permissionRead, GetUserRoles)
				permission.POST("/users/roles/assign", middleware.RequirePermission("permission_manage"), AssignUserRole)
				permission.POST("/users/roles/remove", middleware.RequirePermission("permission_manage"), RemoveUserRole)
				permission.GET("/users/:user_id/permissions", permissionRead, GetUserPermissions)

				// 角色子路由：统一使用 :role_id 参数名，避免 Gin 路由树冲突
				role := permission.Group("/roles")
				{
					role.GET("/:role_id/users", permissionRead, GetRoleUsers)
					role.GET("/:role_id/permissions", permissionRead, GetRolePermissions)
					role.POST("/:role_id/permissions", middleware.RequirePermission("permission_manage"), SaveRolePermissions)
					role.GET("/:role_id/menu", permissionRead, GetMenuPermission)
					role.POST("/:role_id/menu", middleware.RequirePermission("permission_manage"), SaveMenuPermission)
					role.GET("/:role_id/data", permissionRead, GetDataPermission)
					role.POST("/:role_id/data", middleware.RequirePermission("permission_manage"), SaveDataPermission)
					role.PUT("/:role_id", middleware.RequirePermission("permission_manage"), UpdateRole)
				}
			}

			// 閻庡銈庡悁闁哄啨鍎辩换鏂课熼垾铏仴
			audit := authRequired.Group("/audit")
			audit.Use(middleware.RequirePermission("permission_manage", "audit_log:read"))
			{
				audit.GET("/logs", GetAuditLogs)
			}

			// 濞寸姾顕ф慨鐔哥▔椤撶偟濡囨俊顖椻偓铏仴
			jobs := authRequired.Group("/jobs")
			jobs.Use(middleware.RequirePermission("attendance_manage"))
			{
				jobs.GET("", GetJobs)
				jobs.POST("/:id/run", RunJob)
			}

			// 闁告稒锚娴兼劕顩奸敐鍡╂敵濞戞搩鍘肩缓鎯熼垾铏仴
			employee := authRequired.Group("/employee")
			{
				employee.GET("/profiles", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, employeeReadMenus), GetEmployeeProfiles)
				employee.GET("/profiles/:id", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, employeeReadMenus), GetEmployeeProfile)
				employee.POST("/profiles", middleware.RequirePermission("user_manage"), CreateEmployeeProfile)
				employee.PUT("/profiles/:id", middleware.RequirePermission("user_manage"), UpdateEmployeeProfile)
				employee.GET("/ledger", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, employeeReadMenus), GetEmployeeLifecycleLedger)
				employee.GET("/transfers", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, []string{"menu:employee-flow"}), GetTransfers)
				employee.POST("/transfers", middleware.RequirePermission("user_manage"), CreateTransfer)
				employee.GET("/resignations", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, []string{"menu:employee-flow"}), GetResignations)
				employee.POST("/resignations", middleware.RequirePermission("user_manage"), CreateResignation)
				employee.GET("/onboardings", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, []string{"menu:employee-flow"}), GetOnboardings)
				employee.POST("/onboardings", middleware.RequirePermission("user_manage"), CreateOnboarding)
			}

			talent := authRequired.Group("/talent")
			{
				talent.GET("/analysis", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, []string{"menu:talent-analysis"}), GetTalentAnalysisList)
				talent.GET("/analysis/:id", middleware.RequirePermissionOrMenu([]string{"user_manage", "org:read"}, []string{"menu:talent-analysis"}), GetTalentAnalysisDetail)
				talent.POST("/analysis", middleware.RequirePermission("user_manage"), CreateTalentAnalysis)
			}

			weekScheduleRead := middleware.RequirePermissionOrMenu(
				[]string{"attendance_manage"},
				[]string{"menu:week-schedule"},
			)
			leaveOvertimeRead := middleware.RequirePermissionOrMenu(
				[]string{"attendance_manage"},
				[]string{"menu:leave-overtime"},
			)
			shiftConfigRead := middleware.RequirePermissionOrMenu(
				[]string{"attendance_manage"},
				[]string{"menu:employee-shift-config", "menu:week-schedule"},
			)

			weekSchedule := authRequired.Group("/week-schedule")
			{
				weekSchedule.GET("/rules", middleware.RequirePermission("attendance_manage"), GetWeekScheduleRules)
				weekSchedule.POST("/rules", middleware.RequirePermission("attendance_manage"), CreateWeekScheduleRule)
				weekSchedule.POST("/rules/batch", middleware.RequirePermission("attendance_manage"), BatchSetWeekScheduleRules)
				weekSchedule.PUT("/rules/:id", middleware.RequirePermission("attendance_manage"), UpdateWeekScheduleRule)
				weekSchedule.DELETE("/rules/:id", middleware.RequirePermission("attendance_manage"), DeleteWeekScheduleRule)

				weekSchedule.GET("/shifts", middleware.RequirePermission("attendance_manage"), GetDingTalkShifts)
				weekSchedule.POST("/shifts", middleware.RequirePermission("attendance_manage"), CreateDingTalkShift)
				weekSchedule.GET("/debug/attendance-groups", middleware.RequirePermission("attendance_manage"), DebugAttendanceGroups)

				weekSchedule.GET("/calendar", weekScheduleRead, GetWeekCalendar)

				weekSchedule.POST("/overrides", middleware.RequirePermission("attendance_manage"), SetWeekOverride)
				weekSchedule.DELETE("/overrides/:id", middleware.RequirePermission("attendance_manage"), DeleteWeekOverride)

				weekSchedule.POST("/sync/to-dingtalk", middleware.RequirePermission("attendance_manage"), SyncWeekToDingTalk)
				weekSchedule.POST("/sync/from-dingtalk", middleware.RequirePermission("attendance_manage"), SyncWeekFromDingTalk)
				weekSchedule.GET("/sync/logs", middleware.RequirePermission("attendance_manage"), GetWeekSyncLogs)

				weekSchedule.GET("/holidays", middleware.RequirePermission("attendance_manage"), GetHolidays)
				weekSchedule.POST("/holidays", middleware.RequirePermission("attendance_manage"), CreateHoliday)
				weekSchedule.POST("/holidays/batch", middleware.RequirePermission("attendance_manage"), BatchCreateHolidays)
				weekSchedule.POST("/holidays/sync/from-juhe", middleware.RequirePermission("attendance_manage"), SyncHolidaysFromJuhe)
				weekSchedule.DELETE("/holidays/:id", middleware.RequirePermission("attendance_manage"), DeleteHoliday)
			}

			// 妤犵偞娼欐禍锝呂熼垾铏仴
			leave := authRequired.Group("/leave")
			{
				leave.GET("/eligibility", leaveOvertimeRead, GetLeaveEligibility)
				leave.POST("/eligibility/recalculate", middleware.RequirePermission("attendance_manage"), RecalculateLeaveEligibility)
				leave.GET("/grants", leaveOvertimeRead, GetLeaveGrants)
				leave.POST("/grants/run-quarter", middleware.RequirePermission("attendance_manage"), RunQuarterGrant)
				leave.POST("/grants/regrant", middleware.RequirePermission("attendance_manage"), RegrantLeave)
				leave.POST("/grants/sync-to-dingtalk", middleware.RequirePermission("attendance_manage"), SyncGrantsToDingTalk)
				leave.GET("/vacation-types", middleware.RequirePermission("attendance_manage"), ListVacationTypes)
				leave.POST("/consume", middleware.RequirePermission("attendance_manage"), ConsumeAnnualLeave)
				leave.GET("/consume-log", leaveOvertimeRead, GetConsumeLog)
			}

			overtime := authRequired.Group("/overtime")
			{
				overtime.GET("/matches", leaveOvertimeRead, GetOvertimeMatches)
				overtime.POST("/matches/run", middleware.RequirePermission("attendance_manage"), RunOvertimeMatch)
				overtime.POST("/matches/force", middleware.RequirePermission("attendance_manage"), ForceOvertimeMatch)
				overtime.POST("/matches/clear-rematch", middleware.RequirePermission("attendance_manage"), ClearAndRematchOvertime)
				overtime.POST("/matches/delete", middleware.RequirePermission("attendance_manage"), DeleteOvertimeMatchRecords)
				overtime.POST("/sync-and-match", middleware.RequirePermission("attendance_manage"), SyncAndMatch)
				overtime.POST("/reset-manual-leave", middleware.RequirePermission("attendance_manage"), ResetManualLeave)
				overtime.POST("/resync-overtime", middleware.RequirePermission("attendance_manage"), ResyncOvertimeToDingTalk)
				overtime.POST("/supplementary/submit", leaveOvertimeRead, SubmitSupplementaryClockIn)
				overtime.POST("/supplementary/approve", middleware.RequirePermission("attendance_manage"), ApproveSupplementaryClockIn)
				overtime.GET("/supplementary/list", leaveOvertimeRead, GetSupplementaryRequests)
				overtime.POST("/supplementary/sync-dingtalk", middleware.RequirePermission("attendance_manage"), SyncSupplementaryFromDingTalk)
			}
			compTime := authRequired.Group("/comp-time")
			{
				compTime.GET("/balance", leaveOvertimeRead, GetCompTimeBalance)
				compTime.POST("/manual-grant", middleware.RequirePermission("attendance_manage"), ManualGrantCompensatoryLeave)
			}

			// 闁告稒锚娴兼劖绋夌€ｎ剙鐤嗛柡鍐ㄧ埣濡潡鏌婂鍥╂瀭
			shiftConfig := authRequired.Group("/shift-config")
			{
				shiftConfig.GET("/list", shiftConfigRead, GetShiftConfigs)
				shiftConfig.GET("/catalogs", shiftConfigRead, GetShiftCatalogs)
				shiftConfig.POST("/preview", middleware.RequirePermission("attendance_manage"), PreviewShiftConfigs)
				shiftConfig.POST("/set", middleware.RequirePermission("attendance_manage"), SetShiftConfigs)
				shiftConfig.POST("/apply", middleware.RequirePermission("attendance_manage"), ApplyShiftConfigs)
				shiftConfig.DELETE("/:user_id", middleware.RequirePermission("attendance_manage"), DeleteShiftConfig)
				shiftConfig.POST("/get-or-create-shift", middleware.RequirePermission("attendance_manage"), GetOrCreateCustomShift)
			}

			authRequired.POST("/upload", UploadFile)

			performance := authRequired.Group("/performance")
			{
				performance.GET("/activities", GetPerformanceActivities)
				performance.POST("/activities", middleware.RequirePermission("performance:activity:manage"), CreatePerformanceActivity)
				performance.GET("/activities/:activity_id", GetPerformanceActivity)
				performance.PUT("/activities/:activity_id", middleware.RequirePermission("performance:activity:manage"), UpdatePerformanceActivity)

				performance.POST("/activities/:activity_id/start", middleware.RequirePermission("performance:activity:manage"), StartPerformanceActivity)
				performance.POST("/activities/:activity_id/open-self-evaluation", middleware.RequirePermission("performance:activity:manage"), OpenSelfEvaluation)
				performance.POST("/activities/:activity_id/open-manager-evaluation", middleware.RequirePermission("performance:activity:manage"), OpenManagerEvaluation)
				performance.POST("/activities/:activity_id/confirm-results", middleware.RequirePermission("performance:activity:manage"), ConfirmActivityResults)
				performance.POST("/activities/:activity_id/archive", middleware.RequirePermission("performance:activity:manage"), ArchivePerformanceActivity)

				performance.POST("/activities/:activity_id/open-target-setting", middleware.RequirePermission("performance:activity:manage"), OpenTargetSettingHandler)
				performance.POST("/activities/:activity_id/open-employee-confirmation", middleware.RequirePermission("performance:activity:manage"), OpenEmployeeConfirmationHandler)
				performance.POST("/activities/:activity_id/open-manager-confirmation", middleware.RequirePermission("performance:activity:manage"), OpenManagerConfirmationHandler)
				performance.POST("/activities/:activity_id/open-hr-confirmation", middleware.RequirePermission("performance:activity:manage"), OpenHRConfirmationHandler)
				performance.POST("/activities/:activity_id/lock", middleware.RequirePermission("performance:activity:manage"), LockPerformanceActivityHandler)
				performance.POST("/activities/:activity_id/force-lock-overdue-hr", middleware.RequirePermission("performance:activity:manage"), ForceLockOverdueHRConfirmationHandler)

				performance.POST("/activities/:activity_id/publish", middleware.RequirePermission("performance:activity:manage"), PublishPerformanceActivity)
				performance.POST("/activities/:activity_id/close", middleware.RequirePermission("performance:activity:manage"), ClosePerformanceActivity)

				performance.PUT("/activities/:activity_id/distribution-rules", middleware.RequirePermission("performance:distribution:manage"), PutDistributionRules)
				performance.GET("/activities/:activity_id/distribution-rules", GetDistributionRules)
				performance.GET("/activities/:activity_id/result-summary", GetPerformanceResultSummary)
				performance.GET("/activities/:activity_id/distribution-check", GetPerformanceDistributionCheck)
				performance.GET("/activities/:activity_id/realtime-distribution-check", GetRealtimeDistributionCheck)

				performance.POST("/activities/:activity_id/refresh-participants", middleware.RequirePermission("performance:activity:manage"), RefreshPerformanceParticipants)
				performance.GET("/activities/:activity_id/participants", GetPerformanceParticipants)
				performance.GET("/participants/:participant_id", middleware.RequirePermission("performance:result:view"), GetParticipant)

				performance.POST("/participants/:participant_id/self-evaluation", middleware.RequirePermission("performance:self_eval:submit"), SubmitSelfEvaluation)
				performance.POST("/participants/:participant_id/manager-evaluation", middleware.RequirePermission("performance:manager_eval:submit"), SubmitManagerEvaluation)
				performance.POST("/reviews/:participant_id/self-evaluation", middleware.RequirePermission("performance:self_eval:submit"), SubmitReviewSelfEvaluation)
				performance.POST("/reviews/:participant_id/manager-evaluation", middleware.RequirePermission("performance:manager_eval:submit"), SubmitReviewManagerEvaluation)
				performance.POST("/goal-reviews/:participant_id/self-evaluation", middleware.RequirePermission("performance:self_eval:submit"), SubmitGoalSelfEvaluationHandler)
				performance.POST("/goal-reviews/:participant_id/manager-evaluation", middleware.RequirePermission("performance:manager_eval:submit"), SubmitGoalManagerEvaluationHandler)
				performance.POST("/goal-reviews/:participant_id/bonus-penalty", middleware.RequirePermission("performance:manager_eval:submit"), SetBonusPenaltyScoreHandler)
				performance.POST("/auto-score", middleware.RequirePermission("performance:activity:manage"), AutoScoreGoalRecordsHandler)
				performance.POST("/activities/:activity_id/batch-manager-evaluations", middleware.RequirePermission("performance:manager_eval:submit"), BatchSubmitManagerEvaluation)

				performance.POST("/participants/:participant_id/adjust-final-level", middleware.RequirePermission("performance:level_adjust:manage"), AdjustFinalLevel)
				performance.POST("/participants/:participant_id/confirm-result", middleware.RequirePermission("performance:manager_confirm:submit"), ConfirmResult)
				performance.POST("/participants/:participant_id/confirm-employee", middleware.RequirePermission("performance:employee_confirm:submit"), ConfirmEmployeeResultHandler)
				performance.POST("/participants/:participant_id/confirm-manager", middleware.RequirePermission("performance:manager_confirm:submit"), ConfirmManagerResultHandler)
				performance.POST("/participants/:participant_id/confirm-hr", middleware.RequirePermission("performance:hr_confirm:submit"), ConfirmHRResultHandler)
				performance.POST("/participants/:participant_id/trigger-interview", middleware.RequirePermission("performance:activity:manage"), TriggerPerformanceInterview)

				performance.GET("/participants/:participant_id/versions", middleware.RequirePermission("performance:result:view"), GetParticipantVersions)
				performance.GET("/participants/:participant_id/relationship-change-logs", middleware.RequirePermission("performance:result:view"), GetParticipantRelationshipChangeLogs)
				performance.GET("/activities/:activity_id/relationship-change-logs", middleware.RequirePermission("performance:result:view"), GetActivityRelationshipChangeLogs)
				performance.POST("/activities/:activity_id/batch-confirm-results", middleware.RequirePermission("performance:activity:manage"), BatchConfirmResults)
				performance.POST("/activities/:activity_id/batch-confirm", middleware.RequirePermission("performance:activity:manage"), BatchConfirmResults)
				performance.POST("/activities/:activity_id/send-self-eval-reminder", middleware.RequirePermission("performance:activity:manage"), SendSelfEvalReminder)
				performance.POST("/activities/:activity_id/send-manager-eval-reminder", middleware.RequirePermission("performance:activity:manage"), SendManagerEvalReminder)
				performance.POST("/activities/:activity_id/send-hr-confirm-reminder", middleware.RequirePermission("performance:activity:manage"), SendHRConfirmReminder)
				performance.PUT("/activities/:activity_id/finance", middleware.RequirePermission("performance:activity:manage"), SetCompanyFinanceHandler)
				performance.GET("/activities/:activity_id/finance", GetCompanyFinanceHandler)
				performance.GET("/activities/:activity_id/pending-hr-confirm", GetPendingHRConfirmHandler)
				performance.PUT("/activities/:activity_id/hr-confirm-deadline", middleware.RequirePermission("performance:activity:manage"), SetHRConfirmDeadlineHandler)
				performance.GET("/activities/:activity_id/hr-confirm-deadline-status", GetHRConfirmDeadlineStatusHandler)

				performance.GET("/indicator-libraries", GetIndicatorLibraries)
				performance.POST("/indicator-libraries", middleware.RequirePermission("performance:indicator:manage"), CreateIndicatorLibrary)
				performance.GET("/indicator-libraries/:id", GetIndicatorLibrary)
				performance.PUT("/indicator-libraries/:id", middleware.RequirePermission("performance:indicator:manage"), UpdateIndicatorLibrary)
				performance.POST("/indicator-libraries/:id/archive", middleware.RequirePermission("performance:indicator:manage"), ArchiveIndicatorLibrary)
				performance.GET("/indicator-libraries/department/:department_id", GetIndicatorLibrariesByDepartment)
				performance.POST("/indicator-libraries/inherit", middleware.RequirePermission("performance:indicator:manage"), InheritIndicatorLibrary)

				performance.GET("/indicator-items", GetIndicatorItems)
				performance.POST("/indicator-items", middleware.RequirePermission("performance:indicator:manage"), CreateIndicatorItem)
				performance.PUT("/indicator-items/:id", middleware.RequirePermission("performance:indicator:manage"), UpdateIndicatorItem)
				performance.DELETE("/indicator-items/:id", middleware.RequirePermission("performance:indicator:manage"), DeleteIndicatorItem)
				performance.GET("/indicator-items/search", SearchIndicatorItems)

				performance.GET("/templates", GetPerformanceTemplates)
				performance.POST("/templates", middleware.RequirePermission("performance:activity:manage"), CreatePerformanceTemplate)
				performance.GET("/templates/:id", GetPerformanceTemplate)
				performance.PUT("/templates/:id", middleware.RequirePermission("performance:activity:manage"), UpdatePerformanceTemplate)

				performance.GET("/goal-records/:participant_id", GetGoalRecords)
				performance.POST("/goal-records/:participant_id", middleware.RequirePermission("performance:goal:manage"), BatchSaveGoalRecords)
				performance.POST("/goal-records/:participant_id/submit", middleware.RequirePermission("performance:goal:manage"), SubmitGoalApprovalHandler)
				performance.POST("/goal-records/:participant_id/approve", middleware.RequirePermission("performance:goal:manage"), ApproveGoalRecords)
				performance.POST("/goal-records/:participant_id/reject", middleware.RequirePermission("performance:goal:manage"), RejectGoalRecords)
				performance.GET("/goal-records/:participant_id/manager-goals", middleware.RequirePermission("performance:goal:manage"), GetManagerGoals)
				performance.GET("/goal-records/:participant_id/suggestions", middleware.RequirePermission("performance:goal:manage"), GetGoalSuggestions)
				performance.POST("/activities/:activity_id/batch-assign-goals", middleware.RequirePermission("performance:goal:manage"), BatchAssignGoals)

				performance.POST("/participants/:participant_id/bonus-penalty", middleware.RequirePermission("performance:manager_eval:submit"), SetBonusPenaltyScoreHandler)
			}
		}
	}

	registerFrontendRoutes(router)

	return router
}

func resolveCORSConfig() ([]string, func(string) bool) {
	allowOrigins := make([]string, 0)
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOW_ORIGINS"), ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "*" {
			continue
		}
		allowOrigins = append(allowOrigins, origin)
	}
	if len(allowOrigins) > 0 {
		return allowOrigins, nil
	}

	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	ginMode := strings.ToLower(strings.TrimSpace(os.Getenv("GIN_MODE")))
	if env == "production" || ginMode == "release" {
		return nil, func(string) bool { return false }
	}

	// 开发环境：允许 localhost 常见端口 + 任意 origin（局域网访问等）
	return []string{"http://localhost:5173", "http://localhost:3000"}, func(origin string) bool { return true }
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
			c.String(http.StatusServiceUnavailable, "frontend build not found at %s, please run npm run build in D:\\ai濡炪倕婀卞ú鐧╘frontend", indexFile)
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
