---
purpose: 项目整体架构、数据流、核心设计约束
last_updated: 2026-07-29
source_of_truth:
  - go.mod（后端技术栈）
  - frontend/package.json（前端技术栈）
  - internal/api/router.go（路由架构）
  - internal/database/database.go（数据库初始化）
  - frontend/src/services/api.ts（前端 API 封装）
  - frontend/src/store/authStore.ts（状态管理）
update_when:
  - 修改技术栈时
  - 修改分层架构时
  - 修改数据流时
  - 修改跨模块调用方式时
  - 新增架构约束时
---

# 架构设计

## 技术栈

### 后端
- Go 1.20
- Gin（HTTP 框架）
- GORM（ORM）
- MySQL（主业务库）
- Redis（缓存）
- JWT（认证）
- Logrus（日志）

### 前端
- React 18
- TypeScript
- Vite 4
- Ant Design 5
- Zustand（状态管理）
- Axios（HTTP 客户端）
- React Query（数据获取）

### 外部集成
- 钉钉开放平台 API
- 聚合数据节假日接口（可选）

---

## 数据流

```
钉钉开放平台
    ↓ 同步（定时/手动）
本地 MySQL
    ↓ API 查询
前端页面
```

### 核心流程

1. **组织架构同步**：钉钉部门/用户 → 本地 `departments` / `users` 表
2. **考勤同步（钉钉 API）**：钉钉打卡记录 → 本地 `attendances` 表
3. **考勤同步（外部 Doris，一期）**：Doris 只读表 → `external_*` staging → `attendances` / `user_department_relations`（按 JWT org 隔离，未知 corp 拒绝）
4. **审批同步**：短请求启动 → 持久化 `SyncStatus` → 按当前企业流程代码逐流程拉取钉钉审批实例 → `approvals` 幂等 upsert → 逐条下游业务对账 → 前端轮询完整终态
5. **年假发放**：本地计算 → 写入 `annual_leave_grants` → 同步到钉钉假期配置
6. **加班匹配**：钉钉审批 + 本地打卡 → 计算有效加班时长 → 写入 `overtime_match_results` → 生成调休余额 → 同步到钉钉
7. **大小周排班与通知**：本地配置 `week_schedule_rules` → 计算每周班次 → 同步到钉钉考勤组；或生成月作息表 → 个人消息 / 已绑定企业内部机器人群消息
8. **绩效管理**：活动创建 → 参与人刷新 →（沐腾）双门槛开启 + 参与人独立流水线 /（旧流程）统一阶段推进 → 结果确认/公布 → 归档
9. **员工全生命周期**：入职 → 档案管理 → 转岗 → 离职
10. **补卡申请**：员工提交补卡 → 审批流程 → 钉钉同步
11. **考勤工具箱文件计算**：React multipart 参数/Excel → Go `AttendanceToolboxService` → Python runner；Python 生成业务 Excel 与 `kind=meta` JSON，Go 合并到结构化运行响应 `meta/stats`，前端据此展示异常提示且不把 meta 文件作为用户下载结果

### 外部 Doris 考勤同步（一期）

1. **首次回填**：无 cursor 时从 `EXTERNAL_ATTENDANCE_INITIAL_START_TIME`（默认 Unix epoch，全量历史）读取 Doris 只读表
2. **写入 staging**：`external_attendance_raw` / `external_user_department_raw` / `external_attendance_approve_links`，按 org 隔离
3. **落业务表**：映射本地用户后 upsert `attendances` / `user_department_relations`
4. **推进 cursor**：成功完成后写入 `(cursor_time, cursor_tie_key)`；之后 cron 用 cursor + lookback 增量
5. **串行锁**：同一 org 下 attendance/department/all 共用 `scope_key=external-attendance`

### 审批异步同步

