# PeopleOps 后端 API 设计

本文按当前代码实现维护。完整路由注册入口是 `internal/api/router.go`，前端调用封装入口是 `frontend/src/services/api.ts`。`api-docs/swagger.json` 目前只覆盖早期基础接口，不是完整 API 清单。

## 技术栈

- Go 1.20
- Gin 1.9
- GORM
- MySQL
- Redis 可选缓存
- JWT Bearer Token
- 钉钉开放平台 API

## 通用约定

### API 前缀

除健康检查和前端静态资源外，业务接口统一使用：

```text
/api/v1
```

健康检查：

```text
GET /health
```

文件访问：

```text
GET /api/v1/files/:filename
```

### 统一响应格式

后端通用响应结构：

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

### 分页约定

列表接口通常使用：

| 参数 | 默认值 | 说明 |
|---|---:|---|
| `page` | `1` | 页码 |
| `page_size` | `10` 或 `20` | 每页数量，具体以 handler 为准 |

分页响应通常包含：

```json
{
  "items": [],
  "total": 0
}
```

部分旧接口使用 `PagedResponse`，字段仍是 `items` 与 `total`。

### 认证约定

- 登录接口返回 JWT。
- 受保护接口需要请求头：`Authorization: Bearer <token>`。
- JWT 中间件会把 `userID` 和 `userName` 写入 Gin context。
- 当前除了登录、钉钉登录配置/回调、健康检查、文件访问外，大多数业务接口都需要 JWT。

## 路由分组

### 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/auth/login` | 账号密码登录 |
| `POST` | `/api/v1/auth/logout` | 登出 |
| `GET` | `/api/v1/auth/me` | 获取当前用户 |
| `GET` | `/api/v1/auth/dingtalk/qr/start` | 获取钉钉扫码登录地址 |
| `POST` | `/api/v1/auth/dingtalk/in-app` | 钉钉内免登，body 使用 `code` |
| `GET` | `/api/v1/auth/dingtalk/callback` | 钉钉 OAuth 回调 |
| `GET` | `/api/v1/auth/dingtalk/config` | 获取钉钉前端配置 |

登录请求示例：

```json
{
  "username": "admin",
  "password": "admin123"
}
```

钉钉内免登请求示例：

```json
{
  "code": "code_from_dingtalk_js_sdk"
}
```

### 用户、部门与基础同步

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/users` | 用户列表 |
| `GET` | `/api/v1/users/:id` | 用户详情 |
| `PUT` | `/api/v1/users/:id` | 更新用户本地扩展字段 |
| `GET` | `/api/v1/departments` | 当前可见部门列表 |
| `GET` | `/api/v1/departments/:id` | 部门详情 |
| `POST` | `/api/v1/sync/departments` | 同步钉钉部门 |
| `POST` | `/api/v1/sync/users` | 同步钉钉用户 |
| `GET` | `/api/v1/sync/status` | 基础同步状态 |

### 组织与员工

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/org/overview` | 组织概览 |
| `GET` | `/api/v1/org/departments/tree` | 部门树 |
| `GET` | `/api/v1/org/departments/:id/history` | 部门变更历史 |
| `GET` | `/api/v1/org/employees` | 组织员工列表 |
| `GET` | `/api/v1/org/employees/:id` | 聚合员工详情 |
| `POST` | `/api/v1/org/sync` | 组织数据同步 |
| `GET` | `/api/v1/employee/profiles` | 员工档案列表 |
| `GET` | `/api/v1/employee/profiles/:id` | 员工档案详情 |
| `POST` | `/api/v1/employee/profiles` | 创建员工档案 |
| `PUT` | `/api/v1/employee/profiles/:id` | 更新员工档案 |
| `GET` | `/api/v1/employee/ledger` | 入转调离台账 |
| `GET` | `/api/v1/employee/transfers` | 调岗记录 |
| `POST` | `/api/v1/employee/transfers` | 创建调岗记录 |
| `GET` | `/api/v1/employee/resignations` | 离职记录 |
| `POST` | `/api/v1/employee/resignations` | 创建离职记录 |
| `GET` | `/api/v1/employee/onboardings` | 入职记录 |
| `POST` | `/api/v1/employee/onboardings` | 创建入职记录 |
| `GET` | `/api/v1/talent/analysis` | 人才分析列表 |
| `GET` | `/api/v1/talent/analysis/:id` | 人才分析详情 |
| `POST` | `/api/v1/talent/analysis` | 创建人才分析 |

