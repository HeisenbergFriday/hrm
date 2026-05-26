# PeopleOps 数据库设计

本文按当前 GORM 模型维护。模型真实来源是 `internal/database/models.go` 和 `internal/database/performance_models.go`，迁移入口是 `internal/database/database.go`。

## 数据库选型

- 主业务库：MySQL
- ORM：GORM
- 主键：当前模型统一使用 `uint` 自增主键
- JSON 字段：使用 MySQL JSON 类型，GORM tag 为 `type:json;serializer:json`
- 软删除：主要业务模型使用 `gorm.DeletedAt`
- 迁移方式：服务启动时执行 `AutoMigrate`，部分历史兼容字段和索引通过手写 DDL 补齐

## 初始化与迁移

启动时 `database.Init()` 会执行：

1. 读取 `DATABASE_URL`。
2. 使用 MySQL driver 连接数据库。
3. 如果连接失败，尝试按 DSN 中的库名自动创建数据库后重连。
4. 执行手写兼容迁移，例如年假发放同步字段、用户直属主管字段、部分唯一索引修复。
5. 执行 GORM `AutoMigrate`。
6. 初始化默认管理员、默认部门、默认角色和默认权限。

默认管理员：

| 字段 | 值 |
|---|---|
| 用户名 | `admin` |
| 密码 | `admin123` |

## 基础主数据模型

| 模型 | 表 | 说明 | 关键唯一性 |
|---|---|---|---|
| `User` | `users` | 钉钉用户及本地登录用户 | `user_id` |
| `Department` | `departments` | 钉钉部门 | `department_id` |
| `DepartmentChangeLog` | `department_change_logs` | 部门变更历史 | 无 |
| `Attendance` | `attendances` | 考勤打卡记录 | `user_id + check_time + check_type` |
| `Approval` | `approvals` | 审批实例 | `process_id` |
| `ApprovalTemplate` | `approval_templates` | 审批模板 | `template_id` |
| `SyncStatus` | `sync_statuses` | 同步状态 | `type` |

### User

核心字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | `uint` | 本地自增主键 |
| `user_id` | `varchar(64)` | 钉钉用户 ID 或本地账号 ID |
| `name` | `varchar(128)` | 姓名 |
| `email` | `varchar(128)` | 邮箱 |
| `mobile` | `varchar(32)` | 手机号 |
| `password` | `varchar(256)` | 密码哈希，不输出到 JSON |
| `department_id` | `varchar(64)` | 钉钉部门 ID |
| `manager_user_id` | `varchar(64)` | 直属主管用户 ID |
| `manager_name` | `varchar(128)` | 直属主管姓名快照 |
| `extension` | `json` | 本地扩展字段 |

### Department

核心字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | `uint` | 本地自增主键 |
| `department_id` | `varchar(64)` | 钉钉部门 ID |
| `name` | `varchar(128)` | 部门名称 |
| `parent_id` | `varchar(64)` | 父部门钉钉 ID |
| `order` | `int` | 排序 |
| `extension` | `json` | 本地扩展字段 |

## 认证、权限与审计模型

| 模型 | 表 | 说明 |
|---|---|---|
| `Role` | `roles` | 角色 |
| `Permission` | `permissions` | 权限 |
| `RolePermission` | `role_permissions` | 角色权限关联 |
| `UserRole` | `user_roles` | 用户角色关联 |
| `OperationLog` | `operation_logs` | 操作审计日志 |
| `DingTalkBinding` | `ding_talk_bindings` | 本地用户与钉钉账号绑定 |
| `UserSession` | `user_sessions` | 会话记录 |
| `LoginLog` | `login_logs` | 登录日志 |

当前权限 API 只开放角色列表、创建角色、权限列表等基础能力。菜单权限和数据权限页面仍使用同一批角色/权限数据作为入口。

## 员工档案与生命周期模型

| 模型 | 表 | 说明 |
|---|---|---|
| `EmployeeProfile` | `employee_profiles` | 员工档案 |
| `EmployeeTransfer` | `employee_transfers` | 调岗记录 |
| `EmployeeResignation` | `employee_resignations` | 离职记录 |
| `EmployeeOnboarding` | `employee_onboardings` | 入职记录 |
| `TalentAnalysis` | `talent_analyses` | 人才分析 |

`EmployeeProfile` 是本地扩展数据的核心表，包含工号、用工类型、学历、职级、岗位序列、入职日期、计划转正日期、实际转正日期、合同、银行卡等字段。

当前 `employee_onboardings.employee_id` 是员工工号，不是 `users.user_id`。入转调离台账中候选入职人员通过工号和档案表匹配。

## 考勤导出与排班模型

