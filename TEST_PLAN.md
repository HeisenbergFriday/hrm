# 测试计划

最后更新：2026-05-26

## 当前测试资产

### 已落地

- 后端测试：`go test ./...`
  - `internal/database/database_test.go`
  - `internal/dingtalk/dingtalk_test.go`
  - `internal/repository/user_repository_test.go`
  - `internal/repository/week_schedule_repository_test.go`
  - `internal/service/*_test.go`
- 前端单测配置：`frontend/vite.config.test.ts`
  - 技术栈：Vitest + React Testing Library + jsdom
  - 命令：`cd frontend && npm run test`
  - 当前配置允许无前端单测文件时通过，用于保持验证命令可执行。
- 前端 E2E：`cd frontend && npm run e2e`
  - `frontend/tests/e2e/login.spec.ts`
  - `frontend/tests/e2e/organization.spec.ts`
  - `frontend/tests/e2e/attendance.spec.ts`
- 测试辅助设施
  - `tests/config/test_config.go`
  - `tests/database/init_test_db.go`
  - `tests/mock/dingtalk_mock.go`
  - `tests/reports/acceptance-report.md`
  - `tests/reports/acceptance-requirements-matrix.md`
- 前端报告脚本：`cd frontend && npm run generate-report`

### 待补齐

- 前端业务单测文件：`frontend/src/**/*.test.tsx` 或 `frontend/src/**/*.spec.tsx`
- 权限专项测试：当前没有 `*-permission.test.tsx` 或 `handlers_permission_test.go`
- 回归专项测试目录：当前尚未建立
- Redis mock、通用 test-utils、测试种子数据：当前未落地独立文件
- API 契约测试：`tests/api-contract-test.go` 已存在，但不是标准 `_test.go` 命名，且期望状态码需要结合当前鉴权和路由行为重新校准后再纳入必跑测试
- 覆盖率门禁：当前未配置强制覆盖率阈值；前端覆盖率还需要确认 coverage provider 依赖

## 分层策略

### 1. 后端单元与服务测试

- 技术栈：Go testing + testify
- 覆盖范围：数据库初始化、钉钉客户端、Repository、Service 业务逻辑
- 必跑命令：`go test ./...`
- 目标：新增服务逻辑优先补充同包 `_test.go`，避免只依赖接口层或人工验证

### 2. 前端单元测试

- 技术栈：Vitest + React Testing Library
- 覆盖范围：页面组件、表单校验、状态变化、接口 mock 后的渲染逻辑
- 必跑命令：`cd frontend && npm run test`
- 文件约定：`frontend/src/**/*.{test,spec}.{ts,tsx}`
- 目标：先覆盖登录、组织、考勤、审批等主流程页面，再扩展到通用组件

### 3. 前端 E2E 验收测试

- 技术栈：Playwright
- 覆盖范围：登录、组织架构、考勤等用户主链路
- 必跑命令：`cd frontend && npm run e2e`
- 文件约定：`frontend/tests/e2e/**/*.spec.ts`
- 说明：E2E 依赖前后端服务和测试账号数据，CI 中应单独准备环境

### 4. API 契约测试

- 目标范围：响应结构、状态码、错误码、鉴权行为、参数校验
- 当前状态：存在草稿文件 `tests/api-contract-test.go`，尚未纳入可靠必跑集合
- 后续要求：重命名为标准 `_test.go` 文件后，根据 `internal/api/router.go` 的真实路由与中间件更新用例

### 5. 权限与回归测试

- 权限测试目标：菜单权限、接口权限、数据权限
- 回归测试目标：登录、组织员工、考勤、审批、权限中心主链路
- 当前状态：专项目录和专项测试文件未落地
- 后续要求：从高风险接口和页面开始补齐，不在文档中引用尚不存在的路径作为已落地资产

## 本地验证命令

```bash
go test ./...

cd frontend
npm run build
npm run lint
npm run test
npm run e2e
```

说明：`npm run build` 已包含 TypeScript 编译。`npm run e2e` 需要可访问的前后端测试环境；只做快速静态验证时可先运行 build、lint、unit test。

## 覆盖率与报告

- 后端覆盖率：`go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out`
- 前端覆盖率：待补齐 coverage provider 后再作为稳定命令纳入 CI
- 验收报告：`cd frontend && npm run generate-report`
- 报告目录：`tests/reports/`

## 通用测试场景

- Happy Path：正常流程
- Empty State：空数据
- Error State：接口失败、解析失败、保存失败
- Unauthorized/Forbidden：未登录、无权限、权限过期
- 参数非法：必填、格式、边界值
- 幂等性：重复提交、重复同步、重复审批
- 分页与筛选：页码、关键字、状态、日期范围
- 审计日志：关键操作的日志写入和查询
- 同步任务：成功、失败、重试、部分成功

## 模块测试重点

- 登录认证：账号密码登录、钉钉内免登、扫码登录失败、会话过期
- 组织架构：部门树、员工列表、员工详情、同步失败
- 考勤管理：记录查询、统计计算、导出、最近同步时间
- 请假与加班：审批同步、加班匹配、补录审批、手工重置
- 审批管理：模板、实例列表、详情、同步任务
- 权限管理：角色创建、权限分配、无权限访问
- 员工档案：档案创建、更新、查询；入转调离列表、创建、详情
- 人才分析：分析创建、详情、列表统计
- 周排班与班次：班次配置、周计划生成、发布与查询

## 风险与约束

- 钉钉 API 依赖应通过 mock 或测试环境隔离，避免本地测试依赖真实外部服务。
- 数据库测试应使用测试库或隔离数据源，避免污染生产数据。
- E2E 需要明确测试账号、后端地址和基础数据。
- 覆盖率阈值应在测试资产稳定后再启用，避免用空阈值制造假安全感。
