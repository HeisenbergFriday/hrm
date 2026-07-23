package database

import "time"

// External attendance staging / sync models (local MySQL only).
// Doris tables are never AutoMigrated.

// ExternalAttendanceRaw stores one Doris attendance row in staging form.
type ExternalAttendanceRaw struct {
	ID                       uint       `gorm:"primaryKey" json:"id"`
	OrgID                    string     `gorm:"size:64;not null;uniqueIndex:uk_ext_att_org_row,priority:1;index:idx_ext_att_org_work,priority:1;index:idx_ext_att_org_src_updated,priority:1;index:idx_ext_att_org_user_work,priority:1" json:"org_id"`
	SourceTable              string     `gorm:"size:128;not null" json:"source_table"`
	SourceRowKey             string     `gorm:"size:64;not null;uniqueIndex:uk_ext_att_org_row,priority:2" json:"source_row_key"`
	ExternalUserID           string     `gorm:"size:64;not null;index:idx_ext_att_org_user_work,priority:2" json:"external_user_id"`
	LocalUserID              string     `gorm:"size:128;not null;index:idx_ext_att_org_user_work,priority:3" json:"local_user_id"`
	CorpID                   string     `gorm:"size:128" json:"corp_id"`
	CorpName                 string     `gorm:"size:256" json:"corp_name"`
	WorkDate                 string     `gorm:"size:32;index:idx_ext_att_org_work,priority:2;index:idx_ext_att_org_user_work,priority:4" json:"work_date"`
	RecordID                 string     `gorm:"size:128" json:"record_id"`
	CheckType                string     `gorm:"size:64" json:"check_type"`
	UserCheckTime            *time.Time `json:"user_check_time"`
	PlanCheckTime            *time.Time `json:"plan_check_time"`
	PlanID                   string     `gorm:"size:128" json:"plan_id"`
	ProcInstID               string     `gorm:"size:128" json:"proc_inst_id"`
	SourceType               string     `gorm:"size:64" json:"source_type"`
	TimeResult               string     `gorm:"size:64" json:"time_result"`
	LocationResult           string     `gorm:"size:64" json:"location_result"`
	GroupID                  string     `gorm:"size:64" json:"group_id"`
	UserAddress              string     `gorm:"size:512" json:"user_address"`
	RawJSON                  string     `gorm:"type:longtext" json:"raw_json"`
	ApproveListJSON          string     `gorm:"type:longtext" json:"approve_list_json"`
	CheckRecordListJSON      string     `gorm:"type:longtext" json:"check_record_list_json"`
	AttendanceResultListJSON string     `gorm:"type:longtext" json:"attendance_result_list_json"`
	ClassSettingInfoJSON     string     `gorm:"type:longtext" json:"class_setting_info_json"`
	UserAttendanceInfoJSON   string     `gorm:"type:longtext" json:"user_attendance_info_json"`
	SourceUpdatedAt          time.Time  `gorm:"not null;index:idx_ext_att_org_src_updated,priority:2" json:"source_updated_at"`
	SyncStatus               string     `gorm:"size:32;not null;default:pending" json:"sync_status"` // pending|applied|partial|failed
	ApplyError               string     `gorm:"size:512" json:"apply_error"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

func (ExternalAttendanceRaw) TableName() string { return "external_attendance_raw" }

// ExternalAttendanceApproveLink stores approve_list items without creating Approval rows.
// Unique key uses non-null ItemKey so nullable BeginTime cannot create duplicate rows.
type ExternalAttendanceApproveLink struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	OrgID        string     `gorm:"size:64;not null;uniqueIndex:uk_ext_appr_item,priority:1;index:idx_ext_appr_proc,priority:1" json:"org_id"`
	SourceRowKey string     `gorm:"size:64;not null;uniqueIndex:uk_ext_appr_item,priority:2" json:"source_row_key"`
	ItemKey      string     `gorm:"size:64;not null;uniqueIndex:uk_ext_appr_item,priority:3" json:"item_key"`
	ProcInstID   string     `gorm:"size:128;not null;index:idx_ext_appr_proc,priority:2" json:"proc_inst_id"`
	TagName      string     `gorm:"size:64" json:"tag_name"`
	BizType      string     `gorm:"size:64" json:"biz_type"`
	SubType      string     `gorm:"size:64" json:"sub_type"`
	BeginTime    *time.Time `json:"begin_time"`
	EndTime      *time.Time `json:"end_time"`
	Duration     string     `gorm:"size:64" json:"duration"`
	DurationUnit string     `gorm:"size:32" json:"duration_unit"`
	GmtFinished  *time.Time `json:"gmt_finished"`
	RawItemJSON  string     `gorm:"type:longtext" json:"raw_item_json"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (ExternalAttendanceApproveLink) TableName() string { return "external_attendance_approve_links" }

// ExternalUserDepartmentRaw is staging for Doris department relation rows.
// External column is spelled deparment_*; local fields use department_* naming.
type ExternalUserDepartmentRaw struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	OrgID                    string    `gorm:"size:64;not null;uniqueIndex:uk_ext_dept_org_row,priority:1;index:idx_ext_dept_org_user,priority:1" json:"org_id"`
	SourceRowKey             string    `gorm:"size:64;not null;uniqueIndex:uk_ext_dept_org_row,priority:2" json:"source_row_key"`
	ExternalUserID           string    `gorm:"size:64;not null;index:idx_ext_dept_org_user,priority:2" json:"external_user_id"`
	LocalUserID              string    `gorm:"size:128;not null;index:idx_ext_dept_org_user,priority:3" json:"local_user_id"`
	CorpName                 string    `gorm:"size:256" json:"corp_name"`
	UserName                 string    `gorm:"size:128" json:"user_name"`
	Title                    string    `gorm:"size:128" json:"title"`
	WorkPlace                string    `gorm:"size:256" json:"work_place"`
	DepartmentID             string    `gorm:"size:64;not null" json:"department_id"` // from deparment_id
	DepartmentName           string    `gorm:"size:256" json:"department_name"`
	DepartmentLevel          string    `gorm:"size:64" json:"department_level"`
	FirstLevelDepartmentID   string    `gorm:"size:64" json:"first_level_department_id"`
	FirstLevelDepartmentName string    `gorm:"size:256" json:"first_level_department_name"`
	SourceUpdatedAt          time.Time `gorm:"not null" json:"source_updated_at"`
	SnapshotID               string    `gorm:"size:64;index:idx_ext_dept_snapshot,priority:1" json:"snapshot_id"`
	SyncStatus               string    `gorm:"size:32;not null;default:pending" json:"sync_status"`
	ApplyError               string    `gorm:"size:512" json:"apply_error"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (ExternalUserDepartmentRaw) TableName() string { return "external_user_department_raw" }

// UserDepartmentRelation is the multi-department business table.
// Phase-1 does NOT overwrite users.department_id.
type UserDepartmentRelation struct {
	ID                       uint       `gorm:"primaryKey" json:"id"`
	OrgID                    string     `gorm:"size:64;not null;uniqueIndex:uk_user_dept_rel,priority:1;index:idx_user_dept_org_user,priority:1" json:"org_id"`
	UserID                   string     `gorm:"size:128;not null;uniqueIndex:uk_user_dept_rel,priority:2;index:idx_user_dept_org_user,priority:2" json:"user_id"`
	ExternalUserID           string     `gorm:"size:64;not null" json:"external_user_id"`
	DepartmentID             string     `gorm:"size:64;not null;uniqueIndex:uk_user_dept_rel,priority:3" json:"department_id"`
	DepartmentName           string     `gorm:"size:256" json:"department_name"`
	DepartmentLevel          string     `gorm:"size:64" json:"department_level"`
	FirstLevelDepartmentID   string     `gorm:"size:64" json:"first_level_department_id"`
	FirstLevelDepartmentName string     `gorm:"size:256" json:"first_level_department_name"`
	Title                    string     `gorm:"size:128" json:"title"`
	WorkPlace                string     `gorm:"size:256" json:"work_place"`
	IsActive                 bool       `gorm:"not null;default:true" json:"is_active"`
	SourceUpdatedAt          time.Time  `json:"source_updated_at"`
	LastSeenSnapshotID       string     `gorm:"size:64" json:"last_seen_snapshot_id"`
	LastSeenAt               *time.Time `json:"last_seen_at"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

func (UserDepartmentRelation) TableName() string { return "user_department_relations" }

// ExternalSyncCursor tracks high-water marks per org + source table.
type ExternalSyncCursor struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	OrgID            string     `gorm:"size:64;not null;uniqueIndex:uk_ext_sync_cursor,priority:1" json:"org_id"`
	SourceTable      string     `gorm:"size:128;not null;uniqueIndex:uk_ext_sync_cursor,priority:2" json:"source_table"`
	CursorTime       *time.Time `json:"cursor_time"`
	CursorTieKey     string     `gorm:"size:128" json:"cursor_tie_key"`
	LastSuccessAt    *time.Time `json:"last_success_at"`
	LastErrorSummary string     `gorm:"size:512" json:"last_error_summary"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (ExternalSyncCursor) TableName() string { return "external_sync_cursors" }

// ExternalSyncJob records one manual/cron sync run.
type ExternalSyncJob struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	OrgID          string     `gorm:"size:64;not null;index:idx_ext_sync_job_org_started,priority:1" json:"org_id"`
	Trigger        string     `gorm:"size:32;not null" json:"trigger"` // manual|cron
	Source         string     `gorm:"size:64;not null" json:"source"`  // attendance|department|all
	Status         string     `gorm:"size:32;not null" json:"status"`  // running|success|partial|failed
	StartedAt      time.Time  `gorm:"not null;index:idx_ext_sync_job_org_started,priority:2" json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CursorFrom     *time.Time `json:"cursor_from"`
	CursorTo       *time.Time `json:"cursor_to"`
	Inserted       int        `gorm:"not null;default:0" json:"inserted"`
	Updated        int        `gorm:"not null;default:0" json:"updated"`
	Skipped        int        `gorm:"not null;default:0" json:"skipped"`
	Failed         int        `gorm:"not null;default:0" json:"failed"`
	ErrorSummary   string     `gorm:"size:1024" json:"error_summary"`
	OperatorUserID string     `gorm:"size:128" json:"operator_user_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (ExternalSyncJob) TableName() string { return "external_sync_jobs" }

// ExternalSyncLock provides DB-level mutex for multi-instance deployments.
// Phase-1 serializes ALL external sync for an org under a single ScopeKey
// ("external-attendance"). Source (attendance|department|all) is recorded on
// external_sync_jobs only — not on the lock row.
type ExternalSyncLock struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     string    `gorm:"size:64;not null;uniqueIndex:uk_ext_sync_lock,priority:1" json:"org_id"`
	ScopeKey  string    `gorm:"size:64;not null;uniqueIndex:uk_ext_sync_lock,priority:2" json:"scope_key"`
	Owner     string    `gorm:"size:128;not null" json:"owner"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ExternalSyncLock) TableName() string { return "external_sync_locks" }
