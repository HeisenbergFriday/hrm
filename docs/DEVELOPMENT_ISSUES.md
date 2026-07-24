---
purpose: 开发问题复盘日志——沉淀已定位根因且有复用价值的缺陷与防复发约束，供开发前查阅、开发后更新
last_updated: 2026-07-23
source_of_truth:
  - 本文件（问题条目与防复发索引）
  - AGENTS.md（开发前必读 / 开发后必记流程）
  - .ai/AI_WORKFLOW.md（步骤 3 / 步骤 16 闭环）
  - .ai/MODULES/*.md（模块长期约束）
update_when:
  - 定位到有复用价值的业务/代码/测试/审查/部署问题时
  - 同一根因修复方案或防复发约束变更时
  - 防复发索引需要增删改时
  - 问题状态从 open 变为 fixed / wontfix 时
---

# 开发问题复盘日志

## 开发前必读索引（防复发）

开发任何功能或修 Bug 前，先读本索引，再按模块跳到相关条目。

### 认证 / 多组织登录

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `auth` `logout` `csrf` | Header「退出登录」必须 `authAPI.logout()`（带 withCredentials + `X-CSRF-Token`）；禁止裸 `axios.post('/api/v1/auth/logout')`，否则 JWTAuth CSRF 校验 403 后仅清前端、HttpOnly 会话残留 | [2026-07-21 Header 退出登录未带 CSRF](#2026-07-21-p1-header-退出登录未带-csrf-导致服务端会话可能残留) |
| `ui` `security` `write-guard` | 危险写操作需 Modal.confirm + 与后端一致的 feature 权限门闩（disabled+Tooltip）；禁止假 success；org sync 统一 `confirmOrgSync`/`attendance_manage` | [2026-07-21 UI 交互安全执行序收口](#2026-07-21-p1p2-ui-交互安全执行序收口假成功写门闩导出会话) |
| `attendance-toolbox` `permission` | 工具箱写 API 必须 `attendance_manage` 或 `attendance_toolbox_operate`（钉钉同步含 dingtalk_sync）；禁止仅凭 `menu:attendance-toolbox` 绕过 | 同上 |
| `auth` `multi-org` `user-service` | 已解析 `orgID` 后的用户读写必须用 `NewUserServiceWithOrgID`；实体 `User.OrgID` 不能代替仓储构造绑定；空 org fail-closed | [2026-07-20 钉钉多组织登录更新用户缺少组织作用域](#2026-07-20-p1-钉钉多组织登录更新用户时缺少组织作用域) |
| `auth` `multi-org` `dingtalk-login` `first-login` | 首次自动补用户 `ensureLocalUserForDingTalkLogin` 必须校验非空 org 后用 `NewUserServiceWithOrgID` + `NewPermissionServiceWithOrgID`；禁止 `NormalizeOrganizationID("")` 成 default | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |
| `auth` `logout` `audit` | `/auth/logout`、`/auth/me` 必须 `JWTAuth+TenantContext`；登出 `OperationLog` 必须写 JWT `org_id` | 同上 |
| `auth` `jwt` | JWT 必须携带 `org_id`；缺省返回 `code=token_missing_org_id`，禁止回退 `default` | 见 `.ai/MODULES/auth.md` 认证安全约束 |
| `auth` `dingtalk-login` | 多企业扫码/免登必须显式或可解析的本地 `org_id`；禁止静默落到 default；仅有 unionId/openId 不得跨企业反查自动选企 | 见 `.ai/MODULES/auth.md` 2026-07 多企业登录隔离 |
| `auth` `permission` `user-role` `multi-tenant` | 角色 preset / 用户角色绑定必须按 `org_id`；`user_roles.role_id` 不得指向外组织 `roles`；权限 JOIN 要求 `roles.org_id=user_roles.org_id` | [2026-07-21 列德管理员跨 org 角色中毒](#2026-07-21-p1-列德管理员角色跨-org-中毒导致权限查询-0-行) |

### 多租户 / 仓储构造

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `multi-tenant` `repository` | tenant-scoped repository 缺 `orgID` 必须 fail-closed；优先 `NewXxxWithOrgID`；实体字段 `OrgID` ≠ 仓储绑定 | `.ai/ARCHITECTURE.md` 多租户隔离运行时边界 |
| `multi-tenant` `handler` | 普通接口只认 JWT `orgID`；禁止 body/query/header 切 org | `.ai/ARCHITECTURE.md` / cerebrum |
| `database-migration` `multi-tenant` `unique-index` | 租户业务唯一键必须以 `org_id` 开头；模型标签、迁移矩阵、旧兼容逻辑必须一致；启动迁移按 Prepare → AutoMigrate → Verify 执行；禁止自动改写组织归属、删行或合并 | [2026-07-21 多组织唯一索引与旧兼容迁移不一致导致连续部署失败](#2026-07-21-p1-多组织唯一索引与旧兼容迁移不一致导致连续部署失败) |
| `database-migration` `mysql` `collation` | 迁移 SQL 跨数值/文本 ID 关联时必须使用不依赖 collation 的精确比较；禁止把数值 `CAST AS CHAR` 后直接与文本列比较 | [2026-07-23 绩效迁移 ID 比较触发排序规则冲突](#2026-07-23-p1-绩效迁移-id-比较触发-mysql-排序规则冲突) |
| `shift-config` `attendance` `gorm` | 服务内直接 `s.db` 查用户/部门/节假日/班次目录必须 org 过滤；缺 org fail-closed 禁止全表；优先 `NewShiftConfigServiceWithOrgID` / `NewAttendanceServiceWithOrgID` | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |

### 考勤工具箱

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `attendance-toolbox` `run-store` | 结果绑定 `user_id+org_id`；磁盘仅 `rootDir/<runID>`；禁止返回服务器绝对路径 | `.ai/MODULES/attendance.md` |
| `attendance-toolbox` `permission` | 计算/审计/模板需 `attendance_toolbox_operate`；钉钉同步需 `attendance_toolbox_dingtalk_sync`；一键联动 AND | `.ai/MODULES/attendance.md` 权限矩阵 |

### API / 路由契约

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `api-contract` `router` `attendance` | 前端已调用且后端已有 Handler 的接口必须同时注册到 Router；新增 API 必须有路由清单回归测试，避免运行时 404 | [2026-07-23 考勤查询外部结果路由漏注册](#2026-07-23-p1-考勤查询外部结果路由漏注册导致进入页面-404) |
| `attendance` `data-contract` `metrics` `frontend` | 同一业务指标必须明确数据源、计数单位和分母；禁止首页和业务页面各自维护不同统计口径 | [2026-07-23 P2 考勤统计口径分裂](#2026-07-23-p2-考勤统计口径分裂导致数字不可比) |

### 部署 / 配置

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `deploy` `test-server` | 测试服隔离目录/端口/Compose 项目名；完整变量见 `deploy/peopleops.test.env.example`，文档不复制密钥 | `deploy/TEST_SERVER_DEPLOY.md` |
| `deploy` `upload-and-restart` | 上传失败续传用独立脚本，禁止改 `build-and-deploy.ps1` 行为 | cerebrum Decision Log |
| `git` `merge` `release` | 跨分支冲突必须按语义单元核对生产代码、配套测试和 `go.mod/go.sum`；禁止按文件整侧选取后直接推送，至少执行编译、全量测试与双远端祖先检查 | [2026-07-23 双远端 master 合并遗漏依赖与安全配套代码](#2026-07-23-p1-双远端-master-合并遗漏依赖与安全配套代码) |

---

## 写入规则

### 开发前（必读）

1. 打开本文件，阅读「开发前必读索引」。
2. 按当前任务模块/标签，读取相关条目正文（不必通读全部历史）。
3. 在修改计划中写明：**适用的防复发约束**（引用条目日期或标签）。

### 开发后（必记）

任务收尾、写复盘总结前，必须判断：

| 情况 | 动作 |
|---|---|
| 已定位根因，且对后续开发有复用价值 | **新增**条目，或**更新**同根因已有条目 |
| 同根因再次出现 | 更新原条目的修复/验证/状态，禁止另开重复条目 |
| 仅一次性环境故障、外部工具临时不可用，且未形成项目内改进项 | 可不写入本文件，但最终总结必须说明原因 |
| 敏感信息（密码、Token、个人手机号/邮箱等） | **禁止**写入 |

### 记录范围

**应记录：**

- 业务缺陷（错误规则、错误状态流转）
- 代码缺陷（错误构造器、错误作用域、竞态、fail-open）
- 测试遗漏（本应覆盖却未覆盖的回归点）
- 代码审查发现的可复现问题
- 部署/配置问题（错误默认、错误回退、隔离失败）

**不记录：**

- 密码、Token、密钥、证书内容
- 可识别个人信息（完整手机号、邮箱、真实姓名+身份证等）
- 纯文案/样式微调且无行为影响
- 未定位根因的临时猜测

### 日志结构说明

- **索引区**（本文顶部）：短约束 + 跳转，避免每次全量读历史。
- **条目区**（下方）：完整复盘；按时间倒序，新条目置顶于「问题条目」第一节。
- 模块标签用于过滤；状态：`open` / `fixed` / `wontfix`。

---

## 统一条目模板

复制以下模板新增条目：

```markdown
### YYYY-MM-DD P? <简短标题>

| 字段 | 内容 |
|---|---|
| 日期 | YYYY-MM-DD |
| 级别 | P0 / P1 / P2 / P3 |
| 模块/标签 | `module` `tag` |
| 范围 | 后端 / 前端 / 全栈 / 部署 |
| 现象 | 用户可见或可复现的表现；可附错误文案（脱敏） |
| 根因 | 已验证的根本原因 |
| 修复 | 改了什么（文件/关键调用），不要贴密钥 |
| 验证 | 运行了哪些命令、结果 |
| 防复发 | 以后必须遵守的约束（可进索引） |
| 状态 | open / fixed / wontfix |
```

---

## 问题条目

### 2026-07-23 P1 考勤查询外部结果路由漏注册导致进入页面 404

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `attendance` `api-contract` `router` `frontend` |
| 范围 | 全栈（前后端 API 契约） |
| 现象 | 进入考勤查询页面时，前端请求 `/attendance/external-sync/daily-results` 返回 404，页面显示“考勤结果加载失败”。状态、同步任务等同组外部考勤接口也未注册。 |
| 根因 | 前端 `attendanceAPI.externalSync` 和后端 `ExternalAttendance*` Handler 已实现，但 `internal/api/router.go` 的 `/attendance` 路由组遗漏了 `external-sync` 子路由；原有 Handler 单测未覆盖 Router 路径清单。 |
| 修复 | 在 `internal/api/router.go` 注册状态、每日结果、同步执行、任务列表和任务详情 5 个路由；查询沿用考勤菜单读权限或 `attendance_manage`，同步执行保留 `attendance_manage`；增加 `TestExternalAttendanceRoutesRegistered` 路由回归测试。 |
| 验证 | `go test ./internal/api -run 'TestExternalAttendance|Test.*Attendance.*Router' -count=1`、`go vet ./internal/api`、`golangci-lint run`、`cd frontend && npm run lint`、`cd frontend && npm run build` 均通过。 |
| 防复发 | 1. 新增或迁移前端 API 时，必须同时核对后端 Handler、Router 注册和权限中间件。<br>2. 每个跨前后端 API 子模块至少保留一条 Router 路径清单测试。<br>3. 页面首次加载使用的 GET 接口不能只依赖 Handler 单测，必须验证 `SetupRouter().Routes()` 中存在完整路径。 |
| 状态 | fixed |

### 2026-07-23 P1 绩效迁移 ID 比较触发 MySQL 排序规则冲突

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `database-migration` `performance` `mysql` `collation` `deploy` |
| 范围 | 后端 / 数据库 / 部署 |
| 现象 | 测试服应用容器在启动迁移 `MigratePerformanceParticipantOrgIDsFromActivity` 的预检查询中失败，MySQL 返回 1267：`Illegal mix of collations (utf8mb4_general_ci,IMPLICIT) and (utf8mb4_unicode_ci,IMPLICIT) for operation '='`，导致健康检查失败。 |
| 根因 | `performance_activities.id` 是数值主键，`performance_participants.activity_id` 是 `varchar(64)`；迁移 SQL 使用 `CAST(a.id AS CHAR) = p.activity_id`。数值转字符表达式继承连接排序规则，而历史文本列使用另一排序规则，两个隐式字符串排序规则无法比较。原测试仅验证软删除、事务与关联表更新，没有锁定 JOIN 表达式的 collation 安全性。 |
| 修复 | `internal/database/schema_expand_migrations.go` 的预检和更新 JOIN 均改为 `CAST(a.id AS BINARY) = CAST(p.activity_id AS BINARY)`，保留原有文本精确匹配语义并绕开字符排序规则；`performance_org_migrate_test.go` 增加两条 SQL 的回归断言，禁止退回 `CAST(a.id AS CHAR)`。 |
| 验证 | `go test ./internal/database -run TestMigratePerformanceParticipantOrgIDs -count=1` 通过；`go test ./internal/database -count=1` 通过；`go vet ./internal/database` 通过；`go build ./cmd/...` 通过；`golangci-lint run --tests=false ./internal/database` 为 `0 issues`。全仓 `go vet ./...` 因工作区已有无权限目录 `codex_tmp_ascii/tmp*` 在包展开阶段阻塞，未作为代码失败处理。 |
| 防复发 | 1. MySQL 迁移中跨类型 ID 关联不得依赖连接/列默认 collation。<br>2. 需要保持文本精确语义时，双方显式转换为二进制字符串比较；禁止用宽松数值转换自动匹配异常历史值。<br>3. 启动迁移回归测试必须同时覆盖预检 SELECT 与事务内 UPDATE 的 JOIN 表达式。<br>4. 不通过全库改 collation 或修改生产数据来掩盖单条迁移 SQL 的类型问题。 |
| 状态 | fixed |

### 2026-07-23 P1 双远端 master 合并遗漏依赖与安全配套代码

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P1 |
| 模块/标签 | `git` `merge` `release` `go-mod` `security` `multi-tenant` `test` |
| 范围 | 全栈 / 合并 / 发布 |
| 现象 | 将功能分支合入 GitHub `master` 时出现多文件冲突；初次按文件选择版本后，Go 构建缺少钉钉 Stream SDK 与 SQLite 测试依赖，同时已自动合入的安全测试找不到配套的路由白名单、管理员配置透传和迁移索引常量。若未执行完整验证，代码可能在推送后才暴露编译失败或 fail-open 回退。 |
| 根因 | 冲突解决以“文件选边”为单位，而实际变更跨越生产代码、测试和依赖清单：选择远端 `go.mod` 丢失当前分支依赖；选择当前生产文件又遗漏远端安全提交的配套符号。Git 自动合并测试文件不代表对应生产实现已经完整合入。 |
| 修复 | 合并依赖集合并保留 `golang.org/x/text v0.39.0`；恢复钉钉 `AppConfig.AdminUserID` 透传、空组织 fail-closed、按组织排班查询封装、路由只读菜单白名单和迁移索引常量；继续保留统一复合索引迁移以及 `DingTalkEventLog` AutoMigrate。 |
| 验证 | `go test ./cmd/... ./internal/... ./tools/...` 通过；`go vet` 通过；`golangci-lint` 0 issues；前端 lint、18 files / 280 tests、生产构建通过；Playwright 45 tests（三浏览器）通过。 |
| 防复发 | 1. 冲突按功能语义单元审查，生产代码、测试、`go.mod/go.sum` 必须联动。<br>2. 选择整侧版本后必须对照另一侧提交列出丢失符号，禁止仅凭“无冲突标记”判定完成。<br>3. 先跑冲突包的定向编译/测试，再跑全量 lint、单测、构建和关键 E2E。<br>4. 推送两个 `master` 前重新 fetch，并确认两个远端当前头都是集成提交的祖先；禁止强推。 |
| 状态 | fixed |

### 2026-07-21 P1/P2 UI 交互安全执行序收口（假成功/写门闩/导出/会话）

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 / P2 |
| 模块/标签 | `ui` `security` `csrf` `logout` `org-sync` `attendance-toolbox` `export` `pii` |
| 范围 | 全栈（以前端门闩与路由权限对齐为主；不改业务状态机/权限码语义/配置文件） |
| 现象 | Setting 假保存密钥；SyncJobs 假 success；推送默认写死人名；org sync 多入口无 confirm/权限 UI；档案/流程写按钮无 hasPermission；导出 window.open；logout 后 RQ 缓存残留；Login `?error=` 反射；工具箱 API 可仅凭菜单绕过 feature 权限；upload/departments 门闩过松。 |
| 根因 | 前端写路径与后端权限不一致、假成功文案、硬编码测试数据、会话缓存未在 logout 清理；工具箱路由 `RequirePermissionOrMenu(..., menu)` 允许 menu-only。 |
| 修复 | Setting 改为密钥只读说明；SyncJobs 改真实 sync API+confirm；推送默认空选；`orgSyncAction` 统一 org sync；Profile/Flow/Detail/Shift/Processing 写门闩；export `downloadAuthorizedFile`；`queryClient.clear` on logout；`safeLoginErrorMessage`；列表 mobile/email 脱敏；绩效附件 `AuthorizedImage`；router toolbox 细权限 + upload/departments 门闩 + files TenantContext。 |
| 验证 | `cd frontend && npm run lint` 通过；`npm run test` 18 files / 280 tests 通过；`npm run build` 通过；`go test ./internal/api -run 'TestRunAttendanceToolbox\|TestRouter\|TestUpload\|TestLogout' -count=1` 通过。 |
| 防复发 | 1. 禁止 UI 假 success（不调 API 却提示成功）。<br>2. 写按钮缺 feature 权限用 disabled+Tooltip，与后端码一致。<br>3. 工具箱写路径禁止 menu-only 旁路。<br>4. logout 必须清 QueryClient；cookie 写请求走 api 实例 CSRF。<br>5. 登录错误文案走白名单，禁止 `?error=` 原样展示。 |
| 状态 | fixed |

### 2026-07-21 P1 Header 退出登录未带 CSRF 导致服务端会话可能残留

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `auth` `logout` `csrf` `frontend` |
| 范围 | 前端 |
| 现象 | 点击 Header「退出登录」后前端已跳登录页，但服务端 HttpOnly 会话 cookie 可能仍有效；共享电脑或直接调 API 仍可带旧会话。 |
| 根因 | `App.tsx` `handleLogout` 使用裸 `axios.post('/api/v1/auth/logout')`，未走 `api` 实例拦截器，因而缺少 `X-CSRF-Token`。`JWTAuth` 对 cookie 会话的 POST 会校验 CSRF，失败返回 403；`finally` 仍清 zustand 并跳转，造成「客户端已退、服务端未退」。组织切换路径已正确使用 `authAPI.logout()`。 |
| 修复 | `frontend/src/App.tsx`：`handleLogout` 改为 `await authAPI.logout()`（自动 `withCredentials` + CSRF）；跳转与 remember org 行为不变。不改后端 Logout 业务语义。 |
| 验证 | `cd frontend && npm run lint`（含 App.tsx）通过。 |
| 防复发 | 1. 凡依赖 HttpOnly cookie 的写请求（含 logout）必须走 `frontend/src/services/api.ts` 的 `api` 实例或等价地附加 `csrfHeadersForMethod`。<br>2. 禁止在业务页对 `/api/v1/auth/logout` 使用裸 `axios.post`。<br>3. 退出/换组织两条路径应共用同一 logout 客户端实现。 |
| 状态 | fixed |

### 2026-07-21 P1 列德管理员角色跨 org 中毒导致权限查询 0 行

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `auth` `permission` `user-role` `multi-tenant` `migration` `liede-admin` |
| 范围 | 后端 / 启动迁移 / 权限解析 |
| 现象 | 目标管理员登录 muteng/xiaotie 后 `permissions`/`menu_keys` SQL `rows=0`，前端表现为无权限；重启后若曾手工修好会再次丢失。 |
| 根因 | `migrateLiedeOrganizationAdminRoles` 启动时按姓名「列德」强制写管理员，但 `ensureRolePreset` 只按 `name` 取第一条角色（通常是 default 的管理员），再 `ensureUserRoleInOrg` 写入 `user_roles.org_id=muteng/xiaotie + role_id=default管理员`。权限查询要求 `roles.org_id = user_roles.org_id`，JOIN 失败返回空权限。 |
| 修复 | `ensureRolePresetInOrg` / `ensureRoleMenuPermissionInOrg` / `ensureRoleDataPermissionInOrg`；Liede 迁移对每个 org 取本组织管理员；`ensureUserRoleInOrg` 拒绝跨 org `role_id`；启动后 `remapCrossOrgUserRoleBindings` 修复已中毒绑定。单测：`liede_admin_role_org_isolation_test.go`。 |
| 验证 | `go build ./internal/database` 通过；`go test` 以 package sources + `liede_admin_role_org_isolation_test.go` 运行 4 用例通过；`go test ./internal/service -run 'TestAssignDefaultEmployeeRole_UsesCurrentOrgRole\|TestAssignUserRoleInOrg_OrgIsolation'` 通过。 |
| 防复发 | 1. 角色 preset/菜单/数据权限写入必须带 `org_id`，禁止只按 role 名或 role_id 跨 org 复用。<br>2. 写 `user_roles` 前必须校验 `roles.org_id == user_roles.org_id`。<br>3. 启动迁移若自动授权，必须按目标 org 取角色，不得把 default 角色 ID 绑到其他 org。<br>4. 权限解析 JOIN 保持 fail-closed（跨 org role_id 不得出权）。 |
| 状态 | fixed |

### 2026-07-21 P1 多组织唯一索引与旧兼容迁移不一致导致连续部署失败

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-21 |
| 级别 | P1 |
| 模块/标签 | `database-migration` `multi-tenant` `overtime` `dingtalk-binding` `user-role` `unique-index` `deploy` |
| 范围 | 后端 / 数据库 / 部署 |
| 现象 | 测试服连续三次在启动迁移阶段失败：先是 `overtime_rule_configs.rule_key`，再是 `ding_talk_bindings.ding_talk_user_id` 被错误按全局唯一处理；模型与统一迁移矩阵对齐后，旧角色兼容迁移又在 `(org_id, user_id)` 唯一索引已存在时执行 `UPDATE user_roles ... SET org_id = users.org_id`，触发 `idx_user_roles_org_user` 重复键，健康检查失败。 |
| 根因 | 多组织唯一约束没有形成单一迁移契约：部分 GORM 模型仍声明单列全局唯一，统一迁移矩阵未覆盖全部模型；同时 `migrateRolePermissionOrganizationScope`、`migrateUserRolesSingleRole`、`migrateMultitenantUniqueIndexes` 等旧兼容逻辑仍会在复合索引建立后重新改写 `org_id`、自动删除重复行或重复管理索引。模型标签、迁移矩阵和旧兼容回填三者不一致，导致每修复一张表就暴露下一处。另发现 `Department.DingTalkDepartmentID` 必须显式映射真实列名 `dingtalk_department_id`，不能依赖 GORM 默认拆词。 |
| 修复 | 统一对齐多组织模型的复合唯一标签与 `OrgCompositeUniqueSpec` 迁移矩阵；迁移顺序收口为 Prepare → AutoMigrate → Verify，并在旧兼容逻辑后做最终 Verify。角色和旧多租户兼容入口停止根据非唯一 `user_id` 推断/覆盖业务行 `org_id`，停止自动删除、合并重复 `user_roles`，停止重复创建统一矩阵已管理的索引；仅允许对缺失值做 `default` 归一。部署未上传或修改服务器运行配置。 |
| 验证 | `go build ./cmd/...` 通过；数据库非测试文件 `go vet` 通过；`golangci-lint run --tests=false --disable=unused ./internal/database` 为 `0 issues`（仅本机 lint 缓存目录无写权限警告）；独立 GORM schema 检查为 `mismatches=0`。`go test ./internal/database` 仍被工作区已有未定义测试辅助符号（`MigrateAnnualLeaveConsumeLogSchema`、`MigrateShiftCatalogSchema`、`backfillUserDingTalkIDDB`、`MigratePerformanceParticipantOrgIDsFromActivity`）阻塞。执行 `deploy/build-and-deploy.ps1 -SkipConfigUpload` 后应用、MySQL、Redis 容器均为 `healthy`，首次 `/health` 检查通过；服务器日志确认服务启动、定时任务启动且后续健康检查持续返回 200。 |
| 防复发 | 1. 多组织业务表的唯一键必须显式以 `org_id` 开头，禁止单列全局唯一。<br>2. 新增/修改租户模型时，必须同时校验 GORM schema 与统一迁移矩阵，列名特殊拆词必须显式写 `column:`。<br>3. 既有表迁移统一执行 Prepare → AutoMigrate → Verify；旧兼容入口不得在 Verify 后再次改写组织归属或索引。<br>4. 跨组织相同业务键合法；任何会改变 `org_id` 的回填必须先做目标状态冲突审计，歧义时 fail-closed 并人工确认。<br>5. 禁止通过自动删除、合并或选择保留业务记录来让唯一索引迁移通过；回归测试应覆盖模型标签、迁移矩阵、旧索引替换、跨组织同键和同组织冲突。 |
| 状态 | fixed |
### 2026-07-20 P1/P2 org_id 隔离缺口收口（首次补用户/登出审计/直接 GORM fail-open）

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-20 |
| 级别 | P1 / P2 |
| 模块/标签 | `auth` `multi-org` `dingtalk-login` `first-login` `logout` `audit` `shift-config` `attendance` `gorm` |
| 范围 | 后端 |
| 现象 | 1) 钉钉首次登录自动补用户仍用未绑定 org 的 `NewUserService`/`NewPermissionService`，与「已存在用户登录更新」同类缺口；2) `/auth/logout`、`/auth/me` 仅 JWTAuth 无 TenantContext，登出审计 `OperationLog` 缺 `org_id`；3) `ShiftConfigService`/`AttendanceService` 直接 `s.db` 查用户/部门/节假日/班次目录时可能无 org 过滤（fail-open 全表风险）。 |
| 根因 | 登录更新路径已改为 `WithOrgID`，但首次补用户与组织同步赋权路径未同步；会话路由未挂 TenantContext；服务构造只绑了 repository，业务内直接 GORM 未强制 org。 |
| 修复 | `handlers.go`：`ensureLocalUserForDingTalkLogin` 空 org fail-closed + `NewUserServiceWithOrgID`/`NewPermissionServiceWithOrgID`；`SyncUsers`/`SyncOrgData` permission 改为 WithOrgID；`Logout` 写 `OperationLog.OrgID`；`createOperationAuditLog` 带 OrgID。<br>`router.go`：`/auth/logout`、`/auth/me` 增加 `TenantContext`。<br>`shift_config_service.go`：`NewShiftConfigServiceWithOrgID` + `scopedDB`/`requireBoundOrgID` 覆盖用户/部门/节假日/catalog 查询。<br>`attendance_service.go`：构造注入 `orgID`；`loadUsers`/`loadDepartmentNames` 缺 org 返回 `ErrMissingOrgID`。 |
| 验证 | `go test ./internal/api -run 'Test.*(DingTalk\|Logout\|GetCurrentUser\|ShiftConfig\|MultiOrg\|EnsureLocalUser)' -count=1`<br>`go test ./internal/service -run 'Test.*(ShiftConfig\|Attendance\|Org\|GetAllWithUsers\|LoadUsers)' -count=1`<br>`go test ./internal/repository -run 'TestUserRepository_\|Test.*Org\|Test.*Isolation' -count=1`<br>`go vet ./internal/api ./internal/service ./internal/repository` |
| 防复发 | 1. 已解析 org 后用户/权限/业务读写必须 `NewXxxWithOrgID`。<br>2. 实体 `OrgID` 不能代替仓储/服务构造绑定。<br>3. 空 org fail-closed，禁止 `NormalizeOrganizationID("")` 发明 default 后继续写。<br>4. 直接 GORM 业务查询必须 org 条件或 1=0；禁止「if org!=\"\" 才过滤」的 fail-open。<br>5. 会话路由 logout/me 必须 TenantContext；审计日志必须写 org_id。 |
| 状态 | fixed |

### 2026-07-20 P1 钉钉多组织登录更新用户时缺少组织作用域

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-20 |
| 级别 | P1 |
| 模块/标签 | `auth` `multi-org` `dingtalk-login` `user-service` |
| 范围 | 后端（钉钉内免登 + 扫码回调） |
| 现象 | 任意组织登录失败，报错：`update local user failed: repository: orgID required for tenant-scoped operation` |
| 根因 | 钉钉内免登与扫码回调已解析 `orgID` / `resolvedOrgID`，但使用未绑定组织的 `NewUserService` 更新用户。实体字段 `User.OrgID` 不能代替仓储构造时的组织绑定；tenant-scoped repository 在空 org 下 fail-closed，因而更新失败。 |
| 修复 | `internal/api/handlers.go` 两处改为带组织构造：<br>- 内免登：`service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)`<br>- 扫码回调：`service.NewUserServiceWithOrgID(middleware.RequestDB(c), resolvedOrgID)` |
| 验证 | `go test ./internal/repository -run '^TestUserRepository_' -count=1` 通过<br>`go test ./internal/api -run 'Test.*(DingTalk\|Dingtalk)' -count=1` 通过<br>`go vet ./internal/api ./internal/repository` 通过 |
| 防复发 | 1. 已解析组织后的用户读写必须用 `NewUserServiceWithOrgID`（或等价 `NewXxxWithOrgID`）。<br>2. 空组织继续 fail-closed，禁止发明 `default`。<br>3. 跨组织实体继续拒绝；禁止用实体上的 `OrgID` 字段代替仓储/服务构造绑定。<br>4. 相关回归：登录路径更新用户、缺 org、跨 org 写。 |
| 状态 | fixed |

### 2026-07-23 P2 考勤统计口径分裂导致数字不可比

| 字段 | 内容 |
|---|---|
| 日期 | 2026-07-23 |
| 级别 | P2 |
| 模块/标签 | `attendance` `data-contract` `metrics` `frontend` `api` |
| 范围 | 前端首页 / 考勤查询 / 旧异常统计页 / 后端统计接口 |
| 现象 | 考勤查询按外部同步每日结果展示员工日，旧异常统计页却按本地打卡记录和规则引擎重新计算；页面同时把员工人数、员工日和异常次数标成“人数”，导致同一日期范围的数字无法直接比较。首页考勤率继续依赖旧 `/attendance/stats`，进一步放大双口径问题。 |
| 根因 | 同一业务指标存在两套数据源和两套聚合逻辑，且没有固定“人数/人次/员工日”的展示契约；首页直接复用旧统计接口，使删除页面时仍残留后端依赖。 |
| 修复 | 统一以外部同步每日结果为准；首页读取 `external-sync/daily-results` 最近 30 天的 `summary`，按 `normal / total` 计算正常率，审批员工日保留在分母；删除旧异常统计页面、菜单、权限、路由、`/attendance/stats` Handler、旧规则引擎与相关文档。 |
| 验证 | 首页口径单测 4/4 通过；考勤 service/API 定向测试通过；前端 lint、build 通过；`go vet ./...` 通过；全量旧引用搜索为 0。`golangci-lint` 被本次范围外未跟踪文件 `internal/repository/external_approval_repository.go` 的 `rows.Close()` 未检查问题阻塞。 |
| 防复发 | 1. 考勤展示与首页指标统一读取外部每日结果，禁止另起规则引擎重复计算。<br>2. 新增统计字段必须写明数据源、时间范围、计数单位和分母。<br>3. 删除统计页面时必须同步核对首页、菜单权限、API 客户端、Router、Handler、Service 和文档引用。<br>4. 正常率固定为 `summary.normal / summary.total`，审批员工日保留在 `total` 分母。 |
| 状态 | fixed |

---

## 维护约定

- 新增条目后，同步更新顶部「开发前必读索引」对应行。
- 同一根因只保留一条主记录；历史过程写在同一条目内追加，不拆多条重复现象。
- 本文件由 AI 与人工共同维护；流程入口见 `AGENTS.md` 步骤 3 / 步骤 16 与 `.ai/AI_WORKFLOW.md`。
