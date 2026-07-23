---
purpose: 大小周排班模块业务规则说明
last_updated: 2026-07-20
source_of_truth:
  - internal/api/handlers.go（排班相关 handler）
  - internal/service/week_schedule_service.go
  - internal/dingtalk/dingtalk.go（GetScheduleListBatchByDayForOrg / ResolveAdminUserID / UploadImageMediaForOrg / SendCorpImageToUserForOrg）
  - internal/database/models.go（WeekScheduleRule、WeekScheduleOverride、WeekScheduleSyncLog、StatutoryHoliday 模型）
  - frontend/src/pages/WeekSchedule.tsx（排班管理页面）
update_when:
  - 修改大小周计算规则时
  - 修改排班同步逻辑时
  - 修改节假日处理逻辑时
  - 修改手动覆盖逻辑时
  - 修改多企业排班读取/写入隔离时
  - 修改作息表个人推送（图片消息）时
---

# 大小周排班模块

## 模块定位

管理大小周排班规则、法定节假日、钉钉班次配置、手动覆盖、双向同步到钉钉考勤组；以及月作息时间表个人推送（文字+图片，不写考勤排班）。

---

## 数据模型

### WeekScheduleRule
大小周规则

```go
type WeekScheduleRule struct {
    ID           uint
    ScopeType    string  // company / department / user
    ScopeID      string  // 空=全公司，或部门ID/用户ID
    ScopeName    string
    BaseDate     string  // 基准日期，格式 YYYY-MM-DD
    Pattern      string  // big_first 等模式
    ShiftID      int64
    Status       string  // active / inactive
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### WeekScheduleOverride
大小周手动覆盖

```go
type WeekScheduleOverride struct {
    ID            uint
    ScopeType     string  // company / department / user
    ScopeID       string
    WeekStartDate string  // 该周周一，YYYY-MM-DD
    WeekType      string  // big / small
    Reason        string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### StatutoryHoliday
法定节假日/调休上班日

```go
type StatutoryHoliday struct {
    ID        uint
    Date      string  // YYYY-MM-DD
    Name      string  // 节假日名称
    Type      string  // holiday / workday
    Year      int
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

唯一索引：`date`

### WeekScheduleSyncLog
大小周同步钉钉日志

```go
type WeekScheduleSyncLog struct {
    ID         uint
    SyncType   string  // to_dingtalk / from_dingtalk
    TargetDate string
    UserCount  int
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
    Name      string  // 班次名称
    ShiftKey  string  // 稳定签名
    ShiftID   int64   // 钉钉班次 ID
    CheckIn   string
    CheckOut  string
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
当前实现返回全部规则；筛选在前端完成。

#### POST /api/v1/week-schedule/rules
创建大小周规则

Body：
```json
{
    "scope_type": "department",
    "scope_id": "2",
    "scope_name": "技术部",
    "base_date": "2024-01-01",
    "pattern": "big_first",
    "shift_id": 0,
    "status": "active"
}
```

#### POST /api/v1/week-schedule/rules/batch
批量设置大小周规则

Body：
```json
{
    "user_ids": ["xxx"],
    "base_date": "2024-01-01",
    "pattern": "big_first",
    "shift_id": 0,
    "conflict_mode": "overwrite",
    "dry_run": false
}
```

#### PUT /api/v1/week-schedule/rules/:id
更新大小周规则

Body：
```json
{
    "base_date": "2024-01-08",
    "pattern": "small_first",
    "shift_id": 0,
    "status": "active"
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
- `department_id`：部门 ID
- `weeks`：返回周数，默认 8
- `start_date`：起始日期（YYYY-MM-DD，会归一到该周周一）

Response：
```json
{
    "code": 200,
    "message": "success",
    "data": {
        "items": [
            {
                "week_start": "2024-01-15",
                "week_end": "2024-01-21",
                "week_type": "big",
                "is_override": false,
                "saturday_work": false,
                "holidays": []
            }
        ]
    }
}
```

#### POST /api/v1/week-schedule/overrides
设置周覆盖

Body：
```json
{
    "scope_type": "user",
    "scope_id": "xxx",
    "week_start_date": "2024-01-15",
    "week_type": "small",
    "reason": "临时调整"
}
```

#### DELETE /api/v1/week-schedule/overrides/:id
删除周覆盖

---

### 同步

#### POST /api/v1/week-schedule/sync/to-dingtalk
**旧接口**：按日期范围把大小周排班写入钉钉考勤组。

- 页面「作息表推送」按钮**不再调用**本接口。
- 仅保留 API，供运维/脚本或其他入口使用。

Body：
```json
{
    "weeks": 4
}
```

#### POST /api/v1/week-schedule/push/personal
权限：`attendance_manage`

月作息表个人推送（multipart）：`image`（PNG/JPEG）、`user_ids`（JSON 数组/逗号/重复字段）、`title`、`content`。

行为：按 JWT `org_id` 查用户 → 上传一次图片得 `media_id` → 逐人发送文字 + 图片消息（`msgtype: image`）；优先 `DingTalkUserID`，回退 `UserID`；单人失败不阻断；返回 success/partial/failed 与 `recipients[]`。**不写考勤排班**。

#### POST /api/v1/week-schedule/sync/from-dingtalk
从钉钉同步

无请求体；服务会保守拉取钉钉排班信号。

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
    "year": 2024
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
   - 基准日期：`base_date`（某个大周/小周的周一）
   - 当前周起始日期：`week_start`
   - 周差：`week_diff = (week_start - base_date) / 7`
   - Pattern：`big_first` / `small_first`
   - 当前周类型：按周差奇偶在大周、小周之间切换

3. **应用覆盖**
   - 检查是否有手动覆盖（`WeekScheduleOverride`）
   - 如果有，使用覆盖值

4. **应用节假日**
   - 检查当前日期是否为法定节假日
   - 如果是 `holiday`，不工作
   - 如果是 `workday`，工作

### 同步到钉钉流程

1. **解析当前 org 与企业 admin**
   - `requireOrgIDFromDB` fail-closed
   - `dingtalk.ResolveAdminUserID(orgID)`；非 default 禁止回退 `DINGTALK_ADMIN_USER_ID`
2. **计算每个用户的排班**
   - 按日期范围计算每天的班次（业务规则不变）
3. **调用钉钉 API（按 org）**
   - `GetShiftListForOrg` / `GetAttendanceGroupsForOrg` / `BatchSetAttendanceScheduleForOrg`
   - access_token 与 `op_user_id` 均取自当前企业配置
4. **记录同步日志**
   - 写入 `WeekScheduleSyncLog`（仅同步过程开始后；缺配置时不得落库）

### 周五自动提醒（可选）

默认关闭。开启后按本地时区（建议 TZ=Asia/Shanghai）在每周五指定时刻向配置名单发送文字提醒：

- 内容：本周大/小周 + 明天（周六）是否上班
- 不写考勤排班；不上传图片
- 环境变量：
  - WEEK_SCHEDULE_FRIDAY_REMINDER_ENABLED=true
  - WEEK_SCHEDULE_FRIDAY_REMINDER_HOUR（默认 17）
  - WEEK_SCHEDULE_FRIDAY_REMINDER_MINUTE（默认 0）
  - WEEK_SCHEDULE_FRIDAY_REMINDER_USER_IDS：本地 users.user_id 逗号分隔；为空则跳过发送

实现：internal/service/week_schedule_jobs.go + SendPersonalTextNotice。

### 从钉钉回读

- 一律 `GetScheduleListBatchByDayForOrg(orgID, userIDs, workDate)`（含 chunked 辅助函数）
- 禁止非 default 企业调用默认版 `GetScheduleListBatchByDay` / `GetAccessToken`

### 多企业隔离要点

- 企业 A/B 使用各自 access token 与 admin user id
- 企业 B 大小周同步不得打到 default 企业接口
- 缺企业配置时不产生规则/同步日志写库

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `WeekScheduleService` | `week_schedule_service.go` | 大小周管理 |

---

## 前端页面

### 大小周管理页面
`frontend/src/pages/WeekSchedule.tsx`

功能：
- 大小周规则管理
- 法定节假日管理
- 周历查看
- 手动覆盖
- 作息表推送（月历 PNG + 文字到选定员工的个人钉钉；默认不预选）
- 旧「按日期写入考勤排班」接口仍保留为 `/sync/to-dingtalk`，页面主按钮不再调用

---

## 环境变量

- `DINGTALK_ATTENDANCE_GROUP_ID`：钉钉考勤组 ID
- `DINGTALK_ATTENDANCE_GROUP_NAME`：钉钉考勤组名称
- `JUHE_API_KEY`：聚合数据节假日接口 Key（可选）

---

## 常见问题

### 大小周计算不对
- 检查 `base_date` 是否为一个周一
- 检查 `pattern` 是否为 `big_first` 或 `small_first`
- 检查是否有手动覆盖

### 同步到钉钉失败
- 检查当前企业 `organizations.ding_talk_app_key/secret` 与 `ding_talk_admin_user_id` 是否齐全
- 非 default 企业报 `missing dingtalk admin user id for organization ...` 时，禁止靠全局 `DINGTALK_ADMIN_USER_ID` 兜底
- 检查钉钉应用权限（需要“考勤排班权限”）
- 检查 `DINGTALK_ATTENDANCE_GROUP_ID` / 企业内考勤组是否正确
- 检查钉钉班次是否存在于**当前企业**

### 作息表推送失败
- 检查钉钉应用是否具备企业内部消息与媒体上传权限
- 检查收件人 dingtalk_user_id/user_id 是否可通知
- 单人失败不影响其他人，查看响应 recipients[]

### 法定节假日不生效
- 检查 `StatutoryHoliday` 表是否有数据
- 检查日期格式是否正确（YYYY-MM-DD）
- 检查 `type` 是否正确（holiday/workday）

### 从聚合数据同步失败
- 检查 `JUHE_API_KEY` 是否正确
- 检查网络连接
- 聚合数据 API 有调用限制