### 考勤与审批

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/attendance/records` | 考勤记录 |
| `GET` | `/api/v1/attendance/stats` | 考勤统计 |
| `POST` | `/api/v1/attendance/sync` | 同步考勤 |
| `POST` | `/api/v1/attendance/export` | 创建考勤导出任务 |
| `GET` | `/api/v1/attendance/exports` | 查询导出任务 |
| `GET` | `/api/v1/attendance/last-sync` | 最近同步时间 |
| `GET` | `/api/v1/approvals/templates` | 审批模板 |
| `GET` | `/api/v1/approvals/instances` | 审批实例 |
| `GET` | `/api/v1/approvals/:id` | 审批详情 |
| `POST` | `/api/v1/approvals/sync` | 同步审批，body 必须包含 `process_code` |

`/api/v1/approvals/sync` 请求示例：

```json
{
  "process_code": "PROC-OVERTIME",
  "start_date": "2026-05-01",
  "end_date": "2026-05-26"
}
```

缺少 `process_code` 时返回 `400`，不会执行同步。

### 权限、审计与任务

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/permission/roles` | 角色列表 |
| `POST` | `/api/v1/permission/roles` | 创建角色 |
| `GET` | `/api/v1/permission/permissions` | 权限列表 |
| `GET` | `/api/v1/audit/logs` | 审计日志 |
| `GET` | `/api/v1/jobs` | 任务列表 |
| `POST` | `/api/v1/jobs/:id/run` | 运行任务 |

