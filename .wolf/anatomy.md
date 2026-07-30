# anatomy.md

> Auto-maintained by OpenWolf. Last scanned: 2026-07-28T10:16:21.773Z
> Files: 94 tracked | Anatomy hits: 0 | Misses: 0

## ../complaint-rate-alert/


## ../complaint-rate-alert/app/


## ../complaint-rate-alert/configs/


## ../complaint-rate-alert/scripts/


## ../complaint-rate-alert/sql/


## ../complaint-rate-alert/static/


## ../complaint-rate-alert/tests/


## ./

- `.tmp_check_bundle.sh` (~306 tok)
- `.tmp_check_handlelogout.sh` (~350 tok)
- `.tmp_dingtalk_list_all_names.py` — -*- coding: utf-8 -*- (~698 tok)
- `.tmp_dingtalk_name_lookup.py` — -*- coding: utf-8 -*- (~1438 tok)
- `.tmp_dingtalk_process_check.py` — One-off readonly DingTalk process/permission check. Secrets stay in env file only. (~2895 tok)
- `.tmp_dingtalk_process_check2.py` — Expand process list names for leave/overtime/correction/trip/out and check more permissions. (~2800 tok)
- `.tmp_find_models.py` — extract_writes (~892 tok)
- `.tmp_fix_role_org.py` — -*- coding: utf-8 -*- (~3067 tok)
- `.tmp_logout_csrf_smoke.sh` (~826 tok)
- `.tmp_recover_stream.py` — strip_line_prefix, walk (~1433 tok)
- `.tmp_restore_dingtalk.py` — Declares Config (~3309 tok)
- `.tmp_schema_names.py` — -*- coding: utf-8 -*- (~832 tok)

## .ai/

- `DESIGN_SYSTEM.md` — 设计规范 (~2877 tok)

## .ai/MODULES/

- `approval.md` — 审批模块 (~713 tok)
- `attendance.md` — 考勤模块 (~3853 tok)
- `auth.md` — 认证模块 (~2909 tok)
- `org.md` — 组织架构模块 (~2735 tok)
- `shift-config.md` — 员工下班时间配置 (~532 tok)

## .ai/plans/


## .claude/


## .claude/rules/


## .github/workflows/


## C:/Users/吴列德/.claude/plans/


## C:/Users/吴列德/.claude/projects/d--AITEAM-HR/memory/


## api-docs/


## cmd/

- `main.go` (~436 tok)

## cmd/dingtalk_stream/


## deploy/

- `peopleops.env.example` (~822 tok)

## docs/

- `DEVELOPMENT_ISSUES.md` — 开发问题复盘日志 (~8237 tok)

## frontend/

- `package.json` — Node.js package manifest (~519 tok)

## frontend/playwright-report/


## frontend/scripts/

- `check-user-facing-en.mjs` — 面向用户文案的中文化护栏：扫描 src 下 tsx/ts，拦截"会显示给用户的纯英文文案"。 (~860 tok)

## frontend/src/

- `App.tsx` — 全局中文 locale：强制 Table/Empty 空状态为中文，避免个别版本回退到 "No data" (~8128 tok)
- `main.tsx` (~172 tok)
- `queryClient.ts` — 单例 QueryClient：登出/换组织时必须 clear，避免跨用户/跨 org 缓存泄漏 (~44 tok)

## frontend/src/components/


## frontend/src/config/


## frontend/src/pages/