1. **租户与计划**：Handler 只使用 JWT `org_id`；空 `process_code` 从 `ConfigForOrgID(orgID).ProcessCodes` 生成去空、去重、稳定排序的全量计划，非空只生成单流程计划。非默认企业缺配置直接失败，不回退默认企业。
2. **短启动与持久化**：`POST /approvals/sync/start` 在返回 `202 + request_id` 前写入 `SyncStatus(type=approvals,status=running)`；前端用 `GET /approvals/sync/:request_id` 短轮询，查询同时绑定 `org_id + type + request_id`。
3. **后台执行**：执行上下文通过 `context.WithoutCancel` 脱离客户端断开并受 15 分钟上限约束；终态写入使用独立 5 秒上下文。同企业进程内 `TryLock` 防重，不同企业并行；多实例部署需要分布式互斥或任务队列。
4. **分流程容错**：`listids` 每次只查询一个流程代码，日期按最多 120 天连续分片、结束时间不超过安全当前时间，并按实例 ID 去重。流程之间独立拉取和写入，一个流程失败不阻断后续流程。
5. **逐条业务对账**：每条审批 upsert 成功后调用可注入的 `ApprovalBusinessReconciliationService`。`COMPLETED + agree` 为有效态；有效→无效执行冲正，无效→有效重新入账一次。年假使用表单业务日期，加班使用实际工作日期；所有读写显式绑定当前 `org_id`。
6. **终态与安全**：对账结果区分 `applied/skipped/reversed/retryable/failed`，逐流程汇总计数并聚合为 `success/partial/failed`。retryable 或 failed 进入 partial；单条失败不回滚审批且不阻断后续记录。响应与 `SyncStatus.details` 只保存稳定错误码和安全文案。
7. **业务幂等与事务**：审批按 `org_id + process_id` upsert。年假请求门闩 `AnnualLeaveConsumeRequest(org_id, request_ref)` 与 grant 行锁、余额、正向/冲正日志同事务，适配 MySQL `REPEATABLE READ`；加班调休按来源净额在锁定匹配记录后 credit/rollback。事务失败不得遗留永久 running/pending。

---

## 核心设计约定

### 后端

#### 分层架构
```
Handler (api/)
    ↓
Service (service/)
    ↓
Repository (repository/)
    ↓
Model (database/models.go)
```

- **Handler 层**：Gin handler 直接在 `api/` 包，无 controller 层分离
- **Service 层**：业务逻辑，调用 repository 和外部服务（钉钉）
- **Repository 层**：数据访问封装，GORM 操作
- **Model 层**：GORM 模型定义，集中在 `models.go`

