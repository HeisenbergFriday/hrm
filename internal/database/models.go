package database

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID           uint                   `gorm:"primaryKey" json:"id"`
	UserID       string                 `gorm:"type:varchar(64);unique;not null" json:"user_id"` // 钉钉用户ID
	Name         string                 `gorm:"type:varchar(128);not null" json:"name"`
	Email        string                 `gorm:"type:varchar(128);unique" json:"email"`
	Mobile       string                 `gorm:"type:varchar(32);unique" json:"mobile"`
	Password     string                 `gorm:"type:varchar(256)" json:"-"` // 密码哈希，JSON 不输出
	DepartmentID string                 `gorm:"type:varchar(64);not null" json:"department_id"`
	Position     string                 `gorm:"type:varchar(128)" json:"position"`
	Avatar       string                 `gorm:"type:varchar(256)" json:"avatar"`
	Status       string                 `gorm:"type:varchar(32);not null" json:"status"`
	Extension    map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	DeletedAt    gorm.DeletedAt         `gorm:"index" json:"-"`
}

// Department 部门模型
type Department struct {
	ID           uint                   `gorm:"primaryKey" json:"id"`
	DepartmentID string                 `gorm:"type:varchar(64);unique;not null" json:"department_id"` // 钉钉部门ID
	Name         string                 `gorm:"type:varchar(128);not null" json:"name"`
	ParentID     string                 `gorm:"type:varchar(64)" json:"parent_id"`
	Order        int                    `gorm:"default:0" json:"order"`
	Extension    map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	DeletedAt    gorm.DeletedAt         `gorm:"index" json:"-"`
}

// Attendance 考勤模型
type Attendance struct {
	ID        uint                   `gorm:"primaryKey" json:"id"`
	UserID    string                 `gorm:"type:varchar(64);not null;index:idx_user_time_type,unique" json:"user_id"`
	UserName  string                 `gorm:"type:varchar(128);not null" json:"user_name"`
	CheckTime time.Time              `gorm:"not null;index:idx_user_time_type,unique" json:"check_time"`
	CheckType string                 `gorm:"type:varchar(32);not null;index:idx_user_time_type,unique" json:"check_type"` // 上班/下班
	Location  string                 `gorm:"type:varchar(256)" json:"location"`
	Extension map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	DeletedAt gorm.DeletedAt         `gorm:"index" json:"-"`
}

// Approval 审批模型
type Approval struct {
	ID            uint                   `gorm:"primaryKey" json:"id"`
	ProcessID     string                 `gorm:"type:varchar(64);unique;not null" json:"process_id"` // 钉钉审批流程ID
	Title         string                 `gorm:"type:varchar(256);not null" json:"title"`
	ApplicantID   string                 `gorm:"type:varchar(64);not null" json:"applicant_id"`
	ApplicantName string                 `gorm:"type:varchar(128);not null" json:"applicant_name"`
	Status        string                 `gorm:"type:varchar(32);not null" json:"status"`
	CreateTime    time.Time              `gorm:"not null" json:"create_time"`
	FinishTime    time.Time              `json:"finish_time"`
	Content       map[string]interface{} `gorm:"type:json;serializer:json" json:"content"` // JSON格式
	Extension     map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	DeletedAt     gorm.DeletedAt         `gorm:"index" json:"-"`
}

