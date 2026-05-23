---
purpose: 绩效管理模块业务规则说明
last_updated: 2026-05-22
source_of_truth:
  - internal/api/performance_handlers.go（绩效相关 handler）
  - internal/service/performance_service.go（绩效服务）
  - internal/service/performance_indicator_service.go（指标库服务）
  - internal/repository/performance_repository.go（绩效数据访问）
  - internal/repository/performance_indicator_repository.go（指标库数据访问）
  - internal/database/performance_models.go（绩效相关模型）
  - frontend/src/pages/PerformanceOverview.tsx（绩效总览页面）
update_when:
  - 修改绩效活动状态流时
  - 修改评分规则时
  - 修改强制分布逻辑时
  - 新增绩效相关 API 时
  - 修改前端绩效页面时
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
- 支持部门级模板和继承关系
- 包含模板快照功能

### 指标库（PerformanceIndicatorLibrary）
- 部门级指标库，支持继承
- 包含量化指标、关键行动、附加考核项
- 支持指标项的搜索和匹配

### 参与人（PerformanceParticipant）
- 某次绩效活动的参与者
- 包含员工信息、评分、等级、确认状态等
- 支持主管关系和收支系数

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
    
    // 关联模板和指标库
    TemplateID        *uint `gorm:"index" json:"template_id"`
    IndicatorLibraryID *uint `gorm:"index" json:"indicator_library_id"`
    
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
    
    // 评价文本
    SelfEvaluationComment    string `gorm:"type:text" json:"self_evaluation_comment"`
    ManagerEvaluationComment string `gorm:"type:text" json:"manager_evaluation_comment"`
    
    // 系统计算总分
    TotalSelfScore    float64 `gorm:"default:0" json:"total_self_score"`
    TotalManagerScore float64 `gorm:"default:0" json:"total_manager_score"`
    
    // 附加项
    BonusScore    float64 `gorm:"default:0" json:"bonus_score"`
    PenaltyScore  float64 `gorm:"default:0" json:"penalty_score"`
    AdjustedScore float64 `gorm:"default:0" json:"adjusted_score"`
    
    // 收支系数
    RevenueCoefficient float64 `gorm:"default:1" json:"revenue_coefficient"`
    
    // 三级确认
    EmployeeConfirmedAt *time.Time `json:"employee_confirmed_at"`
    EmployeeConfirmedBy string     `gorm:"type:varchar(64)" json:"employee_confirmed_by"`
    ManagerConfirmedAt  *time.Time `json:"manager_confirmed_at"`
    ManagerConfirmedBy  string     `gorm:"type:varchar(64)" json:"manager_confirmed_by"`
    HRConfirmedAt       *time.Time `json:"hr_confirmed_at"`
    HRConfirmedBy       string     `gorm:"type:varchar(64)" json:"hr_confirmed_by"`
    
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
绩效模板

```go
type PerformanceTemplate struct {
    ID               uint       `gorm:"primaryKey" json:"id"`
    Name             string     `gorm:"type:varchar(128);not null;index" json:"name"`
    Description      string     `gorm:"type:text" json:"description"`
    DepartmentID     string     `gorm:"type:varchar(64);index" json:"department_id"`
    DepartmentName   string     `gorm:"type:varchar(128)" json:"department_name"`
    ApplicableCycles []string   `gorm:"type:json;serializer:json" json:"applicable_cycles"`
    Status           string     `gorm:"type:varchar(32);not null;index;default:draft" json:"status"`
    ParentTemplateID *uint      `gorm:"index" json:"parent_template_id"`
    IsSnapshot       bool       `gorm:"default:false" json:"is_snapshot"`
    CreatedAt        time.Time  `json:"created_at"`
    UpdatedAt        time.Time  `json:"updated_at"`
    DeletedAt        *time.Time `gorm:"index" json:"-"`
    CreatedBy        string     `gorm:"type:varchar(64)" json:"created_by"`
    UpdatedBy        string     `gorm:"type:varchar(64)" json:"updated_by"`
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
pending → target_pending_approval → target_rejected → target_set → self_submitted → manager_submitted → result_confirmed → inactive / removed_from_scope
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
    "template_id": 1,
    "indicator_library_id": 1,
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

### 参与人管理

#### POST /activities/:activity_id/refresh-participants
刷新参与人列表

#### GET /activities/:activity_id/participants
查询参与人列表

#### GET /participants/:participant_id
获取参与人详情

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
提交目标自评

#### POST /goal-reviews/:participant_id/manager-evaluation
提交目标上级评分

#### POST /goal-reviews/:participant_id/bonus-penalty
设置附加分

### 确认链

#### POST /participants/:participant_id/confirm-employee
员工确认结果

#### POST /participants/:participant_id/confirm-manager
上级确认结果

#### POST /participants/:participant_id/confirm-hr
人力确认结果

#### POST /activities/:activity_id/batch-confirm
批量确认

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
查询指标库列表

#### POST /indicator-libraries
创建指标库

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

#### POST /activities/:activity_id/send-manager-eval-reminder
发送评分提醒

#### POST /activities/:activity_id/send-hr-confirm-reminder
发送人力确认提醒

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
   - 根据上级评分总分自动生成绩效等级：S(>=100)、A(90-99)、B(80-89)、C(60-79)、D(<60)
   - 绩效系数分别为 S=1.2、A=1.1、B=1.0、C=0.8、D=0.4
   - 实时检查自动等级对应的强制分布配额
   - 人工调整最终等级必须走 `AdjustFinalLevel`，不要在上级评分提交接口覆盖自动等级

6. **确认与冻结**（`ConfirmResult`）
   - 员工确认结果
   - 上级确认结果；上级确认成功后立即写入锁定字段，冻结该参与人的评分、等级和附加项
   - 人力确认作为后续确认/归档节点，不再覆盖上级冻结时写入的锁定人和锁定时间

7. **锁定与归档**（`LockActivity` / `ArchiveActivity`）
   - 锁定活动，防止修改
   - 归档活动，保存历史

### 强制分布流程

1. **设置规则**（`PutDistributionRules`）
   - 定义各等级比例
   - 设置适用范围

2. **评分时检查**（`GetRealtimeDistributionCheck`）
   - 实时计算当前分布
   - 提示可选等级

3. **结果检查**（`GetDistributionCheck`）
   - 验证最终分布是否合规
   - 不合规时提示调整

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

---

## 关键 Service

| Service | 文件 | 说明 |
|---|---|---|
| `PerformanceService` | `performance_service.go` | 绩效核心服务 |
| `PerformanceIndicatorService` | `performance_indicator_service.go` | 指标库服务 |
| `PerformanceTemplateSupport` | `performance_template_support.go` | 模板支持服务 |

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

### 个人绩效结果页
`frontend/src/pages/PerformanceResultView.tsx`

功能：
- 个人评分明细、附加考核项、自评与上级评价展示
- 员工、主管、人力三级确认进度展示与确认操作
- Excel 风格“个人绩效考核表”归档展示
- 基于浏览器打印能力支持打印 / PDF 保存
- 基于 HTML 表格 Blob 下载 `.xls`，用于一人一表线下复核

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
