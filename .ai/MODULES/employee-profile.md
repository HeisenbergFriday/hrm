---
purpose: 员工档案模块业务规则说明
last_updated: 2026-05-03
source_of_truth:
  - internal/api/handlers.go（员工档案相关 handler）
  - internal/database/models.go（EmployeeProfile、EmployeeTransfer、EmployeeResignation、EmployeeOnboarding、TalentAnalysis 模型）
  - internal/repository/employee_repository.go（入转调离台账查询）
  - internal/service/employee_service.go（入转调离台账服务）
  - frontend/src/pages/EmployeeProfile.tsx（员工档案）
  - frontend/src/pages/EmployeeFlow.tsx（员工流程）
update_when:
  - 修改员工档案字段时
  - 修改调岗流程时
  - 修改离职流程时
  - 修改入职流程时
  - 修改入转调离台账列表口径时
  - 修改人才分析逻辑时
---

# 员工档案模块

## 模块定位

管理员工档案、入职、转岗、离职流程、人才分析。

本次阶段 1A 文档更新只聚焦员工档案字段与组织详情联动补齐，不涉及人才分析、绩效、强制分布、权限、C/D 面谈等能力变更。

---

## 数据模型

### EmployeeProfile
员工档案

说明：
- 以下代码块描述的是当前 `EmployeeProfile` 的长期字段截面，不表示这些字段都在阶段 1A 本次新增。
- 本次阶段 1A 真正新增的模型字段只有 `job_level`、`job_family`。
- 本次阶段 1A 重点补齐的档案能力只围绕 `employment_type`、`planned_regular_date`、`actual_regular_date`、`education`、`job_level`、`job_family`。
- `EmployeeID`、`WorkEmail`、`PersonalEmail`、`ProfileStatus`、银行相关字段属于历史已有字段范畴，不作为本阶段重点能力描述。

```go
type EmployeeProfile struct {
    ID                uint
    UserID            string  // 关联 User.UserID
    EmployeeID        string  // 员工工号
    BirthDate         string  // 出生日期
    Gender            string  // 性别
    MaritalStatus     string  // 婚姻状况
    EmploymentType    string  // 雇佣类型
    EntryDate         string  // 入职日期
    ProbationEndDate  string  // 试用期结束日期
    PlannedRegularDate string // 计划转正日期
    ActualRegularDate string  // 实际转正日期
    JobLevel          string  // 职级
    JobFamily         string  // 岗位序列/人员类别
    ContractStartDate string  // 合同开始日期
    ContractEndDate   string  // 合同结束日期
    WorkEmail         string  // 工作邮箱
    PersonalEmail     string  // 个人邮箱
    EmergencyContact  string  // 紧急联系人
    EmergencyPhone    string  // 紧急联系电话
    Education         string  // 学历
    Major             string  // 专业
    School            string  // 毕业院校
    GraduationDate    string  // 毕业日期
    IDCard            string  // 身份证号
    BankName          string  // 开户行
    BankCard          string  // 银行卡号
    Address           string  // 家庭住址
    ProfileStatus     string  // active / inactive
    Extension         map[string]interface{}
    CreatedAt         time.Time
    UpdatedAt         time.Time
    DeletedAt         gorm.DeletedAt
}
```

## 档案字段约定

- 本次阶段 1A 重点补齐的是 `employment_type`、`planned_regular_date`、`actual_regular_date`、`education`、`job_level`、`job_family`
- 其中模型层新增字段只有 `job_level`、`job_family`
- `employment_type`：前后端统一枚举，当前选项为 `正式 / 试用 / 实习 / 劳务 / 兼职`
- `education`：前后端统一枚举，当前选项为 `高中 / 大专 / 本科 / 硕士 / 博士 / 其他`
- `job_family`：前后端统一枚举，当前选项为 `管理 / 专业 / 技术`
- `job_level`：自由输入文本，用于记录职级
- `planned_regular_date` / `actual_regular_date`：分别表示计划转正与实际转正日期，既用于档案录入，也会出现在组织模块员工详情时间轴与聚合卡片中

### EmployeeOnboarding
入职记录

