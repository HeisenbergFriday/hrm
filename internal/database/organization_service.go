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

// ListSwitchableOrganizationsForUser returns organizations that satisfy both
// sides of the organization-switch boundary:
//  1. the local users table has a configured active account for the same
//     DingTalk identity in that organization; and
//  2. organization_users confirms that identity is an active DingTalk member
//     of the organization.
//
// The current organization is included by the caller so an account with no
// additional membership still has a stable current context.
func ListSwitchableOrganizationsForUser(user *User) ([]Organization, error) {
	if user == nil {
		return []Organization{}, nil
	}
	identity := organizationIdentitySetForUser(user)
	if identity.empty() {
		return []Organization{}, nil
	}

	var orgs []Organization
	query := DB.Model(&Organization{}).
		Joins("JOIN organization_users ou ON ou.org_id = organizations.org_id AND ou.deleted_at IS NULL").
		Joins("JOIN users target_users ON target_users.org_id = organizations.org_id AND target_users.deleted_at IS NULL AND target_users.status = ?", "active").
		Joins("LEFT JOIN ding_talk_bindings target_bindings ON target_bindings.org_id = target_users.org_id AND target_bindings.user_id = target_users.user_id").
		Where("organizations.status = ? AND organizations.deleted_at IS NULL", "active").
		Where("ou.user_id = target_users.ding_talk_user_id OR ou.user_id = target_users.user_id").
		Where("ou.status = ?", "active")
	query = query.Where(identitySQLCondition(identity), identitySQLArgs(identity)...)
	err := query.Distinct("organizations.id, organizations.org_id, organizations.name, organizations.corp_id, organizations.status, organizations.created_at, organizations.updated_at").Order("organizations.id ASC").Find(&orgs).Error
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

// FindSwitchableUser resolves the local target-org account after the
// intersection check. It deliberately does not trust a client-supplied user
// id, and returns only an active user in the target organization.
func FindSwitchableUser(user *User, targetOrgID string) (*User, error) {
	if user == nil {
		return nil, gorm.ErrRecordNotFound
	}
	targetOrgID = strings.TrimSpace(targetOrgID)
	if targetOrgID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	identity := organizationIdentitySetForUser(user)
	if identity.empty() {
		return nil, gorm.ErrRecordNotFound
	}

	var target User
	query := DB.Table("users AS target_users").
		Joins("LEFT JOIN ding_talk_bindings target_bindings ON target_bindings.org_id = target_users.org_id AND target_bindings.user_id = target_users.user_id").
		Where("target_users.org_id = ? AND target_users.status = ? AND target_users.deleted_at IS NULL", targetOrgID, "active").
		Where("(target_users.ding_talk_user_id IN (SELECT ou.user_id FROM organization_users ou WHERE ou.org_id = target_users.org_id AND ou.status = ? AND ou.deleted_at IS NULL) OR target_users.user_id IN (SELECT ou.user_id FROM organization_users ou WHERE ou.org_id = target_users.org_id AND ou.status = ? AND ou.deleted_at IS NULL))", "active", "active").
		Where(identitySQLCondition(identity), identitySQLArgs(identity)...).
		Select("target_users.*")
	err := query.First(&target).Error
	if err != nil {
		return nil, err
	}

	var membership OrganizationUser
	if err := DB.Where("org_id = ? AND (user_id = ? OR user_id = ?) AND status = ? AND deleted_at IS NULL", targetOrgID, target.DingTalkUserID, target.UserID, "active").First(&membership).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

type organizationIdentitySet struct {
	dingTalkUserIDs []string
	unionIDs        []string
	openIDs         []string
}

func (s organizationIdentitySet) empty() bool {
	return len(s.dingTalkUserIDs) == 0 && len(s.unionIDs) == 0 && len(s.openIDs) == 0
}

func organizationIdentitySetForUser(user *User) organizationIdentitySet {
	identity := organizationIdentitySet{}
	if user == nil {
		return identity
	}
	if value := strings.TrimSpace(user.DingTalkUserID); value != "" {
		identity.dingTalkUserIDs = append(identity.dingTalkUserIDs, value)
	}
	if DB == nil {
		return identity
	}
	var bindings []DingTalkBinding
	if err := DB.Where("org_id = ? AND user_id = ?", user.OrgID, user.UserID).Find(&bindings).Error; err != nil {
		return identity
	}
	for _, binding := range bindings {
		if value := strings.TrimSpace(binding.DingTalkUserID); value != "" && !containsString(identity.dingTalkUserIDs, value) {
			identity.dingTalkUserIDs = append(identity.dingTalkUserIDs, value)
		}
		if value := strings.TrimSpace(binding.UnionID); value != "" && !containsString(identity.unionIDs, value) {
			identity.unionIDs = append(identity.unionIDs, value)
		}
		if value := strings.TrimSpace(binding.OpenID); value != "" && !containsString(identity.openIDs, value) {
			identity.openIDs = append(identity.openIDs, value)
		}
	}
	return identity
}

func identitySQLCondition(identity organizationIdentitySet) string {
	conditions := make([]string, 0, 3)
	if len(identity.dingTalkUserIDs) > 0 {
		conditions = append(conditions, "target_users.ding_talk_user_id IN ?")
	}
	if len(identity.unionIDs) > 0 {
		conditions = append(conditions, "target_bindings.union_id IN ?")
	}
	if len(identity.openIDs) > 0 {
		conditions = append(conditions, "target_bindings.open_id IN ?")
	}
	// This expression is used with GORM placeholders supplied separately by
	// identitySQLArgs; keeping a non-empty predicate here is fail-closed.
	if len(conditions) == 0 {
		return "1 = 0"
	}
	return "(" + strings.Join(conditions, " OR ") + ")"
}

func identitySQLArgs(identity organizationIdentitySet) []interface{} {
	args := make([]interface{}, 0, 3)
	if len(identity.dingTalkUserIDs) > 0 {
		args = append(args, identity.dingTalkUserIDs)
	}
	if len(identity.unionIDs) > 0 {
		args = append(args, identity.unionIDs)
	}
	if len(identity.openIDs) > 0 {
		args = append(args, identity.openIDs)
	}
	return args
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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
