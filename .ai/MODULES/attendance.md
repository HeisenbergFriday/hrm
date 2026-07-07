---
purpose: 考勤模块业务规则说明
last_updated: 2026-07-02
source_of_truth:
  - internal/api/handlers.go（考勤相关 handler）
  - internal/api/attendance_toolbox_handlers.go（考勤工具箱上传计算 handler）
  - internal/service/attendance_service.go（考勤服务）
  - internal/service/attendance_toolbox_service.go（考勤工具箱服务）
  - internal/database/models.go（Attendance 模型）
  - frontend/src/pages/Attendance.tsx（考勤查询）
  - frontend/src/pages/AttendanceStats.tsx（考勤统计）
  - frontend/src/pages/AttendanceToolbox.tsx（考勤工具箱）
  - frontend/src/pages/OvertimeRulesEditor.tsx（加班规则配置编辑器）
  - tools/attendance_toolbox/python/runner.py（Excel 计算入口，支持 --action 扩展）
  - tools/attendance_toolbox/python/requirements.txt（Python 依赖）
update_when:
  - 修改考勤同步逻辑时
  - 修改考勤查询逻辑时
  - 修改考勤统计逻辑时
  - 修改考勤工具箱上传计算逻辑时
  - 修改加班规则配置逻辑时
---

# 考勤模块

## 模块定位

从钉钉同步打卡记录，查询考勤记录，统计考勤异常，导出考勤报表；考勤工具箱提供系统内 Excel 上传、计算和结果下载能力。

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

唯一索引：`user_id + check_time + check_type`

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

### GET /api/v1/attendance/stats
考勤异常统计

Query 参数：
- `start_date`：开始日期（YYYY-MM-DD）
- `end_date`：结束日期（YYYY-MM-DD）
- `department_id`：部门 ID（可选）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "total_days": 20,
        "total_users": 100,
        "late_count": 10,
        "early_leave_count": 5,
        "absent_count": 2,
        "details": [
            {
                "user_id": "xxx",
                "user_name": "张三",
                "late_count": 2,
                "early_leave_count": 1,
                "absent_count": 0
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

### POST /api/v1/attendance/toolbox/:module/run
在系统内运行考勤 Excel 工具箱。

Path 参数：
- `module`：`leave` / `overtime` / `subsidy` / `final` / `parttime` / `dingtalk_sync`

Request：
- `multipart/form-data`
- 文件字段由前端 `AttendanceToolbox.tsx` 统一维护
- 文本名单字段支持逗号分隔

Response：
- 成功时返回 `.xlsx` 或 `.zip` 附件
- 失败时返回统一 JSON 错误信息

### POST /api/v1/attendance/toolbox/dingtalk-sync
从钉钉同步审批数据，生成中间表。

Body：
```json
{
  "start_date": "2026-06-01",
  "end_date": "2026-06-30",
  "flow_keys": ["leave", "overtime", "attendance_correction", "position_transfer"],
  "max_instances": 100,
  "padding_days": 31
}
```

Response：
- 成功时返回 `.xlsx` 或 `.zip` 附件

### GET /api/v1/attendance/toolbox/defaults

读取考勤工具箱 Python 规则中的默认文本名单，前端进入 `AttendanceToolbox.tsx` 时会自动带入输入框。

### POST /api/v1/attendance/toolbox/rules/export
导出加班规则配置为 Excel 文件。

Response：
- 成功时返回 `.xlsx` 附件

### POST /api/v1/attendance/toolbox/rules/import-preview
导入加班规则配置 Excel，返回 JSON 预览。

Request：
- `multipart/form-data`
- `rules_file`：规则配置 Excel 文件

Response：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "premium_rules": [...],
    "department_rules": [...],
    "params": {...}
  }
}
```

### POST /api/v1/attendance/toolbox/:module/validate
校验上传的 Excel 文件表头是否匹配预期。

Request：
- `multipart/form-data`
- 文件字段与对应模块一致

Response：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "ok": true,
    "results": {
      "leave_export": {"ok": true, "headers": [...], "missing": []}
    }
  }
}
```

### 考勤工具箱文件兼容与校验口径

- 花名册/员工信息表需要兼容 `.xlsx` 和旧版 `.xls`。`.xlsx` 继续使用 `openpyxl`，`.xls` 通过 `tools/attendance_toolbox/python/excel_compat.py` 使用 `xlrd` 读取；运行时依赖必须包含 `xlrd>=2.0,<3`。
- 加班导出文件可能包含历史 sheet。节假日年份校验应先按作息表或加班数据推断出的目标月份过滤有效行，再校验实际参与本次输出的年份，避免不参与输出的历史年份阻塞计算。

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

### 考勤异常统计流程

1. **查询打卡记录**
   - 按日期范围和部门查询
   - 按用户分组

2. **计算异常**
   - 迟到：上班打卡时间 > 规定上班时间
   - 早退：下班打卡时间 < 规定下班时间
   - 缺卡：应打卡未打卡

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
| `AttendanceRuleEngine` | `attendance_rule_engine.go` | 考勤规则引擎（计算异常） |
| `AttendanceToolboxService` | `attendance_toolbox_service.go` | 保存上传 Excel、调用内置 Python 计算引擎、返回结果文件；支持 rules export/import/validate/preview 动作 |

---

## 前端页面

### 考勤查询页面
`frontend/src/pages/Attendance.tsx`

功能：
- 考勤记录查询（支持分页、筛选）
- 同步考勤记录

### 考勤异常统计页面
`frontend/src/pages/AttendanceStats.tsx`

功能：
- 考勤异常统计
- 按部门/用户查看

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
- 上传 Excel 后由 HR 后端调用 `tools/attendance_toolbox/python/runner.py`
- 结果直接下载，不再跳转到外部 Streamlit 工具
- 特殊名单、成都作息名单、产研部门关键字、晚走补贴人员、兼职特殊人员名单进入页面时直接展示原工具默认值，用户可按需修改或清空
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

---

## 常见问题

### 同步失败
- 检查钉钉应用权限（需要"考勤打卡权限"）
- 检查日期范围是否超过 7 天
- 检查用户 ID 是否正确

### 考勤记录重复
- 检查唯一索引是否生效（`user_id + check_time + check_type`）
- 重新同步会自动去重

### 考勤异常统计不准确
- 检查考勤规则配置（上下班时间）
- 检查打卡记录是否完整
- 检查 `AttendanceRuleEngine` 逻辑

### 导出任务一直 pending
- 检查异步任务是否正常运行
- 检查日志：`logrus` 会输出详细错误信息
- 检查文件存储路径是否可写
