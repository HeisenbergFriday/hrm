---
purpose: 考勤模块业务规则说明
last_updated: 2026-08-04
source_of_truth:
  - internal/api/handlers.go（考勤相关 handler）
  - internal/api/attendance_toolbox_handlers.go（考勤工具箱上传计算 handler）
  - internal/service/attendance_service.go（考勤服务）
  - internal/service/attendance_toolbox_service.go（考勤工具箱服务）
  - internal/database/models.go（Attendance 模型）
  - frontend/src/pages/Attendance.tsx（考勤查询）
  - frontend/src/pages/AttendanceToolbox.tsx（考勤工具箱）
  - frontend/src/pages/OvertimeRulesEditor.tsx（加班规则配置编辑器）
  - tools/attendance_toolbox/python/runner.py（Excel 计算入口，支持 --action 扩展）
  - tools/attendance_toolbox/python/requirements.txt（Python 依赖）
update_when:
  - 修改考勤同步逻辑时
  - 修改考勤查询逻辑时
  - 修改考勤工具箱上传计算逻辑时
  - 修改加班规则配置逻辑时
---

# 考勤模块

## 模块定位

从外部数据源同步打卡与审批结果，按员工和日期查询每日考勤，导出考勤报表；考勤工具箱提供系统内 Excel 上传、计算和结果下载能力。

---

## 数据模型

### Attendance
考勤记录

```go
type Attendance struct {
    ID        uint
    UserID    string     // 钉钉用户 ID
    UserName  string
    CheckTime time.Time  // 打卡时间
    CheckType string     // OnDuty / OffDuty（上班/下班）
    Location  string     // 打卡地点
    Extension map[string]interface{}
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt
}
```

唯一索引：`org_id + user_id + check_time + check_type`

### 外部 Doris 考勤同步（一期）

数据流：`Doris 只读 SELECT → external_* staging → attendances / user_department_relations`

关键本地表：
- `external_attendance_raw`：按 `source_row_key` 幂等；比较 `source_updated_at` 禁止旧盖新
- `external_attendance_approve_links`：`approve_list` 关联，唯一键 `(org_id, source_row_key, item_key)`，不创建 Approval
- `external_user_department_raw` / `user_department_relations`：多部门关系，一期不覆盖 `users.department_id`
- `external_sync_cursors`：高水位 `(cursor_time, cursor_tie_key)`
- `external_sync_jobs` / `external_sync_locks`：任务记录与 DB 级互斥

分页游标：`PageTieKey` 在 Doris SQL 侧用 `SHA2`/`CONCAT_WS` 子查询计算（`r:{record_id}` 优先，否则语义字段哈希）；**不做 Go 端 over-fetch**。空 record_id 时禁止仅用空字符串推进。

Doris 分页兼容：当前源库不接受 `LIMIT ?` 参数占位符；页大小必须先在 Go 端限制到安全范围，再作为整数写入 SQL，其他查询条件继续参数绑定。

业务考勤解析：优先使用 `attendance_result_list` 中带 `check_type` 的上下班结果；`check_record_list` 可能只有时间而没有 OnDuty/OffDuty，不得用主记录类型套用到全部嵌套打卡。重复命中已有考勤时必须通过模型 serializer 更新 JSON `extension`。

首次同步（无 cursor）：使用 `EXTERNAL_ATTENDANCE_INITIAL_START_TIME`，未配置时默认 Unix epoch（全量历史）；成功完成后写入 cursor，之后 cron 使用 `cursor + lookback` 增量。

锁：attendance / department / all 共用同一串行锁 `scope_key=external-attendance`（按 org 唯一）。

Job 状态：`success`（无阶段错误且 Failed=0）/ `partial`（有成功且有失败或阶段错误）/ `failed`（零成功且有失败或阶段错误）；`approve_list` 解析失败会写入 `ErrorSummary`。

部门完整快照：仅 `full_department_snapshot=true` 且本阶段零失败时才失活缺失关系。

### AttendanceExport
考勤导出任务记录