#### 统一响应格式
```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

#### 认证
- JWT Bearer token
- Claims 含 `UserID` + `UserName` + `OrgID`
- Handler 内通过 `c.Get("userID")` 取当前用户，通过 `c.Get("orgID")` 取当前企业上下文
- 钉钉多企业登录必须使用选中企业的 `org_id` 解析钉钉应用配置、换取用户信息并签发同企业 JWT，禁止选中企业后静默回退默认钉钉应用
- 中间件：`internal/middleware/jwt.go`

#### 通用文件上传/下载（组织隔离）
- 元数据表 `uploaded_files`（`UploadedFile`）：`org_id` 所有权 + `stored_name` / `original_name` / `content_type` / `size` / 软删除
- 磁盘路径：`uploads/<safe_org_id>/<stored_name>`；`safe_org_id` 仅允许 `[A-Za-z0-9._-]`，含 `..` 或路径分隔符则拒绝
- 对外 URL：`/api/v1/files/:file_id`（数字主键），不暴露物理路径与组织目录
- 路由：`POST /upload` 与 `GET /files/:file_id` 均在 `JWTAuth + TenantContext` 下；归属只取 JWT `org_id`，禁止 body/query/header 切 org
- 下载查询：`middleware.RequestDB(c)` + org-scoped repository；当前组织无记录时统一 **404**（跨 org 不暴露是否存在）
- 旧 URL `/api/v1/files/<random_filename>`：**fail-closed**（无元数据所有权证明即 404），不开放全企业访问
- 安全保留：扩展名白名单、内容嗅探、ClamAV（可选）、`X-Content-Type-Options: nosniff`、路径穿越防护
- 绩效附件：业务字段仍存 URL 字符串；`AttachmentUpload` + `authFileUrl` 用 `credentials: include` 预览，新上传自然得到 file_id URL

#### 作息表群聊临时图片
- 群机器人 Markdown 不支持复用个人企业消息 `media_id`，使用钉钉可访问的 HTTPS 临时地址。
- 图片仅在内存短期保存；URL 令牌为 32 字节 CSPRNG，map key 仅存 SHA-256，默认 10 分钟过期；无效/过期统一 404。
- 公共入口固定为 `/api/v1/week-schedule/group-image?token=...`，不得返回物理路径、组织目录或永久公开文件；访问日志不得记录查询令牌。

#### 钉钉 ID 存储
- `User.UserID` 和 `Department.DepartmentID` 存钉钉原始 ID（字符串），不是本地自增主键
- 本地自增主键是 `ID` 字段（uint）

#### 软删除
- 主要模型使用 `gorm.DeletedAt` 软删除
- 查询时 GORM 自动过滤已删除记录

#### JSON 字段
- 扩展数据用 `map[string]interface{}` 配合 `gorm:"type:json;serializer:json"`
- 例如：`User.Extension`、`Approval.Content`

#### 幂等性设计
- **年假消费**：`AnnualLeaveConsumeRequest(org_id, request_ref)` 请求级门闩；`AnnualLeaveConsumeLog` 可按多个 grant 记录正向/冲正明细
- **加班同步**：`OvertimeSyncHistory` 快照，避免重复同步
- **加班匹配**：`OvertimeMatchResult.match_ref` 用于当前幂等，历史数据仍兼容 `user_id+work_date` 口径

#### 数据库初始化
- 启动时自动迁移（`AutoMigrate`）
- 库为空时种默认管理员 `admin / admin123`
- MySQL 连接失败时会尝试自动创建数据库后重连

#### 容错设计
- Redis 或钉钉客户端初始化失败不会阻止服务启动
- 相关功能会受影响，但基础服务可用

---

### 前端

#### 状态管理
- 认证状态：Zustand store 持久化到 localStorage，key 为 `peopleops-auth`
- 包含：`user`、`token`、`isLoggedIn`
- 动作：`login(user, token)`、`logout()`

#### API 调用
- 统一从 `frontend/src/services/api.ts` 进入
- Axios 实例 baseURL=`/api/v1`，timeout=10s
- 请求拦截：自动从 authStore 读取 token 加入 `Authorization: Bearer`
- 响应拦截：401 自动 logout + 跳转 `/login`

#### 路由与菜单
- `frontend/src/App.tsx` 同时负责主路由、菜单和钉钉内免登流程
- `/employees/:id` 会复用 `/employees` 的菜单高亮

#### 本地开发
- Vite 代理 `/api` 到 `http://localhost:8080`
- 前端默认端口 `3000`

#### UI 库
- Ant Design 5
- 所有表格/表单/弹窗使用 antd 组件

---

## 关键业务流程

### 加班→调休流程
1. 从钉钉同步加班审批（异步审批同步；单模板或当前企业全量流程）
2. 审批 upsert 后对明确通过记录直接运行单条匹配；定时/手动 `RunOvertimeMatch` 继续作为补偿入口。匹配使用审批表单的实际加班日期，不使用同步日或审批完成日
3. 暂缺考勤的零分钟状态保留为 retryable；考勤补齐后原位重算，成功后写入 `CompensatoryLeaveLedger`（credit）并关闭 pending 补卡请求
4. 审批撤销写 `rollback` 台账并回写年度绝对余额；审批恢复后只重新 credit/同步一次

### 年假发放流程
1. 按季度计算员工年假资格（`AnnualLeaveEligibility`）
2. 运行季度发放（`RunQuarterGrant`）→ 写入 `AnnualLeaveGrant`
3. 同步到钉钉假期配置（`SyncGrantsToDingTalk`）

