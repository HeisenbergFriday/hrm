package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShiftConfigRepository struct {
	db *gorm.DB
}

func NewShiftConfigRepository(db *gorm.DB) *ShiftConfigRepository {
	return &ShiftConfigRepository{db: db}
}

func (r *ShiftConfigRepository) FindAll() ([]database.EmployeeShiftConfig, error) {
	var configs []database.EmployeeShiftConfig
	err := r.db.Find(&configs).Error
	return configs, err
}

func (r *ShiftConfigRepository) FindByUserID(userID string) (*database.EmployeeShiftConfig, error) {
	var config database.EmployeeShiftConfig
	if err := r.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

// Upsert 创建或更新（按 user_id 唯一键）
func (r *ShiftConfigRepository) Upsert(config *database.EmployeeShiftConfig) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_name", "shift_id", "end_time", "note", "updated_at"}),
	}).Create(config).Error
}

func (r *ShiftConfigRepository) DeleteByUserID(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&database.EmployeeShiftConfig{}).Error
}
