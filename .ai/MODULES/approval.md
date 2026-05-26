---
purpose: 审批模块业务规则说明
last_updated: 2026-05-26
source_of_truth:
  - internal/api/router.go
  - internal/api/handlers.go
  - internal/service/approval_service.go
  - internal/database/models.go
  - frontend/src/pages/ApprovalTemplate.tsx
  - frontend/src/pages/ApprovalInstance.tsx
  - frontend/src/pages/ApprovalDetail.tsx
  - frontend/src/pages/ApprovalStats.tsx
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
- `applicant_id`
- `start_date`
- `end_date`

## 数据模型

核心模型定义在 `internal/database/models.go`：

- `Approval`：审批实例，`process_id` 保存钉钉审批实例 ID。
- `ApprovalTemplate`：审批模板，`template_id` 保存钉钉模板标识。同步审批实例时仍要在请求体里传 `process_code`。

`Approval.Content` 和 `Approval.Extension` 使用 MySQL JSON 字段保存审批表单内容与本地扩展信息。

## 前端页面

| 路由 | 页面 | 说明 |
|---|---|---|
| `/approval-templates` | `ApprovalTemplate.tsx` | 审批模板 |
| `/approval-instances` | `ApprovalInstance.tsx` | 审批实例 |
| `/approval-detail/:id` | `ApprovalDetail.tsx` | 审批详情 |
| `/approval-stats` | `ApprovalStats.tsx` | 审批统计 |

`/approval` 仍保留页面文件和路由，但当前主菜单入口使用模板、实例和统计页。

## 注意事项

- 旧接口 `/api/v1/approvals` 不再是列表入口，当前列表入口是 `/api/v1/approvals/instances`。
- 审批同步依赖钉钉应用权限和 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`。
- 加班匹配会读取审批数据，改审批字段时要同步检查 `overtime_matching_service.go`。
