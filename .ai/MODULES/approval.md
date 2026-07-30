---
purpose: 审批模块业务规则说明
last_updated: 2026-07-23
source_of_truth:
  - internal/api/router.go
  - internal/api/handlers.go
  - internal/api/external_approval_handlers.go
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
| `POST` | `/approvals/sync` | 同步审批，必须传 `process_code` |
| `GET` | `/approvals/oa-data` | 沐腾组织外部 OA 审批明细（关键词 + 服务端分页） |

`/approvals/sync` Body：

```json
{
  "process_code": "PROC-OVERTIME",
  "start_date": "2026-05-01",
  "end_date": "2026-05-26"
}
```

`process_code` 是钉钉审批流程代码；缺少时后端返回 `400`，不会再返回“成功但 count=0”。

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
- `ApprovalTemplate`：审批模板，`template_id` 保存钉钉模板标识。同步审批实例时仍要在请求体里传 `process_code`。

`Approval.Content` 和 `Approval.Extension` 使用 MySQL JSON 字段保存审批表单内容与本地扩展信息。

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
