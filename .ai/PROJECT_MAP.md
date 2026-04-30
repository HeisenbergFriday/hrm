---
purpose: 项目目录结构、模块职责、代码入口索引
last_updated: 2026-04-30
source_of_truth:
  - 项目实际目录结构
  - internal/api/router.go（后端路由）
  - frontend/src/App.tsx（前端路由）
  - internal/database/models.go（数据模型）
update_when:
  - 新增目录时
  - 新增模块时
  - 新增入口文件时
  - 新增路由时
  - 新增数据模型时
---

# 项目结构索引

## 项目定位

PeopleOps 是一个以钉钉为主数据源的人事后台系统。系统负责同步组织、员工、考勤、审批等基础数据，并在本地扩展年假、调休、大小周排班、员工档案、人才分析等业务能力。

---

## 目录结构

```text
D:\ai项目
├─ cmd\main.go                      # 后端入口
├─ internal\
│  ├─ api\                          # 路由注册与 HTTP handlers
│  │  ├─ router.go                  # 所有路由注册（入口）
│  │  ├─ handlers.go                # 通用业务 handler
│  │  └─ leave_handlers.go          # 年假/调休/加班/排班 handler
│  ├─ cache\                        # Redis 初始化
│  ├─ config\                       # 配置与 holidays.json
│  ├─ database\                     # GORM 初始化、迁移、模型
│  │  ├─ database.go                # MySQL 初始化 + AutoMigrate
│  │  └─ models.go                  # 所有 GORM 模型定义（核心）
│  ├─ dingtalk\                     # 钉钉客户端与同步逻辑
│  │  └─ dingtalk.go                # 钉钉 API 封装（token 自动刷新、考勤组缓存）
│  ├─ middleware\                   # JWT 中间件
│  │  └─ jwt.go                     # JWT 验证中间件
│  ├─ repository\                   # 数据访问层
│  │  ├─ user_repository.go
│  │  ├─ audit_repository.go
│  │  ├─ annual_leave_grant_repository.go
│  │  ├─ compensatory_leave_ledger_repository.go
│  │  └─ overtime_match_result_repository.go
│  └─ service\                      # 业务逻辑层
│     ├─ user_service.go
│     ├─ attendance_service.go
│     ├─ attendance_rule_engine.go
│     ├─ annual_leave_grant_service.go
│     ├─ compensatory_leave_service.go
│     ├─ overtime_matching_service.go
│     ├─ leave_jobs.go              # 定时任务
│     ├─ org_service.go
│     ├─ dingtalk_vacation_service.go
│     └─ dingtalk_client.go
├─ frontend\
│  ├─ src\App.tsx                   # 主布局、菜单、路由、免登流程
│  ├─ src\pages\                    # 页面组件（31 个）
│  ├─ src\services\api.ts           # 前端 API 封装（Axios，baseURL=/api/v1）
│  ├─ src\store\authStore.ts        # 登录态持久化（Zustand，key: peopleops-auth）
│  └─ vite.config.ts                # 本地开发代理
├─ tests\                           # 测试辅助代码
├─ tools\                           # 运维/修复脚本
│  ├─ hooks\                        # Git hooks
│  ├─ resync_overtime_to_dingtalk\
│  ├─ reset_vacation_quota\
│  └─ set_comp_time_balance\
└─ api-docs\                        # 设计文档与方案
```

---

## 核心业务模块

