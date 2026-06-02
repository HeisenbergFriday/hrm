# anatomy.md

> Auto-maintained by OpenWolf. Last scanned: 2026-06-02T04:00:00.609Z
> Files: 180 tracked | Anatomy hits: 0 | Misses: 0

## ./

- `.editorconfig` — Editor configuration (~51 tok)
- `.gitignore` — Git ignore rules (~106 tok)
- `ARCHITECTURE.md` — PeopleOps 架构设计 (~690 tok)
- `BACKEND_API_DESIGN.md` — PeopleOps 后端 API 设计 (~2133 tok)
- `CLAUDE.md` — AI 项目协作规则 (~2529 tok)
- `DATABASE_DESIGN.md` — PeopleOps 数据库设计 (~1485 tok)
- `DEPLOYMENT.md` — PeopleOps 联调与部署说明 (~792 tok)
- `ENVIRONMENT.md` — 环境变量清单 (~763 tok)
- `FRONTEND_DESIGN.md` — PeopleOps 前端设计 (~934 tok)
- `frontend-dev-server.err.log` (~0 tok)
- `frontend-dev-server.out.log` (~274 tok)
- `go.mod` — Go module definition (~542 tok)
- `go.sum` — Go dependency checksums (~3113 tok)
- `PERFORMANCE.md` — 系统性能测试和优化 (~1075 tok)
- `query` (~2 tok)
- `README.md` — Project documentation (~973 tok)

## .ai/

- `AI_WORKFLOW.md` — AI 开发工作流 (~7840 tok)
- `ARCHITECTURE.md` — 架构设计 (~1482 tok)
- `COMMANDS.md` — 常用命令 (~673 tok)
- `CONVENTIONS.md` — 编码规范 (~1582 tok)
- `DESIGN_SYSTEM.md` — 设计规范 (~2240 tok)
- `PROJECT_MAP.md` — 项目结构索引 (~3053 tok)

## .ai/MODULES/

- `approval.md` — 审批模块 (~458 tok)
- `attendance.md` — 考勤模块 (~1352 tok)
- `auth.md` — 认证模块 (~1737 tok)
- `employee-profile.md` — 员工档案与入转调离 (~1550 tok)
- `leave-overtime.md` — 年假与调休模块 (~2911 tok)
- `org.md` — 组织架构模块 (~2508 tok)
- `organization.md` — 组织模块 (~327 tok)
- `performance.md` — 绩效管理模块 (~5977 tok)
- `shift-config.md` — 员工下班时间配置 (~329 tok)
- `week-schedule.md` — 大小周排班模块 (~1631 tok)

## .ai/plans/

- `performance-online-form.md` — 绩效表单线上化技术方案 (~3487 tok)

## .claude/

- `settings.json` (~898 tok)
- `settings.local.json` — Declares icons (~1720 tok)

## .claude/rules/

- `openwolf.md` (~317 tok)

## api-docs/

- `ai-dev-spec-leave-overtime.md` — AI Development Spec: Annual Leave and Compensatory Time (~4354 tok)
- `ai-dev-spec-priority-roadmap.md` — AI 开发文案：假勤、组织、绩效分阶段实施 (~2545 tok)
- `org-module-enhancement-plan.md` — 组织模块增强方案 (~1791 tok)
- `swagger.json` (~6725 tok)

## cmd/

- `main.go` (~362 tok)

## frontend/

- `.eslintrc.cjs` — ESLint configuration (~141 tok)
- `index.html` — 钉钉一体化人事后台 (~122 tok)
- `package-lock.json` — npm lock file (~78740 tok)
- `package.json` — Node.js package manifest (~314 tok)
- `playwright.config.ts` — Playwright test configuration (~582 tok)
- `tsconfig.json` — TypeScript configuration (~190 tok)
- `tsconfig.node.json` (~61 tok)
- `vite-3021-err.log` (~0 tok)
- `vite-3021-out.log` (~816 tok)
- `vite.config.ts` — Vite build configuration (~1041 tok)

