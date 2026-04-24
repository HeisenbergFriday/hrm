package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnualLeaveEligibilityRepository struct {
	db *gorm.DB
}

func NewAnnualLeaveEligibilityRepository(db *gorm.DB) *AnnualLeaveEligibilityRepository {
	return &AnnualLeaveEligibilityRepository{db: db}
}

func (r *AnnualLeaveEligibilityRepository) FindByUserYear(userID string, year int) ([]database.AnnualLeaveEligibility, error) {
	var results []database.AnnualLeaveEligibility
	err := r.db.Where("user_id = ? AND year = ?", userID, year).Order("quarter asc").Find(&results).Error
	return results, err
}

func (r *AnnualLeaveEligibilityRepository) FindByUserYearQuarter(userID string, year, quarter int) (*database.AnnualLeaveEligibility, error) {
	var result database.AnnualLeaveEligibility
	err := r.db.Where("user_id = ? AND year = ? AND quarter = ?", userID, year, quarter).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *AnnualLeaveEligibilityRepository) Upsert(e *database.AnnualLeaveEligibility) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "year"}, {Name: "quarter"}},
		DoUpdates: clause.AssignmentColumns([]string{"entry_date", "confirmation_date", "is_eligible", "eligible_start_date", "eligible_end_date", "retroactive_source_quarter", "calc_version", "calc_reason", "updated_at"}),
	}).Create(e).Error
}

func (r *AnnualLeaveEligibilityRepository) FindEligibleByYear(year int) ([]database.AnnualLeaveEligibility, error) {
	var results []database.AnnualLeaveEligibility
	err := r.db.Where("year = ? AND is_eligible = true", year).Find(&results).Error
	return results, err
}
