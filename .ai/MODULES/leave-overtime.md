---
purpose: 年假与调休模块业务规则说明
last_updated: 2026-08-10
source_of_truth:
  - internal/api/router.go（接口注册）
  - internal/api/leave_handlers.go（年假调休相关 handler）
  - internal/api/supplementary_handlers.go（补卡申请 handler）
  - internal/service/annual_leave_grant_service.go（年假发放服务）
  - internal/service/compensatory_leave_service.go（调休服务）
  - internal/service/overtime_matching_service.go（加班匹配服务）
  - internal/service/approval_reconciliation_service.go（审批同步后的业务对账）
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

#### AnnualLeaveConsumeRequest / AnnualLeaveConsumeLog

`AnnualLeaveConsumeRequest` 是审批请求级门闩，唯一键为 `org_id + request_ref`，记录 `applied/reversed` 和当前操作轮次。`AnnualLeaveConsumeLog` 是可审计子台账，同一请求可跨多个 grant；`entry_type=consume/reversal` 分别记录正向消费和冲正，冲正不删除历史。

```go
type AnnualLeaveConsumeLog struct {
    ID          uint
    UserID      string
    GrantID     uint    // 对应的发放记录
    ApprovalRef string  // 审批 ID，重试时用于幂等
    RequestRef  string  // 请求唯一标识（幂等键）
    OperationNo int     // 撤销后恢复时递增
    EntryType   string  // consume / reversal
    BusinessStartDate string
    BusinessEndDate string
    ReversalOfID uint
    Days        float64
    Remark      string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

子台账唯一索引：`org_id + request_ref + grant_id`；请求级唯一索引：`org_id + request_ref`（`AnnualLeaveConsumeRequest`）。

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

1. **按业务日期消费**（`ConsumeAnnualLeaveForPeriod`）
   - 审批自动消费必须解析表单的开始日期、结束日期和天数，使用 `ApprovalBusinessLocation()`；解析失败 fail-closed
   - 只查询业务日期同年度、且季度不晚于该年度日期段结束季度的 grant；同一日期段内按 `year/quarter/id` 既有顺序消费
   - 跨年按各年度覆盖的自然日数量占比分摊总天数，分别写入年度消费明细
   - `AnnualLeaveConsumeRequest` 门闩、grant `FOR UPDATE`、余额和子日志处于同一事务；审批自动消费使用稳定 `approval:<process_id>`
   - 撤销追加 `reversal` 日志并返还原 grant；恢复审批递增操作轮次并重新消费一次

### 加班匹配流程

1. **同步审批**（`SyncApproval`）
   - 从钉钉同步加班审批
   - 审批同步完成后直接调用业务对账服务；历史审批不等待只处理“昨天”的定时任务
   - 仅 `COMPLETED + agree` 生效，实际工作日期从审批表单的加班开始时间提取
   - 写入 `approvals` 表

2. **运行匹配**（`RunOvertimeMatch`）
   - 读取加班审批
   - 读取打卡记录
   - 计算有效加班时长（审批时间 ∩ 打卡时间）
   - 写入 `OvertimeMatchResult`
   - `no_clock_record`、`insufficient_clock_record`、`query_clock_failed`、`unmatched`、`invalid_clock_time` 为可重试状态；后续审批同步或考勤补齐时原位更新同一记录
   - 钉钉考勤 `AttendanceService.SyncRecords` 与 Doris `ExternalAttendanceSyncService` 都在考勤成功写入后，按各自绑定的 `org_id` 收集并去重本批 `user_id + work_date`，只查询上述 retryable 状态；00:00 至 06:00（含）的打卡同时发布打卡日与前一工作日，以覆盖跨午夜加班窗口；单批员工日期查询按 200 分块
   - 考勤写入失败时不启动重算；重算失败不回滚考勤，只记录脱敏安全日志。每日补偿任务逐组织扫描可配置 `OVERTIME_RETRY_LOOKBACK_DAYS`（默认 30、最大 180）范围，单轮最多 500 条；候选按 `updated_at + id` 从最久未尝试开始排序，并在处理前统一刷新尝试时间，避免固定头部记录长期阻塞后续队列
   - 重算成功后关闭 pending 补卡请求；已成功生成本地额度或钉钉同步的记录只补齐未完成阶段

3. **生成调休余额**
   - 读取 `OvertimeMatchResult`
   - 写入 `CompensatoryLeaveLedger`（credit）

4. **同步到钉钉**（`ResyncOvertimeToDingTalk`）
   - 调用钉钉假期余额接口
   - 写入 `OvertimeSyncHistory`（防重复同步）
   - 审批撤销时先执行本地来源净额 rollback；需要校准钉钉时先落 `rollback_pending`，外部失败/超时落 `rollback_failed` 和脱敏错误，只有年度绝对余额确认成功才落 `rollback_success/rolled_back`
   - `rollback_pending`、`rollback_failed` 或其他不确定状态重试，以及已有外部同步历史的撤销恢复，都使用年度绝对余额接口；禁止重复增量补偿。绝对同步失败只更新当前触发记录，不得污染同员工同年度其他成功记录；从未同步钉钉且同步开关关闭的记录恢复时只恢复本地额度并标记外部同步 `skipped`。`OvertimeSyncHistory.sync_mode` 使用 `rollback`、`retry`、`reactivation` 保留审计轨迹

---

## 幂等性设计

审批同步后的年假消费和加班匹配都必须显式绑定当前 `org_id`。单条对账失败只影响该条对账状态，审批原始数据保留并允许后续重复同步安全重试。

### 年假消费幂等
- `AnnualLeaveConsumeRequest` 的 `org_id + request_ref` 唯一键是请求级门闩；不能用单条消费日志的 `approval_ref` 唯一代替，因为一次审批可能拆分多个 grant
- 门闩插入/锁定与 grant 扣减、正向/冲正日志同事务；事务失败不遗留 running/pending，可安全重试
- MySQL `REPEATABLE READ` 下使用唯一键竞争 + `SELECT ... FOR UPDATE` 锁定门闩，禁止“先查日志、后锁 grant”

### 加班同步幂等
- `OvertimeSyncHistory` 记录已同步的加班记录
- 同步前检查是否已存在

### 加班匹配幂等
- `OvertimeMatchResult.match_ref` 用于当前匹配幂等
- 历史数据仍兼容 `user_id + work_date` 口径
- 调休 credit/rollback 按来源净额在锁定匹配记录的事务内判断；重复撤销不重复扣减，撤销后恢复可重新 credit 一次
- 本地 rollback、外部绝对余额调用、状态更新分阶段执行；外部调用不得放进长数据库事务，失败后依靠状态机安全重试

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