## frontend/playwright-report/

- `index.html` — Playwright Test Report (~140723 tok)

## frontend/scripts/

- `generate-report.js` — __filename: list, table, generateReport (~801 tok)

## frontend/src/

- `App.tsx` — Login — uses useState, useNavigate, useEffect (~5491 tok)
- `index.css` — Styles: 42 rules, 79 vars (~2186 tok)
- `main.tsx` — queryClient (~162 tok)
- `vite-env.d.ts` — / <reference types="vite/client" /> (~12 tok)

## frontend/src/components/

- `AttachmentUpload.tsx` — AttachmentUpload (~753 tok)
- `ErrorBoundary.tsx` — Exports ErrorBoundary (~493 tok)
- `PageCard.tsx` — PageCard (~208 tok)
- `PageContainer.tsx` — PageContainer (~580 tok)
- `PerformanceActivityEditor.tsx` — activitySections — renders form — uses useState, useMemo (~4904 tok)
- `RouteGuard.tsx` — RouteGuard (~408 tok)
- `StatusTag.tsx` — StatusTag (~131 tok)

## frontend/src/config/

- `menu.tsx` — menuPermissionKey — renders chart (~1796 tok)

## frontend/src/pages/

- `Approval.tsx` — fetchApprovals — renders table — uses useQuery (~1368 tok)
- `ApprovalDetail.tsx` — ApprovalDetail — uses useNavigate, useQuery, useMutation (~2062 tok)
- `ApprovalInstance.tsx` — ApprovalInstance — renders table — uses useNavigate, useState, useQuery, useMutation (~2166 tok)
- `ApprovalStats.tsx` — ApprovalStats — renders table, chart — uses useQuery, useMutation (~2218 tok)
- `ApprovalTemplate.tsx` — ApprovalTemplate — renders table, modal — uses useState, useQuery, useMutation (~2315 tok)
- `Attendance.tsx` — Attendance — renders table, modal — uses useState, useQuery, useMutation (~2924 tok)
- `AttendanceExport.tsx` — AttendanceExport — renders table, modal — uses useState, useQuery, useMutation (~2532 tok)
- `AttendanceStats.tsx` — AttendanceStats — renders table — uses useState, useQuery, useMutation (~3076 tok)
- `AuditLogs.tsx` — AuditLogs — renders table — uses useState, useQuery (~2141 tok)
- `Callback.tsx` — isDingTalkEnv — uses useState, useNavigate, useSearchParams, useEffect (~712 tok)
- `DataPermission.tsx` — DataPermission — renders form — uses useState, useQuery, useEffect (~2425 tok)
- `DepartmentTree.tsx` — departmentEmployeePageSize — uses useNavigate, useState, useEffect, useMemo (~5856 tok)
- `EmployeeDetail.tsx` — employmentTypeOptions (~6505 tok)
- `EmployeeFlow.tsx` — trimText — uses useState, useQuery, useMemo, useMutation (~9455 tok)
- `EmployeeList.tsx` — emptySummary — renders table — uses useNavigate, useState, useMemo, useEffect (~3447 tok)
- `EmployeeProfile.tsx` — employmentTypeOptions — renders table, modal — uses useState, useQuery, useMemo, useMutation (~5341 tok)
- `EmployeeShiftConfig.tsx` — unwrapEnvelope — uses useState, useQuery, useMemo, useMutation (~5560 tok)
- `Home.tsx` — statCards — uses useNavigate, useQuery (~3253 tok)
- `LeaveOvertime.tsx` — formatWorkingYears — renders form, table, modal (~10072 tok)
- `Log.tsx` — Log — renders table — uses useQuery (~1114 tok)
- `Login.tsx` — isDingTalkEnv — uses useState, useSearchParams, useEffect (~1922 tok)
- `LoginError.tsx` — LoginError — uses useNavigate, useSearchParams (~273 tok)
- `MenuPermission.tsx` — MenuPermission — uses useQuery, useEffect (~1858 tok)
- `Organization.tsx` — connectedEntries — uses useNavigate, useQuery (~2080 tok)
- `PerformanceGoalSetting.tsx` — PerformanceGoalSetting — uses useNavigate, useState, useCallback, useEffect (~7105 tok)
- `PerformanceIndicatorLibrary.tsx` — isValidWeight (~11071 tok)
- `PerformanceManagerEval.tsx` — LEVEL_OPTIONS — renders form (~6348 tok)
- `PerformanceOverview.tsx` — normalizeIDArray — renders chart (~18500 tok)
- `PerformanceResultView.tsx` — LEVEL_COLOR — renders table (~10509 tok)
- `PerformanceSelfEval.tsx` — PerformanceSelfEval — renders form, table (~3089 tok)
- `Permission.tsx` — fetchRoles — renders form, table, modal — uses useState, useQuery (~1773 tok)
- `RoleManagement.tsx` — RoleManagement (~9726 tok)
- `Setting.tsx` — Setting — renders form — uses useState, useQuery (~1459 tok)
- `SyncJobs.tsx` — SyncJobs — renders table — uses useQuery, useMutation (~1295 tok)
- `SyncLog.tsx` — 格式化时间函数 (~1542 tok)
- `TalentAnalysis.tsx` — TalentAnalysis — renders table, chart — uses useState, useForm, useQuery, useMutation (~4880 tok)
- `WeekSchedule.tsx` — unwrapData — uses useState, useQuery, useEffect, useMemo (~13733 tok)

