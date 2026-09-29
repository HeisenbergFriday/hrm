package database

import (
	"time"

	"gorm.io/gorm"
)

// OrganizationSyncLink defines a one-way mirror from a source organization
// (文娱) into a target organization (沐腾). The target is a read-only mirror in
// the first phase; it does not change the source business records.
type OrganizationSyncLink struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	SourceOrgID    string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_sync_link_source_target,priority:1;index" json:"source_org_id"`
	TargetOrgID    string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_sync_link_source_target,priority:2;index" json:"target_org_id"`
	Status         string         `gorm:"type:varchar(32);not null;default:active;index" json:"status"` // active / paused
	EmployeeSync   bool           `gorm:"not null;default:true" json:"employee_sync"`
	BusinessSync   bool           `gorm:"not null;default:true" json:"business_sync"`
	BusinessScopes string         `gorm:"type:text" json:"business_scopes"` // comma-separated allow-list
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// OrganizationSyncRun records one idempotent mirror execution.
type OrganizationSyncRun struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	RequestID     string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_sync_run_request;index" json:"request_id"`
	SourceOrgID   string     `gorm:"type:varchar(64);not null;index:idx_org_sync_run_source_target,priority:1" json:"source_org_id"`
	TargetOrgID   string     `gorm:"type:varchar(64);not null;index:idx_org_sync_run_source_target,priority:2" json:"target_org_id"`
	Status        string     `gorm:"type:varchar(32);not null;index" json:"status"` // running / success / partial / failed
	TriggeredBy   string     `gorm:"type:varchar(128);index" json:"triggered_by"`
	EmployeeCount int        `gorm:"not null;default:0" json:"employee_count"`
	BusinessCount int        `gorm:"not null;default:0" json:"business_count"`
	FailureCount  int        `gorm:"not null;default:0" json:"failure_count"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// OrganizationEmployeeMirror is a read-only employee snapshot in the target
// organization. SourceUserID is the stable identity; target users are never
// created from this row in phase one.
type OrganizationEmployeeMirror struct {
	ID               uint                   `gorm:"primaryKey" json:"id"`
	TargetOrgID      string                 `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_employee_mirror_source_user,priority:1;index" json:"target_org_id"`
	SourceOrgID      string                 `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_employee_mirror_source_user,priority:2;index" json:"source_org_id"`
	SourceUserID     string                 `gorm:"type:varchar(128);not null;uniqueIndex:idx_org_employee_mirror_source_user,priority:3" json:"source_user_id"`
	SourceDingTalkID string                 `gorm:"type:varchar(128);index" json:"source_dingtalk_id"`
	EmployeeID       string                 `gorm:"type:varchar(128);index" json:"employee_id"`
	Name             string                 `gorm:"type:varchar(128);not null" json:"name"`
	Email            string                 `gorm:"type:varchar(128)" json:"email"`
	Mobile           string                 `gorm:"type:varchar(32)" json:"mobile"`
	DepartmentID     string                 `gorm:"type:varchar(128);index" json:"department_id"`
	DepartmentName   string                 `gorm:"type:varchar(128)" json:"department_name"`
	Position         string                 `gorm:"type:varchar(128)" json:"position"`
	Status           string                 `gorm:"type:varchar(32);not null;index" json:"status"`
	Profile          map[string]interface{} `gorm:"type:json;serializer:json" json:"profile"`
	SourceUpdatedAt  *time.Time             `json:"source_updated_at,omitempty"`
	LastSyncedAt     time.Time              `json:"last_synced_at"`
	LastSyncRunID    uint                   `gorm:"index" json:"last_sync_run_id"`
	SyncStatus       string                 `gorm:"type:varchar(32);not null;default:success;index" json:"sync_status"`
	SyncError        string                 `gorm:"type:text" json:"sync_error"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// OrganizationBusinessMirror stores allow-listed business rows as immutable
// source-shaped JSON. Keeping a source key and source org prevents duplicate
// rows and avoids pretending that mirrored data is executable target business.
type OrganizationBusinessMirror struct {
	ID              uint                   `gorm:"primaryKey" json:"id"`
	TargetOrgID     string                 `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_business_mirror_source_key,priority:1;index" json:"target_org_id"`
	SourceOrgID     string                 `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_business_mirror_source_key,priority:2;index" json:"source_org_id"`
	EntityType      string                 `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_business_mirror_source_key,priority:3;index" json:"entity_type"`
	SourceKey       string                 `gorm:"type:varchar(128);not null;uniqueIndex:idx_org_business_mirror_source_key,priority:4" json:"source_key"`
	SourceUserID    string                 `gorm:"type:varchar(128);index" json:"source_user_id"`
	Payload         map[string]interface{} `gorm:"type:json;serializer:json" json:"payload"`
	SourceUpdatedAt *time.Time             `json:"source_updated_at,omitempty"`
	LastSyncedAt    time.Time              `json:"last_synced_at"`
	LastSyncRunID   uint                   `gorm:"index" json:"last_sync_run_id"`
	SyncStatus      string                 `gorm:"type:varchar(32);not null;default:success;index" json:"sync_status"`
	SyncError       string                 `gorm:"type:text" json:"sync_error"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}
