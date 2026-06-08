# anatomy.md

> Auto-maintained by OpenWolf. Last scanned: 2026-06-08T09:30:47.348Z
> Files: 52 tracked | Anatomy hits: 0 | Misses: 0

## ./

- `绩效service测试覆盖率补强报告.md` — 绩效 Service 测试覆盖率补强报告 (~1071 tok)

## .ai/

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
- `peppy-wibbling-liskov.md` — 补强绩效 API handler 覆盖测试计划 (~1366 tok)
- `replicated-plotting-finch.md` — Plan: 为 PerformanceOverview 补充真实组件交互测试 (~596 tok)
- `witty-greeting-blanket.md` — 为 `performance_repository.go` 补充测试覆盖计划 (~1207 tok)

## api-docs/


## cmd/


## frontend/

- `package.json` — Node.js package manifest (~505 tok)
- `playwright.config.ts` — e2e 使用专用端口，避免与本地 `npm run dev`（3000）冲突， (~621 tok)
- `tsconfig.json` — TypeScript configuration (~204 tok)
- `vite.config.test.ts` — /*.{test,spec}.{ts,tsx}'], (~132 tok)

## frontend/playwright-report/


## frontend/scripts/


## frontend/src/

- `App.tsx` — Login (~5529 tok)
- `main.tsx` — queryClient (~180 tok)

## frontend/src/components/

- `PerformanceActivityEditor.interaction.test.tsx` — PerformanceActivityEditor 组件交互测试 (~2378 tok)
- `PerformanceActivityEditor.test.ts` — 测试辅助函数 (~1656 tok)
- `PerformanceActivityEditor.tsx` — activitySections — renders form (~6109 tok)

## frontend/src/config/


## frontend/src/pages/

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
- `PerformanceSelfEval.interaction.test.tsx` — PerformanceSelfEval 组件交互测试 (~2797 tok)
- `PerformanceSelfEval.test.ts` — 测试辅助函数 (~1322 tok)
- `PerformanceSelfEval.tsx` — PerformanceSelfEval — renders form, table (~3189 tok)

## frontend/src/services/


## frontend/src/store/


## frontend/src/test/

- `setup.ts` — 每个用例结束后自动卸载组件，避免跨用例 DOM 残留导致随机失败 (~743 tok)

## frontend/src/utils/

- `performanceHelpers.test.ts` — Declares res (~2969 tok)
- `performanceHelpers.ts` — Exports STATUS_MAP, PARTICIPANT_STATUS_MAP, ACTIVITY_FLOW, normalizeIDArray + 23 more (~2660 tok)

## frontend/test-results/


## frontend/tests/e2e/

- `performance.spec.ts` — ', async (route) => { (~7573 tok)
- `README.md` — Project documentation (~489 tok)
- `warning-probe.spec.ts` — ', async route => { (~933 tok)

## internal/api/

- `performance_handlers_coverage_test.go` — TestRefreshPerformanceParticipants_InvalidActivityID, TestRefreshPerformanceParticipants_Success, Te (~19426 tok)
- `performance_handlers_test.go` — TestCreatePerformanceActivityHandlerMissingRequired, TestUpdatePerformanceActivityHandlerMissingRequ (~9419 tok)

## internal/cache/


## internal/config/


## internal/database/


## internal/dingtalk/


## internal/middleware/


## internal/repository/

- `performance_goal_approval_repository_test.go` — Struct: stubGoalApprovalQueryResponse (~3629 tok)
- `performance_goal_record_repository_test.go` — Struct: stubGoalRecordQueryResponse (~4853 tok)
- `performance_indicator_repository_test.go` — Struct: stubIndicatorQueryResponse (~6815 tok)
- `performance_repository_coverage_test.go` — TestActivityRepo_FindAll_DateFiltersBuildExpectedQuery, TestActivityRepo_FindAll_CountError, TestAct (~12186 tok)
- `performance_repository_test.go` — Struct: stubPerformanceQueryResponse (~22562 tok)

## internal/service/

- `performance_indicator_service_test.go` — TestCreateLibrary_Validation, TestCreateLibrary_Success, TestGetLibrary, TestUpdateLibrary_NotFound, (~4958 tok)
- `performance_lifecycle_test.go` — TestPerformanceFullLifecycle_HappyPath (~6199 tok)
- `performance_service_coverage_test.go` — TestUpdateActivity_NotFound, TestUpdateActivity_DraftAllowsScopeChange, TestUpdateActivity_NonDraftR (~9143 tok)
- `performance_service_extended_test.go` — TestGetActivityReturnsActivity, TestGetActivityNotFound, TestGetParticipantReturnsParticipant, TestG (~28236 tok)

## scripts/


## tools/


## tools/hooks/


## tools/ops/resync_comp_time/


## tools/reset_vacation_quota/


## tools/resync_overtime_to_dingtalk/


## tools/set_comp_time_balance/


## tools/setup/create_freedom_leave/


## tools/setup/create_vacation/