### 大小周排班流程
1. 配置 `WeekScheduleRule`（基准周 + pattern）
2. 可手动覆盖特定周（`WeekScheduleOverride`）
3. 同步到钉钉班次（`WeekScheduleService.SyncToDingTalk`）：按用户分配钉钉 Shift；读写钉钉必须走当前 JWT `org_id` 的 access_token / `op_user_id`
4. 从钉钉回读（`SyncFromDingTalk` / `SyncFromDingTalkConservative`）仅调用 `GetScheduleListBatchByDayForOrg(orgID, ...)`，禁止非 default 企业走默认 `GetScheduleListBatchByDay` / `GetAccessToken`
5. 群聊绑定由 `cmd/dingtalk_stream` Chatbot 回调处理；Stream AppKey 必须唯一解析到 active 组织或与显式 `DINGTALK_STREAM_ORG_ID` 一致，禁止 default 回退。
6. 群内 @机器人“绑定作息表”时，发送者需映射当前组织在职用户并具备 `week_schedule_group_push`（兼容 `attendance_manage`）；只把本地群目标 ID 暴露给前端。
7. 群推送通过 `robot/groupMessages/send` 提交 Markdown 文字 + HTTPS 临时图片；`submitted` 只表示钉钉受理，必须在本地日志终态持久化后返回。

### 绩效管理流程
1. **活动配置**：HR 创建绩效活动，设置时间范围、参与人范围、关联指标库；按 `flow_type` 区分小铁文娱旧流程与沐腾科技新流程
2. **参与人刷新**：根据部门/员工范围筛选参与人；沐腾新建参与人默认 `ResultHidden=true`、`reason=system:unpublished`
3. **目标设定**：沐腾由管理员开启目标填写门槛后，参与人独立提交/上级审核，不等待其他人；目标设定活动全部完成后活动汇总 `locked`
4. **员工自评**：沐腾由管理员开启员工自评门槛后，已就绪参与人可独立提交自评并进入上级评分
5. **评分流水线（沐腾独立推进）**：上级评分 → 部门/中心评分 → HR 审核；各节点按**参与人状态**判断，活动汇总状态不阻塞个人
6. **HR 审核即公布（沐腾）**：`ConfirmHRResult` 原子设置 `hr_confirmed` 与公布字段，仅解除 `system:unpublished`；人工屏蔽保留；面谈与申诉同时对该参与人开放
7. **活动汇总与归档**：最后一名有效参与人完成 HR 审核后活动汇总 `result_publish`，可归档；旧流程仍为员工/主管/HR 三级确认后锁定归档

### 员工全生命周期
1. **入职**：新员工入职流程，写入 `EmployeeOnboarding`
2. **档案管理**：维护员工档案信息，写入 `EmployeeProfile`
3. **转岗**：员工转岗流程，写入 `EmployeeTransfer`
4. **离职**：员工离职流程，写入 `EmployeeResignation`

### 权限管理
- RBAC 模型：`Role` → `Permission` → `RolePermission` → `UserRole`
- `Role`、`Permission`、`RolePermission`、菜单权限与数据权限定义保持全局；`UserRole` 按 `(org_id, user_id)` 维度分配，运行时权限必须使用 JWT `org_id` 过滤，避免同一钉钉 `user_id` 在多企业之间串权限
- 支持菜单权限和数据权限
- 前端页面：角色管理、权限管理、菜单权限、数据权限

