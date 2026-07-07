package database

import (
	"fmt"
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
