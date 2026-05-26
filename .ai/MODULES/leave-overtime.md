---
purpose: 年假与调休模块业务规则说明
last_updated: 2026-05-26
source_of_truth:
  - internal/api/router.go（接口注册）
  - internal/api/leave_handlers.go（年假调休相关 handler）
  - internal/api/supplementary_handlers.go（补卡申请 handler）
  - internal/service/annual_leave_grant_service.go（年假发放服务）
  - internal/service/compensatory_leave_service.go（调休服务）
  - internal/service/overtime_matching_service.go（加班匹配服务）
  - internal/database/models.go（年假调休相关模型）
  - frontend/src/pages/LeaveOvertime.tsx（年假调休页面）
update_when:
  - 修改年假资格计算规则时
  - 修改年假发放逻辑时
  - 修改加班匹配规则时
  - 修改调休余额计算逻辑时
  - 修改钉钉同步逻辑时
---

# 年假与调休模块

## 模块定位

管理员工年假资格计算、季度发放、消费台账、加班匹配、调休余额，并同步到钉钉假期配置。

---

## 核心概念

### 年假
- **资格计算**：根据入职时间、司龄计算员工每季度应得年假天数
- **季度发放**：每季度初自动发放年假到员工账户
- **消费台账**：记录年假使用情况，FIFO 扣减

### 调休
- **加班匹配**：将钉钉加班审批与本地打卡记录匹配，计算有效加班时长
- **调休余额**：根据有效加班时长生成调休余额
- **同步到钉钉**：将调休余额同步到钉钉假期配置

---

## 数据模型

### 年假相关

#### LeaveRuleConfig
年假规则配置（rule_type: eligibility/grant）

