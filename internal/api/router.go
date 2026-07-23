package api

import (
	"fmt"
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
	router := gin.New()
	router.Use(querySafeGinLogger(), gin.Recovery())
	router.MaxMultipartMemory = 128 << 20 // 128 MiB，考勤数据处理需要上传多份 Excel

	router.Use(securityHeaders())

	allowOrigins, allowOriginFunc := resolveCORSConfig()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowOriginFunc:  allowOriginFunc,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", middleware.HeaderCSRFToken, "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))
	router.Use(middleware.RequestMetrics())

	router.GET("/health", HealthCheck)
	// 文件下载：JWT + TenantContext，按 org 元数据鉴权（与 authRequired 一致）
	router.GET("/api/v1/files/:filename", middleware.JWTAuth(), middleware.TenantContext(), ServeFile)

	v1 := router.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/login", Login)
			auth.POST("/logout", middleware.JWTAuth(), Logout)
			auth.GET("/me", middleware.JWTAuth(), GetCurrentUser)
			auth.GET("/orgs", ListActiveOrganizations)

			dingtalk := auth.Group("/dingtalk")
			{
				dingtalk.GET("/qr/start", DingTalkQRLoginStart)
				dingtalk.POST("/in-app", DingTalkInAppLogin)
				dingtalk.GET("/callback", DingTalkCallback)
				dingtalk.GET("/config", GetDingTalkConfig)
			}
		}

		authRequired := v1.Group("/")
		authRequired.Use(middleware.JWTAuth(), middleware.TenantContext())
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
				"menu:attendance-processing",
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
				// 部门枚举需 org 读能力或业务菜单，禁止任意登录用户无门闩枚举
				departments.GET("", middleware.RequirePermissionOrMenu(
					[]string{"org:read", "user_manage", "permission_manage", "attendance_manage"},
					orgReadMenus,
				), GetScopedDepartments)
				departments.GET("/:id", middleware.RequirePermissionOrMenu(
					[]string{"org:read", "user_manage", "permission_manage", "attendance_manage"},
					orgReadMenus,
				), GetDepartment)
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
				org.GET("/employees/:id/position-sync-diagnostic", middleware.RequirePermissionOrMenu([]string{"org:read", "user_manage"}, orgReadMenus), GetOrgEmployeePositionSyncDiagnostic)

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
				// 考勤数据处理
				processing := attendance.Group("/processing")
				processing.Use(middleware.RequirePermission("attendance_manage"))
				{
					processing.POST("/leave", ProcessLeaveDetail)
					processing.POST("/overtime", ProcessOvertimeDetailFull)
					processing.POST("/subsidy", ProcessSubsidyCheck)
					processing.POST("/final", ProcessFinalTable)
					processing.POST("/parttime", ProcessParttimeSummary)
				}
				// 工具箱写操作与前端细权限对齐：attendance_manage 或 attendance_toolbox_operate；
				// 禁止仅凭 menu:attendance-toolbox 绕过 feature 权限。只读 defaults 仍允许菜单或管理权限。
				attendance.GET("/toolbox/defaults", middleware.RequirePermissionOrMenu([]string{"attendance_manage", "attendance_toolbox_operate"}, []string{"menu:attendance-toolbox"}), GetAttendanceToolboxDefaults)
				attendance.POST("/toolbox/:module/run", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate"), RunAttendanceToolbox)
				attendance.POST("/toolbox/dingtalk-sync", middleware.RequirePermission("attendance_manage", "attendance_toolbox_dingtalk_sync"), RunDingtalkSync)
				attendance.POST("/toolbox/rules/export", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate", "attendance_toolbox_rules_edit"), ExportOvertimeRules)
				attendance.POST("/toolbox/rules/import-preview", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate", "attendance_toolbox_rules_edit"), ImportOvertimeRulesPreview)
				attendance.POST("/toolbox/:module/validate", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate"), ValidateAttendanceToolbox)
				attendance.POST("/toolbox/templates", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate"), ExportAttendanceToolboxTemplates)
				attendance.POST("/toolbox/audit", middleware.RequirePermission("attendance_manage", "attendance_toolbox_operate"), AuditAttendanceToolbox)
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

			// 通用上传：需具备业务写相关权限之一，禁止任意登录用户无门闩上传
			authRequired.POST("/upload", middleware.RequirePermissionOrMenu(
				[]string{
					"user_manage",
					"attendance_manage",
					"attendance_toolbox_operate",
					"performance:self_eval:submit",
					"performance:manager_eval:submit",
					"performance:goal:manage",
					"performance:activity:manage",
					"performance:result:view",
				},
				[]string{
					"menu:performance-overview",
					"menu:attendance-toolbox",
					"menu:employee-profile",
					"menu:employee-flow",
				},
			), UploadFile)

			performanceReadPermissions := []string{
				"performance:result:view",
				"performance:activity:manage",
				"performance:goal:manage",
				"performance:self_eval:submit",
				"performance:manager_eval:submit",
				"performance:employee_confirm:submit",
				"performance:manager_confirm:submit",
				"performance:hr_confirm:submit",
				"performance:hr_review:submit",
				"performance:result_publish:manage",
				"performance:appeal:manage",
				"performance:department_eval:submit",
			}
			performanceReadMenus := []string{
				"menu:performance-overview",
				"menu:performance-reports",
				"menu:performance-interviews",
				"menu:performance-appeals",
			}
			performanceRead := middleware.RequirePermissionOrMenu(performanceReadPermissions, performanceReadMenus)
			performanceIndicatorRead := middleware.RequirePermissionOrMenu(
				[]string{"performance:indicator:manage"},
				[]string{"menu:performance-indicator-library"},
			)
			performance := authRequired.Group("/performance")
			{
				// 活动编辑器范围选项 / Excel 导入批次（JWT + TenantContext + 绩效权限）
				performance.GET("/scope-options", performanceRead, GetPerformanceScopeOptions)
				performance.POST("/imports/analyze", middleware.RequirePermission("performance:activity:manage"), AnalyzePerformanceActivityImport)
				performance.GET("/imports/:batch_id", middleware.RequirePermission("performance:activity:manage"), GetPerformanceActivityImportBatch)
				performance.POST("/imports/:batch_id/commit", middleware.RequirePermission("performance:activity:manage"), CommitPerformanceActivityImport)

				performance.GET("/activities", performanceRead, GetPerformanceActivities)
				performance.POST("/activities", middleware.RequirePermission("performance:activity:manage"), CreatePerformanceActivity)
				performance.GET("/activities/:activity_id", performanceRead, GetPerformanceActivity)
				performance.PUT("/activities/:activity_id", middleware.RequirePermission("performance:activity:manage"), UpdatePerformanceActivity)

				performance.POST("/activities/:activity_id/start", middleware.RequirePermission("performance:activity:manage"), StartPerformanceActivity)
				performance.POST("/activities/:activity_id/open-self-evaluation", middleware.RequirePermission("performance:activity:manage"), OpenSelfEvaluation)
				performance.POST("/activities/:activity_id/open-manager-evaluation", middleware.RequirePermission("performance:activity:manage"), OpenManagerEvaluation)
				performance.POST("/activities/:activity_id/confirm-results", middleware.RequirePermission("performance:activity:manage"), ConfirmActivityResults)
				performance.POST("/activities/:activity_id/archive", middleware.RequirePermission("performance:activity:manage"), ArchivePerformanceActivity)

				performance.POST("/activities/:activity_id/open-target-setting", middleware.RequirePermission("performance:activity:manage"), OpenTargetSettingHandler)
				performance.POST("/activities/:activity_id/open-target-approval", middleware.RequirePermission("performance:activity:manage"), OpenTargetApprovalHandler)
				performance.POST("/activities/:activity_id/open-department-evaluation", middleware.RequirePermission("performance:activity:manage"), OpenDepartmentEvaluationHandler)
				performance.POST("/activities/:activity_id/open-hr-review", middleware.RequirePermission("performance:activity:manage"), OpenHRReviewHandler)
				performance.POST("/activities/:activity_id/open-result-publish", middleware.RequirePermission("performance:activity:manage"), OpenResultPublishHandler)
				performance.POST("/activities/:activity_id/open-performance-interview", middleware.RequirePermission("performance:activity:manage"), OpenPerformanceInterviewStageHandler)
				performance.POST("/activities/:activity_id/open-performance-appeal", middleware.RequirePermission("performance:activity:manage"), OpenPerformanceAppealHandler)
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
				performance.GET("/activities/:activity_id/report", performanceRead, GetPerformanceReport)
				performance.GET("/activities/:activity_id/report/export", performanceRead, ExportPerformanceReport)

				performance.POST("/activities/:activity_id/refresh-participants", middleware.RequirePermission("performance:activity:manage"), RefreshPerformanceParticipants)
				performance.POST("/participants/import", middleware.RequirePermission("performance:activity:manage"), ImportPerformanceActivityParticipants)
				performance.GET("/activities/:activity_id/participants", performanceRead, GetPerformanceParticipants)
				performance.GET("/activities/:activity_id/assessment-manager-candidates", middleware.RequirePermission("performance:assessment_manager:update"), GetAssessmentManagerCandidates)
				performance.GET("/participants/my", middleware.RequirePermission("performance:result:view", "performance:activity:manage", "performance:goal:manage", "performance:self_eval:submit", "performance:manager_eval:submit", "performance:employee_confirm:submit", "performance:manager_confirm:submit", "performance:hr_confirm:submit"), GetMyPerformanceParticipants)
				performance.GET("/participants/:participant_id", middleware.RequirePermission("performance:result:view", "performance:activity:manage", "performance:goal:manage", "performance:self_eval:submit", "performance:manager_eval:submit", "performance:employee_confirm:submit", "performance:manager_confirm:submit", "performance:hr_confirm:submit"), GetParticipant)
				performance.GET("/participants/:participant_id/previous-result", performanceRead, GetPreviousParticipantResult)
				performance.PUT("/participants/:participant_id/assessment-manager", middleware.RequirePermission("performance:assessment_manager:update"), UpdateParticipantAssessmentManager)
				performance.POST("/participants/:participant_id/admin-progress", middleware.RequirePermission("performance:activity:manage"), AdminAdjustParticipantProgress)
				performance.POST("/participants/:participant_id/remove", middleware.RequirePermission("performance:activity:manage"), RemovePerformanceParticipant)
				performance.POST("/participants/:participant_id/department-evaluation", middleware.RequirePermission("performance:department_eval:submit", "performance:level_adjust:manage", "performance:activity:manage"), DepartmentEvaluateParticipantResult)
				performance.PUT("/participants/:participant_id/result-visibility", middleware.RequirePermission("performance:result_visibility:manage"), SetParticipantResultVisibility)
				performance.POST("/activities/:activity_id/assessment-managers/batch", middleware.RequirePermission("performance:assessment_manager:batch_update"), BatchUpdateAssessmentManagers)

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
				performance.GET("/activities/:activity_id/finance", middleware.RequirePermission("performance:activity:manage"), GetCompanyFinanceHandler)
				performance.GET("/activities/:activity_id/pending-hr-confirm", middleware.RequirePermission("performance:activity:manage", "performance:hr_confirm:submit", "performance:hr_review:submit"), GetPendingHRConfirmHandler)
				performance.PUT("/activities/:activity_id/hr-confirm-deadline", middleware.RequirePermission("performance:activity:manage"), SetHRConfirmDeadlineHandler)
				performance.GET("/activities/:activity_id/hr-confirm-deadline-status", middleware.RequirePermission("performance:activity:manage", "performance:hr_confirm:submit", "performance:hr_review:submit"), GetHRConfirmDeadlineStatusHandler)

				performance.GET("/interviews", middleware.RequirePermission("performance:result:view", "performance:interview:manage", "performance:activity:manage", "performance:department_eval:submit"), GetPerformanceInterviews)
				performance.POST("/interviews", middleware.RequirePermission("performance:interview:manage"), CreatePerformanceInterview)
				performance.PUT("/interviews/:id", middleware.RequirePermission("performance:interview:manage"), UpdatePerformanceInterview)
				performance.GET("/appeals", middleware.RequirePermission("performance:result:view", "performance:appeal:manage", "performance:activity:manage", "performance:hr_review:submit", "performance:result_publish:manage"), GetPerformanceAppeals)
				performance.POST("/appeals", middleware.RequirePermission("performance:result:view", "performance:appeal:manage", "performance:activity:manage"), CreatePerformanceAppeal)
				performance.PUT("/appeals/:id", middleware.RequirePermission("performance:appeal:manage", "performance:activity:manage"), UpdatePerformanceAppeal)
				performance.POST("/appeals/:id/withdraw", middleware.RequirePermission("performance:result:view", "performance:appeal:manage", "performance:activity:manage"), WithdrawPerformanceAppeal)

				performance.GET("/indicator-libraries", performanceIndicatorRead, GetIndicatorLibraries)
				performance.POST("/indicator-libraries", middleware.RequirePermission("performance:indicator:manage"), CreateIndicatorLibrary)
				performance.GET("/indicator-libraries/:id", performanceIndicatorRead, GetIndicatorLibrary)
				performance.PUT("/indicator-libraries/:id", middleware.RequirePermission("performance:indicator:manage"), UpdateIndicatorLibrary)
				performance.POST("/indicator-libraries/:id/archive", middleware.RequirePermission("performance:indicator:manage"), ArchiveIndicatorLibrary)
				performance.GET("/indicator-libraries/department/:department_id", performanceIndicatorRead, GetIndicatorLibrariesByDepartment)
				performance.POST("/indicator-libraries/inherit", middleware.RequirePermission("performance:indicator:manage"), InheritIndicatorLibrary)

				performance.GET("/indicator-items", performanceIndicatorRead, GetIndicatorItems)
				performance.POST("/indicator-items", middleware.RequirePermission("performance:indicator:manage"), CreateIndicatorItem)
				performance.PUT("/indicator-items/:id", middleware.RequirePermission("performance:indicator:manage"), UpdateIndicatorItem)
				performance.DELETE("/indicator-items/:id", middleware.RequirePermission("performance:indicator:manage"), DeleteIndicatorItem)
				performance.GET("/indicator-items/search", performanceIndicatorRead, SearchIndicatorItems)

				performance.GET("/templates", performanceRead, GetPerformanceTemplates)
				performance.POST("/templates", middleware.RequirePermission("performance:activity:manage"), CreatePerformanceTemplate)
				performance.GET("/templates/:id", performanceRead, GetPerformanceTemplate)
				performance.PUT("/templates/:id", middleware.RequirePermission("performance:activity:manage"), UpdatePerformanceTemplate)

				performance.GET("/goal-records/:participant_id", performanceRead, GetGoalRecords)
				performance.POST("/goal-records/:participant_id", middleware.RequirePermission("performance:goal:manage"), BatchSaveGoalRecords)
				performance.POST("/goal-records/:participant_id/review-supplement", middleware.RequirePermission("performance:goal:manage"), BatchSaveReviewGoalRecords)
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

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Writer.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("X-Frame-Options", "SAMEORIGIN")
		c.Next()
	}
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

	// 开发环境：仅允许本机前端常见端口；局域网/测试环境需显式配置 CORS_ALLOW_ORIGINS。
	return []string{
		"http://localhost:5173",
		"http://localhost:3000",
		"http://127.0.0.1:5173",
		"http://127.0.0.1:3000",
	}, nil
}

// querySafeGinLogger 记录访问日志时使用 URL.EscapedPath()，避免把 query string（可能含 token）写入日志。
func querySafeGinLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		requestPath := param.Path
		if param.Request != nil && param.Request.URL != nil {
			requestPath = param.Request.URL.EscapedPath()
		}
		return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			requestPath,
		)
	})
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
