package database

import (
	"time"

	"gorm.io/gorm"
)

// Organization 企业/组织模型（多租户核心表）
type Organization struct {
	ID          uint                   `gorm:"primaryKey" json:"id"`
	OrgID       string                 `gorm:"type:varchar(64);uniqueIndex;not null" json:"org_id"` // 组织标识（如 xiaotie, muteng）
	Name        string                 `gorm:"type:varchar(128);not null" json:"name"`              // 组织名称
	CorpID      string                 `gorm:"type:varchar(128);uniqueIndex;not null" json:"corp_id"` // 钉钉企业ID
	Status      string                 `gorm:"type:varchar(32);not null;default:'active'" json:"status"` // active, inactive
	AppKey      string                 `gorm:"type:varchar(128)" json:"app_key"`                    // 钉钉应用Key
	AppSecret   string                 `gorm:"type:varchar(256)" json:"-"`                          // 钉钉应用Secret（不输出到JSON）
	AgentID     string                 `gorm:"type:varchar(64)" json:"agent_id"`                    // 钉钉AgentID
	AppHomeURL  string                 `gorm:"type:varchar(256)" json:"app_home_url"`               // 应用首页URL
	RedirectURI string                 `gorm:"type:varchar(256)" json:"redirect_uri"`               // OAuth回调地址
	Extension   map[string]interface{} `gorm:"type:json;serializer:json" json:"extension"`          // 扩展字段
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"-"`
}

// OrganizationUser 组织-用户关系表（用户可能属于多个组织）
type OrganizationUser struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	OrgID     string         `gorm:"type:varchar(64);not null;index:idx_org_user,unique" json:"org_id"` // 组织ID
	UserID    string         `gorm:"type:varchar(64);not null;index:idx_org_user,unique" json:"user_id"` // 用户钉钉ID
	Status    string         `gorm:"type:varchar(32);not null;default:'active'" json:"status"` // active, inactive
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
