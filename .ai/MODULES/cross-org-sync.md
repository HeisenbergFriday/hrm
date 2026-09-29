---
purpose: 文娱到沐腾的只读跨组织数据镜像
last_updated: 2026-09-21
source_of_truth:
  - internal/database/cross_org_sync_models.go
  - internal/repository/cross_org_sync_repository.go
  - internal/service/cross_org_sync_service.go
  - internal/api/cross_org_sync_handlers.go
update_when:
  - 修改跨组织同步方向、镜像字段或同步白名单时
  - 新增业务数据镜像适配器时
  - 新增沐腾侧镜像查询页面或权限时
---

# 跨组织同步模块

## 业务定位

文娱继续负责现有考勤、请假、加班、审批、绩效等业务流程；沐腾先作为统一查看和汇总端。第一期只做文娱 → 沐腾的单向只读镜像，不修改文娱业务记录，不回写钉钉。

数据来源链路为：

`钉钉 → 文娱本地组织数据 → 沐腾镜像`

员工资料同步包含用户基础字段、部门名称和员工档案快照；业务数据通过固定白名单写入源数据 JSON 镜像，不能把镜像行当作沐腾本地可执行业务。

## 数据模型

- `OrganizationSyncLink`：源组织到目标组织的同步关系和业务白名单。
- `OrganizationSyncRun`：一次同步批次的状态、计数、错误摘要和请求 ID。
- `OrganizationEmployeeMirror`：按 `target_org_id + source_org_id + source_user_id` 幂等的员工资料快照。
- `OrganizationBusinessMirror`：按 `target_org_id + source_org_id + entity_type + source_key` 幂等的业务数据快照。

源组织和目标组织必须不同；镜像记录保留源组织和源 ID，禁止直接创建沐腾 `users` 或改写源组织业务表。

## API

当前管理员接口位于 `/api/v1/org/cross-sync`：

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/links` | 当前 JWT 源组织创建/更新目标组织同步关系 |
| `GET` | `/links` | 查看当前源组织的同步关系 |
| `POST` | `/run` | 执行一次文娱到沐腾的同步 |
| `GET` | `/runs/:request_id` | 查询同步批次状态 |
| `GET` | `/employees` | 沐腾侧查看员工镜像 |
| `GET` | `/business` | 沐腾侧按业务类型查看镜像 |
| `GET` | `/center/summary` | 人事数据中心汇总沐腾本地与文娱镜像统计 |
| `GET` | `/center/employees` | 人事数据中心统一分页查看员工资料；可用 `source=local|mirror` 区分沐腾员工与文娱员工 |
| `GET` | `/center/business` | 人事数据中心统一分页查看业务数据；可用 `entity_type` 和 `source=local|mirror` 筛选 |
| `GET` | `/center/links` | 沐腾侧查看已配置的入站同步关系 |
| `POST` | `/center/sync` | 沐腾侧启动已保存关系的文娱入站镜像同步（需 `permission_manage`） |
| `GET` | `/center/sync/:request_id` | 沐腾侧轮询入站同步批次 |

创建关系、执行同步和查看批次需要 `permission_manage`；镜像读取需要 `permission_manage`、`org:read` 或组织菜单权限。普通业务接口仍只使用 JWT 当前 `org_id`，不会从请求参数切换租户。

## 当前业务白名单

`attendance`、`approval`、`annual_leave_grant`、`overtime_match`、`performance_activity`、`performance_participant`。新增类型必须添加明确的表名、字段脱敏和回归测试，禁止开放任意表名查询。

## 同步约束

- 重复运行使用唯一键 upsert，不新增重复镜像。
- 同步失败保留旧镜像，并在批次记录失败计数和安全错误摘要。
- 同组织同目标存在 running 批次时拒绝重复启动。
- 镜像查询按目标组织强制过滤，不能读取其他目标组织数据。
- 第一期开启的是查看/汇总，不提供沐腾修改、回写文娱或回写钉钉能力。
- 人事数据中心的沐腾员工姓名可跳转到现有沐腾员工资料详情；文娱员工只在数据中心打开只读镜像详情，不能误当作沐腾本地用户处理。业务数据页固定展示六类白名单业务，并标明来源组织、源记录 ID、业务时间和同步状态。