```go
type EmployeeOnboarding struct {
    ID             uint
    UserID         string
    OnboardingDate string  // 入职日期
    Department     string
    Position       string
    Salary         float64
    Status         string  // pending / approved / rejected
    OnboardingFlow map[string]interface{}  // 入职流程 JSON
    Remark         string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

**注意**：以上模型定义已过时，实际代码中 `EmployeeOnboarding` 字段已更新，详见 `internal/database/models.go`。

## 当前口径与限制（阶段 3B 更新）

### employee_onboardings 与 users 的关联

**当前状态**：
- `employee_onboardings.employee_id` 是员工工号，不是 `users.user_id`
- `employee_onboardings` 表中没有 `user_id` 字段
- 无法通过外键直接关联到 `users` 表

**判断是否已建档的口径**：
- 通过 `employee_profiles.employee_id` 匹配工号判断
- 如果 `employee_profiles` 中存在相同 `employee_id`，视为已建档
- 如果不存在，视为候选入职人员（未建档）

**阶段 3B 实现**（已完成）：
- 台账查询合并候选入职人员（未建档的 onboarding 记录）
- 状态映射：
  - `pending` 且未建档 → 候选入职
  - `processing` 且未建档 → 入职处理中
  - `completed` 且未建档 → 入职已完成/待建档
  - 已建档 → 已入职
- 候选入职人员字段来源：
  - `user_id`: 空字符串（候选人员尚未创建 users 记录）
  - `employee_id`: `onboarding.employee_id`（工号）
  - `onboarding_id`: `onboarding.onboarding_id`（入职记录唯一标识）
  - `is_candidate`: `true`
- 不修改 users 创建时机
- 分页逻辑：候选入职人员排在已入职员工前面，参与 total 计算，支持 keyword / department_id / status 筛选
- 前端 rowKey：`is_candidate ? 'candidate:' + onboarding_id : 'user:' + id`
- 前端状态筛选：增加"候选入职"选项，筛选 `status=candidate` 只显示候选人员

**后续建议**：
- 阶段 4+ 增加 `employee_onboardings.user_id` 字段
- 明确 users 创建时机（入职申请创建时 / 入职流程完成时 / 手动建档时）
- 建立 onboarding → user → employee_profile 的明确关联链路
- 避免依赖 `employee_id` 工号匹配，改用明确的外键关联

---

### EmployeeTransfer
转岗记录

```go
type EmployeeTransfer struct {
    ID                uint
    UserID            string
    FromDepartment    string
    ToDepartment      string
    FromPosition      string
    ToPosition        string
    TransferDate      string
    TransferType      string  // promotion / lateral / demotion
    Status            string  // pending / approved / rejected
    ApprovalProcessID string  // 审批流程 ID
    Remark            string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### EmployeeResignation
离职记录

```go
type EmployeeResignation struct {
    ID                uint
    UserID            string
    ResignationType   string  // voluntary / involuntary
    ResignationDate   string  // 离职日期
    LastWorkingDay    string  // 最后工作日
    Reason            string  // 离职原因
    Status            string  // pending / approved / rejected / completed
    ResignationFlow   map[string]interface{}  // 离职手续 JSON
    Remark            string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### TalentAnalysis
人才分析

```go
type TalentAnalysis struct {
    ID                  uint
    UserID              string
    PerformanceScore    float64  // 绩效评分
    PotentialScore      float64  // 潜力评分
    TurnoverRisk        string   // 离职风险（low/medium/high）
    KeyTalent           bool     // 是否关键人才
    SuccessionPlan      string   // 继任计划
    DevelopmentPlan     string   // 发展计划
    LastReviewDate      string   // 最近评估日期
    NextReviewDate      string   // 下次评估日期
    Extension           map[string]interface{}
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

---

## API 接口

### 员工档案

#### GET /api/v1/employee/profiles
查询员工档案列表

Query 参数：
- `department_id`：部门 ID（可选）
- `keyword`：搜索关键词（可选）
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 20）

#### GET /api/v1/employee/profiles/:id
查询员工档案详情

#### POST /api/v1/employee/profiles
创建员工档案

Body：
```json
{
    "user_id": "xxx",
    "employee_id": "E001",
    "employment_type": "正式",
    "education": "本科",
    "job_level": "P5",
    "job_family": "技术",
    "entry_date": "2024-01-15",
    "planned_regular_date": "2024-04-15",
    "actual_regular_date": "2024-04-20"
}
```

#### PUT /api/v1/employee/profiles/:id
更新员工档案

审计日志约定：
- 创建档案时记录 `employment_type`、`planned_regular_date`、`actual_regular_date`、`education`、`job_level`、`job_family`
- 更新档案时除上述字段外，额外记录 `changed_fields`
- 审计资源固定为 `employee_profile:{user_id}`，供组织模块员工详情时间轴复用

---

### 入职

#### GET /api/v1/employee/onboardings
查询入职记录列表

#### POST /api/v1/employee/onboardings
创建入职记录

Body：
```json
{
    "user_id": "xxx",
    "onboarding_date": "2024-01-15",
    "department": "技术部",
    "position": "工程师",
    "salary": 15000,
    "onboarding_flow": {
        "it_setup": false,
        "badge_issued": false,
        "training_completed": false
    }
}
```

#### PUT /api/v1/employee/onboardings/:id
更新入职记录

---

### 转岗

#### GET /api/v1/employee/transfers
查询转岗记录列表

#### POST /api/v1/employee/transfers
创建转岗记录

Body：
```json
{
    "user_id": "xxx",
    "from_department": "技术部",
    "to_department": "产品部",
    "from_position": "工程师",
    "to_position": "产品经理",
    "transfer_date": "2024-02-01",
    "transfer_type": "lateral",
    "remark": "转岗原因"
}
```

#### PUT /api/v1/employee/transfers/:id
更新转岗记录

---

### 离职

#### GET /api/v1/employee/resignations
查询离职记录列表

#### POST /api/v1/employee/resignations
创建离职记录

Body：
```json
{
    "user_id": "xxx",
    "resignation_type": "voluntary",
    "resignation_date": "2024-03-01",
    "last_working_day": "2024-03-15",
    "reason": "个人原因",
    "resignation_flow": {
        "handover_completed": false,
        "equipment_returned": false,
        "exit_interview": false
    }
}
```

#### PUT /api/v1/employee/resignations/:id
更新离职记录

---

### 人才分析

#### GET /api/v1/talent/analysis
查询人才分析列表

Query 参数：
- `department_id`：部门 ID（可选）
- `key_talent`：是否关键人才（可选，true/false）
- `turnover_risk`：离职风险（可选，low/medium/high）

#### GET /api/v1/talent/analysis/:id
查询人才分析详情

#### POST /api/v1/talent/analysis
创建人才分析

Body：
```json
{
    "user_id": "xxx",
    "performance_score": 4.5,
    "potential_score": 4.0,
    "turnover_risk": "low",
    "key_talent": true,
    "succession_plan": "培养为技术经理",
    "development_plan": "参加领导力培训"
}
```

#### PUT /api/v1/talent/analysis/:id
更新人才分析

---

### 阶段 3A：入转调离台账

阶段 3A 目标：
- 基于现有员工档案、组织关系、员工流转记录和操作日志能力，补齐“入职 / 转正 / 调岗 / 离职”台账的最小可用展示能力。
- 当前阶段只做台账梳理和最小增强，不重做完整入职、转正、调岗、离职审批流程。
- 当前阶段不修改假期、假勤、打卡、排班相关功能。

#### GET /api/v1/employee/ledger
查询入转调离台账列表。

Query 参数：
- `page`：页码，默认 1
- `page_size`：每页数量，默认 20
- `department_id`：按当前部门筛选
- `status`：按 `users.status` 筛选，当前支持 `active / inactive`
- `keyword`：按员工姓名、员工 ID、工号、邮箱、手机号、岗位模糊搜索

当前实现入口：
- 后端：`internal/repository/employee_repository.go` -> `FindLifecycleLedger`
- 服务：`internal/service/employee_service.go` -> `GetLifecycleLedger`
- Handler：`internal/api/handlers.go` -> `GetEmployeeLifecycleLedger`
- 路由：`internal/api/router.go` -> `GET /api/v1/employee/ledger`
- 前端：`frontend/src/pages/EmployeeFlow.tsx` 台账页签

#### 台账基表与默认过滤

- 台账基表是 `users`
- 默认排除 `admin`
- 通过 `users.department_id` 关联当前部门
- 通过 `users.user_id = employee_profiles.user_id` 左关联员工档案
- 当前口径优先体现“当前员工 + 当前组织归属”，不是全量历史快照表

#### 当前字段口径

入职信息：
- `entry_date` 优先取 `employee_profiles.entry_date`
- 档案缺失或未填时，回退到最近一条 `employee_onboardings.entry_date`
- 当前台账只展示入职日期与最近入职状态，不展开完整入职流程节点

用工类型：
- `employment_type` 优先取 `employee_profiles.employment_type`
- 档案缺失或未填时，回退到最近一条 `employee_onboardings.employment_type`

转正信息：
- `planned_regular_date` 来自 `employee_profiles.planned_regular_date`
- `actual_regular_date` 来自 `employee_profiles.actual_regular_date`
- 当前阶段没有独立转正表，仍以员工档案字段为准

调岗信息：
- 取 `employee_transfers` 中按 `transfer_date DESC, id DESC` 排序的最近一条记录
- 当前展示最近一次调岗日期、状态、原部门/原岗位、新部门/新岗位
- 当前只做“最近一次调岗”展示，不做全历史调岗台账

离职 / 停用状态：
- 最近离职记录取 `employee_resignations` 中按 `resign_date DESC, id DESC` 排序的最近一条
- 如果存在最近离职记录，优先展示该离职记录的日期、状态、最后工作日、离职原因
- 如果没有离职记录，但 `users.status = inactive` 或 `employee_profiles.profile_status = inactive`，则按“离职/停用”展示
- 当前阶段不区分停用原因，不额外补离职审批流状态推导

部门与岗位：
- 部门来自 `users.department_id` 关联的当前 `departments.name`
- 岗位来自 `users.position`
- 当前阶段不回溯历史部门树，不做某次调岗时点的组织快照恢复

操作日志 / 时间线口径：
- 员工详情时间线继续复用 `OrgService.buildEmployeeTimeline`
- 当前复用的审计资源包括：
  - `employee_profile:{user_id}`：档案创建 / 更新
  - `employee_transfer:{user_id}`：调岗记录创建
  - `employee_resignation:{user_id}`：离职记录创建
  - `employee_onboarding:{employee_id}`：入职记录创建
- 当前 `GET /api/v1/employee/ledger` 本身不直接返回操作日志字段；操作日志作为员工详情时间线补充能力保留

#### 阶段 3A 已支持字段

- 员工姓名
- 员工 ID / 工号
- 当前部门
- 当前岗位
- 用工类型
- 入职日期
- 计划转正日
- 实际转正日
- 最近一次调岗日期
- 最近一次调岗状态
- 最近一次调岗原部门 / 原岗位
- 最近一次调岗新部门 / 新岗位
- 最近一次离职日期
- 最近一次离职状态
- 最后工作日
- 离职原因
- 当前在职 / 离职 / 停用状态
- 最近入职记录状态

#### 阶段 3A 暂不支持

- 完整入职流程台账明细
- 完整转正审批流与转正历史表
- 多次调岗的完整历史列表与趋势分析
- 完整离职审批流与离职手续看板
- 历史组织快照恢复
- 跨模块的绩效、假勤、打卡、排班字段拼接
- 权限改造、数据范围改造
- 强制分布、C/D 面谈、复杂分析看板、复杂图表

#### 后续 3B-3F 边界

- 3B：只考虑在台账基础上补更完整的历史记录、筛选口径或展示细节，不改假勤、打卡、排班
- 3C：如需做转正能力，只进入独立转正流程 / 审批 / 留痕，不回头扩大 3A
- 3D：如需做调岗审批流，只处理调岗流程闭环，不混入绩效、权限、假勤
- 3E：如需做离职闭环，只处理离职审批、手续和状态一致性，不扩展到考勤或假期结算
- 3F：如需做分析看板，只在已有台账稳定后再考虑，不在 3A 引入复杂图表和跨模块统计

---

## 核心业务流程

### 入职流程

1. **创建入职记录**
   - 填写入职信息
   - 设置入职流程（IT 配置、工牌发放、培训等）

2. **完成入职流程**
   - 逐项完成入职流程
   - 更新 `onboarding_flow` JSON

3. **审批通过**
   - 更新 `status = approved`
   - 自动创建 `EmployeeProfile`（如果不存在）

### 转岗流程

1. **创建转岗记录**
   - 填写转岗信息
   - 发起审批流程

2. **审批通过**
   - 更新 `status = approved`
   - 更新 `User.DepartmentID` 和 `User.Position`

### 离职流程

1. **创建离职记录**
   - 填写离职信息
   - 设置离职手续（交接、设备归还、离职面谈等）

2. **完成离职手续**
   - 逐项完成离职手续
   - 更新 `resignation_flow` JSON

3. **审批通过**
   - 更新 `status = completed`
   - 更新 `User.Status = inactive`

---

## 前端页面

### 员工档案页面
`frontend/src/pages/EmployeeProfile.tsx`

功能：
- 员工档案列表
- 员工档案详情
- 创建/编辑员工档案
- 本次阶段 1A 统一 `employment_type`、`education`、`job_family` 选项
- 本次阶段 1A 补齐 `planned_regular_date`、`actual_regular_date`、`job_level`、`job_family` 的录入、编辑和展示

### 入转调离流程页面
`frontend/src/pages/EmployeeFlow.tsx`

功能：
- 入职记录列表
- 转岗记录列表
- 离职记录列表
- 创建/编辑流程

### 人才分析页面
`frontend/src/pages/TalentAnalysis.tsx`

功能：
- 人才分析列表
- 人才分析详情
- 创建/编辑人才分析
- 九宫格展示（绩效 vs 潜力）

---

## 常见问题

### 员工档案为空
- 检查是否已同步用户
- 同步用户时会自动创建 `EmployeeProfile`
- 如果没有，手动创建

### 新增字段没有出现在页面
- 检查 `EmployeeProfile.tsx` 与 `EmployeeDetail.tsx` 是否都已使用统一选项集
- 检查接口返回是否包含 `job_level`、`job_family`、`planned_regular_date`、`actual_regular_date`
- 检查更新后是否生成对应 `OperationLog`

### 转岗后部门没更新
- 检查转岗记录 `status` 是否为 `approved`
- 检查是否有更新 `User.DepartmentID`

### 离职后用户仍然可以登录
- 检查离职记录 `status` 是否为 `completed`
- 检查是否有更新 `User.Status = inactive`
- 检查登录逻辑是否过滤 `inactive` 用户
