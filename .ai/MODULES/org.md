---
purpose: 组织与员工模块业务规则说明
last_updated: 2026-07-28
source_of_truth:
  - internal/api/handlers.go（组织相关 handler）
  - internal/service/user_service.go（用户服务）
  - internal/service/org_service.go（组织服务）
  - internal/database/models.go（Department、User、EmployeeProfile 模型）
  - frontend/src/pages/DepartmentTree.tsx（部门树）
  - frontend/src/pages/EmployeeList.tsx（员工列表）
  - frontend/src/pages/EmployeeDetail.tsx（员工详情）
update_when:
  - 修改组织同步逻辑时
  - 修改员工查询逻辑时
  - 修改员工详情聚合结构时
  - 修改部门树展示逻辑时
---

# 组织架构模块

## 模块定位

管理部门树、员工列表、聚合员工详情，并从钉钉同步组织架构数据。

组织模块支持多个钉钉企业共用一套系统，统一以 `org_id` 隔离部门、员工、档案和组织同步结果。同一自然人可存在于多个企业；跨企业用户和部门不会复用本地系统 ID。

花名册、组织概览、部门树、员工生命周期台账等组织查询如果使用 `users`、`departments`、`employee_profiles` 的 join 或子查询，必须同时约束当前 `org_id`；员工档案 join 必须使用 `employee_profiles.org_id = users.org_id`，按部门筛选用户的子查询也必须包含 `users.org_id = 当前企业`。

本次阶段 1A 在组织模块侧只沉淀员工详情聚合与档案字段补齐相关长期知识，不涉及组织分析、绩效、权限、强制分布、C/D 面谈等能力变更。

阶段 2A 已补充组织概览的最小统计能力，仅覆盖基础统计卡片，不扩展到趋势分析、复杂图表、绩效、假勤、权限、强制分布、C/D 面谈等内容。

### 阶段 2A：组织概览最小能力

`GET /api/v1/org/overview` 当前沉淀的最小统计项：
- 在职人数
- 试用期人数
- 计划转正预警数量
- 员工类型分布
- 职级分布
- 岗位序列分布

当前统计口径：
- 三组分布按在职员工统计。
- 试用期人数按“在职且未填写实际转正日期，并且存在计划转正日期或试用期结束日期”的员工统计。
- 计划转正预警数量按当前代码实际逻辑统计：`buildEmployeeWarnings()` 会先以 `planned_regular_date` 优先、`probation_end_date` 兜底生成 `probation_due` 预警；仅当员工在职、未填写 `actual_regular_date`、该日期可解析、且落在“今天到未来 30 天”窗口内时，`buildOverviewSummary()` 才会累计到 `planned_regularization_count`。当前该数量与 `probation_due_count` 使用同一触发条件。
- 前端当前只做统计卡片展示，不做趋势、复杂图表。

### 钉钉智能人事字段同步

组织同步除通讯录基础字段外，还会调用 `topapi/smartwork/hrm/employee/v2/list` 拉取员工档案字段：
- 标准字段：计划转正日期 `sys01-planRegularTime`、实际转正日期 `sys01-regularTime`。
- 可配置字段：员工类型、职级、岗位序列、试用期结束日期，以及 HRM 岗位兜底字段。

同步约束：
- 钉钉返回非空值时才更新 `EmployeeProfile`，空值不得覆盖 HR 已手工维护的数据。
- `actual_regular_date` 只写实际转正日期，禁止再写入 `probation_end_date`。
- HRM 接口失败时，通讯录基础资料仍可落库，但员工阶段必须返回 `partial_failed`，并通过 `hrm_field_status/hrm_field_error` 明确提示。
- 同步响应统计在职员工的员工类型、职级、岗位序列和转正日期缺失人数。
- 默认组织可从环境变量读取字段映射；非默认组织只能读取本组织 `organizations.extension`，禁止回退默认企业映射。

多组织字段代码配置示例：
```json
{
  "dingtalk_hrm_field_codes": {
    "employment_type": ["企业实际字段代码"],
    "job_level": ["企业实际字段代码"],
    "job_family": ["企业实际字段代码"],
    "probation_end_date": ["企业实际字段代码"],
    "position": ["企业实际字段代码"]
  }
}
```

