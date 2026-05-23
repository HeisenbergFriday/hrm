package database

import (
	"time"
)

// PerformanceTemplate 绩效模板
type PerformanceTemplate struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"type:varchar(128);not null;index" json:"name"`
	Description string     `gorm:"type:text" json:"description"`
	Status      string     `gorm:"type:varchar(32);not null;index;default:draft" json:"status"` // draft, active, archived
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
	CreatedBy   string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy   string     `gorm:"type:varchar(64)" json:"updated_by"`
}

// PerformanceTemplateSection 绩效模板评分维度
type PerformanceTemplateSection struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	TemplateID        uint       `gorm:"not null;index" json:"template_id"`
	Name              string     `gorm:"type:varchar(128);not null" json:"name"`
	SectionType       string     `gorm:"type:varchar(32);not null" json:"section_type"` // score, text
	Weight            float64    `gorm:"default:0" json:"weight"`
	SortOrder         int        `gorm:"default:0" json:"sort_order"`
	IsScoreRequired   bool       `gorm:"default:false" json:"is_score_required"`
	IsCommentRequired bool       `gorm:"default:false" json:"is_comment_required"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `gorm:"index" json:"-"`
}

// PerformanceTemplateItem 绩效模板评分项
type PerformanceTemplateItem struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	SectionID   uint       `gorm:"not null;index" json:"section_id"`
	Name        string     `gorm:"type:varchar(256);not null" json:"name"`
	Description string     `gorm:"type:text" json:"description"`
	MaxScore    float64    `gorm:"default:100" json:"max_score"`
	Weight      float64    `gorm:"default:0" json:"weight"`
	SortOrder   int        `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
}

type PerformanceActivity struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Name               string `gorm:"type:varchar(128);not null;index" json:"name"`
	CycleType          string `gorm:"type:varchar(32);not null" json:"cycle_type"` // monthly, quarterly, annual
	StartDate          string `gorm:"type:varchar(32);not null" json:"start_date"`
	EndDate            string `gorm:"type:varchar(32);not null" json:"end_date"`
	IndicatorLibraryID *uint  `gorm:"index" json:"indicator_library_id"`

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

	// 附加分配置
	EnableBonusScore bool `gorm:"default:false" json:"enable_bonus_score"` // 启用后附加分计入总分并影响等级

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
	CreatedBy string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy string     `gorm:"type:varchar(64)" json:"updated_by"`
}

// PerformanceLevelRule 绩效等级规则
type PerformanceLevelRule struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"type:varchar(128);not null" json:"name"`
	Status    string     `gorm:"type:varchar(32);not null;default:active" json:"status"` // active, inactive
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

// PerformanceLevelRuleItem 绩效等级规则明细
type PerformanceLevelRuleItem struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	RuleID              uint      `gorm:"not null;index" json:"rule_id"`
	Level               string    `gorm:"type:varchar(32);not null" json:"level"` // S, A, B, C, D
	MinScore            float64   `gorm:"default:0" json:"min_score"`
	MaxScore            float64   `gorm:"default:100" json:"max_score"`
	DistributionPercent float64   `gorm:"default:0" json:"distribution_percent"` // 强制分布比例
	SortOrder           int       `gorm:"default:0" json:"sort_order"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type PerformanceDistributionRule struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	ActivityID          string     `gorm:"type:varchar(64);not null;index" json:"activity_id"`
	Level               string     `gorm:"type:varchar(32);not null;index" json:"level"`
	DistributionPercent int        `gorm:"not null" json:"distribution_percent"`
	Description         string     `gorm:"type:text" json:"description"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DeletedAt           *time.Time `gorm:"index" json:"-"`
	CreatedBy           string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy           string     `gorm:"type:varchar(64)" json:"updated_by"`
}

