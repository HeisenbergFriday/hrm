# 绩效表单线上化技术方案

## 现状分析

现有绩效模块已有完整的基础框架：
- Activity 生命周期：`draft → self_evaluation → manager_evaluation → result_confirmed → archived`
- Participant 状态：`pending → self_submitted → manager_submitted → result_confirmed`
- 模板系统：`PerformanceTemplate` → `Section`（维度）→ `Item`（评分项）
- 钉钉通知：`SendCorpMessageToUser` 已有 6 个通知点
- 强制分布：已有 `PerformanceDistributionRule` 和 `GetDistributionCheck` 接口

**差距**：缺少目标设定环节、三级确认链、实时配额提示、HR 收支规则

---

## 一、数据库变更

### 1.1 新表：`performance_goal_records`（目标/指标记录）

存储每个员工在目标设定阶段填写的指标明细，对应 Excel 模板中"量化指标"和"关键行动"两部分。

```go
type PerformanceGoalRecord struct {
    ID             uint    `gorm:"primaryKey" json:"id"`
    ActivityID     string  `gorm:"type:varchar(64);not null;index" json:"activity_id"`
    ParticipantID  uint    `gorm:"not null;index" json:"participant_id"`
    SectionType    string  `gorm:"type:varchar(32);not null" json:"section_type"` // quantitative（量化指标）, key_action（关键行动）
    ItemName       string  `gorm:"type:varchar(256);not null" json:"item_name"`  // 指标名称
    ItemDefinition string  `gorm:"type:text" json:"item_definition"`             // 指标定义及口径说明
    Weight         float64 `gorm:"default:0" json:"weight"`                      // 权重（小数，如 0.3）
    RedLineValue   string  `gorm:"type:varchar(256)" json:"red_line_value"`      // 红线值
    TargetValue    string  `gorm:"type:varchar(256)" json:"target_value"`        // 目标值
    ChallengeValue string  `gorm:"type:varchar(256)" json:"challenge_value"`     // 挑战值
    ScoringRule    string  `gorm:"type:text" json:"scoring_rule"`                // 考核标准
    ActualResult   string  `gorm:"type:text" json:"actual_result"`               // 实际达成结果（员工填）
    SelfScore      float64 `gorm:"default:0" json:"self_score"`                  // 自评得分
    ManagerScore   float64 `gorm:"default:0" json:"manager_score"`              // 上级评分
    SortOrder      int     `gorm:"default:0" json:"sort_order"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
    DeletedAt      *time.Time `gorm:"index" json:"-"`
}
```

**关键点**：
- `SectionType` 区分"量化指标"（权重合计 70%）和"关键行动"（权重合计 30%）
- 同一 `SectionType` 下所有 `Weight` 合计需等于对应 section 权重
- `ActualResult`、`SelfScore` 在员工自评阶段填写
- `ManagerScore` 在上级评分阶段填写
- 每个员工的总分 = Σ(item.SelfScore × item.Weight) 或 Σ(item.ManagerScore × item.Weight)

### 1.2 新表：`performance_company_finance`（HR 收支状态）

```go
type PerformanceCompanyFinance struct {
    ID           uint      `gorm:"primaryKey" json:"id"`
    ActivityID   string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"activity_id"`
    RevenueSign  string    `gorm:"type:varchar(32);not null" json:"revenue_sign"` // revenue_gt_expense, expense_gt_revenue
    SetBy        string    `gorm:"type:varchar(64);not null" json:"set_by"`       // HR 操作人
    SetAt        time.Time `json:"set_at"`
    Remark       string    `gorm:"type:text" json:"remark"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### 1.3 修改表：`performance_activities`

新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `TargetSetStartAt` | varchar(32) | 目标设定开始日期 |
| `TargetSetEndAt` | varchar(32) | 目标设定结束日期 |
| `EmployeeConfirmStartAt` | varchar(32) | 员工确认结果开始日期 |
| `EmployeeConfirmEndAt` | varchar(32) | 员工确认结果结束日期 |
| `ManagerConfirmStartAt` | varchar(32) | 上级确认结果开始日期 |
| `ManagerConfirmEndAt` | varchar(32) | 上级确认结果结束日期 |
| `HRConfirmStartAt` | varchar(32) | 人力确认开始日期 |
| `HRConfirmEndAt` | varchar(32) | 人力确认结束日期 |

### 1.4 修改表：`performance_participants`

新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `SelfEvaluationComment` | text | 员工自我评价（做得好+需改进） |
| `ManagerEvaluationComment` | text | 上级总体评价（做得好+需改进） |
| `EmployeeConfirmedAt` | datetime | 员工确认结果时间 |
| `EmployeeConfirmedBy` | varchar(64) | 员工确认操作人 |
| `ManagerConfirmedAt` | datetime | 上级确认结果时间 |
| `ManagerConfirmedBy` | varchar(64) | 上级确认操作人 |
| `HRConfirmedAt` | datetime | 人力确认结果时间 |
| `HRConfirmedBy` | varchar(64) | 人力确认操作人 |
| `IsLocked` | bool | 是否已锁定 |
| `LockedAt` | datetime | 锁定时间 |
| `LockedBy` | varchar(64) | 锁定操作人 |
| `TotalSelfScore` | float64 | 系统计算的自评总分 |
| `TotalManagerScore` | float64 | 系统计算的上级评分总分 |
| `RevenueCoefficient` | float64 | 收支系数（部门负责人用，1.2 或 0.8，默认 1.0） |

### 1.5 修改表：`performance_review_versions`

扩展 `ReviewType` 枚举，新增：

| ReviewType | 说明 |
|------------|------|
| `target_set` | 目标设定（上级+下级协作） |
| `employee_confirm` | 员工确认结果 |
| `manager_confirm` | 上级确认结果 |
| `hr_confirm` | 人力确认结果 |

---

## 二、状态机变更

### 2.1 Activity 状态流

```
旧：draft → self_evaluation → manager_evaluation → result_confirmed → archived

新：draft
  → target_setting        （上级设定目标方向/指标）
  → self_evaluation        （员工填写自评+实际达成）
  → manager_evaluation     （上级评分+选等级，含实时配额校验）
  → employee_confirmation  （员工确认结果）
  → manager_confirmation   （上级确认结果）
  → hr_confirmation        （人力确认，确认后锁定）
  → locked                 （锁定状态）
  → archived               （归档）
```

### 2.2 Participant 状态流

```
旧：pending → self_submitted → manager_submitted → result_confirmed

新：pending
  → target_set              （目标已设定）
  → self_submitted          （自评已提交）
  → manager_submitted       （评分已提交）
  → employee_confirmed      （员工已确认结果）
  → manager_confirmed       （上级已确认结果）
  → hr_confirmed            （人力已确认 → 自动锁定）
  → locked                  （锁定）
```

### 2.3 状态推进规则

| 转换 | 触发人 | 前置条件 | 钉钉通知 |
|------|--------|----------|----------|
| draft → target_setting | HR/管理员 | 活动已创建 | 通知所有参与者的直属上级 |
| target_setting → self_evaluation | HR/管理员 | 目标已设定完成 | 通知所有员工 |
| self_evaluation → manager_evaluation | HR/管理员 | — | 通知所有上级 |
| manager_evaluation → employee_confirmation | HR/管理员 | 所有参与者已评分且分布合规 | 通知所有员工 |
| employee_confirmation → manager_confirmation | HR/管理员 | 所有员工已确认 | 通知所有上级 |
| manager_confirmation → hr_confirmation | HR/管理员 | 所有上级已确认 | 通知 HR |
| hr_confirmation → locked | HR | 所有员工已确认 | 通知所有参与者（结果已锁定） |

**注意**：Activity 级别的状态推进是全局操作（HR 点击推进），但前提是所有参与者都已完成当前阶段。

---

## 三、API 接口变更

### 3.1 目标设定

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/performance/goal-records/batch` | 批量创建/更新目标指标（上级或员工提交） |
| GET | `/performance/goal-records/:participant_id` | 获取某员工的目标指标列表 |
| PUT | `/performance/goal-records/:id` | 更新单条指标 |

### 3.2 自评

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/performance/goal-records/:participant_id/self-eval` | 提交自评（actual_result + self_score + 自我评价） |

### 3.3 上级评分

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/performance/goal-records/:participant_id/manager-eval` | 提交评分（manager_score + 上级评价 + 等级） |
| GET | `/performance/distribution-check-realtime/:activity_id` | 实时配额检查（评分过程中随时调用） |

### 3.4 确认链

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/performance/participants/:id/confirm-employee` | 员工确认结果 |
| POST | `/performance/participants/:id/confirm-manager` | 上级确认结果 |
| POST | `/performance/participants/:id/confirm-hr` | 人力确认结果（自动锁定） |
| POST | `/performance/participants/batch-confirm` | 批量确认 |

### 3.5 HR 收支规则

| 方法 | 路径 | 说明 |
|------|------|------|
| PUT | `/performance/activities/:id/finance` | HR 设置收支状态 |
| GET | `/performance/activities/:id/finance` | 获取收支状态 |

### 3.6 现有接口修改

| 现有接口 | 修改内容 |
|----------|----------|
| `CreatePerformanceActivity` | 新增目标设定日期字段 |
| `GetPerformanceDistributionCheck` | 返回值增加实时配额进度详情 |
| `SubmitReviewSelfEvaluation` | 改为读写 `performance_goal_records` 而非仅写 review_version |
| `SubmitReviewManagerEvaluation` | 改为读写 `performance_goal_records`，评分时自动算总分 |
| `ConfirmResult` | 拆分为三个独立的确认接口 |

---

## 四、前端页面拆分

现有 `PerformanceOverview.tsx` 是 966 行的单文件，建议**拆分为独立页面**而非继续在 Drawer 中堆表单。

### 4.1 新增页面

| 页面 | 路由 | 说明 |
|------|------|------|
| `PerformanceGoalSetting.tsx` | `/performance/:activityId/goal-setting/:participantId` | 目标设定表单 |
| `PerformanceSelfEval.tsx` | `/performance/:activityId/self-eval/:participantId` | 员工自评表单 |
| `PerformanceManagerEval.tsx` | `/performance/:activityId/manager-eval/:participantId` | 上级评分表单（含实时配额面板） |
| `PerformanceResultView.tsx` | `/performance/:activityId/result/:participantId` | 结果查看+确认页面 |

### 4.2 表单结构（按 Excel 模板）

#### 目标设定表单
```
┌─────────────────────────────────────────┐
│ 基础信息（自动填充）                      │
│ 姓名: XXX  部门: xxx部  职级: 专员        │
│ 直属上级: XX  考核周期: 2025年1月          │
├─────────────────────────────────────────┤
│ 量化指标（2-5项，权重合计 70%）            │
│ ┌─指标1────────────────────────────────┐ │
│ │ 指标名称: [门店实收达成率________]     │ │
│ │ 指标定义: [______________________]    │ │
│ │ 权重:     [30]%                       │ │
│ │ 红线值:   [17000] 目标值:[18000] 挑战值:[20000] │
│ │ 考核标准: [达成值≥挑战值，得分120分...] │ │
│ └──────────────────────────────────────┘ │
│ [+ 添加指标]  权重合计: 70% ✅            │
├─────────────────────────────────────────┤
│ 关键行动（3-5项，权重合计 30%）            │
│ ┌─行动1────────────────────────────────┐ │
│ │ 指标名称: [XX标准化落地_________]     │ │
│ │ 指标定义: [______________________]    │ │
│ │ 权重:     [10]%                       │ │
│ │ 考核标准: [1.少完成一个扣除...]        │ │
│ └──────────────────────────────────────┘ │
│ [+ 添加行动]  权重合计: 30% ✅            │
├─────────────────────────────────────────┤
│ 总权重: 100% ✅                          │
│ [保存草稿]  [提交目标]                    │
└─────────────────────────────────────────┘
```

#### 员工自评表单
```
┌─────────────────────────────────────────┐
│ 量化指标（只读目标内容）                   │
│ ┌─指标1: 门店实收达成率 ────────────────┐ │
│ │ 实际达成结果: [11月门店实收达成19000]  │ │
│ │ 自评得分:     [30] / 权重 0.3          │ │
│ └──────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│ 关键行动                                  │
│ ┌─行动1: XX标准化落地 ─────────────────┐ │ │
│ │ 实际达成结果: [制度未完成_________]    │ │
│ │ 自评得分:     [0] / 权重 0.1           │ │
│ └──────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│ 系统自动计算总分: 76                      │
├─────────────────────────────────────────┤
│ 员工自我评价                               │
│ 做得好的地方: [____________________]      │
│ 需要改进的地方: [____________________]    │
│ [保存草稿]  [提交自评]                    │
└─────────────────────────────────────────┘
```

#### 上级评分表单（含实时配额面板）
```
┌──────────────────────┬──────────────────┐
│ 评分区域              │ 配额进度          │
│                      │                  │
│ 指标1: 门店实收达成率 │ S  ▓░░░░ 0/0     │
│ 实际达成: 19000      │ A  ▓▓░░░ 2/2     │
│ 自评: 30             │ B  ▓▓▓░░ 3/3     │
│ 上级评分: [30____]   │ C/D ▓░░░░ 1/1    │
│                      │                  │
│ 指标2: ...           │ 当前可选: B       │
│                      │                  │
├──────────────────────┤                  │
│ 总分: 76             │                  │
│ 绩效等级: [B____▼]   │ [提交评分]       │
│ 上级评价:            │                  │
│ 做得好的: [______]   │                  │
│ 需改进的: [______]   │                  │
└──────────────────────┴──────────────────┘
```

### 4.3 现有页面修改

`PerformanceOverview.tsx` 需要修改：
- Activity 详情抽屉中增加新状态的操作按钮
- 参与者列表增加"目标设定"、"三级确认"状态列
- 增加 HR 收支设置入口
- 点击员工行时根据当前阶段跳转到对应表单页面

---

## 五、钉钉通知集成

### 5.1 通知点

| 阶段 | 触发时机 | 接收人 | 通知内容 |
|------|----------|--------|----------|
| 目标设定 | Activity 进入 target_setting | 全部上级 | "请为您的下属设定2025年1月绩效目标" |
| 员工自评 | Activity 进入 self_evaluation | 全部员工 | "请填写2025年1月绩效自评" |
| 上级评分 | Activity 进入 manager_evaluation | 全部上级 | "请为您的下属完成绩效评分" |
| 员工确认 | Activity 进入 employee_confirmation | 全部员工 | "您的绩效结果已出，请确认" |
| 上级确认 | Activity 进入 manager_confirmation | 全部上级 | "请确认您下属的绩效结果" |
| 人力确认 | Activity 进入 hr_confirmation | HR | "请确认本月绩效结果" |
| 结果锁定 | Activity 进入 locked | 全部员工 | "您的2025年1月绩效结果已锁定" |
| 催办 | 手动触发 | 未完成的员工/上级 | "您有未完成的绩效任务" |

### 5.2 实现方式

复用现有 `SendCorpMessageToUser()`，消息体包含跳转链接：

```
链接格式：https://{域名}/performance/{activityId}/{操作类型}/{participantId}
```

员工点击通知 → 跳转到系统 Web 页面 → 直接打开对应表单。

---

## 六、强制分布实时提示方案

### 6.1 后端

新增接口 `GET /performance/distribution-check-realtime/:activity_id`：
- 查询当前 activity 所有 participant 的 `final_level` 和 `suggested_level`
- 按团队分组（按 `manager_id` 分组 = 一个团队）
- 逐团队计算 S/A/B/C/D 的当前数量和配额
- 返回结构：

```json
{
  "teams": [
    {
      "manager_id": "xxx",
      "manager_name": "张三",
      "total": 8,
      "levels": {
        "S": {"current": 0, "max": 1, "percent": 10},
        "A": {"current": 1, "max": 2, "percent": 20},
        "B": {"current": 4, "max": 5, "percent": 60},
        "CD": {"current": 1, "max": 1, "percent": 10}
      }
    }
  ]
}
```

### 6.2 前端

- 上级评分页面右侧显示配额进度面板
- 选择等级时，如果该等级已满（current >= max），弹窗拦截提示
- 每次选择等级后自动调用实时检查接口刷新进度

---

## 七、实现优先级

| 阶段 | 内容 | 依赖 |
|------|------|------|
| **P0** | 数据库迁移（新表+新字段） | 无 |
| **P1** | 目标设定（GoalRecord CRUD + 表单页面） | P0 |
| **P1** | 自评表单（读写 GoalRecord + 评价） | P0 |
| **P1** | 上级评分（含实时配额） | P0 |
| **P2** | 三级确认链（员工/上级/HR） | P1 |
| **P2** | 钉钉通知扩展 | P1 |
| **P2** | HR 收支规则 | P0 |
| **P3** | 锁定 + 归档 | P2 |
| **P3** | 催办功能 | P2 |

---

## 八、影响评估

### 需要修改的文件

| 文件 | 修改类型 |
|------|----------|
| `internal/database/performance_models.go` | 新增 2 个 model，修改 2 个 model |
| `internal/database/database.go` | AutoMigrate 加入新表 |
| `internal/repository/performance_repository.go` | 新增 GoalRecord 和 Finance 的 repository |
| `internal/service/performance_service.go` | 新增目标设定、自评、评分、确认链逻辑 |
| `internal/api/performance_handlers.go` | 新增 ~12 个 handler |
| `internal/api/router.go` | 注册新路由 |
| `frontend/src/pages/PerformanceGoalSetting.tsx` | 新文件 |
| `frontend/src/pages/PerformanceSelfEval.tsx` | 新文件 |
| `frontend/src/pages/PerformanceManagerEval.tsx` | 新文件 |
| `frontend/src/pages/PerformanceResultView.tsx` | 新文件 |
| `frontend/src/pages/PerformanceOverview.tsx` | 修改（新状态、新按钮、新列） |
| `frontend/src/services/api.ts` | 新增 API 方法和类型定义 |
| `frontend/src/App.tsx` | 注册新路由 |

### 验证方式

1. 数据库迁移后检查表结构
2. 创建 Activity → 设定目标 → 员工自评 → 上级评分 → 三级确认全链路测试
3. 强制分布测试：同一团队分配超限等级，验证拦截
4. 钉钉通知验证：每个阶段推进后检查通知是否送达
5. 锁定后验证修改被拒绝
