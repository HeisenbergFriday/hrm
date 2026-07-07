# anatomy.md

> Auto-maintained by OpenWolf. Last scanned: 2026-07-07T10:31:32.049Z
> Files: 88 tracked | Anatomy hits: 0 | Misses: 0

## ./

- `绩效service测试覆盖率补强报告.md` — 绩效 Service 测试覆盖率补强报告 (~1071 tok)
- `快速操作清单.md` — 钉钉登录修复 - 快速操作清单 (~764 tok)
- `FINAL_DEPLOYMENT_SUMMARY.md` — PeopleOps 最终部署总结 (~1421 tok)

## .ai/

- `ARCHITECTURE.md` — 架构设计；JWT org_id 与 UserRole 企业隔离约定 (~1589 tok)
- `COMMANDS.md` — 常用命令 (~690 tok)
- `PROJECT_MAP.md` — 项目结构索引 (~3082 tok)

## .ai/MODULES/


## .ai/plans/


## .claude/


## .claude/rules/


## .github/workflows/

- `ci.yml` — CI: CI (~369 tok)

## C:/Users/吴列德/.claude/plans/

- `cosmic-jingling-acorn.md` — 修复绩效模块 Playwright E2E 执行计划 (~1113 tok)
- `eager-wibbling-music.md` — 计划：补强绩效 API handler 测试覆盖 (~1834 tok)
- `humming-booping-planet.md` — 绩效 Service 覆盖率补强计划 (~1594 tok)
- `jazzy-napping-nova.md` — 集成 D:\app Excel 工具六个 Tab 到 HR Web 系统实施计划 (~2179 tok)
- `peppy-wibbling-liskov.md` — 补强绩效 API handler 覆盖测试计划 (~1366 tok)
- `replicated-plotting-finch.md` — Plan: 为 PerformanceOverview 补充真实组件交互测试 (~596 tok)
- `toasty-dancing-simon.md` — 多企业钉钉登录与权限上下文修复计划 (~1626 tok)
- `witty-greeting-blanket.md` — 为 `performance_repository.go` 补充测试覆盖计划 (~1207 tok)

## api-docs/


## cmd/


## deploy/

- `本地测试报告.md` — 本地多企业支持测试指南 (~1049 tok)
- `测试服务器部署步骤.md` — 测试服务器部署步骤 (~1239 tok)
- `多企业支持-实施总结.md` — 多企业支持实施总结 (~1089 tok)
- `多企业支持实施指南.md` — 多企业支持实施指南 (~1157 tok)
- `build-and-deploy.ps1` — Declares Write (~1149 tok)
- `deploy-via-tar.ps1` — Declares Write (~1081 tok)
- `deploy.sh` — 钉钉登录修复 - 自动部署脚本 (~723 tok)
- `setup-multitenant.ps1` — Declares Write (~1389 tok)
- `update.ps1` — Declares Write (~1138 tok)

## docs/

- `钉钉登录问题修复总结.md` — 钉钉登录问题修复总结 (~1182 tok)
- `钉钉登录问题诊断指南.md` — 钉钉登录问题诊断指南 (~819 tok)

## frontend/

