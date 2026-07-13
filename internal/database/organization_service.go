package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// GetOrgIDByCorpID 根据钉钉 corpID 获取组织ID
func GetOrgIDByCorpID(corpID string) (string, error) {
	var org Organization
	if err := DB.Where("corp_id = ? AND status = ?", corpID, "active").First(&org).Error; err != nil {
		return "", fmt.Errorf("organization not found for corp_id=%s: %w", corpID, err)
	}
	return org.OrgID, nil
}

// GetOrganizationByOrgID 根据 orgID 获取组织信息
func GetOrganizationByOrgID(orgID string) (*Organization, error) {
	var org Organization
	if err := DB.Where("org_id = ? AND status = ?", orgID, "active").First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found for org_id=%s: %w", orgID, err)
	}
	return &org, nil
}

// GetOrganizationByCorpID 根据 corpID 获取组织信息
func GetOrganizationByCorpID(corpID string) (*Organization, error) {
	var org Organization
	if err := DB.Where("corp_id = ? AND status = ?", corpID, "active").First(&org).Error; err != nil {
		return nil, fmt.Errorf("organization not found for corp_id=%s: %w", corpID, err)
	}
	return &org, nil
}

// ListActiveOrganizations returns configured organizations that can be used for login.
func ListActiveOrganizations() ([]Organization, error) {
	var orgs []Organization
	if err := DB.Where("status = ?", "active").Order("id ASC").Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

// ListActiveOrganizationsForUser returns the active organizations whose roster contains userID.
func ListActiveOrganizationsForUser(userID string) ([]Organization, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return []Organization{}, nil
	}

	var orgs []Organization
	err := DB.Model(&Organization{}).
		Joins("JOIN organization_users ON organization_users.org_id = organizations.org_id AND organization_users.deleted_at IS NULL").
		Where("organization_users.user_id = ? AND organization_users.status = ? AND organizations.status = ?", userID, "active", "active").
		Order("organizations.id ASC").
		Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// IsUserInOrganization 检查用户是否属于指定组织（通过 OrganizationUser 表）
func IsUserInOrganization(orgID, userID string) (bool, error) {
	var count int64
	err := DB.Model(&OrganizationUser{}).
		Where("org_id = ? AND user_id = ? AND status = ?", orgID, userID, "active").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureOrganizationUser 确保用户在组织中有记录（如果不存在则创建）
func EnsureOrganizationUser(orgID, userID, status string) error {
	orgID = strings.TrimSpace(orgID)
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if orgID == "" || userID == "" {
		return nil
	}
	if status == "" {
		status = "active"
	}

	var existing OrganizationUser
	err := DB.Unscoped().Where("org_id = ? AND user_id = ?", orgID, userID).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		newRecord := &OrganizationUser{
			OrgID:  orgID,
			UserID: userID,
			Status: status,
		}
		return DB.Create(newRecord).Error
	}

	if existing.Status != status || existing.DeletedAt.Valid {
		existing.Status = status
		existing.DeletedAt = gorm.DeletedAt{}
		existing.UpdatedAt = time.Now()
		return DB.Unscoped().Save(&existing).Error
	}
	return nil
}