```go
type AttendanceExport struct {
    ID         uint
    UserID     string     // 发起导出的用户
    StartDate  string     // YYYY-MM-DD
    EndDate    string     // YYYY-MM-DD
    Status     string     // pending / processing / completed / failed
    FilePath   string     // 导出文件路径
    FileURL    string     // 导出文件 URL
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

---

## API 接口

### 外部同步中心
| 方法 | 路径 | 权限 |
|---|---|---|
| GET | `/attendance/external-sync/status` | menu 可读 / attendance_manage |
| GET | `/attendance/external-sync/daily-results` | menu 可读 / attendance_manage |
| POST | `/attendance/external-sync/run` | attendance_manage；未启用/未配置 503；锁冲突 409 |
| GET | `/attendance/external-sync/jobs` | 同上可读 |
| GET | `/attendance/external-sync/jobs/:id` | 同上可读 |

前端路由：`/attendance/external-sync`（`AttendanceExternalSync.tsx`）。`externalSync.run` 单独 timeout 10 分钟。

定时同步：`ExternalAttendanceJobScheduler` 受 `EXTERNAL_ATTENDANCE_SYNC_ENABLED` 控制，按 `EXTERNAL_ATTENDANCE_SYNC_INTERVAL` 轮询 mapped org。

### GET /api/v1/attendance/records
查询考勤记录

Query 参数：
- `user_id`：用户 ID（可选）
- `start_date`：开始日期（YYYY-MM-DD）
- `end_date`：结束日期（YYYY-MM-DD）
- `check_type`：打卡类型（可选，OnDuty/OffDuty）
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 20）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "total": 100,
        "page": 1,
        "page_size": 20,
        "items": [
            {
                "id": 1,
                "user_id": "xxx",
                "user_name": "张三",
                "check_time": "2024-01-15T09:00:00Z",
                "check_type": "OnDuty",
                "location": "公司"
            }
        ]
    }
}
```

### POST /api/v1/attendance/sync
同步考勤记录

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "synced_count": 1000
    }
}
```

### POST /api/v1/attendance/export
导出考勤记录

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31",
    "department_id": "2"
}
```

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "export_id": 1,
        "status": "pending"
    }
}
```

### GET /api/v1/attendance/exports
查询导出任务列表

Query 参数：
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 20）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "total": 10,
        "items": [
            {
                "id": 1,
                "start_date": "2024-01-01",
                "end_date": "2024-01-31",
                "status": "completed",
                "file_name": "attendance-2026-05.xlsx",
                "file_path": "uploads/attendance-2026-05.xlsx"
            }
        ]
    }
}
```

