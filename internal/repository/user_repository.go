package repository

import (
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

type UserRepository struct {
	db    *gorm.DB
	orgID string
}

func applyDepartmentMembershipFilter(query *gorm.DB, orgID string, departmentIDs []string) *gorm.DB {
	return query.Where(`(EXISTS (
		SELECT 1 FROM user_department_memberships udm
		WHERE udm.org_id = ? AND udm.user_id = users.user_id AND udm.department_id IN ?
	) OR (users.department_id IN ? AND NOT EXISTS (
		SELECT 1 FROM user_department_memberships udm_fallback
		WHERE udm_fallback.org_id = ? AND udm_fallback.user_id = users.user_id
	)))`, orgID, departmentIDs, departmentIDs, orgID)
}

// NewUserRepository constructs a user repository bound to the org context on db
// when present. Without an explicit tenant context the repository stays unbound
// and all read/write methods fail closed (1=0 / ErrMissingOrgID).
func NewUserRepository(db *gorm.DB) *UserRepository {
	orgID := ""
	if db != nil {
		if resolved, err := database.RequireOrganizationIDFromDB(db); err == nil {
			orgID = strings.TrimSpace(resolved)
		}
	}
	return &UserRepository{
		db:    db,
		orgID: orgID,
	}
}

// NewUserRepositoryWithOrgID 构造带 org 隔离的用户仓储；orgID 为空时 fail-closed。
func NewUserRepositoryWithOrgID(db *gorm.DB, orgID string) *UserRepository {
	return &UserRepository{
		db:    db,
		orgID: strings.TrimSpace(orgID),
	}
}

func (r *UserRepository) boundOrgID() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.orgID)
}

func (r *UserRepository) requireBoundOrg() (string, error) {
	orgID := r.boundOrgID()
	if orgID == "" {
		return "", ErrMissingOrgID
	}
	return orgID, nil
}

// scoped 返回已应用 org 过滤的查询；空 org 时 fail-closed（1=0），禁止全表扫描。
func (r *UserRepository) scoped() *gorm.DB {
	return r.db.Scopes(ScopeOrg(r.boundOrgID(), "org_id"))
}

// scopedTable 返回已应用 users.org_id 过滤的查询构造器，供 join 场景使用。
func (r *UserRepository) scopedTable() *gorm.DB {
	return r.db.Model(&database.User{}).Scopes(ScopeOrg(r.boundOrgID(), "users.org_id"))
}

func (r *UserRepository) Create(user *database.User) error {
	if user == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	// 严格 tenant 仓储：写入时强制 org 一致。允许调用方留空以继承仓储 org，
	// 但不允许传入其它组织的 org_id，从而防止越权写入。
	merged, err := EnsureSameOrg(orgID, user.OrgID)
	if err != nil {
		return err
	}
	user.OrgID = merged
	db := r.db
	if strings.TrimSpace(user.Mobile) == "" {
		db = db.Omit("mobile")
	}
	return db.Create(user).Error
}

func (r *UserRepository) Update(user *database.User) error {
	if user == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, user.OrgID)
	if err != nil {
		return err
	}
	user.OrgID = merged
	db := r.scoped().Where("id = ?", user.ID)
	if strings.TrimSpace(user.Mobile) == "" {
		db = db.Omit("mobile")
	}
	return db.Save(user).Error
}

func (r *UserRepository) Delete(userID string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	return r.scoped().Delete(&database.User{}, "user_id = ?", userID).Error
}