## frontend/src/services/

- `api.mock.ts` — 模拟API响应 (~3093 tok)
- `api.ts` — API routes: GET, POST, PUT, DELETE (97 endpoints) (~11253 tok)

## frontend/src/store/

- `authStore.ts` — Exports useAuthStore (~280 tok)

## frontend/src/utils/

- `authFileUrl.ts` — Exports withFileAccessToken (~115 tok)
- `delay.ts` — 延迟函数，用于模拟API响应延迟 (~39 tok)
- `format.ts` — 周期类型中文映射 (~162 tok)
- `permission.ts` — Exports hasPermission, hasMenuPermission (~156 tok)

## frontend/test-results/

- `.last-run.json` (~13 tok)

## internal/api/

- `handlers.go` — Response (133 fields) (~32917 tok)
- `leave_handlers.go` — GetLeaveEligibility, RecalculateLeaveEligibility, GetLeaveGrants, RunQuarterGrant + 11 more (~5603 tok)
- `performance_handlers.go` — GetPerformanceActivities, CreatePerformanceActivity, GetPerformanceActivity, UpdatePerformanceActivity (~25280 tok)
- `router.go` — SetupRouter (~7919 tok)
- `supplementary_handlers.go` — SubmitSupplementaryClockIn, ApproveSupplementaryClockIn, GetSupplementaryRequests, SyncSupplementaryFromDingTalk (~1374 tok)

## internal/cache/

- `redis.go` — Init, Set, Get, Delete, Exists (~379 tok)

## internal/config/

- `config.go` — Config (23 fields) (~430 tok)
- `holidays.json` (~1184 tok)

## internal/database/

- `database.go` — col (120 fields) (~8557 tok)
- `models.go` — User (161 fields) (~11742 tok)
- `performance_models.go` — PerformanceTemplate (143 fields) (~6298 tok)

## internal/dingtalk/

- `dingtalk.go` — attendanceGroupCache (127 fields) (~25084 tok)

## internal/middleware/

- `jwt.go` — Claims (25 fields) (~589 tok)
- `rbac.go` — RequirePermission, RequirePermissionOrMenu, RequireMenuPermission (~745 tok)

## internal/repository/

