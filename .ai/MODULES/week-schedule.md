---
purpose: 大小周排班模块业务规则说明
last_updated: 2026-04-30
source_of_truth:
  - internal/api/leave_handlers.go（排班相关 handler）
  - internal/database/models.go（WeekScheduleRule、WeekScheduleOverride、WeekScheduleSyncLog、StatutoryHoliday 模型）
  - frontend/src/pages/WeekSchedule.tsx（排班管理页面）
update_when:
  - 修改大小周计算规则时
  - 修改排班同步逻辑时
  - 修改节假日处理逻辑时
  - 修改手动覆盖逻辑时
---

# 大小周排班模块

## 模块定位

管理大小周排班规则、法定节假日、钉钉班次配置、手动覆盖、双向同步到钉钉考勤组。

---

## 数据模型

### WeekScheduleRule
大小周规则

```go
type WeekScheduleRule struct {
    ID           uint
    Scope        string  // company / department / user
    TargetID     string  // 公司ID/部门ID/用户ID
    TargetName   string
    BaseWeek     string  // YYYY-Www（基准周，ISO 8601）
    Pattern      string  // 5,6（大周5天小周6天）
    Enabled      bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### WeekScheduleOverride
大小周手动覆盖

```go
type WeekScheduleOverride struct {
    ID         uint
    UserID     string
    WeekNumber string  // YYYY-Www
    WorkDays   int     // 覆盖后的工作天数
    Remark     string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### StatutoryHoliday
法定节假日/调休上班日

```go
type StatutoryHoliday struct {
    ID          uint
    Date        string  // YYYY-MM-DD
    Name        string  // 节假日名称
    Type        string  // holiday / workday
    Description string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

唯一索引：`date`

### WeekScheduleSyncLog
大小周同步钉钉日志

```go
type WeekScheduleSyncLog struct {
    ID         uint
    SyncType   string  // to_dingtalk / from_dingtalk
    StartDate  string
    EndDate    string
    Status     string  // success / failed
    Message    string
    CreatedAt  time.Time
}
```

### DingTalkShiftCatalog
钉钉班次名→ID 映射缓存

```go
type DingTalkShiftCatalog struct {
    ID        uint
    ShiftName string  // 班次名称
    ShiftID   string  // 钉钉班次 ID
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

---

## API 接口

### 大小周规则

#### GET /api/v1/week-schedule/rules
查询大小周规则

Query 参数：
- `scope`：范围（可选，company/department/user）
- `target_id`：目标 ID（可选）

#### POST /api/v1/week-schedule/rules
创建大小周规则

Body：
```json
{
    "scope": "department",
    "target_id": "2",
    "target_name": "技术部",
    "base_week": "2024-W01",
    "pattern": "5,6",
    "enabled": true
}
```

#### POST /api/v1/week-schedule/rules/batch
批量设置大小周规则

Body：
```json
{
    "rules": [
        {
            "scope": "user",
            "target_id": "xxx",
            "target_name": "张三",
            "base_week": "2024-W01",
            "pattern": "5,6"
        }
    ]
}
```

#### PUT /api/v1/week-schedule/rules/:id
更新大小周规则

Body：
```json
{
    "base_week": "2024-W02",
    "pattern": "6,5",
    "enabled": true
}
```

#### DELETE /api/v1/week-schedule/rules/:id
删除大小周规则

---

### 钉钉班次

#### GET /api/v1/week-schedule/shifts
查询钉钉班次列表

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": [
        {
            "shift_id": "xxx",
            "shift_name": "标准班次",
            "work_time_minutes": 480
        }
    ]
}
```

#### POST /api/v1/week-schedule/shifts
创建钉钉班次

Body：
```json
{
    "shift_name": "大周班次",
    "sections": [
        {
            "times": [
                {"check_type": "OnDuty", "check_time": "09:00"},
                {"check_type": "OffDuty", "check_time": "18:00"}
            ]
        }
    ]
}
```

---

### 周历与覆盖

#### GET /api/v1/week-schedule/calendar
查询周历

Query 参数：
- `user_id`：用户 ID
- `start_date`：开始日期（YYYY-MM-DD）
- `end_date`：结束日期（YYYY-MM-DD）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": [
        {
            "date": "2024-01-15",
            "week_number": "2024-W03",
            "is_workday": true,
            "is_holiday": false,
            "work_days_in_week": 5,
            "override": null
        }
    ]
}
```