- `Approval.tsx` — fetchApprovals — renders table (~1449 tok)
- `ApprovalDetail.tsx` — ApprovalDetail (~2950 tok)
- `ApprovalInstance.test.tsx` — mockGetInstances (~688 tok)
- `ApprovalInstance.tsx` — ApprovalInstance — renders table (~2714 tok)
- `ApprovalStats.tsx` — ApprovalStats — renders table (~2232 tok)
- `Attendance.tsx` — BUSINESS_TIMEZONE_OFFSET_MINUTES (~7809 tok)
- `AttendanceExport.tsx` — AttendanceExport — renders table, modal (~2649 tok)
- `AttendanceProcessing.tsx` — processingTabs (~2490 tok)
- `AuditLogs.tsx` — AuditLogs — renders table (~2214 tok)
- `DepartmentTree.tsx` — departmentEmployeePageSize (~5933 tok)
- `EmployeeDetail.tsx` — employmentTypeOptions (~7535 tok)
- `EmployeeFlow.tsx` — trimText (~9430 tok)
- `EmployeeList.tsx` — emptySummary (~4451 tok)
- `EmployeeProfile.tsx` — missingUserManageTip — renders table, modal (~5438 tok)
- `EmployeeShiftConfig.tsx` — unwrapEnvelope (~5573 tok)
- `Log.tsx` — Log — renders table (~1187 tok)
- `Login.tsx` — isDingTalkEnv (~4141 tok)
- `LoginError.tsx` — LoginError (~284 tok)
- `OAApprovalData.tsx` — display (~4151 tok)
- `PerformanceResultView.tsx` — LEVEL_COLOR — renders table (~16914 tok)
- `Setting.tsx` — Setting (~1481 tok)
- `SyncJobs.tsx` — 将列表中的任务 id/type 映射到真实同步调用（避免 RunJob 仅改状态的假成功） (~1609 tok)
- `SyncLog.tsx` — SyncLog — renders table (~1599 tok)
- `WeekSchedule.tsx` — 分页拉全量用户，避免 page_size 被后端截断后只能看到前 100 人 (~17988 tok)

## frontend/src/services/

- `api.ts` — 组织全量同步响应（POST /org/sync） (~20392 tok)

## frontend/src/store/

- `authStore.ts` — Exports useAuthStore (~333 tok)

## frontend/src/test/


## frontend/src/utils/

- `loginErrorMessage.ts` — 登录错误码/短词白名单 → 中文文案；未知内容不原样展示（防钓鱼） (~415 tok)
- `maskPii.ts` — 手机号脱敏：保留前 3 后 4；短号只显示末 2 位 (~342 tok)
- `orgSyncAction.test.ts` — Declares err (~2431 tok)
- `orgSyncAction.ts` — 是否正在同步中（防重复点击） (~1325 tok)

## frontend/test-results/


## frontend/tests/e2e/


## internal/api/

- `approval_sync_handlers.go` — GetAttendanceApprovalSyncList, GetAttendanceApprovalSyncDetail, GetAttendanceApprovalSyncFailures, R (~1889 tok)
- `handlers_dingtalk_login_test.go` — TestGenerateAndValidateLoginStateKeepsUnscopedQRLogin, TestGenerateAndValidateLoginStateKeepsOAuthOr (~2722 tok)
- `handlers.go` — Struct: orgSyncStatusUpdate (~59193 tok)
- `router.go` — SetupRouter (~10209 tok)
- `sync_org_data_test.go` — Struct: orgSyncTestEnvelope (~15265 tok)

## internal/cache/


## internal/config/


## internal/database/

- `database.go` — Struct: envOrganization (~26446 tok)
- `liede_admin_role_org_isolation_test.go` — TestEnsureRolePresetInOrg_DoesNotReuseOtherOrgRole, TestEnsureUserRoleInOrg_RejectsCrossOrgRole, Tes (~2265 tok)
- `models.go` — Struct: Organization (~15069 tok)
- `org_unique_index_migration_test.go` — Struct: orgUniqueTestState (~8062 tok)
- `organization_process_codes_test.go` — TestOrganizationExtensionWithDingTalkProcessCodesPreservesExistingValues, TestOrganizationFromEnvCon (~315 tok)
- `process_codes_test.go` — TestNormalizeAndValidateDingTalkProcessCodesFiveKeys, TestValidateDingTalkProcessCodesAllowsMissingT (~982 tok)
- `process_codes.go` — IsKnownApprovalBusinessKey, IsPrimaryApprovalBusinessKey, OrganizationDingTalkProcessCodes, Validate (~2206 tok)
- `schema_expand_migrations.go` — MigrateAnnualLeaveConsumeLogSchema, MigrateShiftCatalogSchema, MigratePerformanceParticipantOrgIDsFr (~2448 tok)

