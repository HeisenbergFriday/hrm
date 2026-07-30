---
purpose: 大小周排班模块业务规则说明
last_updated: 2026-07-29
source_of_truth:
  - internal/api/handlers.go（排班相关 handler）
  - internal/service/week_schedule_service.go
  - internal/service/week_schedule_group_service.go
  - internal/dingtalk/dingtalk.go（GetScheduleListBatchByDayForOrg / ResolveAdminUserID / UploadImageMediaForOrg / SendCorpImageToUserForOrg）
  - internal/dingtalk/group_robot.go（企业内部机器人群消息）
  - internal/api/week_schedule_group_handlers.go
  - internal/database/models.go（WeekScheduleRule、WeekScheduleOverride、WeekScheduleSyncLog、WeekScheduleGroupTarget、WeekScheduleGroupPushLog、StatutoryHoliday 模型）
  - cmd/dingtalk_stream/main.go（群聊绑定 Stream 回调）
  - frontend/src/pages/WeekSchedule.tsx（排班管理页面）
update_when:
  - 修改大小周计算规则时
  - 修改排班同步逻辑时
  - 修改节假日处理逻辑时
  - 修改手动覆盖逻辑时
  - 修改多企业排班读取/写入隔离时
  - 修改作息表个人推送（图片消息）时
  - 修改作息表群聊绑定、解绑、推送或临时图片访问时
---

# 大小周排班模块

## 模块定位

管理大小周排班规则、法定节假日、钉钉班次配置、手动覆盖、双向同步到钉钉考勤组；以及月作息时间表个人/群聊推送（文字+图片，不写考勤排班）。

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

### WeekScheduleGroupTarget / WeekScheduleGroupPushLog

- `WeekScheduleGroupTarget` 保存当前组织绑定的本地群目标 ID、群名称和服务端专用 `openConversationId`；唯一键为 `(org_id, open_conversation_id)`，API JSON 禁止暴露组织 ID 和会话 ID。
- `WeekScheduleGroupPushLog` 保存操作人、组织、月份、本地群目标、群名称、`processing/submitted/rejected/failed` 状态、钉钉请求 ID 与安全错误摘要；不得记录 Webhook、凭据、临时图片令牌或原始第三方错误。

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

#### GET /api/v1/week-schedule/group-targets

权限：`week_schedule_group_push`，兼容 `attendance_manage`。只返回 JWT 当前 `org_id` 的 active 群目标，响应不包含 `openConversationId`。

#### DELETE /api/v1/week-schedule/group-targets/:id

按 JWT 当前 `org_id` 软解绑本地群目标；跨组织 ID 一律按不存在处理。

#### POST /api/v1/week-schedule/push/group

权限：`week_schedule_group_push`，兼容 `attendance_manage`。

multipart 字段：`image`（PNG/JPEG）、`group_target_id`、`title`、`content`、`month`（`YYYY-MM`）。客户端只能提交本地 `group_target_id`；请求出现 `org_id`、Webhook、AppKey/Secret、RobotCode 或任意 `openConversationId` 时拒绝。

钉钉群消息接口仅表示异步受理，因此成功响应状态固定为 `submitted`，页面文案为“已提交”，不得显示已送达或推送成功。同组织、月份、群目标 10 分钟内的 `processing/submitted` 会被拦截。

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

### 群聊绑定与推送流程

1. 每个 Stream 实例显式配置 `DINGTALK_STREAM_ORG_ID` 时，必须从该 active 组织读取配套 AppKey/Secret；未显式配置时才使用全局 AppKey/Secret 唯一匹配 active 组织。组织不存在、凭据缺失、0 条/多条匹配均启动失败，禁止回退 default 或混用其他组织凭据。
2. 群内 @机器人发送精确文本“绑定作息表”；发送者必须映射为当前组织在职本地用户，并具备 `week_schedule_group_push` 或 `attendance_manage`。
3. 服务端保存 `openConversationId` 和群名称；前端只查询/提交本地群目标 ID。
4. 群图片不能复用个人消息 `media_id`：生成 32 字节随机令牌的 HTTPS 临时地址，内存仅保存令牌 SHA-256，默认 10 分钟过期；无效/过期返回 404，禁止日志记录访问令牌。
5. 通过 `robot/groupMessages/send` 提交 `sampleMarkdown`（文字 + HTTPS 图片）；RobotCode、AppKey、Secret 只从当前组织配置解析。
6. 只有群推送日志成功落为 `submitted` 后才能向页面返回“已提交”；钉钉拒绝/超时/配置缺失映射为安全错误，原始第三方信息和凭据不回显。
7. 个人/群聊推送文案以最近周六的实际日历状态为准，首行突出“今天/明天/本周六/下周六需上班或休息”；大周/小周仅作为补充说明，不得代替 `saturday_work` 与节假日调班状态判断。

### 周五自动提醒（可选）

默认关闭。开启后按本地时区（建议 TZ=Asia/Shanghai）在每周五指定时刻向配置名单发送文字提醒：

- 内容：首行突出“明天需上班/明天休息”，随后显示周六日期和本周大/小周；是否上班以实际日历状态为准
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
- 推送弹窗支持“员工/群聊”切换；两种模式默认均不选择目标，群聊显示已绑定群名称并在提交前二次确认
- 缺少群推送权限时写按钮 disabled + Tooltip；钉钉受理后只显示“已提交”
- 推送文字按发送日期自适应：周五写“明天”、周六写“今天”、周一至周四写“本周六”、周日写“下周六”，并将“需上班/休息”置于首行
- 旧「按日期写入考勤排班」接口仍保留为 `/sync/to-dingtalk`，页面主按钮不再调用

---

## 环境变量

- `DINGTALK_ATTENDANCE_GROUP_ID`：钉钉考勤组 ID
- `DINGTALK_ATTENDANCE_GROUP_NAME`：钉钉考勤组名称
- `JUHE_API_KEY`：聚合数据节假日接口 Key（可选）
- `DINGTALK_STREAM_ORG_ID`：Stream 实例显式所属组织（可选；配置后从该组织读取 AppKey/Secret，未配置时全局 AppKey 必须唯一匹配 active 组织）
- 当前组织 `AppHomeURL`：必须为钉钉可访问的 HTTPS 地址，用于短期群图片访问
- 当前组织扩展 `dingtalk_robot_code`：群机器人编码；未配置时使用该组织 AppKey

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
- 群聊推送需先在群内 @机器人发送“绑定作息表”，并检查绑定人本地映射与 `week_schedule_group_push`/`attendance_manage`
- 群聊图片要求当前组织配置可公网访问的 HTTPS `AppHomeURL`；临时地址过期后返回 404 属正常安全行为
- “已提交”仅代表钉钉接口受理，不代表最终送达

### 法定节假日不生效
- 检查 `StatutoryHoliday` 表是否有数据
- 检查日期格式是否正确（YYYY-MM-DD）
- 检查 `type` 是否正确（holiday/workday）

### 从聚合数据同步失败
- 检查 `JUHE_API_KEY` 是否正确
- 检查网络连接
- 聚合数据 API 有调用限制