```go
type LeaveRuleConfig struct {
    ID            uint
    RuleType      string  // eligibility / grant
    RuleKey       string  // 规则唯一键
    RuleName      string
    RuleValueJSON string  // 规则内容（JSON 字符串）
    Status        string  // active / inactive
    EffectiveFrom string
    EffectiveTo   string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

#### AnnualLeaveEligibility
年假资格（按 user_id+year+quarter 唯一）

```go
type AnnualLeaveEligibility struct {
    ID                       uint
    UserID                   string
    Year                     int
    Quarter                  int
    EntryDate                string
    ConfirmationDate         string
    IsEligible               bool
    EligibleStartDate        string
    EligibleEndDate          string
    RetroactiveSourceQuarter int
    CalcVersion              string
    CalcReason               string
    CreatedAt                time.Time
    UpdatedAt                time.Time
}
```

唯一索引：`user_id + year + quarter`

#### AnnualLeaveGrant
年假发放台账（含钉钉同步状态）

```go
type AnnualLeaveGrant struct {
    ID                 uint
    UserID             string
    Year               int
    Quarter            int
    WorkingYears       float64
    BaseDays           float64
    GrantedDays        float64
    RetroactiveDays    float64
    UsedDays           float64
    RemainingDays      float64
    GrantType          string  // normal / retroactive / adjustment
    SourceEligibilityID uint
    Remark             string
    DingTalkSyncStatus string  // pending / success / failed / skipped
    DingTalkSyncError  string
    DingTalkSyncedAt   *time.Time
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

#### AnnualLeaveConsumeLog
年假消费台账（FIFO 扣减，幂等 via request_ref）

```go
type AnnualLeaveConsumeLog struct {
    ID          uint
    UserID      string
    GrantID     uint    // 对应的发放记录
    ApprovalRef string  // 审批 ID，重试时用于幂等
    RequestRef  string  // 请求唯一标识（幂等键）
    Days        float64
    Remark      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

唯一索引：`request_ref + grant_id`

---

### 加班与调休相关

#### OvertimeRuleConfig
加班规则配置

```go
type OvertimeRuleConfig struct {
    ID            uint
    RuleKey       string
    RuleName      string
    RuleValueJSON string  // JSON 字符串
    Status        string  // active / inactive
    EffectiveFrom string
    EffectiveTo   string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

#### OvertimeMatchResult
加班审批↔打卡匹配结果（当前幂等键为 `match_ref`，历史数据仍兼容 `user_id+work_date` 口径）

```go
type OvertimeMatchResult struct {
    ID                       uint
    UserID                   string
    UserName                 string
    WorkDate                 string  // YYYY-MM-DD
    MatchRef                 string
    ApprovalID               uint
    ApprovalProcessID        string
    ApprovalStatus           string
    ApprovalStartTime        time.Time
    ApprovalEndTime          time.Time
    ApprovalDurationMinutes  int
    OvertimeStartTime        time.Time
    OvertimeEndTime          time.Time
    OvertimeDurationMinutes  int
    ActualFirstClockTime     *time.Time
    ActualLastClockTime      *time.Time
    ActualClockSpanMinutes   int
    BreakDeductMinutes       int
    EffectiveOvertimeMinutes int
    MatchStatus              string  // matched / no_clock_record / insufficient_clock_record / synced ...
    MatchReason              string
    LocalBalanceStatus       string
    DingtalkSyncStatus       string
    DingtalkSyncRequestID    string
    DingtalkSyncError        string
    CalcVersion              string
    CreatedAt                time.Time
    UpdatedAt                time.Time
}
```

`match_ref` 用于当前匹配幂等；数据库仍保留 `user_id + work_date` 历史唯一索引兼容。

#### OvertimeSyncHistory
已同步钉钉的加班记录快照

```go
type OvertimeSyncHistory struct {
    ID                       uint
    UserID                   string
    WorkDate                 string
    ApprovalID               uint
    ApprovalProcessID        string
    EffectiveOvertimeMinutes int
    SyncRequestID            string
    SyncMode                 string
    SyncedAt                 *time.Time
    CreatedAt                time.Time
    UpdatedAt                time.Time
}
```

#### OvertimeSupplementaryRequest
加班补卡申请

```go
type OvertimeSupplementaryRequest struct {
    ID                    uint
    MatchResultID         uint
    UserID                string
    WorkDate              string
    ApprovalID            uint
    SupplementaryClockIn  time.Time
    SupplementaryClockOut time.Time
    SupplementaryReason   string
    Status                string  // pending / approved / rejected
    DingtalkProcessID     string
    ApprovedBy            string
    ApprovedAt            *time.Time
    RejectedReason        string
    CreatedAt             time.Time
    UpdatedAt             time.Time
}
```

#### CompensatoryLeaveLedger
调休余额台账（credit/debit/rollback/adjustment）

```go
type CompensatoryLeaveLedger struct {
    ID             uint
    UserID         string
    SourceType     string  // overtime
    SourceMatchID  uint    // 对应的 OvertimeMatchResult ID
    SourceMatchRef string  // 匹配记录引用
    CreditMinutes  int     // 增加分钟数
    DebitMinutes   int     // 减少分钟数
    BalanceMinutes int     // 余额分钟数
    LedgerType     string  // credit / debit / rollback / adjustment
    EffectiveDate  string  // YYYY-MM-DD
    ExpireDate     string  // YYYY-MM-DD
    Remark         string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

---

## API 接口

### 年假

#### GET /api/v1/leave/eligibility
查询年假资格

Query 参数：
- `user_id`：用户 ID
- `year`：年份

#### POST /api/v1/leave/eligibility/recalculate
重新计算年假资格

Body：
```json
{
    "user_id": "xxx",
    "year": 2024
}
```

#### GET /api/v1/leave/grants
查询年假发放记录

Query 参数：
- `user_id`：用户 ID
- `year`：年份
- `quarter`：季度

#### POST /api/v1/leave/grants/run-quarter
运行季度发放

Body：
```json
{
    "year": 2024,
    "quarter": 1
}
```

#### POST /api/v1/leave/grants/regrant
补发年假

Body：
```json
{
    "user_id": "xxx",
    "year": 2024,
    "quarter": 1,
    "days": 2.5,
    "remark": "补发原因"
}
```

#### POST /api/v1/leave/grants/sync-to-dingtalk
同步年假到钉钉

Body：
```json
{
    "confirm": true
}
```

#### GET /api/v1/leave/vacation-types
查询钉钉假期类型列表

#### POST /api/v1/leave/consume
消费年假

Body：
```json
{
    "user_id": "xxx",
    "days": 1.0,
    "approval_ref": "xxx",
    "remark": "请假"
}
```

#### GET /api/v1/leave/consume-log
查询年假消费台账

Query 参数：
- `user_id`：用户 ID
- `year`：年份

---

### 加班与调休

#### GET /api/v1/overtime/matches
查询加班匹配记录

Query 参数：
- `user_id`：用户 ID
- `start_date`：开始日期
- `end_date`：结束日期

#### POST /api/v1/overtime/matches/run
运行加班匹配

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### POST /api/v1/overtime/matches/force
强制匹配指定记录

Body：
```json
{
    "approval_id": 123
}
```

#### POST /api/v1/overtime/matches/clear-rematch
清空并重新匹配

Body：
```json
{
    "user_id": "xxx",
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### POST /api/v1/overtime/matches/delete
删除匹配记录

Body：
```json
{
    "user_id": "xxx",
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### POST /api/v1/overtime/sync-and-match
同步审批并匹配

Body：
```json
{
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### POST /api/v1/overtime/reset-manual-leave
重置钉钉 ManualLeave 余额，并将本地有效加班同步状态重置为 `pending`

Body：
```json
{
    "dry_run": true
}
```

#### POST /api/v1/overtime/resync-overtime
重新同步加班到钉钉

Body：
```json
{
    "dry_run": false,
    "user_id": "xxx",
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
}
```

#### POST /api/v1/overtime/supplementary/submit
提交补卡申请

Body：
```json
{
    "match_result_id": 1,
    "clock_in": "2024-01-15 18:30",
    "clock_out": "2024-01-15 21:00",
    "reason": "补充加班打卡"
}
```

#### POST /api/v1/overtime/supplementary/approve
审批补卡申请

Body：
```json
{
    "request_id": 1,
    "approved": true,
    "rejected_reason": ""
}
```

#### GET /api/v1/overtime/supplementary/list
查询补卡申请

Query 参数：
- `user_id`：用户 ID（可选）
- `start_date`：开始日期（可选）
- `end_date`：结束日期（可选）

#### POST /api/v1/overtime/supplementary/sync-dingtalk
从钉钉同步补卡审批

当前 handler 返回 `501 Not Implemented`，需要补充补卡审批 `process_code` 后再实现。

#### GET /api/v1/comp-time/balance
查询调休余额

Query 参数：
- `user_id`：用户 ID

#### POST /api/v1/comp-time/manual-grant
手动发放调休

Body：
```json
{
    "user_id": "xxx",
    "minutes": 120,
    "remark": "手动发放原因"
}
```

---

## 核心业务流程

### 年假发放流程

1. **计算资格**（`RecalculateLeaveEligibility`）
   - 根据入职时间、司龄计算应得天数
   - 写入 `AnnualLeaveEligibility`

2. **季度发放**（`RunQuarterGrant`）
   - 读取 `AnnualLeaveEligibility`
   - 写入 `AnnualLeaveGrant`
   - 标记 `dingtalk_sync_status = pending`

3. **同步到钉钉**（`SyncGrantsToDingTalk`）
   - 调用钉钉假期配额接口
   - 更新 `dingtalk_sync_status = success / failed / skipped`
   - 记录 `dingtalk_synced_at` 或 `dingtalk_sync_error`

### 年假消费流程

1. **FIFO 扣减**（`ConsumeAnnualLeave`）
   - 按 `effective_date` 升序查询 `AnnualLeaveGrant`
   - 依次扣减，直到扣完
   - 写入 `AnnualLeaveConsumeLog`（幂等 via `request_ref`）

### 加班匹配流程

1. **同步审批**（`SyncApproval`）
   - 从钉钉同步加班审批
   - 写入 `approvals` 表

2. **运行匹配**（`RunOvertimeMatch`）
   - 读取加班审批
   - 读取打卡记录
   - 计算有效加班时长（审批时间 ∩ 打卡时间）
   - 写入 `OvertimeMatchResult`

3. **生成调休余额**
   - 读取 `OvertimeMatchResult`
   - 写入 `CompensatoryLeaveLedger`（credit）

4. **同步到钉钉**（`ResyncOvertimeToDingTalk`）
   - 调用钉钉假期余额接口
   - 写入 `OvertimeSyncHistory`（防重复同步）

---

## 幂等性设计

### 年假消费幂等
- `AnnualLeaveConsumeLog.request_ref` 唯一索引
- 同一个 `request_ref` 只能消费一次

### 加班同步幂等
- `OvertimeSyncHistory` 记录已同步的加班记录
- 同步前检查是否已存在

### 加班匹配幂等
- `OvertimeMatchResult.match_ref` 用于当前匹配幂等
- 历史数据仍兼容 `user_id + work_date` 口径

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `AnnualLeaveGrantService` | `annual_leave_grant_service.go` | 年假发放 |
| `CompensatoryLeaveService` | `compensatory_leave_service.go` | 调休管理 |
| `OvertimeMatchingService` | `overtime_matching_service.go` | 加班匹配 |
| 钉钉假期相关函数 | `internal/dingtalk/dingtalk.go` | 钉钉假期类型、余额和配额同步 |

---

## 定时任务

定时任务在 `internal/service/leave_jobs.go`：

- **季度发放任务**：每季度初自动发放年假
- **加班匹配任务**：每天凌晨自动匹配前一天的加班记录

---

## 前端页面

主页面：`frontend/src/pages/LeaveOvertime.tsx`

功能：
- 年假资格查询
- 年假发放记录
- 年假消费台账
- 加班匹配记录
- 匹配记录删除、清空并重跑
- 补卡申请提交、审批与查询
- 调休余额查询
- 手动发放调休
- ManualLeave 重置
- 重新同步到钉钉

---

## 常见问题

### 年假发放后钉钉看不到
- 检查 `dingtalk_sync_status` 是否为 `success`
- 检查 `DINGTALK_LEAVE_SYNC_ENABLED` 环境变量
- 检查钉钉假期类型配置（`DINGTALK_ANNUAL_LEAVE_CODE`）

### 加班匹配不准确
- 检查打卡记录是否完整
- 检查加班审批时间是否正确
- 检查加班规则配置（`OvertimeRuleConfig`）

### 调休余额不对
- 检查 `CompensatoryLeaveLedger` 台账
- 检查是否有 rollback 或 adjustment 记录
- 重新运行加班匹配

### 重复同步到钉钉
- 检查 `OvertimeSyncHistory` 是否有记录
- 如果需要重新同步，先删除 `OvertimeSyncHistory` 记录