### 阶段 2B：部门维度基础统计 / 组织结构轻量分析

- 本阶段在阶段 2A `org/overview` 全局组织概览基础上，补齐部门维度的最小统计能力。
- 部门树节点人数口径：
- `direct_active_count` = 当前部门直接归属且在职人数。
- `active_count` = 当前部门含下级部门汇总后的在职人数。
- 选中部门后的统计卡片口径：
- 复用 `GET /api/v1/org/overview?department_id=...`。
- 默认含下级部门汇总。
- 统计项包括：汇总在职人数、试用期人数、计划转正预警数量。
- 前端入口：
- `frontend/src/pages/DepartmentTree.tsx`
- 只做轻量统计卡片，不做趋势、排行榜、复杂图表。
- 阶段 2B 明确未做：
- 绩效、假勤、权限、强制分布、C/D 面谈、薪酬、编制、预算、成本。

---

## 数据模型

### Department
部门模型

```go
type Department struct {
    ID           uint
    OrgID        string  // 当前钉钉企业/租户 ID
    DepartmentID string  // 系统内部门 ID（多企业时可为 org_id:钉钉部门ID）
    DingTalkDepartmentID string // 钉钉原始部门 ID
    Name         string
    ParentID     string  // 父部门钉钉 ID
    Order        int
    Extension    map[string]interface{}
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt
}
```

### User
用户模型（与认证模块共用）

```go
type User struct {
    ID           uint
    OrgID        string  // 当前钉钉企业/租户 ID
    UserID       string  // 系统内用户 ID（多企业时可为 org_id:钉钉用户ID）
    DingTalkUserID string // 钉钉原始用户 ID
    Name         string
    Email        string
    Mobile       string
    DepartmentID string  // 所属部门钉钉 ID
    Position     string
    Avatar       string
    Status       string  // active / inactive
    Extension    map[string]interface{}
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt
}
```

### UserDepartmentMembership

员工—部门多对多关系，按 `org_id + user_id + department_id` 唯一：

- `User.DepartmentID` 继续保存主部门，兼容现有业务和历史数据。
- 组织同步对 `DeptIDList` 去重，只保存当前租户中可解析的有效部门；第一条有效部门写 `is_primary=true`，并同步写回 `User.DepartmentID`。
- 员工已有关系记录时，部门查询与统计只认关系表；完全没有关系记录时才回退 `User.DepartmentID`。
- 关系替换必须在绑定租户的仓储中执行，并校验用户、部门都属于同一 `org_id`。
- 部门直属人数按关系表完整归属统计；含下级人数按 `user_id` 集合并集计算，兼任多个下级部门的员工只能汇总一次。
- 数据库迁移的唯一契约为 `org_id + user_id + department_id`，索引名为 `idx_user_department_membership`；迁移必须可重复执行，并禁止把缺失组织的关系行回填到默认租户。

### EmployeeAggregate
员工详情聚合视图（`GET /api/v1/org/employees/:id` 返回结构）

- `employee`：组织侧基础员工信息（`User`）
- `profile`：员工档案快照（`EmployeeProfile`，可为空）
- `scope`：当前登录人可见组织范围
- `department`：当前部门与组织路径
- `org_relation`：直属上级、直属下属、同部门人数
- `timeline`：入职、计划转正、实际转正、合同到期、调岗、离职、档案审计日志时间轴
- `warnings`：该员工关联的组织预警

---

## API 接口

### GET /api/v1/org/departments/tree
获取部门树