- `annual_leave_eligibility_repository.go` — AnnualLeaveEligibilityRepository (10 fields); methods: FindByUserYear, FindByUserYearQuarter, Upsert, FindEligibleByYear (~484 tok)
- `annual_leave_grant_repository.go` — AnnualLeaveGrantRepository (23 fields); methods: FindByUserYear, FindByUserYearQuarterType, FindGrantsWithRemaining, FindFailedSyncGrants (~1003 tok)
- `approval_repository.go` — ApprovalRepository (30 fields); methods: Create, FindByID, FindAll, Create (~706 tok)
- `attendance_repository.go` — AttendanceRepository (36 fields); methods: Create, Upsert, FindByID, FindAll (~968 tok)
- `audit_repository.go` — AuditRepository (17 fields); methods: Create, FindAll (~412 tok)
- `compensatory_leave_ledger_repository.go` — CompensatoryLeaveLedgerRepository (24 fields); methods: FindByUser, FindBySourceMatch, FindBySourceMatchKey, GetBalance (~904 tok)
- `department_repository.go` — DepartmentRepository (45 fields); methods: Create, Update, Delete, FindByDepartmentID (~866 tok)
- `employee_repository.go` — EmployeeRepository (117 fields); methods: CreateProfile, UpdateProfile, FindProfileByID, FindProfileByUserID (~5125 tok)
- `filter_helpers.go` (~92 tok)
- `leave_rule_config_repository.go` — LeaveRuleConfigRepository (13 fields); methods: FindActiveByType, FindByKey, Upsert (~406 tok)
- `overtime_match_result_repository.go` — OvertimeMatchResultRepository (16 fields); methods: FindByApprovalID, FindByUserAndWorkDate, FindByUserDateRange, FindByDateRange (~717 tok)
- `overtime_rule_config_repository.go` — OvertimeRuleConfigRepository (13 fields); methods: FindActiveAll, FindByKey, Upsert (~399 tok)
- `performance_goal_approval_repository.go` — PerformanceGoalApprovalRepository (14 fields); methods: Create, FindByParticipant, FindByGoalRecord, GetLatestByParticipant (~444 tok)
- `performance_goal_record_repository.go` — PerformanceGoalRecordRepository (23 fields); methods: GetByID, FindByParticipant, FindByActivity, FindByActivityAndParticipant (~888 tok)
- `performance_indicator_repository.go` — PerformanceIndicatorLibraryRepository (49 fields); methods: Create, GetByID, Update, Delete (~1640 tok)
- `performance_repository.go` — PerformanceActivityRepository (131 fields); methods: Create, GetByID, Update, UpdateStatus (~6306 tok)
- `role_repository.go` — RoleRepository (61 fields); methods: Create, Update, FindAll, FindAll (~2007 tok)
- `shift_config_repository.go` — ShiftConfigRepository (9 fields); methods: FindAll, FindByUserID, Upsert, DeleteByUserID (~349 tok)
- `supplementary_request_repository.go` — SupplementaryRequestRepository (22 fields); methods: Create, FindByMatchResultID, FindPendingByMatchResultID, FindByUserID (~899 tok)
- `sync_repository.go` — SyncRepository (8 fields); methods: Upsert, FindByType, FindAll (~267 tok)
- `talent_repository.go` — TalentRepository (14 fields); methods: Create, FindByID, FindAll (~332 tok)
- `user_repository.go` — UserRepository (47 fields); methods: Create, Update, Delete, FindByUserID (~1146 tok)
- `week_schedule_repository.go` — WeekScheduleRepository (50 fields); methods: CreateRule, UpdateRule, DeleteRule, FindRuleByID (~1722 tok)

## internal/service/