// PerformanceDistributionException 强制分布例外记录
type PerformanceDistributionException struct {
	ID                     uint                   `gorm:"primaryKey" json:"id"`
	ActivityID             string                 `gorm:"type:varchar(64);not null;index" json:"activity_id"`
	OperatorID             string                 `gorm:"type:varchar(64);not null" json:"operator_id"`
	Reason                 string                 `gorm:"type:text;not null" json:"reason"`
	BeforeDistributionJSON map[string]interface{} `gorm:"type:json;serializer:json" json:"before_distribution"`
	AfterDistributionJSON  map[string]interface{} `gorm:"type:json;serializer:json" json:"after_distribution"`
	CreatedAt              time.Time              `json:"created_at"`
}

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
	SelfScore   float64 `gorm:"default:0" json:"self_score"`
	SelfLevel   string  `gorm:"type:varchar(32)" json:"self_level"`
	SelfSummary string  `gorm:"type:text" json:"self_summary"`

	ManagerScore   float64 `gorm:"default:0" json:"manager_score"`
	ManagerComment string  `gorm:"type:text" json:"manager_comment"`
	SuggestedLevel string  `gorm:"type:varchar(32)" json:"suggested_level"`
	FinalLevel     string  `gorm:"type:varchar(32)" json:"final_level"`
	AdjustReason   string  `gorm:"type:text" json:"adjust_reason"`

	// 评价文本
	SelfEvaluationComment    string `gorm:"type:text" json:"self_evaluation_comment"`
	ManagerEvaluationComment string `gorm:"type:text" json:"manager_evaluation_comment"`

	// 拆分评价字段
	SelfEvaluationGood           string `gorm:"type:text" json:"self_evaluation_good"`
	SelfEvaluationImprovement    string `gorm:"type:text" json:"self_evaluation_improvement"`
	ManagerEvaluationGood        string `gorm:"type:text" json:"manager_evaluation_good"`
	ManagerEvaluationImprovement string `gorm:"type:text" json:"manager_evaluation_improvement"`

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

	// 兼容旧接口
	ConfirmedAt *time.Time `json:"confirmed_at"`
	ConfirmedBy string     `gorm:"type:varchar(64)" json:"confirmed_by"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
	CreatedBy string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy string     `gorm:"type:varchar(64)" json:"updated_by"`
}

type PerformanceReview struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	ParticipantID uint   `gorm:"not null;index" json:"participant_id"`
	ActivityID    string `gorm:"type:varchar(64);not null;index" json:"activity_id"`

	ReviewType string `gorm:"type:varchar(32);not null;index" json:"review_type"` // self, manager
	ReviewerID string `gorm:"type:varchar(64);index" json:"reviewer_id"`

	SelfScore      float64 `gorm:"default:0" json:"self_score"`
	SelfLevel      string  `gorm:"type:varchar(32)" json:"self_level"`
	SelfSummary    string  `gorm:"type:text" json:"self_summary"`
	ManagerScore   float64 `gorm:"default:0" json:"manager_score"`
	SuggestedLevel string  `gorm:"type:varchar(32)" json:"suggested_level"`
	ManagerComment string  `gorm:"type:text" json:"manager_comment"`
	FinalLevel     string  `gorm:"type:varchar(32)" json:"final_level"`
	AdjustReason   string  `gorm:"type:text" json:"adjust_reason"`
	ConfirmComment string  `gorm:"type:text" json:"confirm_comment"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
	CreatedBy string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy string     `gorm:"type:varchar(64)" json:"updated_by"`
}

type PerformanceReviewVersion struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	ParticipantID uint   `gorm:"not null;index" json:"participant_id"`
	ActivityID    string `gorm:"type:varchar(64);not null;index" json:"activity_id"`

	ReviewType string `gorm:"type:varchar(32);not null;index" json:"review_type"` // self, manager, adjust, confirm
	CreatedBy  string `gorm:"type:varchar(64)" json:"created_by"`

	// self
	SelfScore           float64  `gorm:"default:0" json:"self_score"`
	SelfLevel           string   `gorm:"type:varchar(32)" json:"self_level"`
	SelfSummary         string   `gorm:"type:text" json:"self_summary"`
	SelfAttachmentsJSON []string `gorm:"type:json;serializer:json" json:"self_attachments"`

	// manager
	ManagerScore        float64     `gorm:"default:0" json:"manager_score"`
	SuggestedLevel      string      `gorm:"type:varchar(32)" json:"suggested_level"`
	ManagerComment      string      `gorm:"type:text" json:"manager_comment"`
	EvaluationItemsJSON interface{} `gorm:"type:json;serializer:json" json:"evaluation_items"`

	// adjust/confirm
	FinalLevel     string     `gorm:"type:varchar(32)" json:"final_level"`
	AdjustReason   string     `gorm:"type:text" json:"adjust_reason"`
	ConfirmComment string     `gorm:"type:text" json:"confirm_comment"`
	ConfirmedAt    *time.Time `json:"confirmed_at"`

	OperationMeta interface{} `gorm:"type:json;serializer:json" json:"operation_meta"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

type PerformanceRelationshipChangeLog struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	ActivityID    string `gorm:"type:varchar(64);not null;index" json:"activity_id"`
	ParticipantID uint   `gorm:"not null;index" json:"participant_id"`

	ChangeType string    `gorm:"type:varchar(64);not null;index" json:"change_type"`
	FieldName  string    `gorm:"type:varchar(64)" json:"field_name"`
	OldValue   string    `gorm:"type:text" json:"old_value"`
	NewValue   string    `gorm:"type:text" json:"new_value"`
	ChangedAt  time.Time `json:"changed_at"`
	Source     string    `gorm:"type:varchar(64)" json:"source"`
	CreatedBy  string    `gorm:"type:varchar(64)" json:"created_by"`

	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

// PerformanceGoalRecord 目标/指标记录
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

// PerformanceGoalApprovalLog 目标审批日志
type PerformanceGoalApprovalLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ParticipantID uint      `gorm:"not null;index" json:"participant_id"`
	ActivityID    string    `gorm:"type:varchar(64);not null;index" json:"activity_id"`
	GoalRecordID  uint      `gorm:"index" json:"goal_record_id"`
	Action        string    `gorm:"type:varchar(32);not null" json:"action"` // submit, approve, reject
	Comment       string    `gorm:"type:text" json:"comment"`
	ApproverID    string    `gorm:"type:varchar(64)" json:"approver_id"`
	ApproverName  string    `gorm:"type:varchar(128)" json:"approver_name"`
	Version       int       `gorm:"default:1" json:"version"`
	Snapshot      string    `gorm:"type:text" json:"snapshot"`
	CreatedBy     string    `gorm:"type:varchar(64)" json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

