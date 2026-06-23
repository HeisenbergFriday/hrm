---
purpose: 绩效管理模块业务规则说明
last_updated: 2026-06-22
source_of_truth:
  - internal/api/performance_handlers.go（绩效相关 handler）
  - internal/service/performance_service.go（绩效服务）
  - internal/service/performance_indicator_service.go（指标库服务）
  - internal/service/scoring_engine.go（自动评分引擎）
  - internal/repository/performance_repository.go（绩效数据访问）
  - internal/repository/performance_indicator_repository.go（指标库数据访问）
  - internal/repository/performance_goal_record_repository.go（目标记录数据访问）
  - internal/repository/performance_goal_approval_repository.go（目标审批日志数据访问）
  - internal/database/performance_models.go（绩效相关模型）
  - frontend/src/pages/PerformanceOverview.tsx（绩效总览页面）
update_when:
  - 修改绩效活动状态流时
  - 修改评分规则时
  - 修改强制分布逻辑时
  - 新增绩效相关 API 时
  - 修改前端绩效页面时
  - 新增自动评分引擎时
  - 新增 Review 版接口时
---

# 绩效管理模块

## 模块定位

管理绩效活动全生命周期：模板管理、指标库、活动创建、参与人管理、目标设定、自评、上级评分、强制分布、三级确认、结果锁定与归档。

---

## 核心概念

### 绩效活动（PerformanceActivity）
- 一次绩效考核周期的完整实例
- 包含活动名称、周期类型、时间范围、状态等
- 状态流：`draft → target_setting → self_evaluation → manager_evaluation → employee_confirmation → manager_confirmation → hr_confirmation → locked → archived`
- 主管确认参与人结果时立即冻结该参与人的绩效结果；HR 确认只作为后续确认/归档节点，不再作为冻结前置条件。

### 绩效模板（PerformanceTemplate）
- 定义评分维度和评分项的模板
- 简单结构：名称、描述、状态
- 评分维度通过 `PerformanceTemplateSection` 关联
- 评分项通过 `PerformanceTemplateItem` 关联

### 模板评分维度（PerformanceTemplateSection）
- 绩效模板的评分维度（如：业绩、能力、态度）
- 包含权重配置

### 模板评分项（PerformanceTemplateItem）
- 维度下的具体评分项
- 包含评分标准和说明

### 评审记录（PerformanceReview）
- 员工在评审阶段的评分记录
- 支持多版本（通过 `PerformanceReviewVersion` 追踪）

### 评审版本（PerformanceReviewVersion）
- 评审记录的版本链
- 记录每次修改的历史

### 强制分布规则（PerformanceDistributionRule）
- 定义各绩效等级的比例要求
- 支持按活动配置分布口径；业务口径为先按员工架构所属部门计算，部门人数/名额不足时再汇总到中心口径
- 管理员可提前统一录入各部门/中心名额或比例，也可配置小样本部门的特殊名额规则（例如 3 人部门 `S/A` 合计最多 1 人，其他等级不限）

### 强制分布例外（PerformanceDistributionException）
- 例外于强制分布规则的参与人记录

### 绩效等级规则（PerformanceLevelRule）
- 定义绩效等级的划分标准
- 关联 `PerformanceLevelRuleItem` 细分规则

### 关系变更日志（PerformanceRelationshipChangeLog）
- 记录参与人主管关系变更的历史

### 指标库（PerformanceIndicatorLibrary）
- 部门级指标库，支持继承
- 指标库按绩效流程模板隔离：小铁文娱流程模版与沐腾科技流程模版需要维护各自的指标库，创建活动时只能选择同一模板下的指标库
- 包含量化指标、关键行动、附加考核项
- 支持指标项的搜索和匹配

### 参与人（PerformanceParticipant）
- 某次绩效活动的参与者
- 包含员工信息、评分、等级、确认状态等
- 支持主管关系和收支系数
- 双向/多向汇报场景下，参与人需要支持多个评价人及工作占比权重；当前单 `manager_id` 模型仅覆盖主考核上级，完整多评价人模型待落地

### 目标记录（PerformanceGoalRecord）
- 员工在目标设定阶段填写的指标明细
- 包含量化指标和关键行动
- 支持审批流程

---

## 数据模型

### PerformanceActivity
绩效活动