- `package.json` — Node.js package manifest (~505 tok)
- `playwright.config.ts` — e2e 使用专用端口，避免与本地 `npm run dev`（3000）冲突， (~621 tok)
- `tsconfig.json` — TypeScript configuration (~204 tok)
- `vite.config.test.ts` — /*.{test,spec}.{ts,tsx}'], (~132 tok)

## frontend/playwright-report/


## frontend/scripts/


## frontend/src/

- `App.tsx` — Login (~5525 tok)
- `main.tsx` — queryClient (~180 tok)

## frontend/src/components/

- `PerformanceActivityEditor.interaction.test.tsx` — PerformanceActivityEditor 组件交互测试 (~2378 tok)
- `PerformanceActivityEditor.test.ts` — 测试辅助函数 (~1656 tok)
- `PerformanceActivityEditor.tsx` — activitySections — renders form (~6109 tok)

## frontend/src/config/

- `menu.tsx` — menuPermissionKey (~1742 tok)

## frontend/src/pages/

- `Callback.tsx` — isDingTalkEnv (~778 tok)
- `Login.tsx` — isDingTalkEnv — renders modal (~3040 tok)
- `MenuPermission.tsx` — MenuPermission (~1791 tok)
- `PerformanceGoalSetting.interaction.test.tsx` — PerformanceGoalSetting 组件交互测试 (~3720 tok)
- `PerformanceGoalSetting.test.ts` — 测试辅助函数 (~1986 tok)
- `PerformanceGoalSetting.tsx` — targetReadonlyParticipantStatuses (~8009 tok)
- `PerformanceIndicatorLibrary.test.ts` — 测试辅助函数 (~951 tok)
- `PerformanceManagerEval.interaction.test.tsx` — PerformanceManagerEval 组件交互测试 (~2978 tok)
- `PerformanceManagerEval.test.ts` — 测试辅助函数 (~1332 tok)
- `PerformanceManagerEval.tsx` — LEVEL_OPTIONS — renders form (~6289 tok)
- `PerformanceOverview.interaction.test.tsx` — PerformanceOverview 组件交互测试 (~7783 tok)
- `PerformanceOverview.test.ts` — ==================== 辅助函数测试 ==================== (~8036 tok)
- `PerformanceOverview.tsx` — normalizeIDArray (~24988 tok)
- `PerformanceResultView.interaction.test.tsx` — PerformanceResultView 组件交互测试 (~3867 tok)
- `PerformanceResultView.test.ts` — 测试辅助函数 (~2199 tok)
- `PerformanceResultView.tsx` — LEVEL_COLOR — renders table (~10317 tok)
- `PerformanceSelfEval.interaction.test.tsx` — PerformanceSelfEval 组件交互测试；提交失败场景用 fireEvent 快速填表避免超时 (~2804 tok)
- `PerformanceSelfEval.test.ts` — 测试辅助函数 (~1322 tok)
- `PerformanceSelfEval.tsx` — PerformanceSelfEval — renders form, table (~3189 tok)

## frontend/src/services/


## frontend/src/store/

- `authStore.ts` — Exports useAuthStore (~338 tok)

## frontend/src/test/

- `setup.ts` — Vitest/jsdom 全局清理与浏览器 API polyfills；requestAnimationFrame 使用 window.setTimeout 返回 number (~758 tok)

## frontend/src/utils/

- `org.ts` — 解析当前应该使用的 org_id： (~560 tok)
- `performanceHelpers.test.ts` — Declares res (~2969 tok)
- `performanceHelpers.ts` — Exports STATUS_MAP, PARTICIPANT_STATUS_MAP, ACTIVITY_FLOW, normalizeIDArray + 23 more (~2660 tok)

## frontend/test-results/


## frontend/tests/e2e/

- `performance.spec.ts` — ', async (route) => { (~7573 tok)
- `README.md` — Project documentation (~489 tok)
- `warning-probe.spec.ts` — ', async route => { (~933 tok)

## internal/api/

- `handlers.go` — Struct: Response (~33582 tok)
- `performance_handlers_coverage_test.go` — TestRefreshPerformanceParticipants_InvalidActivityID, TestRefreshPerformanceParticipants_Success, Te (~19426 tok)
- `performance_handlers_test.go` — TestCreatePerformanceActivityHandlerMissingRequired, TestUpdatePerformanceActivityHandlerMissingRequ (~9419 tok)
- `performance_handlers.go` (~35730 tok)

## internal/cache/


## internal/config/


## internal/database/

- `database.go` — Struct: col (~11540 tok)
- `models.go` — Struct: User (~10924 tok)
- `organization_models.go` — Struct: Organization (~556 tok)
- `organization_service.go` — GetOrgIDByCorpID, GetOrganizationByOrgID, GetOrganizationByCorpID (~278 tok)

## internal/dingtalk/

- `dingtalk.go` — Struct: AppConfig (~30081 tok)

## internal/middleware/

- `auth_context.go` — Struct: AuthContext (~1934 tok)
- `jwt.go` — Struct: Claims (~549 tok)

## internal/repository/

- `performance_goal_approval_repository_test.go` — Struct: stubGoalApprovalQueryResponse (~3629 tok)
- `performance_goal_record_repository_test.go` — Struct: stubGoalRecordQueryResponse (~4853 tok)
- `performance_indicator_repository_test.go` — Struct: stubIndicatorQueryResponse (~6815 tok)
- `performance_repository_coverage_test.go` — TestActivityRepo_FindAll_DateFiltersBuildExpectedQuery, TestActivityRepo_FindAll_CountError, TestAct (~12186 tok)
- `performance_repository_test.go` — Struct: stubPerformanceQueryResponse (~22562 tok)
- `role_repository.go` — Struct: RoleRepository (~2002 tok)
- `user_repository.go` — Struct: UserRepository (~1288 tok)

## internal/service/

- `performance_indicator_service_test.go` — TestCreateLibrary_Validation, TestCreateLibrary_Success, TestGetLibrary, TestUpdateLibrary_NotFound, (~4958 tok)
- `performance_lifecycle_test.go` — TestPerformanceFullLifecycle_HappyPath (~6199 tok)
- `performance_service_coverage_test.go` — TestUpdateActivity_NotFound, TestUpdateActivity_DraftAllowsScopeChange, TestUpdateActivity_NonDraftR (~9143 tok)
- `performance_service_extended_test.go` — TestGetActivityReturnsActivity, TestGetActivityNotFound, TestGetParticipantReturnsParticipant, TestG (~28236 tok)
- `permission_service.go` — Struct: PermissionService (~6424 tok)
- `user_service.go` — Struct: UserService (~681 tok)

## scripts/


## tools/


## tools/hooks/


## tools/migrate_multitenant/

- `main.go` (~2143 tok)

## tools/ops/resync_comp_time/


## tools/reset_vacation_quota/


## tools/resync_overtime_to_dingtalk/


## tools/set_comp_time_balance/


## tools/setup/create_freedom_leave/


## tools/setup/create_vacation/

