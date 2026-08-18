---
purpose: 开发问题复盘日志——沉淀已定位根因且有复用价值的缺陷与防复发约束，供开发前查阅、开发后更新
last_updated: 2026-08-18
source_of_truth:
  - 本文件（防复发索引、写入规则与归档导航）
  - docs/development-issues/2026.md（2026 年完整问题条目）
  - AGENTS.md（开发前必读 / 开发后必记流程）
  - .ai/workflow/requirements.md（开发前读取）
  - .ai/workflow/retrospective.md（开发后闭环）
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
| `auth` `logout` `csrf` | Header「退出登录」必须 `authAPI.logout()`（带 withCredentials + `X-CSRF-Token`）；禁止裸 `axios.post('/api/v1/auth/logout')`，否则 JWTAuth CSRF 校验 403 后仅清前端、HttpOnly 会话残留 | [2026-07-21 Header 退出登录未带 CSRF](development-issues/2026.md#2026-07-21-p1-header-退出登录未带-csrf-导致服务端会话可能残留) |
| `ui` `security` `write-guard` | 危险写操作需 Modal.confirm + 与后端一致的 feature 权限门闩（disabled+Tooltip）；禁止假 success；org sync 统一 `confirmOrgSync`/`attendance_manage` | [2026-07-21 UI 交互安全执行序收口](development-issues/2026.md#2026-07-21-p1p2-ui-交互安全执行序收口假成功写门闩导出会话) |
| `attendance-toolbox` `permission` | 工具箱写 API 必须 `attendance_manage` 或 `attendance_toolbox_operate`（钉钉同步含 dingtalk_sync）；禁止仅凭 `menu:attendance-toolbox` 绕过 | 同上 |
| `auth` `multi-org` `user-service` | 已解析 `orgID` 后的用户读写必须用 `NewUserServiceWithOrgID`；实体 `User.OrgID` 不能代替仓储构造绑定；空 org fail-closed | [2026-07-20 钉钉多组织登录更新用户缺少组织作用域](development-issues/2026.md#2026-07-20-p1-钉钉多组织登录更新用户时缺少组织作用域) |
| `auth` `multi-org` `dingtalk-login` `first-login` | 首次自动补用户 `ensureLocalUserForDingTalkLogin` 必须校验非空 org 后用 `NewUserServiceWithOrgID` + `NewPermissionServiceWithOrgID`；禁止 `NormalizeOrganizationID("")` 成 default | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](development-issues/2026.md#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |
| `auth` `logout` `audit` | `/auth/logout`、`/auth/me` 必须 `JWTAuth+TenantContext`；登出 `OperationLog` 必须写 JWT `org_id` | 同上 |
| `auth` `jwt` | JWT 必须携带 `org_id`；缺省返回 `code=token_missing_org_id`，禁止回退 `default` | 见 `.ai/MODULES/auth.md` 认证安全约束 |
| `auth` `dingtalk-login` | 多企业扫码/免登必须显式或可解析的本地 `org_id`；禁止静默落到 default；仅有 unionId/openId 不得跨企业反查自动选企 | 见 `.ai/MODULES/auth.md` 2026-07 多企业登录隔离 |
| `auth` `permission` `user-role` `multi-tenant` | 角色 preset / 用户角色绑定必须按 `org_id`；`user_roles.role_id` 不得指向外组织 `roles`；权限 JOIN 要求 `roles.org_id=user_roles.org_id` | [2026-07-21 列德管理员跨 org 角色中毒](development-issues/2026.md#2026-07-21-p1-列德管理员角色跨-org-中毒导致权限查询-0-行) |

### 多租户 / 仓储构造

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `multi-tenant` `repository` | tenant-scoped repository 缺 `orgID` 必须 fail-closed；优先 `NewXxxWithOrgID`；实体字段 `OrgID` ≠ 仓储绑定 | `.ai/ARCHITECTURE.md` 多租户隔离运行时边界 |
| `multi-tenant` `handler` | 普通接口只认 JWT `orgID`；禁止 body/query/header 切 org | `.ai/ARCHITECTURE.md` / cerebrum |
| `database-migration` `multi-tenant` `unique-index` | 租户业务唯一键必须以 `org_id` 开头；模型标签、迁移矩阵、旧兼容逻辑必须一致；启动迁移按 Prepare → AutoMigrate → Verify 执行；禁止自动改写组织归属、删行或合并 | [2026-07-21 多组织唯一索引与旧兼容迁移不一致导致连续部署失败](development-issues/2026.md#2026-07-21-p1-多组织唯一索引与旧兼容迁移不一致导致连续部署失败) |
| `database-migration` `ops-tool` `read-only` `logging` | 只读/定向运维工具必须使用只连接入口：连接前静默 SQL，禁止建库、迁移和 seed；连接失败 fail-closed；需组织作用域的模式必须显式提供非空 org，禁止回退 default；禁止调用应用完整 `database.Init()` | [2026-08-18 组织同步运维工具误跑启动迁移与种子](development-issues/2026.md#2026-08-18-p1-组织同步运维工具误跑启动迁移与种子) |
| `database-migration` `mysql` `collation` | 迁移 SQL 跨数值/文本 ID 关联时必须使用不依赖 collation 的精确比较；禁止把数值 `CAST AS CHAR` 后直接与文本列比较 | [2026-07-23 绩效迁移 ID 比较触发排序规则冲突](development-issues/2026.md#2026-07-23-p1-绩效迁移-id-比较触发-mysql-排序规则冲突) |
| `shift-config` `attendance` `gorm` | 服务内直接 `s.db` 查用户/部门/节假日/班次目录必须 org 过滤；缺 org fail-closed 禁止全表；优先 `NewShiftConfigServiceWithOrgID` / `NewAttendanceServiceWithOrgID` | [2026-07-20 首次补用户/登出审计/直接GORM fail-open 收口](development-issues/2026.md#2026-07-20-p1p2-org_id-隔离缺口收口首次补用户登出审计直接-gorm-fail-open) |

### 考勤工具箱

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `attendance-toolbox` `run-store` | 结果绑定 `user_id+org_id`；磁盘仅 `rootDir/<runID>`；禁止返回服务器绝对路径 | `.ai/MODULES/attendance.md` |
| `attendance-toolbox` `permission` | 计算/审计/模板需 `attendance_toolbox_operate`；钉钉同步需 `attendance_toolbox_dingtalk_sync`；一键联动 AND | `.ai/MODULES/attendance.md` 权限矩阵 |
| `attendance-toolbox` `dingtalk-sync` `frontend-refresh` | 工具箱不展示独立“钉钉同步”页签；钉钉拉取仅从请假、加班和固定配置按需入口执行，导航配置与回归测试必须一致 | [2026-08-01 花名册选错文件与运维组规则](development-issues/2026.md#2026-08-01-p1-花名册选错文件--运维组未强制标记未加) |
| `attendance-toolbox` `parttime` `leave-priority` | 兼职汇总同一日同时含外出/出差与事假时，必须先按事假处理并停止出勤计算；组合状态判断必须早于外出计出勤的提前返回 | [2026-08-01 外出/出差与事假并存误计出勤](development-issues/2026.md#2026-08-01-p1-外出出差与事假并存误计出勤) |
| `attendance-toolbox` `dingtalk` `approval` `time-window` | `processinstance/listids` 禁止直接提交超长或未来时间范围；客户端必须按最多 120 天连续分片、结束时间预留时钟偏差，并跨片去重、全局执行条数上限 | [2026-08-01 钉钉审批查询时间范围非法](development-issues/2026.md#2026-08-01-p1-钉钉审批查询时间范围非法导致工具箱同步失败) |
| `attendance-toolbox` `structured-result` `auto-fill` | 自动回填必须从 structured run 按 `kind=export + flow_key` 下载业务表；审计/元数据不得上传，也不得因多文件而改走 ZIP 或重跑同步 | [2026-08-01 自动回填误判多文件](development-issues/2026.md#2026-08-01-p1-自动回填将审计文件计入结果导致同步成功后仍报错) |
| `attendance-toolbox` `roster` `data-contract` `multi-tenant` | 自动回填到加班入口的组织花名册必须使用当前 org 的 `EmployeeProfile.EmployeeID`、真实姓名与有效部门路径；当前 org 的 `<org>:0` 是兼容历史数据的根哨兵，外组织同形值仍属悬空；缺工号/姓名/路径整体 400；仅姓名文件不得自动回填加班；自动响应不得覆盖请求期间的用户选择/删除/替换；禁止审批字段/position_transfer 充当花名册 | [2026-08-01 花名册选错文件与运维组规则](development-issues/2026.md#2026-08-01-p1-花名册选错文件--运维组未强制标记未加) |
| `attendance-toolbox` `roster` `org-sync` `auto-repair` | 部门路径错误可在 `attendance_manage` 权限和统一确认框后复用异步组织同步，成功/部分成功后按原文件快照重试一次；其他错误、缺权限或重试失败禁止自动循环 | 同上 |
| `attendance-toolbox` `playwright` `locator` | 页面存在重复 placeholder/文案时，E2E 必须先用 tabpanel、form 或可访问名称缩小作用域；禁止使用全页面模糊定位 | [2026-07-30 考勤工具箱 E2E 重复日期占位符](development-issues/2026.md#2026-07-30-p2-考勤工具箱-e2e-重复日期占位符导致-strict-mode-失败) |
| `attendance-toolbox` `date-boundary` `probation` | 日期区间条件必须使用闭区间语义，覆盖月初、月末、节假日和闰年边界；禁止对工作日使用严格大于/小于导致边界漏算 | [2026-08-02 当月转正天数少算一天](development-issues/2026.md#2026-08-02-p1-当月转正天数少算一天日期区间左边界漏算) |
| `attendance-toolbox` `subsidy` `data-source` `column-alias` | 补贴扣款真实数据来源是"考勤统计→报表管理→月度汇总表（补贴及扣款）"人工导出 Excel，不是钉钉审批流程；列名必须精确匹配，禁止使用"迟到""早退"等宽泛别名；A1 日期强校验仅作用于 `_is_all_people_monthly_summary` 已识别的钉钉原始报表，且统计范围必须精确覆盖处理月份的完整自然月，系统模板和历史兼容格式不要求 A1 日期 | [2026-08-03 补贴扣款数据来源纠正](development-issues/2026.md#2026-08-03-p2-补贴扣款数据来源纠正) |

### API / 路由契约

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `employee-profile` `search` `api-contract` `test` | 员工档案搜索必须走 `/employee/profiles` 的 handler → EmployeeService → EmployeeRepository 真实链路；禁止用 `/org/employees` 花名册测试代替；关键词、分页和 URL 状态需做前后端契约回归 | [2026-07-31 员工档案搜索错测花名册链路](development-issues/2026.md#2026-07-31-p1-员工档案搜索错测花名册链路导致页面无法搜索) |
| `org-sync` `frontend` `timeout` `api-contract` `multi-tenant` `security` | 用户/部门/全量组织同步共享同组织门闩；JWT `org_id` 唯一可信；超过网关时限的全量同步必须短请求启动+轮询；执行上下文脱离客户端取消，终态用独立短上下文持久化；HTTP 207 必须刷新已成功数据；响应/状态不得回显原始错误 | [2026-07-27 组织全量同步被前端 10 秒超时误判失败](development-issues/2026.md#2026-07-27-p1-组织全量同步被前端-10-秒超时误判失败) |
| `approval-sync` `whitelist` `async-boundary` `timeout` `partial` `multi-tenant` `idempotency` | 审批同步范围只取当前 JWT 企业 `ConfigForOrgID(orgID).ProcessCodes`；`Prepare` 禁止外部调用；任务与 running 状态先落库、HTTP 202 先写出，再调度后台外部调用；逐流程失败隔离，审批及下游入账均须幂等 | [2026-07-27 组织全量同步被前端 10 秒超时误判失败](development-issues/2026.md#2026-07-27-p1-组织全量同步被前端-10-秒超时误判失败) |
| `approval-template` `approval-sync` `process-code` `multi-tenant` | 模板列表必须以当前组织配置的 `ProcessCodes` 补齐目录；数据库模板优先保留表单/节点详情；实例与模板关联统一使用 `extension.process_code`，禁止只查询未被写入的 `approval_templates` 表 | [2026-08-17 审批实例存在但模板目录为空](development-issues/2026.md#2026-08-17-p1-审批实例存在但模板目录为空) |
| `approval-sync` `reconciliation` `annual-leave` `overtime` `attendance` `state-reversal` `dingtalk` `concurrency` | 审批逐条对账覆盖有效↔无效冲正/恢复；凌晨 6 点前考勤同时影响打卡日与前一工作日；补偿队列按最久未尝试轮转；钉钉绝对同步失败只标记触发记录，从未外部同步的记录在开关关闭时可仅恢复本地额度 | [2026-08-07 历史审批补同步未触发下游业务对账](development-issues/2026.md#2026-08-07-p1-历史审批补同步未触发下游业务对账) |
| `org-sync` `department` `stable-id` `transaction` `release` | 钉钉同步必须按租户内稳定外部 ID 匹配历史部门/员工，保留既有本地 ID 与引用；部门写入失败整事务回滚并跳过员工；发布镜像必须来自可追溯干净 Commit | [2026-07-28 组织同步历史 ID 冲突](development-issues/2026.md#2026-07-28-p1-组织同步历史本地-id-与租户前缀-id-冲突导致部门落库失败) |
| `org-sync` `department-membership` `counting` `multi-tenant` | 完整部门归属写租户隔离关系表；查询仅在无关系时回退主部门；直属人数按完整关系，父级汇总按员工集合去重；**部署 membership 特性后所有已有组织必须重新同步，否则 0 条 membership 导致多部门员工被遗漏** | [2026-07-28 组织同步仅保存主部门导致部门人数偏少](development-issues/2026.md#2026-07-28-p1-组织同步仅保存主部门导致部门人数偏少) |
| `org-sync` `dingtalk` `mobile` `unique-index` `null` | 钉钉空手机号不得转换为共享占位值；新员工空手机号写 `NULL`，已有真实手机号不得被空值或占位值覆盖 | [2026-07-28 组织同步共享手机号占位值冲突](development-issues/2026.md#2026-07-28-p1-组织同步共享手机号占位值触发唯一索引冲突) |
| `attendance-toolbox` `timeout` `nginx` `gateway` | 工具箱长任务必须保持后端超时 < 网关超时 < 客户端超时；502/503/504 或 HTML 网关页只能映射为安全中文提示，禁止原样展示 | 同上（同根因复发记录） |
| `org-sync` `dingtalk` `hrm` `employee-profile` `field-filter-list` | 花名册职级/岗位序列/员工类型必须有钉钉 HRM 字段映射；**`field_filter_list` 必须包含标准系统字段代码（`sys01-employeeType`、`sys01-positionLevel`、`sys00-position`），否则 API 不返回对应字段**；空外部值不得覆盖本地人工值；HRM 接口成功但目标字段全空时返回 `success_no_fields` 诊断，禁止简单显示成功 | [2026-07-29 钉钉 HRM 字段代码未加入请求导致字段缺失](development-issues/2026.md#2026-07-29-p1-钉钉-hrm-字段代码未加入-field_filter_list-导致员工类型职级全缺失) |
| `org-sync` `dingtalk` `employee-profile` `entry-date` `timezone` | 钉钉 `hired_date` 必须兼容数字、数字字符串和日期字符串；时间戳固定按 UTC+8 转换，禁止依赖容器/CI 宿主时区；非空外部值用于修正档案，空值不得覆盖人工值 | [2026-08-18 钉钉入职时间解析依赖宿主时区与单一数值类型](development-issues/2026.md#2026-08-18-p1-钉钉入职时间解析依赖宿主时区与单一数值类型) |
| `org-sync` `dingtalk` `deactivation` `fail-closed` `multi-tenant` `session` | 只有部门+员工完整拉取成功后才收口历史员工；钉钉源为空、请求失败、权限失败或同步被取消时禁止批量停用；停用仅作用于本组织 active 且有稳定 DingTalkUserID 的同步用户，admin/手工账号/无稳定 ID 账号不动；用户与档案状态同事务更新，会话撤销只针对实际被停用员工 | [2026-07-28 组织同步历史员工状态收口](development-issues/2026.md#2026-07-28-p1-组织同步未收口历史员工状态导致离职员工仍为-active) |
| `api-contract` `router` `attendance` | 前端已调用且后端已有 Handler 的接口必须同时注册到 Router；新增 API 必须有路由清单回归测试，避免运行时 404 | [2026-07-23 考勤查询外部结果路由漏注册](development-issues/2026.md#2026-07-23-p1-考勤查询外部结果路由漏注册导致进入页面-404) |
| `external-sync` `timeout` `goroutine-leak` `ctx-cancel` `terminal-status` `db-lock` | 长任务必须先以 DB 原子门闩落 running 后返回 202；后台上下文脱离客户端取消且有硬超时；所有 error/panic/取消路径用独立短上下文写终态；启动与查询幂等收敛超时 running 为 failed；前端只在 running 时轮询并禁用重复提交 | [2026-08-15 外部考勤同步任务卡在 running 导致无法重新启动](development-issues/2026.md#2026-08-15-p1-外部考勤同步任务卡在-running-导致无法重新启动) |
| `gorm` `update-columns` `terminal-status` `dingtalk` | `Updates/UpdateColumns(map)` 的 key 必须使用真实数据库列名；缩写字段优先显式 `column:` 或查 schema；终态日志写入错误不得忽略，更不得继续返回受理/成功 | [2026-07-29 群推送日志终态未落库](development-issues/2026.md#2026-07-29-p2-gorm-缩写字段列名不一致导致群推送日志停留-processing) |
| `dingtalk-stream` `week-schedule` `chatbot` `group-binding` `fail-closed` | 当前接入不能靠机器人进群事件取得 `openConversationId`，禁止编造事件；首次群内任意非空 @从真实 ChatBot 回调绑定；退群不可实时感知；只有真实验证并显式登记的原始错误码可停用，禁止按错误文案猜测 | [2026-07-31 Stream 无进群事件与群失效误判风险](development-issues/2026.md#2026-07-31-p1-stream-无进群事件与群失效误判风险) |
| `attendance` `data-contract` `metrics` `frontend` | 同一业务指标必须明确数据源、计数单位和分母；禁止首页和业务页面各自维护不同统计口径 | [2026-07-23 P2 考勤统计口径分裂](development-issues/2026.md#2026-07-23-p2-考勤统计口径分裂导致数字不可比) |

### 前端 / 时间展示

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `attendance` `frontend` `timezone` `test` | 考勤时间必须显式按业务时区 UTC+8 格式化；禁止依赖浏览器、Node 或 CI 宿主机时区 | [2026-07-24 考勤时间展示依赖宿主时区](development-issues/2026.md#2026-07-24-p2-考勤时间展示依赖宿主时区导致-ci-失败) |
| `week-schedule` `notification-copy` `date-aware` | 作息表通知必须定位最近周六并读取实际日历状态；周五写“明天”、周六写“今天”、周一至周四写“本周六”、周日写“下周六”；是否上班置于首行，大/小周仅作补充 | [2026-07-29 作息表推送只判断明天导致周六提醒不灵活](development-issues/2026.md#2026-07-29-p2-作息表推送只判断明天导致周六上班提醒不灵活) |

### 测试 / 验证

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `go-test` `sqlite` `test-isolation` `count` | 共享内存 SQLite 测试 DSN 必须每次调用唯一并在测试结束关闭连接；禁止只用 `t.Name()`，否则 `go test -count=N` 会跨轮复用数据 | [2026-08-07 SQLite 测试 DSN 跨轮复用](development-issues/2026.md#2026-08-07-p2-sqlite-测试-dsn-跨轮复用导致重复执行污染) |

### 部署 / 配置

| 标签 | 约束摘要 | 条目 / 文档 |
|---|---|---|
| `deploy` `test-server` | 测试服隔离目录/端口/Compose 项目名；完整变量见 `deploy/peopleops.test.env.example`，文档不复制密钥 | `deploy/TEST_SERVER_DEPLOY.md` |
| `dingtalk-stream` `multi-tenant` `credentials` `app-home-url` `fail-closed` | Stream 显式组织必须读取同一组织的 AppKey/Secret；Compose 禁止默认覆盖为 `default`；非 default 组织群图片推送必须配置组织级公网 HTTPS AppHomeURL/RedirectURI；上线核对 org、healthy、重启次数并执行真实绑定/推送 | [2026-07-29 Stream 连接错误组织导致群机器人无响应](development-issues/2026.md#2026-07-29-p1-dingtalk-stream-默认绑定错误组织导致群机器人无响应) |
| `deploy` `upload-and-restart` | 上传失败续传用独立脚本，禁止改 `build-and-deploy.ps1` 行为 | cerebrum Decision Log |
| `git` `merge` `release` | 跨分支冲突必须按语义单元核对生产代码、配套测试和 `go.mod/go.sum`；禁止按文件整侧选取后直接推送，至少执行编译、全量测试与双远端祖先检查 | [2026-07-23 双远端 master 合并遗漏依赖与安全配套代码](development-issues/2026.md#2026-07-23-p1-双远端-master-合并遗漏依赖与安全配套代码) |

### AI 协作 / 上下文

| 标签 | 约束摘要 | 条目 |
|---|---|---|
| `ai-context` `agents-md` `documentation` `token-budget` | 根 `AGENTS.md` 只保留硬规则与路由，详细流程按阶段读取；纯后端任务不加载设计规范；问题日志只读索引和相关条目；禁止在 `AGENTS.md`/`CLAUDE.md` 复制同一套规则 | [2026-08-15 AI 默认指令与工作流文档过大](development-issues/2026.md#2026-08-15-p2-ai-默认指令与工作流文档过大) |

---

## 写入规则

### 开发前（必读）

1. 打开本文件，阅读「开发前必读索引」。
2. 按当前任务模块/标签，沿索引链接读取对应年度归档中的相关条目正文（不必通读全部历史）。
3. 在修改计划中写明：**适用的防复发约束**（引用条目日期或标签）。

### 开发后（必记）

任务收尾、写复盘总结前，必须判断：

| 情况 | 动作 |
|---|---|
| 已定位根因，且对后续开发有复用价值 | 在对应年度归档文件中**新增**条目，或**更新**同根因已有条目；同步本文件索引 |
| 同根因再次出现 | 在对应年度归档文件更新原条目的修复/验证/状态，禁止另开重复条目；同步本文件索引 |
| 仅一次性环境故障、外部工具临时不可用，且未形成项目内改进项 | 可不写入问题日志，但最终总结必须说明原因 |
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
- **归档区**（`docs/development-issues/YYYY.md`）：完整复盘；按时间倒序，新条目置顶于对应年度文件。
- 模块标签用于过滤；状态：`open` / `fixed` / `wontfix`。

---

## 统一条目模板

在对应年度归档文件顶部复制以下模板新增条目：

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

## 问题归档

| 年份 | 完整条目 | 维护方式 |
|---|---|---|
| 2026 | [2026 年开发问题条目](development-issues/2026.md) | 新条目置顶；同根因更新原条目 |

开发前只沿顶部索引打开相关条目，不默认全文读取年度归档。

---

## 维护约定

- 在对应年度归档文件新增条目后，同步更新本文件顶部「开发前必读索引」对应行。
- 同一根因只保留一条主记录；历史过程写在同一年度归档条目内追加，不拆多条重复现象。
- 新年份首次出现条目时，新建 `docs/development-issues/YYYY.md`，并在本文件「问题归档」登记。
- 索引与年度归档由 AI 与人工共同维护；流程入口见 `AGENTS.md`、`.ai/workflow/requirements.md` 与 `.ai/workflow/retrospective.md`。