// ApprovalTemplate 审批模板模型
type ApprovalTemplate struct {
	ID          uint                   `gorm:"primaryKey" json:"id"`
	TemplateID  string                 `gorm:"type:varchar(64);unique;not null" json:"template_id"` // 钉钉模板ID
	Name        string                 `gorm:"type:varchar(128);not null" json:"name"`
	Description string                 `gorm:"type:text" json:"description"`
	Category    string                 `gorm:"type:varchar(64)" json:"category"`            // 模板分类
	Status      string                 `gorm:"type:varchar(32);not null" json:"status"`     // 状态：active, inactive
	FormItems   map[string]interface{} `gorm:"type:json;serializer:json" json:"form_items"` // 表单字段
	FlowNodes   map[string]interface{} `gorm:"type:json;serializer:json" json:"flow_nodes"` // 审批节点
	Extension   map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"-"`
}

// Role 角色模型
type Role struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(64);unique;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// Permission 权限模型
type Permission struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"type:varchar(64);unique;not null" json:"name"`
	Code        string         `gorm:"type:varchar(64);unique;not null" json:"code"`
	Description string         `gorm:"type:text" json:"description"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// RolePermission 角色权限模型
type RolePermission struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RoleID       uint      `gorm:"not null" json:"role_id"`
	PermissionID uint      `gorm:"not null" json:"permission_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserRole 用户角色模型
type UserRole struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"type:varchar(64);not null" json:"user_id"`
	RoleID    uint      `gorm:"not null" json:"role_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OperationLog 操作日志模型
type OperationLog struct {
	ID        uint                   `gorm:"primaryKey" json:"id"`
	UserID    string                 `gorm:"type:varchar(64);not null" json:"user_id"`
	UserName  string                 `gorm:"type:varchar(128);not null" json:"user_name"`
	Operation string                 `gorm:"type:varchar(128);not null" json:"operation"`
	Resource  string                 `gorm:"type:varchar(256);not null" json:"resource"`
	IP        string                 `gorm:"type:varchar(64);not null" json:"ip"`
	Details   map[string]interface{} `gorm:"type:json;serializer:json" json:"details"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// SyncStatus 同步状态模型
type SyncStatus struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Type         string    `gorm:"type:varchar(32);unique;not null" json:"type"`
	LastSyncTime time.Time `json:"last_sync_time"`
	Status       string    `gorm:"type:varchar(32);not null" json:"status"`
	Message      string    `gorm:"type:text" json:"message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DingTalkBinding 钉钉绑定模型
type DingTalkBinding struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         string    `gorm:"type:varchar(64);unique;not null" json:"user_id"`          // 本地用户ID
	DingTalkUserID string    `gorm:"type:varchar(64);unique;not null" json:"dingtalk_user_id"` // 钉钉用户ID
	UnionID        string    `gorm:"type:varchar(64);unique" json:"union_id"`                  // 钉钉UnionID
	OpenID         string    `gorm:"type:varchar(64);unique" json:"open_id"`                   // 钉钉OpenID
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserSession 用户会话模型
type UserSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"type:varchar(64);not null" json:"user_id"`            // 本地用户ID
	SessionID string    `gorm:"type:varchar(128);unique;not null" json:"session_id"` // 会话ID
	Token     string    `gorm:"type:varchar(512);not null" json:"token"`             // JWT token
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`                          // 过期时间
	IP        string    `gorm:"type:varchar(64)" json:"ip"`                          // 登录IP
	UserAgent string    `gorm:"type:varchar(512)" json:"user_agent"`                 // 用户代理
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoginLog 登录日志模型
type LoginLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      string    `gorm:"type:varchar(64)" json:"user_id"`               // 本地用户ID
	UserName    string    `gorm:"type:varchar(128)" json:"user_name"`            // 用户名
	LoginType   string    `gorm:"type:varchar(32);not null" json:"login_type"`   // 登录类型：dingtalk_qr, dingtalk_in_app, dingtalk_account, local
	LoginStatus string    `gorm:"type:varchar(32);not null" json:"login_status"` // 登录状态：success, failed
	IP          string    `gorm:"type:varchar(64);not null" json:"ip"`           // 登录IP
	UserAgent   string    `gorm:"type:varchar(512)" json:"user_agent"`           // 用户代理
	ErrorMsg    string    `gorm:"type:text" json:"error_msg"`                    // 错误信息
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AttendanceExport 考勤导出记录模型
type AttendanceExport struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      string    `gorm:"type:varchar(64);not null" json:"user_id"`    // 导出人ID
	UserName    string    `gorm:"type:varchar(128);not null" json:"user_name"` // 导出人姓名
	FileName    string    `gorm:"type:varchar(256);not null" json:"file_name"` // 文件名
	FilePath    string    `gorm:"type:varchar(512)" json:"file_path"`          // 文件路径
	RecordCount int       `gorm:"default:0" json:"record_count"`               // 导出记录数
	Status      string    `gorm:"type:varchar(32);not null" json:"status"`     // 状态：pending, processing, completed, failed
	ErrorMsg    string    `gorm:"type:text" json:"error_msg"`                  // 错误信息
	StartDate   string    `gorm:"type:varchar(32)" json:"start_date"`          // 开始日期
	EndDate     string    `gorm:"type:varchar(32)" json:"end_date"`            // 结束日期
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EmployeeProfile 员工档案模型（本地业务字段）
type EmployeeProfile struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID string `gorm:"type:varchar(64);unique;not null" json:"user_id"` // 关联钉钉用户ID
	// 基本信息（本地业务字段）
	EmployeeID   string `gorm:"type:varchar(64);unique;not null" json:"employee_id"` // 员工工号
	Gender       string `gorm:"type:varchar(16)" json:"gender"`                      // 性别
	BirthDate    string `gorm:"type:varchar(32)" json:"birth_date"`                  // 出生日期
	Nationality  string `gorm:"type:varchar(64)" json:"nationality"`                 // 国籍
	IDCardNumber string `gorm:"type:varchar(32)" json:"id_card_number"`              // 身份证号
	// 工作信息（本地业务字段）
	EmploymentType     string `gorm:"type:varchar(32)" json:"employment_type"`      // 雇佣类型：全职、兼职、实习
	EntryDate          string `gorm:"type:varchar(32)" json:"entry_date"`           // 入职日期
	ProbationEndDate   string `gorm:"type:varchar(32)" json:"probation_end_date"`   // 试用期结束日期
	PlannedRegularDate string `gorm:"type:varchar(32)" json:"planned_regular_date"` // 计划转正日期
	ActualRegularDate  string `gorm:"type:varchar(32)" json:"actual_regular_date"`  // 实际转正日期
	ContractStartDate  string `gorm:"type:varchar(32)" json:"contract_start_date"`  // 合同开始日期
	ContractEndDate    string `gorm:"type:varchar(32)" json:"contract_end_date"`    // 合同结束日期
	WorkEmail          string `gorm:"type:varchar(128)" json:"work_email"`          // 工作邮箱
	PersonalEmail      string `gorm:"type:varchar(128)" json:"personal_email"`      // 个人邮箱
	EmergencyContact   string `gorm:"type:varchar(128)" json:"emergency_contact"`   // 紧急联系人
	EmergencyPhone     string `gorm:"type:varchar(32)" json:"emergency_phone"`      // 紧急联系电话
	// 教育背景（本地业务字段）
	Education      string `gorm:"type:varchar(64)" json:"education"`        // 学历
	GraduateSchool string `gorm:"type:varchar(256)" json:"graduate_school"` // 毕业院校
	Major          string `gorm:"type:varchar(128)" json:"major"`           // 专业
	GraduationDate string `gorm:"type:varchar(32)" json:"graduation_date"`  // 毕业日期
	// 工作经历（本地业务字段，存储为JSON）
	WorkExperience map[string]interface{} `gorm:"type:json;serializer:json" json:"work_experience"` // 工作经历
	// 技能证书（本地业务字段，存储为JSON）
	Skills map[string]interface{} `gorm:"type:json;serializer:json" json:"skills"` // 技能证书
	// 其他信息（本地业务字段）
	BankAccount string `gorm:"type:varchar(128)" json:"bank_account"` // 银行账号
	BankName    string `gorm:"type:varchar(128)" json:"bank_name"`    // 银行名称
	TaxNumber   string `gorm:"type:varchar(64)" json:"tax_number"`    // 税号
	Address     string `gorm:"type:varchar(256)" json:"address"`      // 地址
	// 状态信息
	ProfileStatus string `gorm:"type:varchar(32);not null;default:active" json:"profile_status"` // 档案状态：active, inactive
	// 扩展字段
	Extension map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"` // 扩展字段
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	DeletedAt gorm.DeletedAt         `gorm:"index" json:"-"`
}

// EmployeeTransfer 员工转岗模型
type EmployeeTransfer struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	TransferID        string         `gorm:"type:varchar(64);unique;not null" json:"transfer_id"`   // 转岗ID
	UserID            string         `gorm:"type:varchar(64);not null" json:"user_id"`              // 员工ID
	UserName          string         `gorm:"type:varchar(128);not null" json:"user_name"`           // 员工姓名
	OldDepartmentID   string         `gorm:"type:varchar(64);not null" json:"old_department_id"`    // 原部门ID
	OldDepartmentName string         `gorm:"type:varchar(128);not null" json:"old_department_name"` // 原部门名称
	OldPosition       string         `gorm:"type:varchar(128);not null" json:"old_position"`        // 原职位
	NewDepartmentID   string         `gorm:"type:varchar(64);not null" json:"new_department_id"`    // 新部门ID
	NewDepartmentName string         `gorm:"type:varchar(128);not null" json:"new_department_name"` // 新部门名称
	NewPosition       string         `gorm:"type:varchar(128);not null" json:"new_position"`        // 新职位
	TransferDate      string         `gorm:"type:varchar(32);not null" json:"transfer_date"`        // 转岗日期
	Reason            string         `gorm:"type:text" json:"reason"`                               // 转岗原因
	Status            string         `gorm:"type:varchar(32);not null" json:"status"`               // 状态：pending, approved, rejected
	ApproverID        string         `gorm:"type:varchar(64)" json:"approver_id"`                   // 审批人ID
	ApproverName      string         `gorm:"type:varchar(128)" json:"approver_name"`                // 审批人姓名
	ApprovalTime      time.Time      `json:"approval_time"`                                         // 审批时间
	ApprovalComment   string         `gorm:"type:text" json:"approval_comment"`                     // 审批意见
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// EmployeeResignation 员工离职模型
type EmployeeResignation struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ResignationID   string    `gorm:"type:varchar(64);unique;not null" json:"resignation_id"` // 离职ID
	UserID          string    `gorm:"type:varchar(64);not null" json:"user_id"`               // 员工ID
	UserName        string    `gorm:"type:varchar(128);not null" json:"user_name"`            // 员工姓名
	DepartmentID    string    `gorm:"type:varchar(64);not null" json:"department_id"`         // 部门ID
	DepartmentName  string    `gorm:"type:varchar(128);not null" json:"department_name"`      // 部门名称
	Position        string    `gorm:"type:varchar(128);not null" json:"position"`             // 职位
	ResignDate      string    `gorm:"type:varchar(32);not null" json:"resign_date"`           // 离职日期
	LastWorkingDay  string    `gorm:"type:varchar(32);not null" json:"last_working_day"`      // 最后工作日
	ResignReason    string    `gorm:"type:text" json:"resign_reason"`                         // 离职原因
	Status          string    `gorm:"type:varchar(32);not null" json:"status"`                // 状态：pending, approved, rejected
	ApproverID      string    `gorm:"type:varchar(64)" json:"approver_id"`                    // 审批人ID
	ApproverName    string    `gorm:"type:varchar(128)" json:"approver_name"`                 // 审批人姓名
	ApprovalTime    time.Time `json:"approval_time"`                                          // 审批时间
	ApprovalComment string    `gorm:"type:text" json:"approval_comment"`                      // 审批意见
	// 离职手续（存储为JSON）
	ExitProcess map[string]interface{} `gorm:"type:json;serializer:json" json:"exit_process"` // 离职手续
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"-"`
}

// EmployeeOnboarding 员工入职模型
type EmployeeOnboarding struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	OnboardingID string `gorm:"type:varchar(64);unique;not null" json:"onboarding_id"` // 入职ID
	// 基本信息
	EmployeeID   string `gorm:"type:varchar(64);unique;not null" json:"employee_id"` // 员工工号
	Name         string `gorm:"type:varchar(128);not null" json:"name"`              // 姓名
	Gender       string `gorm:"type:varchar(16)" json:"gender"`                      // 性别
	BirthDate    string `gorm:"type:varchar(32)" json:"birth_date"`                  // 出生日期
	IDCardNumber string `gorm:"type:varchar(32)" json:"id_card_number"`              // 身份证号
	Mobile       string `gorm:"type:varchar(32)" json:"mobile"`                      // 手机号
	Email        string `gorm:"type:varchar(128)" json:"email"`                      // 邮箱
	// 工作信息
	DepartmentID     string `gorm:"type:varchar(64);not null" json:"department_id"`    // 部门ID
	DepartmentName   string `gorm:"type:varchar(128);not null" json:"department_name"` // 部门名称
	Position         string `gorm:"type:varchar(128);not null" json:"position"`        // 职位
	EntryDate        string `gorm:"type:varchar(32);not null" json:"entry_date"`       // 入职日期
	EmploymentType   string `gorm:"type:varchar(32);not null" json:"employment_type"`  // 雇佣类型
	ProbationEndDate string `gorm:"type:varchar(32)" json:"probation_end_date"`        // 试用期结束日期
	// 其他信息
	EmergencyContact string `gorm:"type:varchar(128)" json:"emergency_contact"` // 紧急联系人
	EmergencyPhone   string `gorm:"type:varchar(32)" json:"emergency_phone"`    // 紧急联系电话
	Education        string `gorm:"type:varchar(64)" json:"education"`          // 学历
	GraduateSchool   string `gorm:"type:varchar(256)" json:"graduate_school"`   // 毕业院校
	Major            string `gorm:"type:varchar(128)" json:"major"`             // 专业
	// 入职流程状态（存储为JSON）
	OnboardingProcess map[string]interface{} `gorm:"type:json;serializer:json" json:"onboarding_process"` // 入职流程
	Status            string                 `gorm:"type:varchar(32);not null" json:"status"`             // 状态：pending, processing, completed
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	DeletedAt         gorm.DeletedAt         `gorm:"index" json:"-"`
}

// TalentAnalysis 人才分析模型
type TalentAnalysis struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	UserID         string `gorm:"type:varchar(64);unique;not null" json:"user_id"`   // 员工ID
	UserName       string `gorm:"type:varchar(128);not null" json:"user_name"`       // 员工姓名
	DepartmentID   string `gorm:"type:varchar(64);not null" json:"department_id"`    // 部门ID
	DepartmentName string `gorm:"type:varchar(128);not null" json:"department_name"` // 部门名称
	Position       string `gorm:"type:varchar(128);not null" json:"position"`        // 职位
	// 绩效评估
	PerformanceScore  float64 `gorm:"default:0" json:"performance_score"`        // 绩效得分
	PerformanceLevel  string  `gorm:"type:varchar(32)" json:"performance_level"` // 绩效等级
	PerformanceReview string  `gorm:"type:text" json:"performance_review"`       // 绩效评价
	// 技能评估
	SkillsAssessment map[string]interface{} `gorm:"type:json;serializer:json" json:"skills_assessment"` // 技能评估
	// 潜力评估
	PotentialScore float64 `gorm:"default:0" json:"potential_score"`        // 潜力得分
	PotentialLevel string  `gorm:"type:varchar(32)" json:"potential_level"` // 潜力等级
	// 培训记录
	TrainingRecords map[string]interface{} `gorm:"type:json;serializer:json" json:"training_records"` // 培训记录
	// 晋升记录
	PromotionRecords map[string]interface{} `gorm:"type:json;serializer:json" json:"promotion_records"` // 晋升记录
	// 离职风险
	TurnoverRiskScore float64 `gorm:"default:0" json:"turnover_risk_score"`        // 离职风险得分
	TurnoverRiskLevel string  `gorm:"type:varchar(32)" json:"turnover_risk_level"` // 离职风险等级
	// 分析时间
	AnalysisDate string `gorm:"type:varchar(32);not null" json:"analysis_date"` // 分析日期
	// 扩展字段
	Extension map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"` // 扩展字段
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	DeletedAt gorm.DeletedAt         `gorm:"index" json:"-"`
}