| 模块 | 说明 | 相关文档 |
|---|---|---|
| 认证 | 账号密码登录、钉钉扫码、钉钉内免登、JWT | `.ai/MODULES/auth.md` |
| 组织与员工 | 部门树、员工列表、聚合员工详情、组织同步 | `.ai/MODULES/org.md` |
| 考勤 | 记录查询、异常统计、导出、最近同步时间 | `.ai/MODULES/attendance.md` |
| 审批 | 审批模板、审批实例、审批详情、审批同步 | `.ai/MODULES/approval.md` |
| 员工档案 | 档案、调岗、离职、入职、人才分析 | `.ai/MODULES/employee-profile.md` |
| 大小周排班 | 大小周规则、节假日、钉钉班次、手动覆盖、双向同步 | `.ai/MODULES/week-schedule.md` |
| 年假与调休 | 资格计算、季度发放、补发、消费台账、同步钉钉假期 | `.ai/MODULES/leave-overtime.md` |
| 加班匹配 | 审批与打卡匹配、调休台账、补发余额、重新同步到钉钉 | `.ai/MODULES/leave-overtime.md` |
| 下班时间配置 | 员工级班次配置与钉钉落地 | `.ai/MODULES/shift-config.md` |

---

## 后端 API 分组

所有接口前缀都是 `/api/v1`，当前主要分组如下：

| 路由组 | 说明 | Handler 文件 |
|---|---|---|
| `/auth` | 登录、登出、钉钉登录、获取当前用户 | `handlers.go` |
| `/users` | 用户 CRUD | `handlers.go` |
| `/departments` | 部门 CRUD | `handlers.go` |
| `/sync` | 钉钉同步（部门、用户、状态） | `handlers.go` |
| `/org` | 组织架构（部门树、员工列表、员工详情、同步） | `handlers.go` |
| `/attendance` | 考勤记录、统计、导出、最近同步时间 | `handlers.go` |
| `/approvals` | 审批模板、审批实例、审批详情、审批同步 | `handlers.go` |
| `/permission` | 角色、权限 | `handlers.go` |
| `/audit` | 审计日志 | `handlers.go` |
| `/jobs` | 任务中心 | `handlers.go` |
| `/employee` | 员工档案、调岗、离职、入职 | `handlers.go` |
| `/talent` | 人才分析 | `handlers.go` |
| `/week-schedule` | 大小周规则、节假日、班次、覆盖、同步 | `leave_handlers.go` |
| `/leave` | 年假资格、发放、消费 | `leave_handlers.go` |
| `/overtime` | 加班匹配、同步 | `leave_handlers.go` |
| `/comp-time` | 调休余额、手动发放 | `leave_handlers.go` |
| `/shift-config` | 员工下班时间配置 | `leave_handlers.go` |

路由注册集中在 `internal/api/router.go`。

---

## 前端页面路由

| 路由 | 页面文件 | 功能 |
|---|---|---|
| `/login` | Login.tsx | 账号密码/钉钉扫码登录 |
| `/callback` | Callback.tsx | 钉钉 OAuth 回调 |
| `/department-tree` | DepartmentTree.tsx | 部门树浏览 |
| `/employees` | EmployeeList.tsx | 员工列表 |
| `/employees/:id` | EmployeeDetail.tsx | 聚合员工详情（组织关系/档案快照/时间轴） |
| `/attendance` | Attendance.tsx | 考勤查询 |
| `/attendance-stats` | AttendanceStats.tsx | 考勤异常统计 |
| `/attendance-export` | AttendanceExport.tsx | 考勤导出 |
| `/approval-templates` | ApprovalTemplate.tsx | 审批模板 |
| `/approval-instances` | ApprovalInstance.tsx | 审批实例 |
| `/approval-detail/:id` | ApprovalDetail.tsx | 审批实例详情 |
| `/employee-profile` | EmployeeProfile.tsx | 员工档案 |
| `/employee-flow` | EmployeeFlow.tsx | 入转调离流程 |
| `/talent-analysis` | TalentAnalysis.tsx | 人才分析 |
| `/week-schedule` | WeekSchedule.tsx | 大小周+法定节假日管理 |
| `/employee-shift-config` | EmployeeShiftConfig.tsx | 员工自定义下班时间 |
| `/leave-overtime` | LeaveOvertime.tsx | 年假与调休管理（核心页面） |
| `/sync-jobs` | SyncJobs.tsx | 同步任务中心 |
| `/audit-logs` | AuditLogs.tsx | 操作日志 |
| `/role-management` | RoleManagement.tsx | 角色管理 |