### 排班、年假与调休

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/week-schedule/rules` | 大小周规则 |
| `POST` | `/api/v1/week-schedule/rules` | 创建大小周规则 |
| `POST` | `/api/v1/week-schedule/rules/batch` | 批量设置规则 |
| `PUT` | `/api/v1/week-schedule/rules/:id` | 更新规则 |
| `DELETE` | `/api/v1/week-schedule/rules/:id` | 删除规则 |
| `GET` | `/api/v1/week-schedule/shifts` | 钉钉班次 |
| `POST` | `/api/v1/week-schedule/shifts` | 创建钉钉班次 |
| `GET` | `/api/v1/week-schedule/debug/attendance-groups` | 调试钉钉考勤组 |
| `GET` | `/api/v1/week-schedule/calendar` | 周历 |
| `POST` | `/api/v1/week-schedule/overrides` | 手动覆盖某周大小周 |
| `DELETE` | `/api/v1/week-schedule/overrides/:id` | 删除手动覆盖 |
| `POST` | `/api/v1/week-schedule/sync/to-dingtalk` | 同步排班到钉钉 |
| `POST` | `/api/v1/week-schedule/sync/from-dingtalk` | 从钉钉同步排班 |
| `GET` | `/api/v1/week-schedule/sync/logs` | 排班同步日志 |
| `GET` | `/api/v1/week-schedule/holidays` | 法定节假日 |
| `POST` | `/api/v1/week-schedule/holidays` | 创建节假日 |
| `POST` | `/api/v1/week-schedule/holidays/batch` | 批量创建节假日 |
| `POST` | `/api/v1/week-schedule/holidays/sync/from-juhe` | 从聚合数据同步节假日 |
| `DELETE` | `/api/v1/week-schedule/holidays/:id` | 删除节假日 |
| `GET` | `/api/v1/shift-config/list` | 员工下班时间配置 |
| `POST` | `/api/v1/shift-config/preview` | 配置预览 |
| `POST` | `/api/v1/shift-config/set` | 保存配置 |
| `POST` | `/api/v1/shift-config/apply` | 应用配置到钉钉 |
| `GET` | `/api/v1/leave/eligibility` | 年假资格 |
| `POST` | `/api/v1/leave/eligibility/recalculate` | 重新计算年假资格 |
| `GET` | `/api/v1/leave/grants` | 年假发放记录 |
| `POST` | `/api/v1/leave/grants/run-quarter` | 运行季度发放 |
| `POST` | `/api/v1/leave/grants/regrant` | 补发年假 |
| `POST` | `/api/v1/leave/grants/sync-to-dingtalk` | 同步年假到钉钉 |
| `GET` | `/api/v1/leave/vacation-types` | 钉钉假期类型 |
| `POST` | `/api/v1/leave/consume` | 消费年假 |
| `GET` | `/api/v1/leave/consume-log` | 年假消费台账 |
| `GET` | `/api/v1/overtime/matches` | 加班匹配记录 |
| `POST` | `/api/v1/overtime/matches/run` | 运行加班匹配 |
| `POST` | `/api/v1/overtime/matches/force` | 强制匹配 |
| `POST` | `/api/v1/overtime/matches/clear-rematch` | 清空并重新匹配 |
| `POST` | `/api/v1/overtime/matches/delete` | 删除匹配记录 |
| `POST` | `/api/v1/overtime/sync-and-match` | 同步审批并匹配 |
| `POST` | `/api/v1/overtime/reset-manual-leave` | 重置钉钉 ManualLeave 余额 |
| `POST` | `/api/v1/overtime/resync-overtime` | 重新同步加班到钉钉 |
| `POST` | `/api/v1/overtime/supplementary/submit` | 提交加班补卡 |
| `POST` | `/api/v1/overtime/supplementary/approve` | 审批加班补卡 |
| `GET` | `/api/v1/overtime/supplementary/list` | 加班补卡列表 |
| `POST` | `/api/v1/overtime/supplementary/sync-dingtalk` | 从钉钉同步补卡审批，当前返回 501 |
| `GET` | `/api/v1/comp-time/balance` | 调休余额 |
| `POST` | `/api/v1/comp-time/manual-grant` | 手动发放调休 |
| `POST` | `/api/v1/upload` | 文件上传 |

### 绩效

绩效接口统一在 `/api/v1/performance` 下，当前覆盖绩效活动、参与人、目标设定、自评、上级评分、强制分布、三级确认、指标库、模板、收支规则、提醒与归档。

常用入口：

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/v1/performance/activities` | 活动列表 |
| `POST` | `/api/v1/performance/activities` | 创建活动 |
| `GET` | `/api/v1/performance/activities/:activity_id` | 活动详情 |
| `PUT` | `/api/v1/performance/activities/:activity_id` | 更新活动 |
| `POST` | `/api/v1/performance/activities/:activity_id/open-target-setting` | 开启目标设定 |
| `POST` | `/api/v1/performance/activities/:activity_id/open-self-evaluation` | 开启自评 |
| `POST` | `/api/v1/performance/activities/:activity_id/open-manager-evaluation` | 开启上级评分 |
| `POST` | `/api/v1/performance/activities/:activity_id/lock` | 锁定活动 |
| `POST` | `/api/v1/performance/activities/:activity_id/batch-confirm` | 批量确认结果 |
| `GET` | `/api/v1/performance/activities/:activity_id/participants` | 参与人列表 |
| `GET` | `/api/v1/performance/participants/:participant_id` | 参与人详情 |
| `POST` | `/api/v1/performance/goal-records/:participant_id` | 保存目标记录 |
| `POST` | `/api/v1/performance/goal-records/:participant_id/submit` | 提交目标审批 |
| `POST` | `/api/v1/performance/reviews/:participant_id/self-evaluation` | Review 版自评 |
| `POST` | `/api/v1/performance/reviews/:participant_id/manager-evaluation` | Review 版上级评分 |
| `POST` | `/api/v1/performance/goal-reviews/:participant_id/bonus-penalty` | 目标绩效加减分 |
| `GET` | `/api/v1/performance/indicator-libraries` | 指标库 |
| `GET` | `/api/v1/performance/indicator-items` | 指标项 |
| `GET` | `/api/v1/performance/templates` | 绩效模板 |

完整绩效路由请查看 `internal/api/router.go` 中 `performance := authRequired.Group("/performance")` 代码块。

## 安全与现状说明

- 已实现 JWT 鉴权和基础 CORS。
- 数据库访问通过 GORM，避免手写拼接 SQL 的主要风险。
- 没有全局 CSRF Token 机制。
- 没有自动生成的完整 OpenAPI 文档。
- 生产 HTTPS、网关限流、统一审计策略等需要由部署层和后续代码配合实现。