### GET /api/v1/attendance/last-sync
获取最近同步时间

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "last_sync_time": "2024-01-15T10:00:00Z"
    }
}
```

### 考勤工具箱权限矩阵（安全收口）

| 能力 | 权限码 | 兼容 |
|---|---|---|
| 页面可见 / 读 defaults | `menu:attendance-toolbox` 或 `attendance_toolbox_operate` | `attendance_manage` |
| 普通计算 / 校验 / 审计 / 模板 / 结果查询下载 / 预览 | `attendance_toolbox_operate` | `attendance_manage` |
| 从本地组织数据生成花名册 | `attendance_toolbox_operate` | `attendance_manage` |
| 钉钉同步（blob 与 structured） | `attendance_toolbox_dingtalk_sync` | `attendance_manage` |
| 一键联动 quick | **同时** `operate` + `dingtalk_sync`（AND） | `attendance_manage` 可兼容两项 |
| 规则导入/应用/导出 / 请求含 `rules_json`/`rules_file` | `attendance_toolbox_rules_edit` | `attendance_manage` |

- 菜单权限不能单独执行计算/同步/下载。
- 通用动态路由 allowlist：`leave` / `overtime` / `subsidy` / `final` / `parttime`。
- `quick` / `dingtalk_sync` **禁止**走 `/toolbox/:module/run` 或 `/toolbox/workflows/:module`，必须用固定权限的专用路由。

### POST /api/v1/attendance/toolbox/:module/run
兼容 blob 计算接口。

Path 参数（allowlist）：
- `module`：`leave` / `overtime` / `subsidy` / `final` / `parttime`
- 若传 `dingtalk_sync` / `quick` → 400

Request：
- `multipart/form-data`
- 加班可传 `rules_json` / `rules_file`；携带自定义规则时后端额外校验 `rules_edit`

Response：
- 成功 `.xlsx` / `.zip`；失败 JSON

### POST /api/v1/attendance/toolbox/workflows/:module
### POST /api/v1/attendance/toolbox/workflows/quick
### POST /api/v1/attendance/toolbox/workflows/dingtalk_sync

DingTalk process-code runtime mapping (`process_codes` keys; global env names are default-org compatibility only):
- leave -> `DINGTALK_PROCESS_LEAVE`
- overtime -> `DINGTALK_PROCESS_OVERTIME`
- attendance_correction -> `DINGTALK_PROCESS_ATTENDANCE_CORRECTION`
- position_transfer -> `DINGTALK_PROCESS_POSITION_TRANSFER`
- 多组织环境中，考勤工具箱必须按 JWT `org_id` 读取该组织的 AppKey/AppSecret 与 `organizations.extension.dingtalk_process_codes`；`DINGTALK_ORGANIZATIONS[].process_codes` 负责初始化/更新该扩展字段。
- 非默认组织禁止静默回退全局 `DINGTALK_APP_*` 或全局流程码，否则会出现用 A 企业凭证查询 B 企业流程并返回 0 条的假空结果。
- AppKey/AppSecret 与审批流程码只能由服务端按 JWT `org_id` 注入；blob/structured 请求不允许通过 body 或 multipart 字段覆盖组织钉钉配置。
- If none of the selected flows has a configured process code, the API must fail with an actionable configuration error instead of storing a meta-only run.
- Meta-only runs are not downloadable results; the frontend must not show success or ZIP download for them.
- `topapi/processinstance/listids` 的单次查询窗口最多 120 天，结束时间不得晚于钉钉当前时间；前端可继续传业务所需 padding，Python 客户端必须自动截断未来结束时间、按连续窗口分片，并对跨片实例 ID 去重。`max_instances` 是所有分片共享的全局上限，禁止每片重新计数。
- `build-and-deploy.ps1 -SkipConfigUpload` keeps the existing server env file, so new/changed process-code values require a deployment without this switch (or an explicit config upload/restart).
结构化工作流：返回 `run_id` + 文件元数据/统计/日志；结果绑定 `user_id + org_id`，磁盘目录仅 `rootDir/<runID>`。

- `quick`：钉钉同步 → 同请求内生成请假/加班；`run_leave`/`run_overtime` 为 true 时后端自动合并对应 `flow_keys`
- 下载 / 预览：
  - `GET /api/v1/attendance/toolbox/runs/:run_id`
  - `GET /api/v1/attendance/toolbox/runs/:run_id/files/:file_key`
  - `GET /api/v1/attendance/toolbox/runs/:run_id/zip`（任一文件缺失则失败，不静默跳过）
  - `GET /api/v1/attendance/toolbox/runs/:run_id/preview?file_key=`（前 200 行，不重跑计算）
- 过期 410 / 不存在 404 / 越权 403；服务重启后内存元数据清空，启动清理孤儿 run 目录
- 环境变量：`ATTENDANCE_TOOLBOX_RUN_TTL_SECONDS`（默认 2h，夹在 5m–24h）、`ATTENDANCE_TOOLBOX_RUN_MAX_BYTES`（默认 512MB）
- run 读取/下载权限按模块收紧：`attendance_manage` / `attendance_toolbox_operate` 可读任意模块结果；仅持 `attendance_toolbox_dingtalk_sync` 的用户只可读/下载**自己同 org** 的 `dingtalk_sync` 结果，禁止借此访问 leave/overtime 等其他模块结果。路由中间件放行三种权限，模块边界由 handler `toolboxRunModuleAccessible` 判断。

请假、异动等钉钉审批数据的页面自动回填必须走 structured workflow，按 `kind=export + flow_key` 下载唯一业务文件；`kind=audit` / `kind=meta` 只用于诊断或手动下载，禁止作为上传输入，也不得因审计文件存在而把结果误判为不可回填的 ZIP。手动“钉钉同步”仍可下载包含业务表和审计表的完整 ZIP。

花名册不属于钉钉审批流程：必须调用本地组织数据接口 `/api/v1/attendance/toolbox/roster/generate`，禁止再次使用 `position_transfer` 或其他 structured workflow 导出表充当花名册。花名册与异动流程的权限、数据来源和自动回填状态必须相互独立。

前端回退策略：`shouldFallbackToLegacyToolboxAPI` 仅在结构化接口 **404/405/501** 时回退旧 blob；403/400/410/5xx/超时/网络不得回退，钉钉同步成功后不得重跑。

### POST /api/v1/attendance/toolbox/roster/generate

从当前 JWT `org_id` 的本地组织数据库生成标准在职花名册 xlsx。

- 权限：`attendance_toolbox_operate` 或兼容的 `attendance_manage`；`attendance_toolbox_dingtalk_sync`、menu-only 均不得调用。
- 组织：JWT `org_id` 是唯一可信来源；缺失时 fail-closed，请求 body/query 不得覆盖；用户查询必须按 `org_id` 隔离并排除软删除数据。
- 员工范围：仅当前组织 `status=active` 且未软删除的用户；业务工号必须取 `EmployeeProfile.EmployeeID`，部门路径必须取该用户当前主部门在本组织中的真实父子层级。所有用户、档案、部门查询均显式绑定 JWT `org_id`。
- 完整性：任一待输出员工缺少 `EmployeeID`，或主部门缺失、跨组织、父级断裂、循环、空名称导致路径不可解析时，接口整体返回包含缺失人数的 400；禁止跳过后静默生成不完整文件。姓名、`UserID`、`DingTalkUserID` 均不得兜底业务工号。
- 输出：xlsx 固定 12 列：工号、姓名、合同主体、一级部门、二级部门、三级部门、岗位、员工类型、人员分类、入职日期、离职日期、转正日期；其中工号、姓名和真实部门路径是加班入口契约，其他无权威来源字段保持空。超过三级的组织路径保留距离叶子最近的三级业务部门，顺序不得重排或猜测。
- 回填：前端自动生成后可将同一份富花名册回填到 `overtime_roster` 与 `final_active`；不得把仅姓名文件自动回填到 `overtime_roster`。最终汇总仍可从用户上传的钉钉月度汇总表补充/纠正身份字段，手工仅姓名花名册只作为最终汇总兼容输入，不是组织生成接口的输出契约。
- 重名：生成文件以权威工号区分同名员工；Python 部门映射只有在姓名全文件唯一时才建立姓名回退键，重名时只允许按工号命中，禁止首条覆盖或按姓名误映射。
- 成功：返回 xlsx、`Content-Disposition`、`X-Content-Type-Options: nosniff`；无有效在职员工、缺业务工号或缺部门路径返回 400，数据查询、runner 失败或无输出返回 500。
- 路由测试：必须调用生产使用的 `registerAttendanceToolboxRoutes` 共享注册逻辑验证 `POST` 路径和权限矩阵；禁止在测试中重复注册路径、中间件与 handler。

### POST /api/v1/attendance/toolbox/dingtalk-sync
旧 blob 钉钉同步；推荐 structured workflows。权限同 `attendance_toolbox_dingtalk_sync`。

- 响应规则（按业务 export 数量）：
  - 仅 audit/meta、无业务 export：返回 **422**（`未生成业务表`），禁止 JSON 200 被旧前端当 Excel。
  - 恰好 1 个业务 export（可含 audit/meta）：直接返回该业务 Excel（`Content-Disposition` 为业务文件名）。
  - 多个业务 export：返回 ZIP。
- 说明：`BusinessExports()` 分支是本实现新增的兼容逻辑，用于保证 audit 不计入“多文件”。

### GET /api/v1/attendance/toolbox/defaults
只读默认名单；`menu` 或 `operate` 即可。

### POST /api/v1/attendance/toolbox/rules/export
导出默认规则；若 body/form 带 `rules_json` 则导出当前自定义规则（需 `rules_edit`）。

### POST /api/v1/attendance/toolbox/rules/import-preview
导入规则 Excel 预览 JSON（需 `rules_edit`）。导入预览 ≠ 应用；应用后前端才把 `rules_json` 发到 `runWorkflow`。

### POST /api/v1/attendance/toolbox/:module/validate
表头校验（需 `operate`）。

### POST /api/v1/attendance/toolbox/templates
导出考勤工具箱模板文件（Excel 等）。

- 权限：`attendance_toolbox_operate`（兼容 `attendance_manage`）
- Body（JSON，可选）：`{ "template_id": "<id>" }`；未指定时由服务按默认模板处理
- 成功：返回可下载文件（`Content-Disposition: attachment`）；若结果为 meta-only 则返回 JSON `data` 元数据
- 用途：前端「下载模板」入口，供用户按标准表头准备上传文件；**不**重跑计算

### POST /api/v1/attendance/toolbox/audit
上传前/上传后文件审计（体积、行列规模等预警）。

- 权限：`attendance_toolbox_operate`（兼容 `attendance_manage`）
- 请求：优先 `multipart/form-data`（实际文件）；也可 JSON：`{ "files": [...], "max_warn_mb", "max_warn_rows", "max_warn_cols" }`
- 响应：JSON 审计结果（警告/统计），**不**执行 leave/overtime 等业务计算
- 用途：计算前 checklist / 体积与行列预警，避免超大表拖垮 Python 引擎

### 旧 `/attendance/processing/*` 兼容
`leave` / `overtime` / `subsidy` / `final` / `parttime` 仍保留旧字段名与 Blob 响应，但已统一映射到 `AttendanceToolboxService`/runner；本轮不删除 `tools/attendance-processing` 副本。

### 考勤工具箱文件兼容与校验口径

- 花名册/员工信息表兼容 `.xlsx` / `.xls`（`excel_compat` + `xlrd>=2.0,<3`）。最终汇总兼容仅姓名名单；加班 `overtime_roster` 必须包含可用工号与部门列，否则 `run_overtime` 返回包含实际表头的明确错误。
- 共享 `_find_header_row` 保持子串匹配，以兼容请假“员工工号/员工姓名/请假类型名称”和加班“2倍加班（小时）”等历史表头；花名册身份列局部精确匹配：姓名仅接受“姓名/员工姓名”，工号仅接受“工号/员工工号/员工编号”，禁止识别发起人、申请人等审批流程字段。部门映射优先工号；姓名仅在全文件唯一时允许回退，重名不得生成姓名映射键。
- 节假日年份按目标月过滤后再校验。
- 业务真源比对：`tools/attendance_toolbox/python/scripts/compare_app_source.py` 全量生成 `SOURCE_MANIFEST.json`（含 `difference_kind=equal|adapter_only`）；禁止手改 manifest。允许 adapter 差异：`sys.path` 注入、`excel_compat`、`load_workbook_compat`。无 `D:\app` 时本地比对 skip；CI 仍跑仓库内 fixture/hash 测试。 Local golden tests use synthetic Excel inputs to compare leave/overtime/subsidy/final/parttime output fingerprints against `D:\app`; when `D:\app` exists, these parity tests must run with 0 skip.

---

## 核心业务流程

### 同步考勤流程

1. **调用钉钉 API**（`SyncAttendance`）
   - 按日期范围获取打卡记录
   - 钉钉 API 限制：每次最多查询 7 天
   - 需要分批查询

2. **写入数据库**
   - Upsert 到 `attendances` 表
   - 唯一键：`user_id + check_time + check_type`

### 导出考勤流程

1. **创建导出任务**
   - 写入 `attendance_exports` 表
   - 状态：`pending`

2. **异步导出**
   - 查询考勤记录
   - 生成 Excel 文件
   - 上传到文件服务器（或本地存储）
   - 更新状态：`completed`

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `AttendanceService` | `attendance_service.go` | 考勤管理 |
| `AttendanceToolboxService` | `attendance_toolbox_service.go` | 保存上传 Excel、调用内置 Python 计算引擎、返回结果文件；支持 rules export/import/validate/preview 动作 |

---

## 前端页面

### 考勤查询页面
`frontend/src/pages/Attendance.tsx`

功能：
- 考勤记录查询（支持分页、筛选）
- 同步考勤记录

### 考勤导出页面
`frontend/src/pages/AttendanceExport.tsx`

功能：
- 创建导出任务
- 查看导出任务列表
- 下载导出文件

### 考勤工具箱页面
`frontend/src/pages/AttendanceToolbox.tsx`

功能：
- 六个页签：钉钉同步、请假明细、加班明细、补贴扣款、最终汇总、兼职汇总
- 请假明细页签可选择日期范围，直接按当前组织的钉钉请假审批流程拉取“请假系统导出表”并自动回填；手动上传继续作为兜底。月结操作默认选择上一个完整自然月（例如 7 月 2 日操作默认 6 月 1 日至 6 月 30 日），钉钉同步页签使用相同默认值；用户仍可手动调整。该数据是审批实例级源表，不等同于考勤查询使用的外部同步每日聚合结果，禁止从每日结果反向拼接审批导出表。
- 上传 Excel 后由 HR 后端调用 `tools/attendance_toolbox/python/runner.py`
- 结果直接下载，不再跳转到外部 Streamlit 工具
- 特殊名单、成都作息名单、产研部门关键字、晚走补贴人员、兼职特殊人员名单进入页面时直接展示原工具默认值，用户可按需修改或清空
- 长期产假不扩大钉钉审批查询跨度；请假页签提供手动兜底入口，用户可填写姓名、可选工号、产假起止日期；配置保存在当前浏览器。普通产假仍自动抓取，与钉钉已有产假重叠时跳过手动记录，避免重复计算。
- 最终表法定假日边界：转正日与法定假日同一天时计入；离职日与法定假日同一天时不计入。产假覆盖法定假日时，从法定天数中扣除重叠天数。
- 加班、补贴页签支持可选的 `YYYY-MM` 月份锁定；默认自动识别，锁定月份与作息表月份冲突时停止计算并提示。
- 补贴模块与加班规则共享法定节假日配置：自定义规则含 `legal_holidays_override` 时优先使用，否则回退作息表；两者日期差异写入补贴结果“异常审计”工作表。
- 补贴结果“异常审计”工作表记录考勤缺失人员、实习生剔除人员和节假日口径差异；考勤缺失人员的补贴字段保持空值，不按 0 计算。结构化运行响应在 `meta.subsidy_audit` 返回缺失人数及名单，前端显示警告。
- **补贴扣款数据来源**：必须来自钉钉考勤后台“考勤统计 → 报表管理 → 月度汇总表（补贴及扣款）”的人工导出 Excel。不是钉钉审批流程实例，也不通过 `getattcolumns`/`getcolumnval` 接口自动获取金额列（当前企业实测未返回目标补贴扣款金额列，现阶段不依赖这些接口自动获取补贴扣款数据）。
- **补贴扣款输入格式与月份校验**：补贴扣款支持两种输入，校验口径不同：
  - **钉钉原始月度汇总表**（A1 包含“月度汇总表（补贴及扣款）”且有 UserId 列）：A1 必须同时包含统计开始日期和统计结束日期，且两个日期必须完整覆盖处理月份的自然月（如 7 月必须为 2026-07-01 至 2026-07-31）；任一日期缺失、无法解析、部分范围或跨月时均 fail-closed，停止计算并提示。使用 `calendar.monthrange` 处理 28/29/30/31 天月份和闰年。
  - **系统模板 / 历史兼容格式**（A1 不含“月度汇总表（补贴及扣款）”或无 UserId 列）：不要求 A1 包含统计日期，月份以页面选择的处理月份和作息表月份校验为准，可正常传入 year/month 参数。
  - 列名匹配使用精确关键字（如“15-30分钟迟到扣款”、“旷工天数”），禁止使用宽泛别名（如“迟到”、“早退”）以免误匹配次数/分钟数列。
- **补贴扣款无自动拉取功能**：不新增 `/attendance/toolbox/subsidy/sync` 接口，不新增数据库表或字段。重复上传同一月份时，新文件替换旧文件，不累计。
- 兼职汇总中，同一日同时包含外出/出差与事假时，事假优先且该日不计出勤；组合状态判断必须早于外出/出差计出勤的提前返回。纯外出/出差仍按原规则处理。
- 大文件上传时显示警告提示
- 运行日志可折叠查看（需后端支持返回 log 字段）

### 加班规则配置编辑器
`frontend/src/pages/OvertimeRulesEditor.tsx`

功能：
- 导出默认加班规则配置为 Excel
- 导入规则配置 Excel 并预览（倍数规则、部门匹配规则、排除名单）
- 支持查看当前规则配置详情

---

## 钉钉 API

### 获取打卡记录
```
POST /attendance/list
```

Body：
```json
{
    "workDateFrom": "2024-01-01 00:00:00",
    "workDateTo": "2024-01-07 23:59:59",
    "userIdList": ["xxx"],
    "offset": 0,
    "limit": 50
}
```

注意：
- 时间范围最多 7 天
- 每次最多返回 50 条
- 需要分页查询

---

## 环境变量

- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID
- `ATTENDANCE_TOOLBOX_DIR`：考勤工具箱 Python 引擎目录，默认自动查找 `tools/attendance_toolbox/python`
- `ATTENDANCE_TOOLBOX_PYTHON`：Python 可执行文件，默认 Windows 使用 `python`，Linux 使用 `python3`
- `ATTENDANCE_TOOLBOX_TIMEOUT_SECONDS`：工具箱单次计算超时时间，默认 600 秒
- 工具箱长任务超时顺序必须保持：后端 `ATTENDANCE_TOOLBOX_TIMEOUT_SECONDS` 默认 600 秒 < Nginx/外层网关 630 秒 < 前端请求 660 秒。任一外层代理仍为默认约 60 秒时，钉钉同步会被提前截断并返回 504；部署配置见 `deploy/README.md`。

---

## 常见问题

### 同步失败
- 检查钉钉应用权限（需要"考勤打卡权限"）
- 检查日期范围是否超过 7 天
- 检查用户 ID 是否正确

### 考勤记录重复
- 检查唯一索引是否生效（`user_id + check_time + check_type`）
- 重新同步会自动去重

### 导出任务一直 pending
- 检查异步任务是否正常运行
- 检查日志：`logrus` 会输出详细错误信息
- 检查文件存储路径是否可写

---

## 外部考勤每日结果视图（2026-07）

- 查询接口：`GET /api/v1/attendance/external-sync/daily-results`
- 聚合键：`org_id + local_user_id + work_date`
- 默认查询最近 7 天，最大日期范围 90 天；支持 `user_id`、`department_id`、分页筛选。
- 打卡时间优先读取 `attendance_result_list` 中带 `check_type` 的结果，缺失时再回退到主记录或带类型的 `check_record_list`。
- `time_result` 映射：`Late=迟到`、`SeriousLate=严重迟到`、`Early=早退`、`NotSigned=缺卡`、`Absenteeism=旷工`。
- `approve_list` 映射请假（保留年假/调休/事假/病假/产假/工伤假等子类型）、出差、外出、补卡申请、加班，并格式化小时/天数。
- 同一天的考勤异常和审批状态不得互相覆盖，例如同时返回“迟到”和“年假 2小时”。
- 前端 `Attendance.tsx` 使用每日结果接口，每人每天一行；详情抽屉展示全部打卡时间、地点、原始结果与审批时间段。
- 查询页交互：顶部统计卡可切换全部/正常/异常/有审批，日期支持今天/昨天/近7天/近30天，员工与部门位于“更多筛选”；桌面端使用精简表格与整行详情，移动端使用卡片列表。
- 每日结果接口返回 `summary`（total/normal/exception/with_approval），并支持 `status` 筛选；汇总基于完整筛选范围计算，不按当前页截断。
- 首页考勤率统一使用每日结果汇总，公式为 `summary.normal / summary.total`；有请假、出差等审批的员工日保留在 `total` 分母中。