// EmployeeShiftConfig 员工自定义下班时间配置（本地存储，同步到钉钉时生效）
type EmployeeShiftConfig struct {
	gorm.Model
	UserID   string `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	UserName string `gorm:"type:varchar(128)" json:"user_name"`
	ShiftID  int64  `gorm:"not null" json:"shift_id"`         // 钉钉班次ID
	EndTime  string `gorm:"type:varchar(16)" json:"end_time"` // 显示用，如 "17:30"
	Note     string `gorm:"type:varchar(256)" json:"note"`
}

// DingTalkShiftCatalog stores local name -> shift ID mappings to avoid repeated DingTalk API calls.
type DingTalkShiftCatalog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"type:varchar(128);not null;index:idx_dingtalk_shift_catalogs_name" json:"name"`
	ShiftKey  string    `gorm:"type:varchar(256);uniqueIndex:idx_dingtalk_shift_catalogs_shift_key;not null" json:"shift_key"` // 稳定签名: normalize(name, check_in, check_out)
	ShiftID   int64     `gorm:"not null" json:"shift_id"`
	CheckIn   string    `gorm:"type:varchar(16)" json:"check_in"`
	CheckOut  string    `gorm:"type:varchar(16)" json:"check_out"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DingTalkShiftCatalog) TableName() string {
	return "dingtalk_shift_catalogs"
}

// ===================== 大小周管理 =====================

// WeekScheduleRule 大小周规则配置
type WeekScheduleRule struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	ScopeType string         `gorm:"type:varchar(32);not null;index:idx_scope,unique" json:"scope_type"` // company/department/user
	ScopeID   string         `gorm:"type:varchar(64);not null;index:idx_scope,unique" json:"scope_id"`   // 空=全公司, 部门ID, 用户ID
	ScopeName string         `gorm:"type:varchar(128)" json:"scope_name"`                                // 显示名称
	BaseDate  string         `gorm:"type:varchar(32);not null" json:"base_date"`                         // 基准日期(某个大周的周一) 2006-01-02
	Pattern   string         `gorm:"type:varchar(32);not null" json:"pattern"`                           // big_first: 基准周为大周
	ShiftID   int64          `gorm:"default:0" json:"shift_id"`                                          // 钉钉班次ID，0=使用默认班次
	Status    string         `gorm:"type:varchar(32);default:active" json:"status"`                      // active/inactive
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// WeekScheduleOverride 大小周手动覆盖（针对特定周）
type WeekScheduleOverride struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ScopeType     string    `gorm:"type:varchar(32);not null;index:idx_scope_date,unique" json:"scope_type"`
	ScopeID       string    `gorm:"type:varchar(64);not null;index:idx_scope_date,unique" json:"scope_id"`
	WeekStartDate string    `gorm:"type:varchar(32);not null;index:idx_scope_date,unique" json:"week_start_date"` // 该周的周一日期
	WeekType      string    `gorm:"type:varchar(16);not null" json:"week_type"`                                   // big/small
	Reason        string    `gorm:"type:varchar(256)" json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// WeekScheduleSyncLog 大小周同步日志
type WeekScheduleSyncLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SyncType   string    `gorm:"type:varchar(32);not null" json:"sync_type"` // to_dingtalk/from_dingtalk
	TargetDate string    `gorm:"type:varchar(32)" json:"target_date"`        // 同步的目标周六日期
	UserCount  int       `gorm:"default:0" json:"user_count"`                // 影响人数
	Status     string    `gorm:"type:varchar(32);not null" json:"status"`    // success/failed
	Message    string    `gorm:"type:text" json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// StatutoryHoliday 法定节假日/调休上班日
type StatutoryHoliday struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Date      string    `gorm:"type:varchar(32);uniqueIndex;not null" json:"date"` // 日期 2006-01-02
	Name      string    `gorm:"type:varchar(128);not null" json:"name"`            // 节假日名称，如"国庆节"、"国庆调休上班"
	Type      string    `gorm:"type:varchar(32);not null" json:"type"`             // holiday=放假, workday=调休上班
	Year      int       `gorm:"not null" json:"year"`                              // 年份，方便按年查询
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ===================== 年假与调休模块 =====================

// LeaveRuleConfig 年假规则配置
type LeaveRuleConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	RuleType      string    `gorm:"type:varchar(32);not null;index" json:"rule_type"`       // eligibility / grant
	RuleKey       string    `gorm:"type:varchar(64);not null" json:"rule_key"`              // 规则唯一键
	RuleName      string    `gorm:"type:varchar(128);not null" json:"rule_name"`            // 规则名称
	RuleValueJSON string    `gorm:"type:json;not null" json:"rule_value_json"`              // 规则内容（JSON字符串）
	Status        string    `gorm:"type:varchar(32);not null;default:active" json:"status"` // active / inactive
	EffectiveFrom string    `gorm:"type:varchar(32)" json:"effective_from"`                 // 生效开始日期
	EffectiveTo   string    `gorm:"type:varchar(32)" json:"effective_to"`                   // 生效结束日期
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// AnnualLeaveEligibility 年假资格（按员工+年+季度持久化）
type AnnualLeaveEligibility struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	UserID                   string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_leave_elig_user_year_q" json:"user_id"`
	Year                     int       `gorm:"not null;uniqueIndex:idx_leave_elig_user_year_q" json:"year"`
	Quarter                  int       `gorm:"not null;uniqueIndex:idx_leave_elig_user_year_q" json:"quarter"` // 1-4
	EntryDate                string    `gorm:"type:varchar(32)" json:"entry_date"`
	ConfirmationDate         string    `gorm:"type:varchar(32)" json:"confirmation_date"`
	IsEligible               bool      `gorm:"not null;default:false" json:"is_eligible"`
	EligibleStartDate        string    `gorm:"type:varchar(32)" json:"eligible_start_date"`
	EligibleEndDate          string    `gorm:"type:varchar(32)" json:"eligible_end_date"`
	RetroactiveSourceQuarter int       `gorm:"default:0" json:"retroactive_source_quarter"` // 追溯来源季度，0表示无
	CalcVersion              string    `gorm:"type:varchar(32)" json:"calc_version"`
	CalcReason               string    `gorm:"type:text" json:"calc_reason"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// AnnualLeaveGrant 年假发放台账
type AnnualLeaveGrant struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	UserID              string     `gorm:"type:varchar(64);not null;index:idx_leave_grant_user_year" json:"user_id"`
	Year                int        `gorm:"not null;index:idx_leave_grant_user_year" json:"year"`
	Quarter             int        `gorm:"not null" json:"quarter"` // 1-4
	WorkingYears        float64    `gorm:"not null;default:0" json:"working_years"`
	BaseDays            float64    `gorm:"not null;default:0" json:"base_days"`
	GrantedDays         float64    `gorm:"not null;default:0" json:"granted_days"`
	RetroactiveDays     float64    `gorm:"default:0" json:"retroactive_days"`
	UsedDays            float64    `gorm:"default:0" json:"used_days"`
	RemainingDays       float64    `gorm:"default:0" json:"remaining_days"`
	GrantType           string     `gorm:"type:varchar(32);not null" json:"grant_type"` // normal / retroactive / adjustment
	SourceEligibilityID uint       `gorm:"default:0" json:"source_eligibility_id"`
	Remark              string     `gorm:"type:text" json:"remark"`
	DingTalkSyncStatus  string     `gorm:"column:dingtalk_sync_status;type:varchar(32);default:pending" json:"dingtalk_sync_status"` // pending / success / failed / skipped
	DingTalkSyncError   string     `gorm:"column:dingtalk_sync_error;type:text" json:"dingtalk_sync_error"`
	DingTalkSyncedAt    *time.Time `gorm:"column:dingtalk_synced_at" json:"dingtalk_synced_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// OvertimeRuleConfig 加班规则配置
type OvertimeRuleConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	RuleKey       string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"rule_key"`
	RuleName      string    `gorm:"type:varchar(128);not null" json:"rule_name"`
	RuleValueJSON string    `gorm:"type:json;not null" json:"rule_value_json"`
	Status        string    `gorm:"type:varchar(32);not null;default:active" json:"status"`
	EffectiveFrom string    `gorm:"type:varchar(32)" json:"effective_from"`
	EffectiveTo   string    `gorm:"type:varchar(32)" json:"effective_to"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OvertimeMatchResult 加班审批与考勤匹配结果
type OvertimeMatchResult struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"type:varchar(64);not null;index" json:"user_id"`
	ApprovalID          uint      `gorm:"not null;uniqueIndex" json:"approval_id"`
	ApprovalProcessID   string    `gorm:"type:varchar(64)" json:"approval_process_id"`
	ApprovalStatus      string    `gorm:"type:varchar(32)" json:"approval_status"`
	ApprovalStartTime   time.Time `json:"approval_start_time"`
	ApprovalEndTime     time.Time `json:"approval_end_time"`
	AttendanceStartTime time.Time `json:"attendance_start_time"`
	AttendanceEndTime   time.Time `json:"attendance_end_time"`
	MatchedMinutes      int       `gorm:"default:0" json:"matched_minutes"`
	QualifiedMinutes    int       `gorm:"default:0" json:"qualified_minutes"`
	MatchStatus         string    `gorm:"type:varchar(32);not null" json:"match_status"` // matched / partial / unmatched / rolled_back
	MatchReason         string    `gorm:"type:text" json:"match_reason"`
	CalcVersion         string    `gorm:"type:varchar(32)" json:"calc_version"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CompensatoryLeaveLedger 调休余额台账
type CompensatoryLeaveLedger struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         string    `gorm:"type:varchar(64);not null;index:idx_comp_leave_user_date" json:"user_id"`
	SourceType     string    `gorm:"type:varchar(32);not null" json:"source_type"` // overtime
	SourceMatchID  uint      `gorm:"default:0" json:"source_match_id"`
	CreditMinutes  int       `gorm:"default:0" json:"credit_minutes"`
	DebitMinutes   int       `gorm:"default:0" json:"debit_minutes"`
	BalanceMinutes int       `gorm:"default:0" json:"balance_minutes"`
	LedgerType     string    `gorm:"type:varchar(32);not null" json:"ledger_type"` // credit / debit / rollback / adjustment
	EffectiveDate  string    `gorm:"type:varchar(32);index:idx_comp_leave_user_date" json:"effective_date"`
	ExpireDate     string    `gorm:"type:varchar(32)" json:"expire_date"`
	Remark         string    `gorm:"type:text" json:"remark"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AnnualLeaveConsumeLog 年假消费台账（防重复，FIFO扣减记录）
type AnnualLeaveConsumeLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      string    `gorm:"type:varchar(64);not null;index" json:"user_id"`
	GrantID     uint      `gorm:"not null;index" json:"grant_id"`   // 对应的发放记录
	ApprovalRef string    `gorm:"type:varchar(128);uniqueIndex" json:"approval_ref"` // 审批ID，防重复消费
	Days        float64   `gorm:"not null;default:0" json:"days"`
	Remark      string    `gorm:"type:text" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
