---
purpose: 审批模块业务规则说明
last_updated: 2026-08-07
source_of_truth:
  - internal/api/router.go
  - internal/api/handlers.go
  - internal/api/approval_sync_handlers.go
  - internal/api/external_approval_handlers.go
  - internal/service/approval_sync_service.go
  - internal/service/approval_reconciliation_service.go
  - internal/service/approval_service.go
  - internal/service/external_approval_service.go
  - internal/repository/external_approval_repository.go
  - internal/database/models.go
  - frontend/src/pages/ApprovalTemplate.tsx
  - frontend/src/pages/ApprovalInstance.tsx
  - frontend/src/pages/ApprovalDetail.tsx
  - frontend/src/pages/ApprovalStats.tsx
  - frontend/src/pages/OAApprovalData.tsx
update_when:
  - 修改审批路由时
  - 修改审批同步逻辑时
  - 修改审批页面时
---

# 审批模块

## 模块定位

从钉钉同步审批模板与审批实例，提供审批实例查询、详情展示和同步入口。加班、年假等业务会复用审批实例作为匹配或消费依据。

## 后端接口

所有接口需要 JWT，前缀为 `/api/v1`。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/approvals/templates` | 审批模板列表 |
| `GET` | `/approvals/instances` | 审批实例列表 |
| `GET` | `/approvals/:id` | 审批详情 |
| `POST` | `/approvals/sync/start` | 异步启动审批同步，返回 `request_id` |
| `GET` | `/approvals/sync/:request_id` | 查询当前企业审批同步任务状态与完整终态 |
| `POST` | `/approvals/sync` | 兼容同步入口，委托同一审批同步服务 |
| `GET` | `/approvals/oa-data` | 沐腾组织外部 OA 审批明细（关键词 + 服务端分页） |

审批同步 Body：

```json
{
  "process_code": "PROC-OVERTIME",
  "start_date": "2026-05-01",
  "end_date": "2026-05-26"
}
```

- `process_code` 非空时只同步该流程，但该代码必须属于当前 JWT 企业配置的 `ProcessCodes` 白名单；否则返回 `APPROVAL_PROCESS_NOT_ACCESSIBLE`，且不得创建同步任务。
- `process_code` 为空或省略时，同步当前 JWT 企业配置的全部流程。
- 全量流程代码只取 `dingtalk.ConfigForOrgID(orgID).ProcessCodes`，去空、去重并稳定排序；非 `default` 企业禁止回退默认企业配置。
- 没有可用流程代码时优先返回 `APPROVAL_PROCESS_CODES_MISSING`，即使请求显式传入了 `process_code` 也不得创建假成功任务。
- 钉钉动态发现的可管理流程只能用于需要流程发现的独立功能，不能参与审批实例同步的全量范围或显式流程授权判断。
- 日期均为空时使用最近一个月；日期必须为 `YYYY-MM-DD`，结束日不得早于开始日或落在未来。

### 异步同步契约

前端页面使用 `POST /approvals/sync/start` 短请求启动，再按返回的 `request_id` 轮询 `GET /approvals/sync/:request_id`：

- `ApprovalSyncService.Prepare` 是纯本地规划：只做日期校验、读取当前企业本地配置并按 `ProcessCodes` 生成计划；禁止调用钉钉流程发现或其他外部网络接口。
- 启动成功返回 HTTP `202`、`status=running` 和 `request_id`；审批任务与 `SyncStatus=running` 必须先持久化，HTTP `202` 必须先写出，随后才能调度后台 goroutine 和钉钉实例拉取。
- 查询必须同时绑定 JWT `org_id`、任务类型 `approvals` 和 `request_id`；其他企业或其他任务类型统一不可见。
- 后台执行脱离客户端断开信号，最长 15 分钟；终态使用独立短上下文持久化，避免执行超时后永久停留在 `running`。
- 同一企业使用 `ApprovalSyncTask.active_key` 数据库唯一键作为审批同步门闩，重复启动返回 `409`；共享同一数据库的应用实例之间仍互斥，不同企业可以并行。
- 每个流程独立拉取和逐条 upsert；单个流程失败后继续其他流程。结果按流程返回拉取/写入计数，以及 `reconciled_count`、`reconcile_skipped_count`、`reconcile_fail_count` 三类下游对账计数。
- 全部流程及对账成功为 `success`；有成功写入但存在流程、明细、写入或下游对账失败为 `partial`；没有任何成功写入结果为 `failed`。响应和同步状态禁止保存第三方原始错误、Token、Secret 或 URL 参数。
- `processinstance/listids` 单次窗口最多 120 天，结束时间不得提交到未来；长日期范围连续分片并按审批实例 ID 全局去重。

审批实例页和统计页未选择模板时显示“同步全部”，选择后显示“同步当前模板”。模板页和详情页始终传显式 `process_code`，继续保持单模板同步。

`/approvals/instances` 常用 query：

- `page`
- `page_size`
- `status`
- `template_id`
- `category`（流程分类，白名单：`leave`/`overtime`/`expense`/`business_trip`/`outing`/`punch_fix`/`other`；仅在未传 `template_id` 时生效，直接按 `approvals.title LIKE '%关键字%'` 命中或排除过滤，因为库里 `extension->>'$.template_id'` 长期为空、模板同步也未必齐全，标题反而稳定含"请假/加班/补卡"等语义词；`other` 走排除法（不含任一已知分类关键字）；映射常量见 `internal/service/approval_category.go`）
- `applicant_id`
- `title`（标题模糊搜索，后端按 `title LIKE %关键词%` 过滤，前端审批实例页 300ms 防抖触发）
- `start_date`
- `end_date`

## 数据模型

核心模型定义在 `internal/database/models.go`：

- `Approval`：审批实例，`process_id` 保存钉钉审批实例 ID。
- `ApprovalTemplate`：审批模板，`template_id` 保存钉钉模板标识。模板页/详情页同步时传显式 `process_code`；实例页/统计页允许省略以触发全量同步。

模板目录查询以当前 JWT 组织的 `dingtalk.ConfigForOrgID(orgID).ProcessCodes` 为配置兜底：当 `approval_templates` 没有对应记录时，仍返回配置流程模板（已有数据库模板的表单/节点信息优先）。审批实例同步写入的 `extension.process_code` 是实例与模板目录的稳定关联键，模板筛选必须同时兼容 `process_code` 与历史 `template_id`。

`Approval.Content` 和 `Approval.Extension` 使用 MySQL JSON 字段保存审批表单内容与本地扩展信息。

审批同步按 `org_id + process_id` upsert，重复同步不得新增重复记录；`extension.result`、`extension.process_code`、`extension.source=dingtalk_sync` 必须保留并与已有扩展字段合并。Stream 已写入的同一实例再次全量同步时更新原记录。

### 下游业务对账

- 每条审批 upsert 成功后，由可注入的 `ApprovalBusinessReconciliationService` 对该条已落库数据执行年假消费或加班匹配，不直接调用定时任务入口。
- 只有状态为 `COMPLETED` 且 `extension.result=agree` 的审批处于业务有效态；有效态变为拒绝、终止、取消等无效态时必须冲正既有年假/调休结果，重复无效状态跳过。无效态恢复为有效态时进入下一操作轮次，只重新入账一次。
- 年假使用 `approval:<process_id>` 作为稳定审批引用。`AnnualLeaveConsumeRequest` 以 `org_id + request_ref` 作为请求级门闩；门闩、grant 行锁、余额和正向/冲正台账在同一事务。冲正只追加 reversal 台账并返还原 grant，禁止删除消费历史。
- 年假开始/结束日期和天数必须从审批表单解析并使用项目业务时区；历史审批按实际请假日期选择同年度、且不晚于业务季度的额度，禁止使用审批完成日期、同步日期或当前余额猜测。日期缺失/异常 fail-closed；跨年按各年度包含的自然日占比分摊天数并记录年度明细。
- 加班直接按已落库审批 ID 匹配；工作日期取表单中的实际加班开始时间，不使用审批完成日期或“昨天”。`no_clock_record`、`insufficient_clock_record`、`query_clock_failed` 等暂缺数据状态允许原记录自动重算；已成功入账/同步的终态不得重复发放。
- 钉钉考勤与 Doris 考勤写入成功后，必须按同步服务绑定的 `org_id` 和本批实际影响的 `user_id + work_date` 去重触发 retryable 加班重算；失败保留考勤并交给有界定时补偿，禁止扫描全部审批。
- 加班撤销先完成本地来源净额冲正，再以 `rollback_pending → rollback_failed/rollback_success` 记录钉钉年度绝对余额校准。审批恢复遇到 pending、failed 或其他不确定状态时必须重新绝对校准，不能沿用撤销前的 `success`；审计模式区分 `rollback`、`retry`、`reactivation`。
- 对账结果区分 `applied`、`skipped`、`reversed`、`retryable`、`failed`；retryable/failed 令同步任务进入 `partial`。
- 对账查询和写入必须显式绑定同步服务的 `org_id`。单条对账失败不回滚已写入的审批，继续处理同批其他审批并将流程/任务标为 `partial`；重试时再次对账。

### 外部 OA 审批数据

- 数据源：Doris 只读表 `dwd.dwd_dingtalk_attendance_approval_detail_di`，复用 `EXTERNAL_ATTENDANCE_*` 数据库连接配置。
- 仅允许 JWT `org_id=muteng` 访问；后端必须 fail-closed，不能只依赖前端隐藏菜单。
- 查询固定使用沐腾 `corp_name=深圳市沐腾科技有限公司`，不接受客户端传入组织或公司主体。
- 源表字段会独立演进，仓储读取实际列并将完整记录放入 `fields`；常用字段用于列表，全部字段在详情抽屉展示。
- 数据源只允许 `SELECT`，禁止通过该连接执行 DML/DDL。

## 前端页面

| 路由 | 页面 | 说明 |
|---|---|---|
| `/approval-templates` | `ApprovalTemplate.tsx` | 审批模板 |
| `/approval-instances` | `ApprovalInstance.tsx` | 审批实例 |
| `/approval-detail/:id` | `ApprovalDetail.tsx` | 审批详情 |
| `/approval-stats` | `ApprovalStats.tsx` | 审批统计 |
| `/oa-approval-data` | `OAApprovalData.tsx` | 沐腾 OA 审批明细与动态字段详情 |

`/approval` 仍保留页面文件和路由，但当前主菜单入口使用模板、实例和统计页。

## 注意事项

- 旧接口 `/api/v1/approvals` 不再是列表入口，当前列表入口是 `/api/v1/approvals/instances`。
- 审批同步依赖钉钉应用权限和 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`。
- 加班匹配会读取审批数据，改审批字段时要同步检查 `overtime_matching_service.go`。
- 已通过的年假/加班审批才可入账；审批后续拒绝、终止或取消时必须幂等冲正，审批恢复同意时必须安全重新入账一次。补卡、转岗等其他审批只落审批表。
- `ListManageableApprovalProcessesForOrg` 等动态发现能力不代表同步授权；审批实例同步必须始终以当前企业 `ProcessCodes` 为唯一白名单。