func (r *UserRepository) FindByUserID(userID string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	tx := r.scoped().Where("user_id = ?", userID).Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *UserRepository) FindByDingTalkUserID(dingTalkUserID string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	tx := r.scoped().Where("ding_talk_user_id = ?", strings.TrimSpace(dingTalkUserID)).Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

// FindByOrgAndUserID 根据组织ID和用户ID查找用户（多租户）
func (r *UserRepository) FindByOrgAndUserID(orgID, userID string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	tx := r.db.Where("org_id = ? AND user_id = ?", orgID, userID).Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(email string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndEmail 根据组织ID和邮箱查找用户（多租户）
func (r *UserRepository) FindByOrgAndEmail(orgID, email string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	err := r.db.Where("org_id = ? AND email = ?", orgID, email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndMobile 根据组织ID和手机号查找用户（多租户）
func (r *UserRepository) FindByOrgAndMobile(orgID, mobile string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	err := r.db.Where("org_id = ? AND mobile = ?", orgID, mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByMobile(mobile string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().Where("mobile = ?", mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindAll(page, pageSize int) ([]database.User, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize

	// 计算总数
	err := r.scopedTable().Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = r.scoped().Offset(offset).Limit(pageSize).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindAllFiltered(page, pageSize int, search string) ([]database.User, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	// 作息表推送等选人场景需要一次拉取更多在职员工；仍保留上限防止误用超大分页。
	if pageSize > 1000 {
		pageSize = 1000
	}

	query := r.scopedTable()
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"users.name LIKE ? OR users.user_id LIKE ? OR users.mobile LIKE ? OR users.email LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []database.User
	err := query.Order("users.name ASC, users.id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&users).Error
	return users, total, err
}

func (r *UserRepository) FindSyncedEmployees(page, pageSize int) ([]database.User, int64, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.org_id = ?", orgID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize

	// 计算总数
	query := applyDepartmentMembershipFilter(r.scopedTable(), orgID, []string{departmentID})
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = applyDepartmentMembershipFilter(r.scopedTable(), orgID, []string{departmentID}).
		Offset(offset).Limit(pageSize).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindSyncedEmployeesByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.org_id = ?", orgID)
	query = applyDepartmentMembershipFilter(query, orgID, []string{departmentID})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// ReplaceDepartmentMemberships 原子替换员工的完整部门归属；departmentIDs[0] 为主部门。
func (r *UserRepository) ReplaceDepartmentMemberships(userID string, departmentIDs []string) error {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return gorm.ErrInvalidData
	}

	uniqueDepartmentIDs := make([]string, 0, len(departmentIDs))
	seen := make(map[string]struct{}, len(departmentIDs))
	for _, departmentID := range departmentIDs {
		departmentID = strings.TrimSpace(departmentID)
		if departmentID == "" {
			continue
		}
		if _, exists := seen[departmentID]; exists {
			continue
		}
		seen[departmentID] = struct{}{}
		uniqueDepartmentIDs = append(uniqueDepartmentIDs, departmentID)
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var userCount int64
		if err := tx.Model(&database.User{}).
			Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, userID).
			Count(&userCount).Error; err != nil {
			return err
		}
		if userCount != 1 {
			return gorm.ErrRecordNotFound
		}
		if len(uniqueDepartmentIDs) > 0 {
			var departmentCount int64
			if err := tx.Model(&database.Department{}).
				Where("org_id = ? AND department_id IN ? AND deleted_at IS NULL", orgID, uniqueDepartmentIDs).
				Distinct("department_id").
				Count(&departmentCount).Error; err != nil {
				return err
			}
			if departmentCount != int64(len(uniqueDepartmentIDs)) {
				return gorm.ErrRecordNotFound
			}
		}
		if err := tx.Where("org_id = ? AND user_id = ?", orgID, userID).
			Delete(&database.UserDepartmentMembership{}).Error; err != nil {
			return err
		}
		if len(uniqueDepartmentIDs) == 0 {
			return nil
		}
		memberships := make([]database.UserDepartmentMembership, 0, len(uniqueDepartmentIDs))
		for index, departmentID := range uniqueDepartmentIDs {
			memberships = append(memberships, database.UserDepartmentMembership{
				OrgID:        orgID,
				UserID:       userID,
				DepartmentID: departmentID,
				IsPrimary:    index == 0,
			})
		}
		return tx.Create(&memberships).Error
	})
}

// DeactivateUsersMissingFromDingTalk 将本次完整钉钉通讯录中已不存在的历史同步用户标为停用。
// 空源列表会 fail-closed，避免第三方异常返回空数据时误停用全员。
func (r *UserRepository) DeactivateUsersMissingFromDingTalk(sourceDingTalkUserIDs []string) ([]string, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, err
	}
	sourceSet := make(map[string]struct{}, len(sourceDingTalkUserIDs))
	for _, userID := range sourceDingTalkUserIDs {
		userID = strings.TrimSpace(userID)
		if userID != "" {
			sourceSet[userID] = struct{}{}
		}
	}
	if len(sourceSet) == 0 {
		return nil, gorm.ErrInvalidData
	}

	var candidates []database.User
	if err := r.db.Where("org_id = ? AND deleted_at IS NULL AND status = ? AND ding_talk_user_id <> ?", orgID, "active", "").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	deactivatedUserIDs := make([]string, 0)
	for _, user := range candidates {
		if _, exists := sourceSet[strings.TrimSpace(user.DingTalkUserID)]; !exists {
			deactivatedUserIDs = append(deactivatedUserIDs, user.UserID)
		}
	}
	if len(deactivatedUserIDs) == 0 {
		return deactivatedUserIDs, nil
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.User{}).
			Where("org_id = ? AND user_id IN ? AND deleted_at IS NULL", orgID, deactivatedUserIDs).
			Update("status", "inactive").Error; err != nil {
			return err
		}
		return tx.Model(&database.EmployeeProfile{}).
			Where("org_id = ? AND user_id IN ? AND deleted_at IS NULL", orgID, deactivatedUserIDs).
			Update("profile_status", "inactive").Error
	})
	return deactivatedUserIDs, err
}
