package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{
		userRepo: repository.NewUserRepository(db),
	}
}

// NewUserServiceWithOrgID 多租户构造：所有仓储调用自动追加 org 过滤。
func NewUserServiceWithOrgID(db *gorm.DB, orgID string) *UserService {
	return &UserService{
		userRepo: repository.NewUserRepositoryWithOrgID(db, orgID),
	}
}

func (s *UserService) CreateUser(user *database.User) error {
	return s.userRepo.Create(user)
}

func (s *UserService) UpdateUser(user *database.User) error {
	return s.userRepo.Update(user)
}

func (s *UserService) DeleteUser(userID string) error {
	return s.userRepo.Delete(userID)
}

func (s *UserService) GetUserByUserID(userID string) (*database.User, error) {
	return s.userRepo.FindByUserID(userID)
}

func (s *UserService) GetUserByDingTalkUserID(dingTalkUserID string) (*database.User, error) {
	return s.userRepo.FindByDingTalkUserID(dingTalkUserID)
}

// GetUserByOrgAndUserID 根据组织ID和用户ID获取用户（多租户）
func (s *UserService) GetUserByOrgAndUserID(orgID, userID string) (*database.User, error) {
	return s.userRepo.FindByOrgAndUserID(orgID, userID)
}

func (s *UserService) GetUserByEmail(email string) (*database.User, error) {
	return s.userRepo.FindByEmail(email)
}

// GetUserByOrgAndEmail 根据组织ID和邮箱获取用户（多租户）
func (s *UserService) GetUserByOrgAndEmail(orgID, email string) (*database.User, error) {
	return s.userRepo.FindByOrgAndEmail(orgID, email)
}

// GetUserByOrgAndMobile 根据组织ID和手机号获取用户（多租户）
func (s *UserService) GetUserByOrgAndMobile(orgID, mobile string) (*database.User, error) {
	return s.userRepo.FindByOrgAndMobile(orgID, mobile)
}

func (s *UserService) GetUserByMobile(mobile string) (*database.User, error) {
	return s.userRepo.FindByMobile(mobile)
}

func (s *UserService) GetUserByID(id string) (*database.User, error) {
	return s.userRepo.FindByID(id)
}

func (s *UserService) GetUsers(page, pageSize int) ([]database.User, int64, error) {
	return s.userRepo.FindAll(page, pageSize)
}

func (s *UserService) SearchUsers(page, pageSize int, search string) ([]database.User, int64, error) {
	return s.userRepo.FindAllFiltered(page, pageSize, search)
}

func (s *UserService) GetUsersByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	return s.userRepo.FindByDepartment(departmentID, page, pageSize)
}

func (s *UserService) GetSyncedEmployees(page, pageSize int) ([]database.User, int64, error) {
	return s.userRepo.FindSyncedEmployees(page, pageSize)
}

func (s *UserService) GetSyncedEmployeesByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	return s.userRepo.FindSyncedEmployeesByDepartment(departmentID, page, pageSize)
}

func (s *UserService) ReplaceDepartmentMemberships(userID string, departmentIDs []string) error {
	return s.userRepo.ReplaceDepartmentMemberships(userID, departmentIDs)
}

func (s *UserService) DeactivateUsersMissingFromDingTalk(sourceDingTalkUserIDs []string) ([]string, error) {
	return s.userRepo.DeactivateUsersMissingFromDingTalk(sourceDingTalkUserIDs)
}

func (s *UserService) UpdateUserExtension(userID string, extension map[string]interface{}) error {
	user, err := s.userRepo.FindByUserID(userID)
	if err != nil {
		return err
	}

	user.Extension = extension
	return s.userRepo.Update(user)
}