阶段 2B 节点人数口径补充：
- `direct_active_count`：当前部门直接归属且在职人数。
- `active_count`：当前部门含下级部门汇总后的在职人数。
- 兼任员工在每个直属部门各计一次；父部门含下级汇总按员工去重。
- 尚未生成 `user_department_memberships` 的历史员工回退 `User.DepartmentID`。

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": [
        {
            "id": 1,
            "department_id": "1",
            "name": "公司",
            "parent_id": "0",
            "children": [
                {
                    "id": 2,
                    "department_id": "2",
                    "name": "技术部",
                    "parent_id": "1",
                    "children": []
                }
            ]
        }
    ]
}
```

### GET /api/v1/org/overview
获取组织概览

补充说明：
- 当传入 `department_id` 时，当前统计默认按该部门及其下级部门汇总。
- 阶段 2B 的部门统计卡片直接复用该接口，不新增专用部门统计接口。

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "overview": {
            "scope": {
                "mode": "all",
                "department_names": []
            },
            "summary": {
                "active_employees": 95,
                "probation_employee_count": 8,
                "planned_regularization_count": 3
            },
            "employee_type_distribution": [
                { "key": "正式", "label": "正式", "count": 80 }
            ],
            "job_level_distribution": [
                { "key": "P5", "label": "P5", "count": 20 }
            ],
            "job_family_distribution": [
                { "key": "技术", "label": "技术", "count": 40 }
            ]
        }
    }
}
```

### GET /api/v1/org/employees
获取员工列表

Query 参数：
- `department_id`：部门 ID（可选）
- `status`：状态（可选，active/inactive）
- `keyword`：搜索关键词（可选，搜索姓名/手机/邮箱）
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 20）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "total": 100,
        "page": 1,
        "page_size": 20,
        "items": [
            {
                "id": 1,
                "user_id": "xxx",
                "name": "张三",
                "email": "zhangsan@example.com",
                "mobile": "13800138000",
                "department_id": "2",
                "position": "工程师",
                "avatar": "https://...",
                "status": "active"
            }
        ]
    }
}
```

### GET /api/v1/org/employees/:id
获取员工详情

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "detail": {
            "employee": {
                "id": 1,
                "user_id": "xxx",
                "name": "张三",
                "email": "zhangsan@example.com",
                "mobile": "13800138000",
                "department_id": "2",
                "position": "工程师",
                "avatar": "https://...",
                "status": "active"
            },
            "profile": {
                "employee_id": "EMP001",
                "profile_status": "active",
                "employment_type": "正式",
                "education": "本科",
                "job_level": "P5",
                "job_family": "技术",
                "entry_date": "2024-01-15",
                "planned_regular_date": "2024-04-15",
                "actual_regular_date": "2024-04-20"
            },
            "scope": {
                "mode": "department",
                "department_names": ["技术部"]
            },
            "department": {
                "id": "2",
                "name": "技术部",
                "path": [
                    { "id": "1", "name": "公司" },
                    { "id": "2", "name": "技术部" }
                ]
            },
            "org_relation": {
                "manager": {
                    "user_id": "leader-1",
                    "name": "李主管",
                    "department_name": "技术部",
                    "position": "技术经理"
                },
                "direct_reports": [],
                "same_department_count": 6
            },
            "timeline": [
                {
                    "type": "regularization_plan",
                    "title": "计划转正",
                    "date": "2024-04-15",
                    "status": "planned"
                },
                {
                    "type": "audit",
                    "title": "更新员工档案",
                    "date": "2026-04-30",
                    "operator_name": "管理员"
                }
            ],
            "warnings": []
        }
    }
}
```

### POST /api/v1/org/sync/start、GET /api/v1/org/sync/:request_id

页面组织同步使用短请求启动与轮询查询，避免 Nginx 或外层网关在完整同步结束前返回 HTTP 504：

- `POST /api/v1/org/sync/start` 校验当前 JWT 组织并取得同组织门闩，将 `organization/running/request_id` 先写入 `SyncStatus`，随后立即返回 HTTP 202。
- 后台任务复用既有全量同步核心，执行上下文脱离客户端取消并设置最多 15 分钟上限；超时、失败或 panic 都必须释放门闩，并使用独立的短收尾上下文写入安全终态。
- `GET /api/v1/org/sync/:request_id` 只查询当前 JWT `org_id` 下 `type=organization` 且 `request_id` 完全匹配的 `SyncStatus`；运行中返回 HTTP 202，完成后返回原完整组织同步结果。
- 完成状态保持原语义：成功 HTTP 200、部分失败 HTTP 207、失败 HTTP 500。查询其他组织或非当前任务的 request_id 统一返回 404，禁止暴露任务是否存在。
- 进程内存只保存同组织执行门闩，任务状态与完整结果必须持久化到 `SyncStatus.details.result`，不得依赖内存结果缓存。