#### POST /api/v1/week-schedule/overrides
设置周覆盖

Body：
```json
{
    "user_id": "xxx",
    "week_number": "2024-W03",
    "work_days": 6,
    "remark": "临时调整"
}
```

#### DELETE /api/v1/week-schedule/overrides/:id
删除周覆盖

---

### 同步

#### POST /api/v1/week-schedule/sync/to-dingtalk
同步到钉钉

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31",
    "user_ids": ["xxx"]
}
```

#### POST /api/v1/week-schedule/sync/from-dingtalk
从钉钉同步

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### GET /api/v1/week-schedule/sync/logs
查询同步日志

---

### 法定节假日

#### GET /api/v1/week-schedule/holidays
查询法定节假日

Query 参数：
- `year`：年份
- `type`：类型（可选，holiday/workday）

#### POST /api/v1/week-schedule/holidays
创建法定节假日

Body：
```json
{
    "date": "2024-01-01",
    "name": "元旦",
    "type": "holiday",
    "description": "元旦假期"
}
```

#### POST /api/v1/week-schedule/holidays/batch
批量创建法定节假日

Body：
```json
{
    "holidays": [
        {
            "date": "2024-01-01",
            "name": "元旦",
            "type": "holiday"
        }
    ]
}
```

#### POST /api/v1/week-schedule/holidays/sync/from-juhe
从聚合数据同步节假日

Body：
```json
{
    "year": 2024
}
```

#### DELETE /api/v1/week-schedule/holidays/:id
删除法定节假日

---

## 核心业务流程

### 大小周计算流程

1. **查询规则**
   - 按优先级：user > department > company
   - 找到最匹配的规则

2. **计算周数**
   - 基准周：`base_week`（例如 `2024-W01`）
   - 当前周：`current_week`
   - 周差：`week_diff = current_week - base_week`
   - Pattern：`5,6`（大周 5 天，小周 6 天）
   - 当前周工作天数：`pattern[week_diff % len(pattern)]`

3. **应用覆盖**
   - 检查是否有手动覆盖（`WeekScheduleOverride`）
   - 如果有，使用覆盖值

4. **应用节假日**
   - 检查当前日期是否为法定节假日
   - 如果是 `holiday`，不工作
   - 如果是 `workday`，工作

### 同步到钉钉流程

1. **计算每个用户的排班**
   - 按日期范围计算每天的班次

2. **调用钉钉 API**
   - 批量设置用户排班
   - 钉钉 API：`/topapi/attendance/schedule/listbyparam`

3. **记录同步日志**
   - 写入 `WeekScheduleSyncLog`

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `WeekScheduleService` | （待确认） | 大小周管理 |

---

## 前端页面

### 大小周管理页面
`frontend/src/pages/WeekSchedule.tsx`

功能：
- 大小周规则管理
- 法定节假日管理
- 周历查看
- 手动覆盖
- 同步到钉钉

---

## 环境变量

- `DINGTALK_ATTENDANCE_GROUP_ID`：钉钉考勤组 ID
- `DINGTALK_ATTENDANCE_GROUP_NAME`：钉钉考勤组名称
- `JUHE_API_KEY`：聚合数据节假日接口 Key（可选）

---

## 常见问题

### 大小周计算不对
- 检查 `base_week` 是否正确
- 检查 `pattern` 格式是否正确（例如 `5,6`）
- 检查是否有手动覆盖

### 同步到钉钉失败
- 检查钉钉应用权限（需要"考勤排班权限"）
- 检查 `DINGTALK_ATTENDANCE_GROUP_ID` 是否正确
- 检查钉钉班次是否存在

### 法定节假日不生效
- 检查 `StatutoryHoliday` 表是否有数据
- 检查日期格式是否正确（YYYY-MM-DD）
- 检查 `type` 是否正确（holiday/workday）

### 从聚合数据同步失败
- 检查 `JUHE_API_KEY` 是否正确
- 检查网络连接
- 聚合数据 API 有调用限制
