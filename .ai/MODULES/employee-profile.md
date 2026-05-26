---
purpose: 员工档案与入转调离模块说明
last_updated: 2026-05-26
source_of_truth:
  - internal/api/router.go
  - internal/api/handlers.go
  - internal/database/models.go
  - internal/repository/employee_repository.go
  - internal/service/employee_service.go
  - internal/service/talent_service.go
  - frontend/src/pages/EmployeeProfile.tsx
  - frontend/src/pages/EmployeeFlow.tsx
  - frontend/src/pages/TalentAnalysis.tsx
update_when:
  - 修改员工档案字段时
  - 修改员工入职、转岗、离职模型或接口时
  - 修改入转调离台账口径时
  - 修改人才分析模型或接口时
---

# 员工档案与入转调离

## 模块定位

该模块覆盖员工档案、入职记录、转岗记录、离职记录、入转调离台账和人才分析。当前代码以本地 MySQL 为扩展数据源，组织主数据仍来自钉钉同步后的 `users` 与 `departments`。

当前实现边界：

- 员工档案支持列表、详情、创建、更新。
- 入职、转岗、离职记录当前只注册列表与创建接口，暂无独立 `PUT` / `DELETE` 路由。
- 人才分析当前只注册列表、详情与创建接口，暂无独立更新接口。
- 入转调离台账是查询聚合视图，不是单独的历史快照表。

---

## 数据模型

模型真实来源是 `internal/database/models.go`。

### EmployeeProfile

员工档案，本地扩展员工信息。核心字段：

- `user_id`：关联 `users.user_id`，唯一。
- `employee_id`：员工工号，唯一。
- 基本信息：`gender`、`birth_date`、`nationality`、`id_card_number`。
- 工作信息：`employment_type`、`entry_date`、`probation_end_date`、`planned_regular_date`、`actual_regular_date`、`job_level`、`job_family`、`contract_start_date`、`contract_end_date`。
- 联系信息：`work_email`、`personal_email`、`emergency_contact`、`emergency_phone`。
- 教育信息：`education`、`graduate_school`、`major`、`graduation_date`。
- JSON 扩展：`work_experience`、`skills`、`extension`。
- 其他信息：`bank_account`、`bank_name`、`tax_number`、`address`、`profile_status`。

字段约定：

- `employment_type` 当前前端选项为 `正式 / 试用 / 实习 / 劳务 / 兼职`。
- `education` 当前前端选项为 `高中 / 大专 / 本科 / 硕士 / 博士 / 其他`。
- `job_family` 当前前端选项为 `管理 / 专业 / 技术`。
- `job_level` 是自由文本。
- `planned_regular_date` 与 `actual_regular_date` 会被组织详情和台账复用。

### EmployeeOnboarding

入职记录。当前模型不包含 `user_id`，候选入职人员通过工号与档案匹配。

核心字段：

- `onboarding_id`：入职记录唯一标识。
- `employee_id`：员工工号，唯一。
- `name`、`gender`、`birth_date`、`id_card_number`、`mobile`、`email`。
- `department_id`、`department_name`、`position`、`entry_date`、`employment_type`、`probation_end_date`。
- `emergency_contact`、`emergency_phone`、`education`、`graduate_school`、`major`。
- `onboarding_process`：入职流程 JSON。
- `status`：`pending / processing / completed`。

### EmployeeTransfer

转岗记录。核心字段：

- `transfer_id`：转岗记录唯一标识。
- `user_id`、`user_name`。
- `old_department_id`、`old_department_name`、`old_position`。
- `new_department_id`、`new_department_name`、`new_position`。
- `transfer_date`、`reason`、`status`。
- `approver_id`、`approver_name`、`approval_time`、`approval_comment`。

### EmployeeResignation

离职记录。核心字段：

- `resignation_id`：离职记录唯一标识。
- `user_id`、`user_name`、`department_id`、`department_name`、`position`。
- `resign_date`、`last_working_day`、`resign_reason`、`status`。
- `approver_id`、`approver_name`、`approval_time`、`approval_comment`。
- `exit_process`：离职手续 JSON。

### TalentAnalysis

人才分析记录。核心字段：

- `user_id`、`user_name`、`department_id`、`department_name`、`position`。
- 绩效：`performance_score`、`performance_level`、`performance_review`。
- 潜力：`potential_score`、`potential_level`。
- 离职风险：`turnover_risk_score`、`turnover_risk_level`。
- JSON 扩展：`skills_assessment`、`training_records`、`promotion_records`、`extension`。
- `analysis_date`。

---

## 后端接口

所有接口需要 JWT，前缀为 `/api/v1`。