### POST /api/v1/org/sync
同步组织架构的兼容长请求入口，保留给服务器内运维工具使用；页面不得再用该入口等待完整同步。

同步只使用当前 JWT 会话的 `org_id` 选择钉钉企业配置，只写入当前企业的部门、员工和档案数据；请求 body、query 或 header 不能切换组织。新实体的本地 ID 默认按当前 `org_id` 生成 scoped ID，原始钉钉 ID 保留在对应 `DingTalk*` 字段。历史实体可能仍使用未加租户前缀的本地 ID；同步必须先按 `org_id + dingtalk_department_id` / `org_id + ding_talk_user_id` 匹配并保留历史本地 ID，再解析父部门、员工部门、主管、档案与角色引用，禁止把历史实体误判为新增。

**并发保护**：`POST /sync/users`、`POST /sync/departments`、`POST /org/sync` 共享同一个按 `org_id` 的进程内门闩。同一组织正在执行任一组织同步时，其余入口返回 HTTP 409；不同组织互不阻塞。门闩通过 `defer` 在成功、失败或 panic 展开时释放。

当前门闩只适用于**单实例部署**。多实例部署需要后续引入分布式锁或异步任务队列；本阶段不引入 Redis 等新基础设施。

**长任务约束**：前端 `orgAPI.syncOrg()` 先短请求启动再轮询短查询，整体等待上限独立于 Axios 全局 10 秒超时；`syncAPI.syncUsers()`、`syncAPI.syncDepartments()` 仍使用各自的长请求超时。若页面停止轮询，后台任务仍按服务器执行上限完成并持久化终态，用户应先查看同步日志，避免重复提交。

Body：
```json
{}
```