```go
type PerformanceActivity struct {
    ID        uint   `gorm:"primaryKey" json:"id"`
    Name      string `gorm:"type:varchar(128);not null;index" json:"name"`
    CycleType string `gorm:"type:varchar(32);not null" json:"cycle_type"` // monthly, quarterly, annual
    StartDate string `gorm:"type:varchar(32);not null" json:"start_date"`
    EndDate   string `gorm:"type:varchar(32);not null" json:"end_date"`

    // 关联指标库
    IndicatorLibraryID *uint `gorm:"index" json:"indicator_library_id"`

    // 附加分控制
    EnableBonusScore bool `gorm:"default:false" json:"enable_bonus_score"` // 附加分是否计入总分并影响等级

    // 目标设定阶段
    TargetSetStartAt string `gorm:"type:varchar(32)" json:"target_set_start_at"`
    TargetSetEndAt   string `gorm:"type:varchar(32)" json:"target_set_end_at"`

    // 自评阶段
    SelfEvalStartAt string `gorm:"type:varchar(32);not null" json:"self_eval_start_at"`
    SelfEvalEndAt   string `gorm:"type:varchar(32);not null" json:"self_eval_end_at"`

    // 上级评分阶段
    ManagerEvalStartAt string `gorm:"type:varchar(32);not null" json:"manager_eval_start_at"`
    ManagerEvalEndAt   string `gorm:"type:varchar(32);not null" json:"manager_eval_end_at"`

    // 结果确认阶段
    ResultConfirmStartAt string `gorm:"type:varchar(32);not null" json:"result_confirm_start_at"`
    ResultConfirmEndAt   string `gorm:"type:varchar(32);not null" json:"result_confirm_end_at"`

    // 三级确认阶段
    EmployeeConfirmStartAt string `gorm:"type:varchar(32)" json:"employee_confirm_start_at"`
    EmployeeConfirmEndAt   string `gorm:"type:varchar(32)" json:"employee_confirm_end_at"`
    ManagerConfirmStartAt  string `gorm:"type:varchar(32)" json:"manager_confirm_start_at"`
    ManagerConfirmEndAt    string `gorm:"type:varchar(32)" json:"manager_confirm_end_at"`
    HRConfirmStartAt       string `gorm:"type:varchar(32)" json:"hr_confirm_start_at"`
    HRConfirmEndAt         string `gorm:"type:varchar(32)" json:"hr_confirm_end_at"`
    HRConfirmDeadline      string `gorm:"type:varchar(32)" json:"hr_confirm_deadline"`

    Status      string `gorm:"type:varchar(32);not null;index" json:"status"`
    Description string `gorm:"type:text" json:"description"`

    // 参与人范围筛选
    TargetDepartmentIDs []string `gorm:"type:json;serializer:json" json:"target_department_ids"`
    TargetEmployeeIDs   []string `gorm:"type:json;serializer:json" json:"target_employee_ids"`

    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
    DeletedAt *time.Time `gorm:"index" json:"-"`
    CreatedBy string     `gorm:"type:varchar(64)" json:"created_by"`
    UpdatedBy string     `gorm:"type:varchar(64)" json:"updated_by"`
}
```

### PerformanceParticipant
绩效参与人

```go
type PerformanceParticipant struct {
    ID             uint   `gorm:"primaryKey" json:"id"`
    ActivityID     string `gorm:"type:varchar(64);not null;index" json:"activity_id"`
    EmployeeID     string `gorm:"type:varchar(64);not null;index" json:"employee_id"`
    EmployeeName   string `gorm:"type:varchar(128);not null" json:"employee_name"`
    DepartmentID   string `gorm:"type:varchar(64);not null;index" json:"department_id"`
    DepartmentName string `gorm:"type:varchar(128)" json:"department_name"`
    Position       string `gorm:"type:varchar(128)" json:"position"`
    Level          string `gorm:"type:varchar(32)" json:"level"`
    EmployeeStatus string `gorm:"type:varchar(32)" json:"employee_status"`

    ManagerID   *string `gorm:"type:varchar(64)" json:"manager_id"`
    ManagerName *string `gorm:"type:varchar(128)" json:"manager_name"`

    Status string `gorm:"type:varchar(32);not null;index" json:"status"`

    // 评分相关
    SelfScore      float64 `gorm:"default:0" json:"self_score"`
    SelfLevel      string  `gorm:"type:varchar(32)" json:"self_level"`
    SelfSummary    string  `gorm:"type:text" json:"self_summary"`
    ManagerScore   float64 `gorm:"default:0" json:"manager_score"`
    ManagerComment string  `gorm:"type:text" json:"manager_comment"`
    SuggestedLevel string  `gorm:"type:varchar(32)" json:"suggested_level"`
    FinalLevel     string  `gorm:"type:varchar(32)" json:"final_level"`
    AdjustReason   string  `gorm:"type:text" json:"adjust_reason"`

    // 评价文本（拆分评价）
    SelfEvaluationComment        string `gorm:"type:text" json:"self_evaluation_comment"`
    SelfEvaluationGood           string `gorm:"type:text" json:"self_evaluation_good"`          // 自评亮点
    SelfEvaluationImprovement    string `gorm:"type:text" json:"self_evaluation_improvement"`   // 自评改进
    ManagerEvaluationComment     string `gorm:"type:text" json:"manager_evaluation_comment"`
    ManagerEvaluationGood        string `gorm:"type:text" json:"manager_evaluation_good"`       // 主管评亮点
    ManagerEvaluationImprovement string `gorm:"type:text" json:"manager_evaluation_improvement"` // 主管评改进

    // 系统计算总分
    TotalSelfScore    float64 `gorm:"default:0" json:"total_self_score"`
    TotalManagerScore float64 `gorm:"default:0" json:"total_manager_score"`

    // 附加项
    BonusScore    float64 `gorm:"default:0" json:"bonus_score"`
    PenaltyScore  float64 `gorm:"default:0" json:"penalty_score"`
    AdjustedScore float64 `gorm:"default:0" json:"adjusted_score"`

    // 收支系数
    RevenueCoefficient float64 `gorm:"default:1" json:"revenue_coefficient"`

    // 目标确认（三级）
    EmployeeTargetConfirmedAt *time.Time `json:"employee_target_confirmed_at"`
    EmployeeTargetConfirmedBy string     `gorm:"type:varchar(64)" json:"employee_target_confirmed_by"`
    ManagerTargetConfirmedAt  *time.Time `json:"manager_target_confirmed_at"`
    ManagerTargetConfirmedBy  string     `gorm:"type:varchar(64)" json:"manager_target_confirmed_by"`
    HRTargetConfirmedAt       *time.Time `json:"hr_target_confirmed_at"`
    HRTargetConfirmedBy       string     `gorm:"type:varchar(64)" json:"hr_target_confirmed_by"`

    // 结果确认（三级）
    EmployeeConfirmedAt *time.Time `json:"employee_confirmed_at"`
    EmployeeConfirmedBy string     `gorm:"type:varchar(64)" json:"employee_confirmed_by"`
    ManagerConfirmedAt  *time.Time `json:"manager_confirmed_at"`
    ManagerConfirmedBy  string     `gorm:"type:varchar(64)" json:"manager_confirmed_by"`
    HRConfirmedAt       *time.Time `json:"hr_confirmed_at"`
    HRConfirmedBy       string     `gorm:"type:varchar(64)" json:"hr_confirmed_by"`
    ConfirmedAt         *time.Time `json:"confirmed_at"`         // 兼容旧接口
    ConfirmedBy         string     `gorm:"type:varchar(64)" json:"confirmed_by"` // 兼容旧接口

    // 锁定
    IsLocked          bool       `gorm:"default:false" json:"is_locked"`
    LockedAt          *time.Time `json:"locked_at"`
    LockedBy          string     `gorm:"type:varchar(64)" json:"locked_by"`
    ForceLocked       bool       `gorm:"default:false" json:"force_locked"`
    ForceLockedReason string     `gorm:"type:varchar(256)" json:"force_locked_reason"`

    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
    DeletedAt *time.Time `gorm:"index" json:"-"`
    CreatedBy string     `gorm:"type:varchar(64)" json:"created_by"`
    UpdatedBy string     `gorm:"type:varchar(64)" json:"updated_by"`
}
```