// PerformanceCompanyFinance 公司收支状态
type PerformanceCompanyFinance struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ActivityID  string    `gorm:"type:varchar(64);not null;index" json:"activity_id"`
	RevenueSign string    `gorm:"type:varchar(32)" json:"revenue_sign"` // revenue_gt_expense, expense_gt_revenue, equal
	Description string    `gorm:"type:text" json:"description"`
	SetBy       string    `gorm:"type:varchar(64)" json:"set_by"`
	SetAt       time.Time `json:"set_at"`
	Remark      string    `gorm:"type:text" json:"remark"`
	CreatedBy   string    `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy   string    `gorm:"type:varchar(64)" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PerformanceIndicatorLibrary 部门指标库
type PerformanceIndicatorLibrary struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	DepartmentID    string     `gorm:"type:varchar(64);not null;index" json:"department_id"`
	DepartmentName  string     `gorm:"type:varchar(128);not null" json:"department_name"`
	ParentLibraryID *uint      `gorm:"index" json:"parent_library_id"`
	Name            string     `gorm:"type:varchar(128);not null" json:"name"`
	Description     string     `gorm:"type:text" json:"description"`
	DefaultCycle    string     `gorm:"type:varchar(32)" json:"default_cycle"`
	Status          string     `gorm:"type:varchar(32);not null;default:active" json:"status"` // active, archived
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `gorm:"index" json:"-"`
	CreatedBy       string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy       string     `gorm:"type:varchar(64)" json:"updated_by"`
}

// PerformanceIndicatorItem 指标项
type PerformanceIndicatorItem struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	LibraryID         uint       `gorm:"not null;index" json:"library_id"`
	ParentIndicatorID *uint      `gorm:"index" json:"parent_indicator_id"`
	SectionType       string     `gorm:"type:varchar(32);not null" json:"section_type"` // quantitative, key_action, bonus_penalty
	Name              string     `gorm:"type:varchar(256);not null" json:"name"`
	Description       string     `gorm:"type:text" json:"description"`
	IndicatorType     string     `gorm:"type:varchar(32)" json:"indicator_type"`
	Keywords          []string   `gorm:"type:json;serializer:json" json:"keywords"`
	CalculationMethod string     `gorm:"type:text" json:"calculation_method"`
	DataSource        string     `gorm:"type:varchar(256)" json:"data_source"`
	Cycle             string     `gorm:"type:varchar(32)" json:"cycle"`
	DefaultWeight     float64    `gorm:"default:0" json:"default_weight"`
	RedLineValue      string     `gorm:"type:varchar(256)" json:"red_line_value"`
	TargetValue       string     `gorm:"type:varchar(256)" json:"target_value"`
	ChallengeValue    string     `gorm:"type:varchar(256)" json:"challenge_value"`
	ScoringRule       string     `gorm:"type:text" json:"scoring_rule"`
	Weight            float64    `gorm:"default:0" json:"weight"`
	IsDefault         bool       `gorm:"default:false" json:"is_default"`
	IsInherited       bool       `gorm:"default:false" json:"is_inherited"`
	IsCustomized      bool       `gorm:"default:false" json:"is_customized"`
	SortOrder         int        `gorm:"default:0" json:"sort_order"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `gorm:"index" json:"-"`
	CreatedBy         string     `gorm:"type:varchar(64)" json:"created_by"`
	UpdatedBy         string     `gorm:"type:varchar(64)" json:"updated_by"`
}