- `annual_leave_grant_service.go` — AnnualLeaveGrantService (113 fields); methods: GrantQuarter, GrantQuarterWithResult, GrantForUser, GrantForUserWithResult (~4496 tok)
- `annual_leave_service.go` — AnnualLeaveService (55 fields); methods: RecalculateEligibility, RecalculateEligibilityBatch, GetEligibility (~1525 tok)
- `approval_service.go` — ApprovalService (5 fields); methods: GetTemplates, GetInstances, GetByID (~247 tok)
- `attendance_record_filter.go` (~438 tok)
- `attendance_rule_engine.go` — attendanceSchedule (125 fields); methods: CalculateAttendance, AggregateAttendance (~3434 tok)
- `attendance_service.go` — Interface: attendanceRepository (20 methods) (~3154 tok)
- `audit_service.go` — AuditService (4 fields); methods: GetLogs, CreateLog (~168 tok)
- `compensatory_leave_service.go` — CompensatoryLeaveService (39 fields); methods: GetBalance, GetOvertimeBalanceByYear, CreditFromOvertime, RollbackCredit (~1169 tok)
- `department_service.go` — DepartmentService (12 fields); methods: CreateDepartment, UpdateDepartment, DeleteDepartment, GetDepartmentByDepartmentID (~460 tok)
- `employee_service.go` — EmployeeService (14 fields); methods: GetProfiles, GetLifecycleLedger, GetProfileByID, GetProfileByUserID (~709 tok)
- `filter_helpers.go` (~92 tok)
- `leave_jobs.go` — LeaveJobScheduler (55 fields); methods: Start, RunManualEligibilityRecalc, RunManualOvertimeMatch, SeedDefaultRules (~2439 tok)
- `org_service.go` — OrgDataScope (163 fields); methods: IsAll, IsSelf, AllowsDepartment, AllowsUser (~16703 tok)
- `overtime_matching_service.go` — OvertimeMatchingService (92 fields); methods: MatchApprovedOvertime, MatchApprovedOvertimeForUser, MatchApproval, MatchApprovalWithForce (~12935 tok)
- `performance_indicator_service.go` — PerformanceIndicatorService (43 fields); methods: CreateLibrary, GetLibrary, UpdateLibrary, ListLibraries (~1768 tok)
- `performance_service.go` — PerformanceService (99 fields); methods: CreateActivity, UpdateActivity, GetActivity, PublishActivity (~26333 tok)
- `permission_service.go` — PermissionService (107 fields); methods: EnsureSystemPermissions, GetRoles, CreateRole, UpdateRole (~4609 tok)
- `scoring_engine.go` — AutoScoreResult (75 fields) (~1954 tok)
- `shift_config_service.go` — ShiftConfigService (153 fields); methods: GetAllWithUsers, SetConfigs, DeleteConfig, GetOrCreateShift (~6323 tok)
- `sync_service.go` — SyncService (5 fields); methods: GetSyncStatus, GetAllSyncStatus, UpdateSyncStatus (~223 tok)
- `talent_service.go` — TalentService (5 fields); methods: GetList, GetByID, Create (~207 tok)
- `user_service.go` — UserService (16 fields); methods: CreateUser, UpdateUser, DeleteUser, GetUserByUserID (~565 tok)
- `week_schedule_service.go` — WeekInfo (113 fields); methods: CreateRule, UpdateRule, DeleteRule, GetRuleByID (~9273 tok)

## scripts/

- `build.bat` (~94 tok)
- `run.bat` (~33 tok)

## tools/

- `install-hooks.sh` — 将 tools/hooks/pre-commit 安装到 .git/hooks/ (~108 tok)

## tools/hooks/

- `pre-commit` — 检测结构性变更时提醒更新 CLAUDE.md (~276 tok)

## tools/ops/resync_comp_time/

- `main.go` (~832 tok)

## tools/reset_vacation_quota/

- `main.go` (~635 tok)

## tools/resync_overtime_to_dingtalk/

- `main.go` — 用法（在项目根目录执行）： (~1101 tok)

## tools/set_comp_time_balance/

- `main.go` (~387 tok)

## tools/setup/create_freedom_leave/

- `main.go` — go:build ignore (~437 tok)

## tools/setup/create_vacation/

- `main.go` — go:build ignore (~175 tok)