### 员工档案

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/employee/profiles` | 员工档案列表 |
| `GET` | `/employee/profiles/:id` | 员工档案详情 |
| `POST` | `/employee/profiles` | 创建员工档案 |
| `PUT` | `/employee/profiles/:id` | 更新员工档案 |

`GET /employee/profiles` 当前 query：

- `page`：默认 `1`
- `page_size`：默认 `10`
- `department_id`
- `status`

### 入转调离记录

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/employee/onboardings` | 入职记录列表 |
| `POST` | `/employee/onboardings` | 创建入职记录 |
| `GET` | `/employee/transfers` | 转岗记录列表 |
| `POST` | `/employee/transfers` | 创建转岗记录 |
| `GET` | `/employee/resignations` | 离职记录列表 |
| `POST` | `/employee/resignations` | 创建离职记录 |

这些列表接口当前支持 `page`、`page_size`、`status`。

### 入转调离台账

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/employee/ledger` | 入转调离台账聚合列表 |

Query 参数：

- `page`：默认 `1`
- `page_size`：默认 `20`
- `department_id`
- `status`
- `keyword`

当前实现入口：

- Repository：`internal/repository/employee_repository.go` -> `FindLifecycleLedger`
- Service：`internal/service/employee_service.go` -> `GetLifecycleLedger`
- Handler：`internal/api/handlers.go` -> `GetEmployeeLifecycleLedger`
- Route：`internal/api/router.go` -> `GET /api/v1/employee/ledger`
- Frontend：`frontend/src/pages/EmployeeFlow.tsx`

### 人才分析

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/talent/analysis` | 人才分析列表 |
| `GET` | `/talent/analysis/:id` | 人才分析详情 |
| `POST` | `/talent/analysis` | 创建人才分析 |

`GET /talent/analysis` 当前 query：

- `page`：默认 `1`
- `page_size`：默认 `10`
- `department_id`

---

## 台账口径

### employee_onboardings 与 users 的关联

- `employee_onboardings.employee_id` 是员工工号，不是 `users.user_id`。
- `employee_onboardings` 当前没有 `user_id` 字段。
- 是否已建档通过 `employee_profiles.employee_id` 匹配工号判断。
- 未匹配到档案的入职记录会作为候选入职人员进入台账。

候选入职人员字段来源：

- `user_id`：空字符串。
- `employee_id`：`employee_onboardings.employee_id`。
- `onboarding_id`：`employee_onboardings.onboarding_id`。
- `is_candidate`：`true`。

### 台账基表与排序

- 台账基表是 `users`，默认排除 `admin`。
- 通过 `users.department_id` 关联当前部门。
- 通过 `users.user_id = employee_profiles.user_id` 左关联员工档案。
- 候选入职人员排在已入职员工前面，并参与 `total` 计算。
- 当前台账优先体现当前员工与当前组织归属，不做历史组织快照恢复。

### 字段取值

- 入职日期优先取 `employee_profiles.entry_date`，缺失时回退最近一条 `employee_onboardings.entry_date`。
- 用工类型优先取 `employee_profiles.employment_type`，缺失时回退最近一条 `employee_onboardings.employment_type`。
- 转正信息取 `employee_profiles.planned_regular_date` 与 `employee_profiles.actual_regular_date`。
- 调岗信息取最近一条 `employee_transfers`。
- 离职信息取最近一条 `employee_resignations`；如果没有离职记录但用户或档案是 inactive，则按离职/停用展示。

---

## 前端页面

| 路由 | 页面 | 说明 |
|---|---|---|
| `/employee-profile` | `EmployeeProfile.tsx` | 员工档案列表、创建与编辑 |
| `/employee-flow` | `EmployeeFlow.tsx` | 入转调离台账、入职/转岗/离职记录列表与创建 |
| `/talent-analysis` | `TalentAnalysis.tsx` | 人才分析列表与创建 |

注意：

- `EmployeeFlow.tsx` 当前只使用创建接口，不调用入职/转岗/离职更新接口。
- `TalentAnalysis.tsx` 当前只使用列表、详情和创建接口，不调用更新接口。
- 员工详情时间线仍复用组织模块聚合逻辑，操作日志作为详情补充能力保留。

---

## 后续建议

- 如需支持入职、转岗、离职或人才分析的编辑能力，应先在 `internal/api/router.go` 注册对应 `PUT` 路由，再补 handler、service、repository 与前端封装。
- 如需让候选入职人员与 `users` 建立稳定关联，应新增 `employee_onboardings.user_id` 或明确 users 创建时机。
- 如需完整历史台账，应新增历史快照或事件表，不要把当前聚合查询误用为审计级历史记录。
