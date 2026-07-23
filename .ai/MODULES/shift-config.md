---
purpose: 员工下班时间配置模块说明
last_updated: 2026-07-20
source_of_truth:
  - internal/api/router.go
  - internal/api/handlers.go
  - internal/service/shift_config_service.go
  - internal/repository/shift_config_repository.go
  - internal/database/models.go
  - frontend/src/pages/EmployeeShiftConfig.tsx
update_when:
  - 修改 shift-config 路由时
  - 修改员工班次配置模型时
  - 修改钉钉班次落地逻辑时
  - 修改多企业班次缓存 / op_user_id 隔离规则时
---

# 员工下班时间配置

## 模块定位

为员工配置自定义下班时间或钉钉班次，并支持预览、保存、应用到钉钉。该模块与大小周排班模块共享钉钉班次缓存能力。

## 后端接口

所有接口需要 JWT，前缀为 `/api/v1`。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/shift-config/list` | 查询员工配置 |
| `GET` | `/shift-config/catalogs` | 查询本地钉钉班次缓存 |
| `POST` | `/shift-config/preview` | 预览配置结果 |
| `POST` | `/shift-config/set` | 保存员工配置 |
| `POST` | `/shift-config/apply` | 应用配置到钉钉 |
| `DELETE` | `/shift-config/:user_id` | 删除员工配置 |
| `POST` | `/shift-config/get-or-create-shift` | 获取或创建自定义班次 |

## 数据模型

核心模型定义在 `internal/database/models.go`：

- `EmployeeShiftConfig`：员工级班次配置，唯一键 `(org_id, user_id)`。
- `DingTalkShiftCatalog`：本地钉钉班次缓存，唯一键 `(org_id, shift_key)`。

`DingTalkShiftCatalog.shift_key` 由班次名、上班时间、下班时间归一化得到，避免同名但时间不同的班次互相覆盖。

## 多企业隔离

- 优先 `NewShiftConfigServiceWithOrgID(db, orgID)`；`NewShiftConfigService` 仅从 RequestDB 解析 org，缺 org 时后续读写 fail-closed。
- 服务内直接 GORM（用户/部门/节假日/`DingTalkShiftCatalog`）必须经 `scopedDB()`/`requireBoundOrgID()` 带 `org_id`；缺 org 返回错误或 `1=0`，**禁止**无过滤全表。
- 进程内 `shiftIDCache` key = `orgID|shiftKey`；测试用 `ClearShiftIDCacheForTest` 清理，避免交叉污染。
- `GetOrCreateShift` / `ApplyAndSync` 的 `op_user_id` 经 `dingtalk.ResolveAdminUserID(orgID)` 解析；非 default 企业不得读 `DINGTALK_ADMIN_USER_ID`。
- 钉钉列表/创建接口一律 `*ForOrg(orgID, ...)`，相同班次名+时间在不同企业各自持有不同 `shift_id`。
- 缺少企业凭证或 admin userid 时：不创建班次、不写 catalog、不写员工配置（`ApplyAndSync` 在本地写入前先校验 admin）。

## 前端页面

| 路由 | 页面 | 说明 |
|---|---|---|
| `/employee-shift-config` | `EmployeeShiftConfig.tsx` | 员工自定义下班时间配置 |

## 注意事项

- `REDIS_URL`、钉钉应用权限、考勤组配置都会影响钉钉落地能力。
- 该模块和 `/week-schedule/shifts` 都会接触钉钉班次，改动时要同时检查 `week_schedule_service.go`。
- 多企业联调时分别为企业 A/B 配置独立 AppKey/Secret 与 `ding_talk_admin_user_id`。
