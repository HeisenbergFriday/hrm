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
	Weight            float64    `gorm:"default:0" json:"weight"`                       // 权重（百分比）
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
	Weight      float64    `gorm:"default:0" json:"weight"` // 在 section 内的权重
	SortOrder   int        `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
}

type PerformanceActivity struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Name       string `gorm:"type:varchar(128);not null;index" json:"name"`
	CycleType  string `gorm:"type:varchar(32);not null" json:"cycle_type"` // monthly, quarterly, annual
	StartDate  string `gorm:"type:varchar(32);not null" json:"start_date"`
	EndDate    string `gorm:"type:varchar(32);not null" json:"end_date"`
	TemplateID *uint  `gorm:"index" json:"template_id"` // 关联绩效模板

	SelfEvalStartAt      string `gorm:"type:varchar(32);not null" json:"self_eval_start_at"`
	SelfEvalEndAt        string `gorm:"type:varchar(32);not null" json:"self_eval_end_at"`
	ManagerEvalStartAt   string `gorm:"type:varchar(32);not null" json:"manager_eval_start_at"`
	ManagerEvalEndAt     string `gorm:"type:varchar(32);not null" json:"manager_eval_end_at"`
	ResultConfirmStartAt string `gorm:"type:varchar(32);not null" json:"result_confirm_start_at"`
	ResultConfirmEndAt   string `gorm:"type:varchar(32);not null" json:"result_confirm_end_at"`

	Status      string `gorm:"type:varchar(32);not null;index" json:"status"` // draft, self_evaluation, manager_evaluation, result_confirmed, archived
	Description string `gorm:"type:text" json:"description"`

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
	Level          string `gorm:"type:varchar(32)" json:"level"`           // 职级
	EmployeeStatus string `gorm:"type:varchar(32)" json:"employee_status"` // active, inactive, exited

	ManagerID   *string `gorm:"type:varchar(64)" json:"manager_id"`    // 直属主管钉钉 UserID
	ManagerName *string `gorm:"type:varchar(128)" json:"manager_name"` // 直属主管姓名

	Status string `gorm:"type:varchar(32);not null;index" json:"status"` // pending, self_submitted, manager_submitted, result_confirmed, inactive, removed_from_scope

	SelfScore   float64 `gorm:"default:0" json:"self_score"`
	SelfLevel   string  `gorm:"type:varchar(32)" json:"self_level"`
	SelfSummary string  `gorm:"type:text" json:"self_summary"`

	ManagerScore   float64 `gorm:"default:0" json:"manager_score"`
	ManagerComment string  `gorm:"type:text" json:"manager_comment"`
	SuggestedLevel string  `gorm:"type:varchar(32)" json:"suggested_level"`
	FinalLevel     string  `gorm:"type:varchar(32)" json:"final_level"`
	AdjustReason   string  `gorm:"type:text" json:"adjust_reason"`

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

	ChangeType string    `gorm:"type:varchar(64);not null;index" json:"change_type"` // manager_changed, department_changed, status_changed
	FieldName  string    `gorm:"type:varchar(64)" json:"field_name"`
	OldValue   string    `gorm:"type:text" json:"old_value"`
	NewValue   string    `gorm:"type:text" json:"new_value"`
	ChangedAt  time.Time `json:"changed_at"`
	Source     string    `gorm:"type:varchar(64)" json:"source"` // refresh_participants, manual
	CreatedBy  string    `gorm:"type:varchar(64)" json:"created_by"`

	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}