## internal/dingtalk/

- `approval_process_list_test.go` — TestParseApprovalProcessTemplates, TestListManageableApprovalProcessesForOrg_Pagination, TestListMan (~1935 tok)
- `approval_process_list.go` — Struct: ApprovalProcessTemplate (~1980 tok)
- `dingtalk_test.go` — TestBuildCorpMessagePayloadUsesAsyncSendSchema, TestBuildCorpImagePayloadUsesImageMsgTypeAndMediaID, (~1222 tok)
- `dingtalk.go` — Struct: AppConfig (~39395 tok)
- `notifiable_user_org_test.go` — TestIsNotifiableUserIDForOrg_IsolatesSameUserIDAcrossOrgs, TestIsNotifiableUserIDForOrg_FailClosedOn (~751 tok)

## internal/middleware/


## internal/repository/

- `approval_repository_security_test.go` — TestMergeApprovalExtensionAppliesPatchWithoutDroppingExistingFields, TestApprovalUpsertLookupUsesOrg (~1223 tok)
- `approval_repository.go` — Struct: ApprovalRepository (~2496 tok)
- `approval_sync_failure_repository.go` — Struct: ApprovalSyncFailureRepository (~1855 tok)
- `user_repository_deactivate_missing_test.go` — TestDeactivateUsersMissingFromDingTalk_DeactivatesHistoricalEmployees, TestDeactivateUsersMissingFro (~2572 tok)
- `user_repository.go` — Struct: UserRepository (~2370 tok)

## internal/requestmeta/


## internal/service/

- `approval_category.go` — ParseApprovalCategory, ApprovalCategoryTitleKeywords, AllApprovalCategoryTitleKeywords (~542 tok)
- `approval_service.go` — Struct: ApprovalService (~436 tok)
- `approval_sync_core_test.go` — TestMergeApprovalStatusMonotonic, TestBuildApprovalFromDetailMapsBusinessType, TestBuildApprovalFrom (~1033 tok)
- `approval_sync_core.go` — Struct: ApprovalDetailBuildInput (~2008 tok)
- `approval_sync_service.go` — Struct: ApprovalSyncService (~4455 tok)
- `attendance_service.go` — Interface: attendanceRepository (~3470 tok)
- `attendance_toolbox_compare_test.go` — TestCompareAppSourceScript_Live, TestCompareAppSourceUnitTests (~971 tok)
- `attendance_toolbox_service.go` — Struct: AttendanceToolboxService (~8278 tok)
- `dingtalk_stream_service.go` — Interface: dingTalkEventStore (~3440 tok)
- `org_service.go` — Struct: OrgDataScope (~16873 tok)
- `shift_config_service.go` — Struct: ShiftConfigService (~6333 tok)
- `week_schedule_jobs.go` — Struct: WeekScheduleJobScheduler (~1899 tok)
- `week_schedule_service.go` — Struct: WeekInfo (~12264 tok)

## internal/tenant/registry/


## scripts/


## tools/


## tools/_tmp_hash/


## tools/attendance-processing/


## tools/attendance_toolbox/python/


## tools/attendance_toolbox/python/finally/


## tools/attendance_toolbox/python/leave/


## tools/attendance_toolbox/python/overtime/


## tools/attendance_toolbox/python/scripts/


## tools/attendance_toolbox/python/subsidy/


## tools/hooks/


## tools/migrate_multitenant/


## tools/ops/


## tools/ops/dingtalk_attendance_preflight/


## tools/ops/push_week_schedule_personal/

- `main.go` — Struct: exportDay (~1951 tok)
- `render_calendar.py` — Render month schedule PNG from real week-schedule JSON (DB-driven). (~1136 tok)

## tools/ops/resync_comp_time/


## tools/org_unique_drill/


## tools/reset_vacation_quota/


## tools/resync_overtime_to_dingtalk/


## tools/set_comp_time_balance/


## tools/setup/create_freedom_leave/


## tools/setup/create_vacation/