### 多租户隔离（org_id）运行时边界
- **入口**：`JWTAuth` 强制 claims 含 `org_id`；`TenantContext` 把 org 写入 `requestmeta.WithTenant` **并回写** `RequestInfo.OrgID`，供 `CurrentOrganizationIDFromDB` 与旧仓储路径使用。
- **Handler**：普通业务只读 JWT `orgID`（`currentOrgID` / `currentOrgIDOrAbort`）；`org_id`/`target_org_id` 请求参数一律 `rejectCrossOrgParam`。
- **Service/Repository**：优先 `NewXxxWithOrgID` + `scoped()`；后台任务（年假/加班/外勤/绩效提醒）必须 `ListActiveOrganizations` 后**逐 org** 构造服务，禁止空 org 全表业务扫描。
- **tenant-scoped 缺 org fail-closed**：仓储/服务在缺少 `orgID` 时必须硬失败（如 `repository: orgID required for tenant-scoped operation` / `ErrMissingOrgID`），禁止发明 `default`、禁止无过滤全表扫描。
- **已解析组织优先 `NewXxxWithOrgID`**：Handler/登录路径一旦解析出 `orgID`（JWT、钉钉回调 `resolvedOrgID`、state 等），构造用户/部门/审批等服务时必须 `NewXxxServiceWithOrgID(db, orgID)`，**禁止**先 `NewXxxService(db)` 再指望实体字段补作用域。
- **实体字段 `OrgID` ≠ 仓储绑定**：`User.OrgID` / 模型上的 `org_id` 列只描述数据归属；**不能**替代 repository/service 构造时注入的组织绑定。跨 org 实体写入必须拒绝。
- **外部稳定身份 ≠ 本地引用 ID**：钉钉同步以租户内 `dingtalk_department_id` / `ding_talk_user_id` 识别同一外部实体；`department_id` / `user_id` 是内部引用，历史值可能未加租户前缀。upsert 必须优先按 `org_id + 外部稳定 ID` 匹配并保留既有本地 ID，再解析父部门、部门、主管、档案和角色引用，禁止用新 scoped ID 直接插入撞唯一键或破坏历史引用。
- **钉钉工具调用**：考勤工具箱等直接调用钉钉的功能必须使用 JWT `org_id` 解析 `organizations` 中的 AppKey/AppSecret；审批流程码存于 `organizations.extension.dingtalk_process_codes`，非默认组织禁止回退全局钉钉凭证或流程码，且请求 body/multipart 不得覆盖服务端解析的组织配置。
- **Stream 回调组织归属**：长连接启动时必须把当前 AppKey fail-closed 绑定到唯一 active `org_id`；群聊绑定只使用该 Stream 组织上下文，不从消息体推断或回退 default。群目标查询、解绑、推送继续只认 JWT `org_id`。
- **钉钉 op_user_id（企业管理员）**：`organizations.ding_talk_admin_user_id`（模型字段 `DingTalkAdminUserID`）为权威来源；`dingtalk.ResolveAdminUserID(orgID)` / `ResolveAdminUserIDFromConfig` 统一解析。非 default 企业**禁止**回退全局 `DINGTALK_ADMIN_USER_ID`；default 企业可在 `DefaultConfig` / `ConfigFromOrganization` 配置解析层兼容环境变量。排班读写、班次创建、年假/调休额度写钉钉均须走该解析，业务层禁止直接 `os.Getenv("DINGTALK_ADMIN_USER_ID")`。
- **班次 ID 进程缓存**：`ShiftConfigService` 的 `shiftIDCache` key 必须为 `orgID|shiftKey`；提供 `ClearShiftIDCacheForTest` 避免测试互相污染。相同班次名+时间在不同企业不得共享钉钉 `shift_id`。
- **缺配置 fail-closed**：非 default 企业缺少 App 凭证或 `DingTalkAdminUserID` 时，排班同步/班次创建/假期写钉钉须直接报错，禁止写库后 partial 成功、禁止静默用 default 企业 token。
- **双上下文**：`requestmeta.TenantID`（严格）与 `RequestInfo.OrgID`（兼容）均可能携带 org；绩效等服务构造时优先 Tenant，再回退 RequestInfo；异步 goroutine 必须两者都注入。
- **全局表**（有意不绑业务 org）：`organizations`、`permissions`、`role_permissions`；`Permission.Code` 等全局唯一。业务唯一键见 `docs/org_composite_unique_index_migration.md`。
- **相关复盘**：`docs/DEVELOPMENT_ISSUES.md`（2026-07-20 钉钉多组织登录更新用户缺少组织作用域）。
### 审计日志
- 记录所有操作日志，写入 `OperationLog`
- 支持按用户、操作类型、时间范围查询

### 补卡申请
1. 员工提交补卡申请，写入 `OvertimeSupplementaryRequest`
2. 审批流程
3. 同步到钉钉

---

## 环境变量

### 基础运行
- `PORT`：服务端口，默认 8080
- `DATABASE_URL`：MySQL 连接串，格式 `user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True`
- `REDIS_URL`：Redis 地址，格式 `localhost:6379`（当前代码直接传给 `redis.Options.Addr`，不要带 `redis://` 前缀）
- `REDIS_PASSWORD`：Redis 密码（可选）
- `JWT_SECRET`：JWT 签名密钥