### PerformanceGoalRecord
目标/指标记录

```go
type PerformanceGoalRecord struct {
    ID              uint       `gorm:"primaryKey" json:"id"`
    ActivityID      string     `gorm:"type:varchar(64);not null;index" json:"activity_id"`
    ParticipantID   uint       `gorm:"not null;index" json:"participant_id"`
    IndicatorItemID *uint      `gorm:"index" json:"indicator_item_id"`
    SectionType     string     `gorm:"type:varchar(32);not null" json:"section_type"` // quantitative, key_action, bonus_penalty
    ItemName        string     `gorm:"type:varchar(256);not null" json:"item_name"`
    ItemDefinition  string     `gorm:"type:text" json:"item_definition"`
    Weight          float64    `gorm:"default:0" json:"weight"`
    RedLineValue    string     `gorm:"type:varchar(256)" json:"red_line_value"`
    TargetValue     string     `gorm:"type:varchar(256)" json:"target_value"`
    ChallengeValue  string     `gorm:"type:varchar(256)" json:"challenge_value"`
    ScoringRule     string     `gorm:"type:text" json:"scoring_rule"`
    ActualResult    string     `gorm:"type:text" json:"actual_result"`
    Attachments     []string   `gorm:"type:json;serializer:json" json:"attachments"`
    SelfScore       float64    `gorm:"default:0" json:"self_score"`
    ManagerScore    float64    `gorm:"default:0" json:"manager_score"`
    BonusScore      float64    `gorm:"default:0" json:"bonus_score"`
    IsFromSuperior  bool       `gorm:"default:false" json:"is_from_superior"`
    ApprovalStatus  string     `gorm:"type:varchar(32);default:pending" json:"approval_status"`
    VisibilityScope string     `gorm:"type:varchar(64);default:department_only" json:"visibility_scope"`
    SortOrder       int        `gorm:"default:0" json:"sort_order"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    DeletedAt       *time.Time `gorm:"index" json:"-"`
}
```

### PerformanceTemplate
绩效模板（简单结构，评分维度和评分项通过关联表管理）

```go
type PerformanceTemplate struct {
    ID          uint       `gorm:"primaryKey" json:"id"`
    Name        string     `gorm:"type:varchar(128);not null;index" json:"name"`
    Description string     `gorm:"type:text" json:"description"`
    Status      string     `gorm:"type:varchar(32);not null;index;default:draft" json:"status"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
    DeletedAt   *time.Time `gorm:"index" json:"-"`
    CreatedBy   string     `gorm:"type:varchar(64)" json:"created_by"`
    UpdatedBy   string     `gorm:"type:varchar(64)" json:"updated_by"`
}
```

### PerformanceTemplateSection
模板评分维度

```go
type PerformanceTemplateSection struct {
    ID           uint   `gorm:"primaryKey" json:"id"`
    TemplateID   uint   `gorm:"not null;index" json:"template_id"`
    Name         string `gorm:"type:varchar(128);not null" json:"name"`
    Weight       float64 `gorm:"default:0" json:"weight"`
    SortOrder    int    `gorm:"default:0" json:"sort_order"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

### PerformanceTemplateItem
模板评分项

```go
type PerformanceTemplateItem struct {
    ID           uint   `gorm:"primaryKey" json:"id"`
    SectionID    uint   `gorm:"not null;index" json:"section_id"`
    Name         string `gorm:"type:varchar(128);not null" json:"name"`
    Description  string `gorm:"type:text" json:"description"`
    Weight       float64 `gorm:"default:0" json:"weight"`
    SortOrder    int    `gorm:"default:0" json:"sort_order"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

### PerformanceIndicatorLibrary
部门指标库

```go
type PerformanceIndicatorLibrary struct {
    ID              uint       `gorm:"primaryKey" json:"id"`
    DepartmentID    string     `gorm:"type:varchar(64);not null;index" json:"department_id"`
    DepartmentName  string     `gorm:"type:varchar(128);not null" json:"department_name"`
    ParentLibraryID *uint      `gorm:"index" json:"parent_library_id"`
    TemplateID      *uint      `gorm:"index" json:"template_id"`
    Name            string     `gorm:"type:varchar(128);not null" json:"name"`
    Description     string     `gorm:"type:text" json:"description"`
    DefaultCycle    string     `gorm:"type:varchar(32)" json:"default_cycle"`
    Status          string     `gorm:"type:varchar(32);not null;default:active" json:"status"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    DeletedAt       *time.Time `gorm:"index" json:"-"`
    CreatedBy       string     `gorm:"type:varchar(64)" json:"created_by"`
    UpdatedBy       string     `gorm:"type:varchar(64)" json:"updated_by"`
}
```

---

## 状态机

### Activity 状态流

```
draft → target_setting → self_evaluation → manager_evaluation → employee_confirmation → manager_confirmation → hr_confirmation → locked → archived
```

### Participant 状态流

```
pending → target_pending_approval → target_rejected → target_set → self_submitted → manager_submitted → employee_confirmed → manager_confirmed → manager_recheck → hr_confirmed / locked → inactive / removed_from_scope
```

### 状态推进规则

| 转换 | 触发人 | 前置条件 | 钉钉通知 |
|------|--------|----------|----------|
| draft → target_setting | HR/管理员 | 活动已创建 | 通知所有参与者的直属上级 |
| target_setting → self_evaluation | HR/管理员 | 目标已设定完成 | 通知所有员工 |
| self_evaluation → manager_evaluation | HR/管理员 | — | 通知所有上级 |
| manager_evaluation → employee_confirmation | HR/管理员 | 所有参与者已评分且分布合规 | 通知所有员工 |
| employee_confirmation → manager_confirmation | HR/管理员 | 所有员工已确认 | 通知所有上级 |
| manager_confirmation → hr_confirmation | HR/管理员 | 所有上级已确认；上级确认参与人结果时已冻结该参与人结果 | 通知 HR |
| hr_confirmation → locked | HR | 人力完成后续确认/归档检查 | 通知所有参与者（结果已锁定） |

---

## API 接口

所有接口前缀：`/api/v1/performance`

### 活动管理

#### GET /activities
查询绩效活动列表

Query 参数：
- `page`：页码（默认 1）
- `page_size`：每页数量（默认 10）
- `status`：状态筛选
- `keyword`：关键词搜索
- `start_date`：开始日期
- `end_date`：结束日期

响应说明：
- `items[]` 每条活动会附带 `my_participant`（当前登录人在该活动中的参与记录，未参与时为空），活动列表可直接渲染“我的目标/填写自评/查看结果”等个人入口，避免再按活动或按列表额外请求参与人映射。

#### POST /activities
创建绩效活动

Body：
```json
{
    "name": "2026年Q1绩效",
    "cycle_type": "quarterly",
    "start_date": "2026-01-01",
    "end_date": "2026-03-31",
    "self_eval_start_at": "2026-04-01",
    "self_eval_end_at": "2026-04-07",
    "manager_eval_start_at": "2026-04-08",
    "manager_eval_end_at": "2026-04-14",
    "result_confirm_start_at": "2026-04-15",
    "result_confirm_end_at": "2026-04-21",
    "status": "draft",
    "indicator_library_id": 1,
    "enable_bonus_score": false,
    "description": "季度绩效考核",
    "target_department_ids": ["1", "2"],
    "target_employee_ids": []
}
```

#### GET /activities/:activity_id
获取绩效活动详情

#### PUT /activities/:activity_id
更新绩效活动

#### POST /activities/:activity_id/start
启动绩效活动

#### POST /activities/:activity_id/publish
发布绩效活动（进入自评阶段）

#### POST /activities/:activity_id/close
关闭绩效活动

#### POST /activities/:activity_id/archive
归档绩效活动

### 阶段管理

#### POST /activities/:activity_id/open-target-setting
开启目标设定阶段

#### POST /activities/:activity_id/open-self-evaluation
开启自评阶段

#### POST /activities/:activity_id/open-manager-evaluation
开启上级评分阶段

#### POST /activities/:activity_id/open-employee-confirmation
开启员工确认阶段

#### POST /activities/:activity_id/open-manager-confirmation
开启上级确认阶段

#### POST /activities/:activity_id/open-hr-confirmation
开启人力确认阶段

#### POST /activities/:activity_id/confirm-results
确认活动结果

#### POST /activities/:activity_id/lock
锁定绩效活动

#### POST /activities/:activity_id/force-lock-overdue-hr
逾期强制锁定 HR 确认

### 参与人管理

#### POST /activities/:activity_id/refresh-participants
刷新参与人列表

#### GET /activities/:activity_id/participants
查询参与人列表

#### GET /participants/my
按 `activity_ids` 批量查询当前登录人在多个绩效活动中的参与人记录，作为兼容/备用接口；活动列表首屏优先使用 `GET /activities` 返回的 `my_participant`。

#### GET /participants/:participant_id
获取参与人详情

#### GET /participants/:participant_id/versions
获取参与人版本记录

#### GET /participants/:participant_id/relationship-change-logs
获取参与人关系变更日志

#### GET /activities/:activity_id/relationship-change-logs
获取活动关系变更日志

### 目标设定

#### GET /goal-records/:participant_id
获取目标记录列表

#### GET /goal-records/:participant_id/manager-goals
获取上级下发的目标

#### GET /goal-records/:participant_id/suggestions
获取目标模板建议

#### POST /goal-records/:participant_id
批量创建/更新目标记录

#### POST /goal-records/:participant_id/submit
提交目标审批

#### POST /goal-records/:participant_id/approve
审批通过目标

#### POST /goal-records/:participant_id/reject
驳回目标

#### POST /activities/:activity_id/batch-assign-goals
批量下发目标给下属

### 自评与评分

#### POST /participants/:participant_id/self-evaluation
提交自评

#### POST /participants/:participant_id/manager-evaluation
提交上级评分

#### POST /activities/:activity_id/batch-manager-evaluations
批量提交上级评分

#### POST /goal-reviews/:participant_id/self-evaluation
提交目标自评（基础版）

#### POST /goal-reviews/:participant_id/manager-evaluation
提交目标上级评分（基础版）

#### POST /goal-reviews/:participant_id/bonus-penalty
设置附加分

#### POST /reviews/:participant_id/self-evaluation
提交目标自评（Review 版，带钉钉审批同步）

#### POST /reviews/:participant_id/manager-evaluation
提交目标上级评分（Review 版，带钉钉审批同步）

#### POST /participants/:participant_id/bonus-penalty
设置附加分（单独注册接口）

#### POST /auto-score
自动评分（基于 scoring_engine.go 三种算法）

### 确认链

#### POST /participants/:participant_id/confirm-employee
员工确认结果

#### POST /participants/:participant_id/confirm-manager
上级确认结果

#### POST /participants/:participant_id/confirm-hr
人力确认结果

#### POST /activities/:activity_id/batch-confirm
批量确认（别名）

#### POST /activities/:activity_id/batch-confirm-results
批量确认结果

### 强制分布

#### PUT /activities/:activity_id/distribution-rules
设置强制分布规则

#### GET /activities/:activity_id/distribution-rules
获取强制分布规则

#### GET /activities/:activity_id/distribution-check
检查强制分布合规性

#### GET /activities/:activity_id/realtime-distribution-check
实时检查强制分布（评分过程中）

### 结果与归档

#### POST /participants/:participant_id/adjust-final-level
调整最终等级

#### POST /participants/:participant_id/confirm-result
确认个人结果

#### POST /participants/:participant_id/trigger-interview
触发绩效面谈

#### GET /activities/:activity_id/result-summary
获取结果汇总

### 指标库管理

#### GET /indicator-libraries
查询指标库列表，支持 `template_id` 按绩效流程模板过滤

#### POST /indicator-libraries
创建指标库，必须传入 `template_id`

#### GET /indicator-libraries/:id
获取指标库详情

#### PUT /indicator-libraries/:id
更新指标库

#### POST /indicator-libraries/:id/archive
归档指标库

#### GET /indicator-libraries/department/:department_id
获取部门指标库

#### POST /indicator-libraries/inherit
继承指标库

#### GET /indicator-items
查询指标项列表

#### POST /indicator-items
添加指标项

#### PUT /indicator-items/:id
更新指标项

#### DELETE /indicator-items/:id
删除指标项

#### GET /indicator-items/search
搜索指标项

### 模板管理

#### GET /templates
查询绩效模板列表

#### POST /templates
创建绩效模板

#### GET /templates/:id
获取绩效模板详情

#### PUT /templates/:id
更新绩效模板

### 通知与催办

#### POST /activities/:activity_id/send-self-eval-reminder
发送自评提醒

#### 自评自动多轮提醒
系统启动后会注册绩效后台任务，每天按 `PERFORMANCE_SELF_EVAL_REMINDER_HOUR`（默认 9 点）扫描 `self_evaluation` 状态活动，默认在自评截止前 3 天、1 天、截止当天自动提醒未提交自评的参与人。

说明：
- 提醒通过钉钉 ActionCard 发送，按钮跳转到对应参与人的自评页面。
- 已提交自评、已进入主管评分/确认/锁定等后续状态的参与人不会再收到自动提醒。
- 使用 `performance_reminder_logs` 记录活动、参与人、阶段、提醒轮次和发送日期，避免同一天同一轮重复发送。
- 活动可通过 `reminder_config.self_eval_reminder_days` / `self_eval_days_before_deadline` 覆盖默认提醒天数；可通过 `self_eval_auto_reminder_enabled=false` 关闭自动提醒。

#### POST /activities/:activity_id/send-manager-eval-reminder
发送评分提醒

#### POST /activities/:activity_id/send-hr-confirm-reminder
发送人力确认提醒

### 钉钉通知链接规则

- 绩效相关钉钉通知统一使用企业内部 `action_card` 消息，按钮链接由 `PerformanceSelfEvalURL`、`PerformanceManagerEvalURL`、`PerformanceResultURL`、`PerformanceOverviewURL` 生成。
- 员工自评通知跳转员工自评页，主管评分通知跳转上级评分页，绩效结果确认/锁定/面谈通知跳转个人绩效结果页，HR 确认提醒跳转绩效总览页。
- 若链接参数缺失导致 URL 为空，底层发送能力可退回纯文本消息，但业务发送点应优先传入明确的操作链接。
- 真实钉钉联调优先使用 `go run ./tools/ops/send_dingtalk_message -user <钉钉user_id> -url <可访问系统地址>` 单独发送给测试人，确认企业应用权限、消息按钮链接和接收人可通知性后，再触发活动级提醒。
- 联调命令必须显式传入 `-user`，不会扫描参与人或绩效活动；未配置 `-url` 时会使用 `DINGTALK_APP_HOME_URL` / `APP_BASE_URL` / `FRONTEND_BASE_URL`，都为空则退回纯文本消息。
- 自评自动多轮提醒联调优先使用单活动命令：`go run ./tools/ops/run_self_eval_reminders -activity <活动ID> -confirm-send -max-recipients <上限>`。若当前日期不在默认提醒节点，可加 `-include-current-day` 仅在内存中临时加入当天距截止日的天数，用于测试自动轮次、幂等日志和钉钉发送链路。

### HR 收支规则

#### PUT /activities/:activity_id/finance
设置公司收支状态

#### GET /activities/:activity_id/finance
获取公司收支状态

### HR 确认管理

#### GET /activities/:activity_id/pending-hr-confirm
获取待人力确认的参与人

#### PUT /activities/:activity_id/hr-confirm-deadline
设置人力确认截止时间

#### GET /activities/:activity_id/hr-confirm-deadline-status
获取人力确认截止时间状态

---

## 核心业务流程

### 绩效活动生命周期

1. **创建活动**（`CreateActivity`）
   - 填写活动基本信息
   - 关联模板和指标库
   - 设置参与人范围

2. **启动活动**（`StartActivity`）
   - 验证活动配置
   - 刷新参与人列表
   - 通知相关人

3. **目标设定**（`OpenTargetSetting`）
   - 上级为下属设定目标
   - 员工确认目标
   - 目标审批流程

4. **员工自评**（`SubmitSelfEvaluation`）
   - 员工填写实际达成结果
   - 系统计算自评总分
   - 员工提交自评

5. **上级评分**（`SubmitManagerEvaluation`）
   - 上级为下属评分
   - 系统计算上级评分总分
   - 根据上级评分总分自动生成建议绩效等级：S(>=100)、A(90-99)、B(80-89)、C(60-79)、D(<60)
   - 绩效系数分别为 S=1.2、A=1.1、B=1.0、C=0.8、D=0.4
   - 实时检查建议等级对应的强制分布配额
   - 当分数对应等级与强制分布冲突时，以强制分布为准；分数等级只作为建议等级，最终等级必须满足分布制度口径
   - 制度依据：部门考评总人数关联绩效等级强制分布比例得出各等级评级人数，即使人员绩效得分集中在同一区间，也需按对应绩效等级评级比例进行强制排名分布
   - 人工调整最终等级必须走 `AdjustFinalLevel`，不要在上级评分提交接口覆盖建议等级

6. **确认与冻结**（`ConfirmResult`）
   - 员工确认结果
   - 上级确认结果；上级确认成功后立即写入锁定字段，冻结该参与人的评分、等级和附加项
   - 人力确认作为后续确认/归档节点，不再覆盖上级冻结时写入的锁定人和锁定时间

7. **锁定与归档**（`LockActivity` / `ArchiveActivity`）
   - 锁定活动，防止修改
   - 归档活动，保存历史

### 强制分布流程

1. **设置规则**（`PutDistributionRules`）
   - 定义各等级比例、人数名额或等级组合上限（如 `S/A` 合计上限）
   - 设置适用范围，优先按员工架构所属部门计算；部门人数或名额规则不足时再按所属中心汇总
   - 员工名额占用其架构所属部门/中心名额，不占用项目部门或临时业务线名额
   - 按部门考评总人数和强制分布比例换算各等级人数，支持在评分集中同一区间时仍按比例强制排名

2. **评分时检查**（`GetRealtimeDistributionCheck`）
   - 实时计算当前分布
   - 提示可选等级

3. **结果检查**（`GetDistributionCheck`）
   - 验证最终分布是否合规
   - 不合规时提示调整

### 双向/多向汇报评分口径（待完整落地）

双向/多向汇报包括员工存在多个考核上级、跨部门参与多个项目、或架构归属与实际业务线不同的情况。例如产品经理架构归属产品中心，但实际参与寄存业务线，由架构负责人和业务负责人共同评价。

- 管理员按考核周期维护评价人、工作占比与权重；项目制场景后续可优先从项目关系自动带出，再由管理员确认。
- 多个上级分别评分后，最终分数按工作占比权重加权汇总。
- 多个上级之间允许互相查看评分。
- 强制分布名额仍按员工架构所属部门/中心占用，不按项目部门占用。

### 目标权重与附加项口径

- 考核周期内的计划目标权重合计必须为 100%。
- “下季度目标计划”权重合计必须为 100%；上季度/本周期完成情况对应的计划权重也按 100% 口径校验。
- 加分、扣分等附加项目单独配置，不参与目标权重合计。
- 目标设定页从指标库获取建议或搜索指标项时，只能使用当前绩效活动 `indicator_library_id` 关联的指标库；活动未关联指标库时不从参与人部门指标库兜底取数。
- 自动评分引擎仍输出旧口径原始分（最高 120）；新流程上级评分页回填一键评分时需换算为 0-10 分制并封顶 10 分，旧流程保持原始分口径。

### 报表口径

绩效报表需要覆盖考核进度、员工目标完成情况、考核结果、部门等级分布、历史对比，并支持按公司、中心、部门、岗位、员工、周期、流程状态、等级等维度筛选取数。

### 三级确认流程

1. **员工确认**（`ConfirmEmployeeResult`）
   - 员工查看评分结果
   - 确认或申诉

2. **上级确认并冻结**（`ConfirmManagerResult`）
   - 上级确认下属结果
   - 确认成功后立即冻结该参与人的绩效结果
   - 冻结后评分、等级、附加分/扣分等结果数据不可再修改

3. **人力确认**（`ConfirmHRResult`）
   - 人力审核整体结果
   - 作为后续确认/归档节点，不覆盖上级确认时写入的锁定人和锁定时间

### 主管确认后自评修改复核（2026-06-23）

- HR 确认前，员工仍可修改自评完成情况、自评分、附件和自评文本；HR 已确认或结果已锁定后禁止普通员工修改。
- 主管确认前的自评修改不额外通知主管；主管确认后员工再次提交自评时，参与人状态变为 `manager_recheck`（待领导复核），保留原主管评分和评价，并临时解除该参与人的锁定。
- 主管收到钉钉 ActionCard 通知，跳转 `PerformanceManagerEvalURL` 对应的上级评分/复核页面；同一参与人同一复核场景 1 小时内多次修改只发送一次通知，通知失败不阻止员工提交，只记录日志。
- 主管复核可以直接“确认查看”，也可以重新提交上级评价；复核完成后参与人回到 `manager_confirmed`，重新写入主管确认时间并冻结结果，HR 确认只能在复核完成后继续。
- 自评修改历史继续通过 `PerformanceReviewVersion` 记录，`operation_meta` 会标记是否发生在主管确认后，并保留修改前自评摘要、原主管确认人和确认时间。

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `PerformanceService` | `performance_service.go` | 绩效核心服务 |
| `PerformanceIndicatorService` | `performance_indicator_service.go` | 指标库服务 |
| `ScoringEngine` | `scoring_engine.go` | 自动评分引擎（三种算法：区间插值/达标制/比率制） |

---

## 前端页面

### 绩效总览页面
`frontend/src/pages/PerformanceOverview.tsx`

功能：
- 绩效活动列表
- 活动状态管理
- 参与人管理
- 强制分布设置
- 结果查看与确认

### 绩效指标库管理
`frontend/src/pages/PerformanceIndicatorLibrary.tsx`

功能：
- 指标库列表与详情
- 指标项 CRUD
- 部门级指标继承
- 指标搜索与匹配

### 绩效目标设定
`frontend/src/pages/PerformanceGoalSetting.tsx`

功能：
- 员工目标填写与提交
- 上级目标下发
- 目标审批流程
- 目标模板建议

### 员工自评
`frontend/src/pages/PerformanceSelfEval.tsx`

功能：
- 自评表单填写
- 实际达成结果录入
- 自评提交与保存

### 上级评分
`frontend/src/pages/PerformanceManagerEval.tsx`

功能：
- 下属评分明细
- 强制分布实时配额显示
- 等级调整与评分提交
- 批量评分操作

### 个人绩效结果
`frontend/src/pages/PerformanceResultView.tsx`

功能：
- 个人评分明细、附加考核项、自评与上级评价展示
- 员工、主管、人力三级确认进度展示与确认操作
- Excel 风格”个人绩效考核表”归档展示
- 归档/导出模板按流程区分：旧流程使用 PARTB 个人绩效模板；新流程使用“上季度指标完成情况 + 下季度目标计划”模板
- 新流程同一活动内按 `goal_phase` 区分：`review` 为上一季度/本期绩效考核记录，`plan` 为下一季度目标计划；开启新流程活动时会尝试从上一期活动的 `plan` 承接为本期 `review`
- 新流程进入或提交自评前必须存在本期 `review` 指标；系统承接上一期 `plan` 后仍缺失时禁止开启/提交自评，并提示补录/导入上一季度考核指标
- 基于浏览器打印能力支持打印 / PDF 保存
- 基于 HTML 表格 Blob 下载 `.xls`，用于一人一表线下复核

### 绩效活动编辑器（组件）
`frontend/src/components/PerformanceActivityEditor.tsx`

功能：
- 活动创建/编辑表单
- 时间范围配置
- 参与人范围筛选

---

## 环境变量

- `DINGTALK_APP_KEY`：钉钉应用 Key
- `DINGTALK_APP_SECRET`：钉钉应用 Secret
- `DINGTALK_CORP_ID`：钉钉企业 ID

---

## 常见问题

### 绩效活动无法启动
- 检查活动状态是否为 `draft`
- 检查是否设置了参与人范围
- 检查模板和指标库是否有效

### 强制分布不合规
- 检查分布规则设置是否正确
- 使用实时检查接口查看当前分布
- 调整评分或分布规则

### 三级确认流程卡住
- 检查每个阶段的参与人是否都已完成
- 使用催办功能提醒未完成的人
- 检查钉钉通知是否正常发送

### 目标设定审批失败
- 检查目标权重是否合计为 100%
- 检查目标内容是否完整
- 检查审批人是否有权限

### 绩效结果无法锁定
- 检查员工确认是否已完成
- 检查上级确认是否已完成；上级确认成功后参与人结果会立即冻结
- 检查是否有未处理的申诉

## 考核上级独立设置（2026-06-03）

本节属于本次绩效考核上级独立设置的业务规则文档变更，用于固化活动内考核上级与组织直属主管解耦后的长期口径。

### 字段语义
- `users.manager_user_id` / `users.manager_name`：员工当前组织关系中的实时直属主管，只作为组织主数据使用，会随钉钉同步或组织调整变化。
- `performance_participants.manager_id` / `manager_name`：某个绩效活动内的考核上级，用于经理评分、经理确认、分布团队统计与结果页展示。它与实时直属主管解耦，历史活动刷新参与人时不得用 `users.manager_*` 覆盖。
- `performance_participants.direct_manager_id_snapshot` / `direct_manager_name_snapshot`：参与人进入活动时的直属主管快照，用于审计和对比。
- `manager_source`：考核上级来源，枚举 `DIRECT_MANAGER`、`DEPARTMENT_HEAD`、`CENTER_HEAD`、`MANUAL`、`IMPORT`、`SYSTEM`。
- `manager_overridden` / `manager_override_reason`：是否被人工、导入或其他非默认逻辑调整，以及调整原因。

### 刷新规则
- `RefreshParticipants` 新增参与人时，默认把当时的直属主管写入 `manager_id` / `manager_name`，并同步写入直属主管快照，来源为 `DIRECT_MANAGER`。
- 对已存在参与人，刷新只更新员工姓名、岗位、部门、在职/移出范围等信息，不再覆盖 `manager_id` / `manager_name`。
- 历史数据迁移只补充新增元数据字段，旧的 `manager_id` / `manager_name` 保持原样，`manager_source` 回填为 `SYSTEM`。

### API
- `GET /api/v1/performance/activities/:activity_id/assessment-manager-candidates`：按活动与可选参与人获取考核上级候选人。
- `PUT /api/v1/performance/participants/:participant_id/assessment-manager`：单人调整考核上级。
- `POST /api/v1/performance/activities/:activity_id/assessment-managers/batch`：批量调整考核上级，返回逐条成功/失败结果。

### 权限
- `performance:assessment_manager:update`：单人调整与候选人查询。
- `performance:assessment_manager:batch_update`：批量调整。
- 兼容迁移会给已有 `performance:activity:manage` 授权角色补发上述权限。

### 校验与审计
- 经理评分、目标评分、附加分设置、经理确认等经理侧操作，必须校验当前用户等于 `performance_participants.manager_id`（管理员和系统用户除外）。
- 调整考核上级会写入 `performance_relationship_change_logs`，记录旧/新考核上级、旧/新来源、调整原因、操作人和时间。
- 已锁定、HR 已确认或 locked 状态的参与人不可再调整考核上级。

### 导入
- 参与人导入模板兼容旧列，并新增可选列：`考核上级工号`、`考核上级姓名`、`考核上级来源`、`调整原因`。
- 导入接口当前是解析/预览接口，不直接落库参与人；解析结果会返回 `manager_assignments`，无法匹配或非在职的考核上级会进入 `skipped_rows` 与 `warnings`。

### 待业务确认
- `DEPARTMENT_HEAD` / `CENTER_HEAD` 当前从部门扩展字段读取，中心层级和负责人字段口径需要组织主数据进一步明确。
- 锁定后是否允许 HR 仅为历史展示修正考核上级，目前实现为禁止。

## 绩效指标库权限范围（2026-06-22）

- 指标库数据可见范围由权限模块的数据权限决定：`all` 可见全公司，`department` 会按配置部门递归包含下级部门，`self` 或空部门范围不会退化为全量数据。
- 部门负责人、主管/经理、HRBP、部门助理等组织管理范围，可通过角色数据权限配置部门，也可从部门扩展字段自动补充管理部门；同一用户管理多个部门时会合并多个部门及其子部门。
- 指标库维护接口必须同时校验功能权限 `performance:indicator:manage` 与指标库所属部门数据范围；只读列表和搜索按数据范围过滤可见指标库/指标项。
- 指标项的查看、创建、修改、删除需先校验其归属指标库的部门范围，避免通过指标项 ID 绕过指标库数据权限。
- 启动迁移会自动补齐指标库维护角色：`绩效管理者-人事`、`人力负责人`、`部门负责人`、`HRBP`、`部门助理`、`部门主管`、`经理`。
- 上述角色默认都拥有 `performance:indicator:manage` 与 `org:read`，菜单包含 `menu:home`、`menu:performance-indicator-library`。
- `绩效管理者-人事`、`人力负责人` 默认数据权限为 `all`，用于维护整体公司的绩效指标库、核查各部门设置情况、给各部门设置绩效指标。
- `部门负责人`、`HRBP`、`部门助理`、`部门主管`、`经理` 默认数据权限为 `department` 且部门列表为空，实际范围由角色数据权限配置或部门扩展字段中的负责人/经理/HRBP/助理字段自动补充；同一人管理多个组织时合并多个组织及其下级范围。