Response（成功 HTTP 200，部分失败 HTTP 207，全部失败 HTTP 500）：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "overall_status": "success",
        "departments": {
            "status": "success",
            "success_count": 10,
            "fail_count": 0,
            "error": "",
            "error_code": ""
        },
        "employees": {
            "status": "success",
            "success_count": 100,
            "fail_count": 0,
            "error": "",
            "error_code": "",
            "position_missing_count": 5,
            "employment_type_missing_count": 3,
            "job_level_missing_count": 4,
            "job_family_missing_count": 2,
            "regularization_date_missing_count": 6,
            "hrm_field_status": "success",
            "hrm_field_error": "",
            "deactivated_missing_count": 2,
            "deactivation_status": "success",
            "deactivation_error": "",
            "overwrite_empty": false,
            "default_role_assigned_count": 10
        },
        "sync_time": "2026-07-27T10:00:00Z",
        "duration_ms": 60000,
        "request_id": "4c58d4c2e7a94a03"
    }
}
```

`overall_status` 枚举：
- `success`：部门和员工全部同步成功（HTTP 200）
- `partial_failed`：有部分成功但存在失败（HTTP 207）
- `failed`：部门和员工均未成功同步（HTTP 500）

`departments.status` 枚举为 `success | failed`；`employees.status` 枚举为 `success | partial_failed | failed | skipped`。部门源拉取、根部门/空数组/响应类型校验或事务落库任一步失败时，不得写入或覆盖旧部门数据，员工阶段必须标记 `skipped`（`EMPLOYEE_SYNC_SKIPPED`），不能调用员工钉钉接口或把空员工列表记为成功。阶段响应与同步状态必须包含安全 `error_code`、`request_id`、`duration_ms`、成功/失败数；禁止保存内部堆栈或原始第三方/数据库错误。

每个去重员工只产生一次最终计数：用户主数据、默认角色、员工档案任一必要步骤失败则计一次失败，全部成功才计一次成功；必须满足 `success_count + fail_count <= 去重员工总数`。角色分配失败后允许继续写档案用于数据修复，但最终仍只计一次失败。

历史员工停用只允许在部门源和员工源完整成功后执行。任一部门员工请求失败，或员工响应缺少/错误的 `result`、`list`、`has_more`、分页游标、用户 ID，或完整结果为空，都必须以 `DINGTALK_USER_SOURCE_INCOMPLETE`/响应格式错误 fail-closed，禁止用部分源数据停用历史员工。同一员工从多个部门响应合并时必须合并全部部门 ID，不能因去重丢失兼任关系。停用失败时员工阶段为 `partial_failed/EMPLOYEE_DEACTIVATION_FAILED`，但不得增加员工源 `fail_count`；`success_count + fail_count` 仍只描述去重后的钉钉源员工处理结果。

响应和用户可见同步状态中的 `error` 只能包含固定安全文案。服务端日志通过 `request_id` 定位，并记录脱敏 `org_id`、阶段、错误分类、脱敏摘要、耗时及成功/失败数；禁止输出 access token、AppSecret、密码、Authorization、DSN 或原始 SQL。

钉钉部门接口必须先成功获取合法根部门，再递归获取子部门；`result` 缺失、类型错误、部门数组为空、根部门详情失败或根部门不存在均返回稳定错误码。非 default 组织缺少 App 凭证或 AgentID 时 fail-closed，不得回退 default 组织的全局配置。

### POST /api/v1/sync/users、POST /api/v1/sync/departments

任务中心保留的用户/部门独立同步入口，组织边界、并发门闩、长任务超时和安全错误规则与 `/org/sync` 一致：只认 JWT `org_id`，query/body/header 不能切换组织，缺少组织上下文返回 401，同组织冲突返回 409。

成功返回 HTTP 200，部分成功返回 HTTP 207，全部失败返回 HTTP 500。响应 `data` 至少包含 `status`、`success_count`、`fail_count`、安全 `error`、`duration_ms` 和 `request_id`；用户同步还返回字段缺失统计、HRM 字段同步状态和默认角色分配数，部门同步返回 `change_log_count`。

前端收到 HTTP 200 或 207 的有效结果后执行完成回调并刷新页面/同步状态；HTTP 500、超时或网络失败时不盲目刷新。超时提示用户前往同步日志确认，未知任务类型明确报错，禁止回退为全量组织同步。

---

## 核心业务流程

### 同步组织架构流程

1. **同步部门**（`SyncDepartments`）
   - 调用钉钉 API 获取部门列表
   - 递归获取子部门
   - 写入 `departments` 表（upsert）

2. **同步用户**（`SyncUsers`）
   - 调用钉钉 API 获取用户列表
   - 按部门分页获取
   - 写入 `users` 表（upsert）
   - 解析 `DeptIDList` 中当前租户的全部有效部门，去重后保存员工—部门关系，第一项为主部门
   - 新增用户创建成功后，如果尚未分配角色，默认分配 `普通员工` 角色
   - 如果用户不存在 `EmployeeProfile`，自动创建
   - 分批读取钉钉智能人事字段并写入 `EmployeeProfile`
   - HRM 字段空值不覆盖本地人工值；HRM 请求失败或响应结构异常时基础通讯录与 HRM 子阶段分开报告，禁止静默成功
   - 钉钉手机号为空或为共享占位值时不覆盖本地真实手机号；新员工空手机号以数据库 `NULL` 保存，禁止使用共享假手机号规避空值
   - 只有部门与员工源完整成功后才收口历史 active 员工；停用失败只影响停用子阶段并返回 partial，不污染员工源成功/失败计数
   - 全量同步使用带 15 分钟上限的服务器执行上下文；客户端或外层代理断开不会取消已开始的服务器同步

### 员工详情聚合流程

1. **校验组织范围**
   - 根据当前登录人解析 `scope`
   - 非全组织账号只能访问授权部门及其下级部门员工

2. **聚合员工主数据**
   - 读取 `User`
   - 按 `user_id` 关联 `EmployeeProfile`
   - 计算部门路径与汇报关系

3. **生成详情扩展信息**
   - 时间轴聚合入职、计划转正、实际转正、合同到期、调岗、离职、档案审计日志
   - 预警信息按员工档案日期与组织规则生成

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `OrgService` | `org_service.go` | 组织架构管理 |
| `UserService` | `user_service.go` | 用户管理 |

---

## 前端页面

### 部门树页面
`frontend/src/pages/DepartmentTree.tsx`

功能：
- 展示部门树
- 点击部门查看员工列表
- 展示部门树节点在职人数，区分 `direct_active_count` 与 `active_count`
- 选中部门后展示轻量统计卡片，复用 `GET /api/v1/org/overview?department_id=...`
- 当前只做轻量统计卡片，不做趋势、排行榜、复杂图表

### 员工列表页面
`frontend/src/pages/EmployeeList.tsx`

功能：
- 组织概览统计卡片：在职人数、试用期人数、计划转正预警
- 组织概览分布卡片：员工类型、职级、岗位序列
- 当前组织概览仅做卡片展示，不做趋势图和复杂图表
- 员工列表（支持分页、搜索、筛选）
- 点击员工查看详情

### 员工详情页面
`frontend/src/pages/EmployeeDetail.tsx`

功能：
- 员工基本信息
- 员工档案聚合信息
- 组织路径、汇报关系、时间轴、预警
- 直接编辑档案字段；无档案时走 `employeeAPI.createProfile`，有档案时走 `employeeAPI.updateProfile`
- 重点展示并维护 `employment_type`、`education`、`job_level`、`job_family`、`planned_regular_date`、`actual_regular_date`

---

## 钉钉 API

### 获取部门列表
```
GET /topapi/v2/department/listsub
```

参数：
- `dept_id`：父部门 ID（根部门为 1）

### 获取部门详情
```
GET /topapi/v2/department/get
```

参数：
- `dept_id`：部门 ID

### 获取部门用户列表
```
GET /topapi/v2/user/list
```

参数：
- `dept_id`：部门 ID
- `cursor`：分页游标
- `size`：每页数量

### 获取用户详情
```
GET /topapi/v2/user/get
```

参数：
- `userid`：用户 ID

### 获取智能人事花名册字段
```
POST /topapi/smartwork/hrm/employee/v2/list
```

参数：
- `agentid`：应用 Agent ID
- `userid_list`：员工用户 ID 列表，当前按 50 人分批
- `field_filter_list`：标准转正字段与企业配置的 HRM 字段代码

---

## 环境变量

- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID
- `DINGTALK_ORGANIZATIONS`：可选，多企业配置 JSON 数组；用于初始化或补充 `organizations` 表
- `DINGTALK_HRM_EMPLOYMENT_TYPE_FIELD_CODES`：默认组织的员工类型字段代码，逗号分隔
- `DINGTALK_HRM_JOB_LEVEL_FIELD_CODES`：默认组织的职级字段代码，逗号分隔
- `DINGTALK_HRM_JOB_FAMILY_FIELD_CODES`：默认组织的岗位序列字段代码，逗号分隔
- `DINGTALK_HRM_PROBATION_END_DATE_FIELD_CODES`：默认组织的试用期结束日期字段代码，逗号分隔
- `DINGTALK_HRM_POSITION_FIELD_CODES`：默认组织的 HRM 岗位兜底字段代码，逗号分隔
- 对应的 `*_FIELD_NAMES` 可补充字段名称别名；字段代码仍是请求钉钉 HRM 接口的必要配置

---

## 常见问题

### 同步失败
- 检查钉钉应用权限（基础组织同步需要“通讯录读权限”，档案字段需要“智能人事花名册字段读取权限”）
- 检查 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_CORP_ID`
- 查看日志：`logrus` 会输出详细错误信息

### 部门树不完整
- 检查钉钉部门结构是否正确
- 检查 `parent_id` 是否正确
- 重新同步部门

### 员工列表为空
- 检查是否已同步用户
- 检查 `department_id` 是否正确
- 检查用户 `status` 是否为 `active`

### 员工详情显示不全
- 检查 `User` 模型字段是否完整
- 检查钉钉用户信息是否完整
- 检查 `EmployeeProfile` 是否存在
- 检查 `planned_regular_date`、`actual_regular_date`、`job_level`、`job_family` 等档案字段是否已录入
- 检查时间轴与审计日志资源 `employee_profile:{user_id}` 是否存在数据