### 钉钉集成
- `DINGTALK_APP_KEY`：钉钉应用 Key（default 企业兼容；多企业优先 `organizations.ding_talk_app_key`）
- `DINGTALK_APP_SECRET`：钉钉应用 Secret（default 企业兼容）
- `DINGTALK_CORP_ID`：钉钉企业 ID（default 企业兼容）
- `DINGTALK_AGENT_ID`：钉钉应用 Agent ID（default 企业兼容）
- `DINGTALK_ADMIN_USER_ID`：default 企业钉钉管理员 userid（`op_user_id`）兼容项；多企业写入 `organizations.ding_talk_admin_user_id`，由配置解析层封装，非 default 禁止业务侧读此环境变量
- `DINGTALK_ORGANIZATIONS`：JSON 数组种子/更新多企业钉钉配置，字段含 `org_id` / `corp_id` / `app_key` / `app_secret` / `agent_id` / `admin_user_id`|`dingtalk_admin_user_id` / `process_codes` 等
- `DINGTALK_REDIRECT_URI`：OAuth 回调地址
- `DINGTALK_APP_HOME_URL`：应用首页地址
- `DINGTALK_STREAM_ORG_ID`：Stream 实例显式组织；未配置时当前 AppKey 必须唯一匹配 active 组织
- `DINGTALK_ROBOT_CODE`：default 企业机器人编码兼容项；多企业优先 `organizations.extension.dingtalk_robot_code`，未配置时使用本组织 AppKey
- `APP_BASE_URL`：后端服务地址
- `FRONTEND_BASE_URL`：前端服务地址

### 假期/调休同步
- `DINGTALK_LEAVE_SYNC_ENABLED`：是否启用年假同步（true/false）
- `DINGTALK_COMP_TIME_SYNC_ENABLED`：是否启用调休同步（true/false）
- `DINGTALK_LEAVE_HOURS_PER_DAY`：每天工作小时数（用于天数换算）
- `DINGTALK_ANNUAL_LEAVE_CODE`：钉钉年假假期类型 Code
- `DINGTALK_ANNUAL_LEAVE_NAME`：钉钉年假假期类型名称
- `DINGTALK_LIEU_LEAVE_CODE`：钉钉调休假期类型 Code
- `DINGTALK_LIEU_LEAVE_NAME`：钉钉调休假期类型名称
- `DINGTALK_COMPENSATORY_LEAVE_CODE`：钉钉补偿假期类型 Code
- `DINGTALK_COMPENSATORY_LEAVE_NAME`：钉钉补偿假期类型名称
- `ANNUAL_LEAVE_APPROVAL_KEYWORD`：年假审批关键词（用于识别年假审批）

### 排班与节假日
- `DINGTALK_ATTENDANCE_GROUP_ID`：钉钉考勤组 ID
- `DINGTALK_ATTENDANCE_GROUP_NAME`：钉钉考勤组名称
- `JUHE_API_KEY`：聚合数据节假日接口 Key（可选）

### 测试
- `TEST_DATABASE_URL`：测试数据库连接串
- `SKIP_INTEGRATION_TESTS`：是否跳过集成测试（true/false）

---

## 数据与同步说明

- MySQL 是主业务库，启动时若数据库不存在会尝试自动创建
- Redis 当前主要用于缓存，初始化失败时服务仍可启动
- 钉钉客户端初始化失败时，服务仍可启动，但免登、同步、假期回写等能力会受影响
- 周排班、年假发放、加班匹配都依赖数据库迁移结果，修改模型时要同时考虑 migration 兼容性
- 后端静态托管的是已构建产物（`frontend/dist`），不会自动触发前端构建

---

## 协作建议

- 看接口入口时，优先从 `internal/api/router.go` 顺着 `handler → service → repository → model` 往下追
- 做排班、年假、调休相关改动时，先确认是否同时影响"本地台账"和"钉钉同步"
- 做前端联调时，注意很多页面依赖登录态和 `/api/v1/auth/me`
- 验证后端托管的前端路由前，记得先执行 `cd frontend && npm run build`
