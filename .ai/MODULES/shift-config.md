---
purpose: 员工下班时间配置模块说明
last_updated: 2026-05-26
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

- `EmployeeShiftConfig`：员工级班次配置，`user_id` 唯一。
- `DingTalkShiftCatalog`：钉钉班次缓存，使用 `shift_key` 做稳定唯一键。

`DingTalkShiftCatalog.shift_key` 由班次名、上班时间、下班时间归一化得到，避免同名但时间不同的班次互相覆盖。

## 前端页面

| 路由 | 页面 | 说明 |
|---|---|---|
| `/employee-shift-config` | `EmployeeShiftConfig.tsx` | 员工自定义下班时间配置 |

## 注意事项

- `REDIS_URL`、钉钉应用权限、考勤组配置都会影响钉钉落地能力。
- 该模块和 `/week-schedule/shifts` 都会接触钉钉班次，改动时要同时检查 `week_schedule_service.go`。