| 模型 | 表 | 说明 | 关键唯一性 |
|---|---|---|---|
| `AttendanceExport` | `attendance_exports` | 考勤导出任务 | 无 |
| `EmployeeShiftConfig` | `employee_shift_configs` | 员工自定义下班时间 | `user_id` |
| `DingTalkShiftCatalog` | `ding_talk_shift_catalogs` | 钉钉班次缓存 | `shift_key` |
| `WeekScheduleRule` | `week_schedule_rules` | 大小周规则 | `scope_type + scope_id` |
| `WeekScheduleOverride` | `week_schedule_overrides` | 手动覆盖 | `scope_type + scope_id + week_start_date` |
| `WeekScheduleSyncLog` | `week_schedule_sync_logs` | 排班同步日志 | 无 |
| `StatutoryHoliday` | `statutory_holidays` | 法定节假日/调休上班日 | `date` |

`DingTalkShiftCatalog.shift_key` 是稳定签名，由班次名、上班时间、下班时间归一化得到，用于避免同名不同时间班次互相覆盖。

## 年假、加班与调休模型

| 模型 | 表 | 说明 | 关键唯一性 |
|---|---|---|---|
| `LeaveRuleConfig` | `leave_rule_configs` | 年假规则配置 | `rule_type + rule_key` |
| `AnnualLeaveEligibility` | `annual_leave_eligibilities` | 年假资格 | `user_id + year + quarter` |
| `AnnualLeaveGrant` | `annual_leave_grants` | 年假发放台账 | `user_id + year + quarter + grant_type` |
| `AnnualLeaveConsumeLog` | `annual_leave_consume_logs` | 年假消费台账 | `request_ref + grant_id` |
| `OvertimeRuleConfig` | `overtime_rule_configs` | 加班规则配置 | `rule_key` |
| `OvertimeMatchResult` | `overtime_match_results` | 加班审批与打卡匹配结果 | `match_ref`，历史上也按 `user_id + work_date` 约束 |
| `OvertimeSyncHistory` | `overtime_sync_histories` | 加班同步快照 | `sync_request_id` |
| `CompensatoryLeaveLedger` | `compensatory_leave_ledgers` | 调休余额台账 | 按业务引用防重复 |
| `OvertimeSupplementaryRequest` | `overtime_supplementary_requests` | 补卡申请记录 | 无 |

`AnnualLeaveGrant` 当前钉钉同步字段为：

| 字段 | 说明 |
|---|---|
| `dingtalk_sync_status` | `pending / success / failed / skipped` |
| `dingtalk_sync_error` | 同步错误信息 |
| `dingtalk_synced_at` | 同步时间 |

## 绩效模型

绩效相关模型在 `internal/database/performance_models.go`。

| 模型 | 说明 |
|---|---|
| `PerformanceActivity` | 绩效活动 |
| `PerformanceParticipant` | 活动参与人 |
| `PerformanceGoalRecord` | 目标/指标记录 |
| `PerformanceGoalApprovalLog` | 目标审批日志 |
| `PerformanceIndicatorLibrary` | 部门指标库 |
| `PerformanceIndicatorItem` | 指标项 |
| `PerformanceTemplate` | 绩效模板 |
| `PerformanceTemplateSection` | 模板评分维度 |
| `PerformanceTemplateItem` | 模板评分项 |
| `PerformanceReview` | 评审记录 |
| `PerformanceReviewVersion` | 评审版本 |
| `PerformanceDistributionRule` | 强制分布规则 |
| `PerformanceDistributionException` | 强制分布例外 |
| `PerformanceLevelRule` | 等级规则 |
| `PerformanceLevelRuleItem` | 等级规则明细 |
| `PerformanceRelationshipChangeLog` | 绩效关系变更日志 |
| `PerformanceCompanyFinance` | 公司收支状态 |

活动状态流：

```text
draft -> target_setting -> self_evaluation -> manager_evaluation -> employee_confirmation -> manager_confirmation -> hr_confirmation -> locked -> archived
```

## 关系与数据边界

- `users.user_id` 和 `departments.department_id` 保存钉钉原始 ID。
- 本地表之间多使用钉钉 ID 或本地自增 ID 做业务关联，不强制依赖数据库外键。
- `database.go` 设置了 `DisableForeignKeyConstraintWhenMigrating: true`，迁移时不自动创建外键约束。
- 钉钉主数据以同步为主；员工档案、年假、调休、排班、绩效等是本地业务扩展数据。

## 备份建议

生产环境至少备份：

- MySQL 主库。
- 上传附件目录 `uploads/`。
- 运行所需 `.env` 或密钥管理配置。

Redis 当前主要作为缓存使用，通常不作为核心持久数据源。

## 性能与维护建议

- 常用列表接口保留分页。
- 修改模型时同步检查 `database.go` 中的手写迁移和唯一索引兼容逻辑。
- 修改钉钉同步、年假发放、加班匹配、绩效锁定等流程时，需要同时核对幂等键。
- 不要再引入 PostgreSQL 专用类型或 UUID 迁移说明，除非代码实际完成数据库迁移。
