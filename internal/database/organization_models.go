package database

import (
	"time"

	"gorm.io/gorm"
)

// OrganizationUser 组织-用户关系表（用户可能属于多个组织）
type OrganizationUser struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	OrgID     string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_user,priority:1" json:"org_id"`  // 组织ID
	UserID    string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_org_user,priority:2" json:"user_id"` // 用户钉钉ID
	Status    string         `gorm:"type:varchar(32);not null;default:'active'" json:"status"`                     // active, inactive
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
