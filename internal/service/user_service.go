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

func (s *UserService) GetUserByID(id string) (*database.User, error) {
	return s.userRepo.FindByID(id)
}

func (s *UserService) GetUsers(page, pageSize int) ([]database.User, int64, error) {
	return s.userRepo.FindAll(page, pageSize)
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

func (s *UserService) UpdateUserExtension(userID string, extension map[string]interface{}) error {
	user, err := s.userRepo.FindByUserID(userID)
	if err != nil {
		return err
	}

	user.Extension = extension
	return s.userRepo.Update(user)
}
