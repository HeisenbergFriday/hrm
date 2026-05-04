---
purpose: 考勤模块业务规则说明
last_updated: 2026-04-30
source_of_truth:
  - internal/api/handlers.go（考勤相关 handler）
  - internal/service/attendance_service.go（考勤服务）
  - internal/database/models.go（Attendance 模型）
  - frontend/src/pages/Attendance.tsx（考勤查询）
  - frontend/src/pages/AttendanceStats.tsx（考勤统计）
update_when:
  - 修改考勤同步逻辑时
  - 修改考勤查询逻辑时
  - 修改考勤统计逻辑时
---

# 考勤模块

## 模块定位

从钉钉同步打卡记录，查询考勤记录，统计考勤异常，导出考勤报表。

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
                "file_url": "https://..."
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