---

## 关键文件入口

### 后端

| 文件 | 说明 |
|---|---|
| `cmd/main.go` | 启动入口 |
| `internal/api/router.go` | 所有路由注册 |
| `internal/api/handlers.go` | 通用业务 handler |
| `internal/api/leave_handlers.go` | 年假/调休/加班/排班 handler |
| `internal/database/models.go` | 所有 GORM 模型定义 |
| `internal/dingtalk/dingtalk.go` | 钉钉 API 封装 |
| `internal/middleware/jwt.go` | JWT 验证中间件 |

### 前端

| 文件 | 说明 |
|---|---|
| `frontend/src/App.tsx` | 主布局、菜单、路由、免登流程 |
| `frontend/src/services/api.ts` | 前端 API 封装 |
| `frontend/src/store/authStore.ts` | 登录态持久化 |

---

## 数据库模型分组

所有模型定义在 `internal/database/models.go`，按业务分组如下：

### 基础模型
- `User`：钉钉用户（user_id 为钉钉 ID，唯一键）
- `Department`：部门（department_id 为钉钉部门 ID）
- `Attendance`：打卡记录（唯一键：user_id+check_time+check_type）
- `Approval`：审批实例（process_id 为钉钉审批 ID）
- `ApprovalTemplate`：审批模板
- `Role` / `Permission` / `RolePermission` / `UserRole`：RBAC 权限体系
- `OperationLog`：操作审计日志
- `SyncStatus`：钉钉同步状态记录
- `DingTalkBinding`：本地用户↔钉钉账号绑定
- `UserSession` / `LoginLog`：会话与登录日志
- `AttendanceExport`：考勤导出任务记录

### 员工档案模型
- `EmployeeProfile`：员工档案（工号、合同、教育背景、银行卡等本地字段）
- `EmployeeTransfer`：转岗记录（含审批流状态）
- `EmployeeResignation`：离职记录（含离职手续 JSON）
- `EmployeeOnboarding`：入职记录（含入职流程 JSON）
- `TalentAnalysis`：人才分析（绩效/潜力/离职风险评分）

### 排班与假期模型
- `EmployeeShiftConfig`：员工自定义下班时间（同步到钉钉生效）
- `DingTalkShiftCatalog`：钉钉班次名→ID 映射缓存
- `WeekScheduleRule`：大小周规则（scope: company/department/user）
- `WeekScheduleOverride`：大小周手动覆盖（针对特定周）
- `WeekScheduleSyncLog`：大小周同步钉钉日志
- `StatutoryHoliday`：法定节假日/调休上班日（type: holiday/workday）

### 年假与调休模型
- `LeaveRuleConfig`：年假规则配置（rule_type: eligibility/grant）
- `AnnualLeaveEligibility`：年假资格（按 user_id+year+quarter 唯一）
- `AnnualLeaveGrant`：年假发放台账（含钉钉同步状态）
- `AnnualLeaveConsumeLog`：年假消费台账（FIFO 扣减，幂等 via request_ref）
- `OvertimeRuleConfig`：加班规则配置
- `OvertimeMatchResult`：加班审批↔打卡匹配结果（唯一键：user_id+work_date）
- `OvertimeSyncHistory`：已同步钉钉的加班记录快照
- `CompensatoryLeaveLedger`：调休余额台账（credit/debit/rollback/adjustment）

---

## 运维与修复脚本

- `tools/resync_overtime_to_dingtalk`：重新同步加班到钉钉
- `tools/reset_vacation_quota`：重置假期配额
- `tools/set_comp_time_balance`：设置调休余额
- `tools/resync_comp_time.go`：重新同步调休
- `tools/hooks/pre-commit`：Git pre-commit hook（检测结构性变更时提醒更新 CLAUDE.md）
- `tools/install-hooks.sh`：一键安装 hooks
