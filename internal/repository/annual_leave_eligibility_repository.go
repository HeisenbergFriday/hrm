package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnualLeaveEligibilityRepository struct {
	db    *gorm.DB
	orgID string
}

func NewAnnualLeaveEligibilityRepository(db *gorm.DB) *AnnualLeaveEligibilityRepository {
	return &AnnualLeaveEligibilityRepository{db: db}
}

func NewAnnualLeaveEligibilityRepositoryWithOrgID(db *gorm.DB, orgID string) *AnnualLeaveEligibilityRepository {
	return &AnnualLeaveEligibilityRepository{db: db, orgID: orgID}
}

func (r *AnnualLeaveEligibilityRepository) scoped() *gorm.DB {
	// Fail-closed: empty org must never return an unfiltered db session.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *AnnualLeaveEligibilityRepository) FindByUserYear(userID string, year int) ([]database.AnnualLeaveEligibility, error) {
	var results []database.AnnualLeaveEligibility
	err := r.scoped().Where("user_id = ? AND year = ?", userID, year).Order("quarter asc").Find(&results).Error
	return results, err
}

func (r *AnnualLeaveEligibilityRepository) FindByUserYearQuarter(userID string, year, quarter int) (*database.AnnualLeaveEligibility, error) {
	var result database.AnnualLeaveEligibility
	err := r.scoped().Where("user_id = ? AND year = ? AND quarter = ?", userID, year, quarter).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *AnnualLeaveEligibilityRepository) Upsert(e *database.AnnualLeaveEligibility) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, e.OrgID)
	if err != nil {
		return err
	}
	e.OrgID = merged
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "user_id"}, {Name: "year"}, {Name: "quarter"}},
		DoUpdates: clause.AssignmentColumns([]string{"entry_date", "confirmation_date", "is_eligible", "eligible_start_date", "eligible_end_date", "retroactive_source_quarter", "calc_version", "calc_reason", "updated_at"}),
	}).Create(e).Error
}

func (r *AnnualLeaveEligibilityRepository) FindEligibleByYear(year int) ([]database.AnnualLeaveEligibility, error) {
	var results []database.AnnualLeaveEligibility
	err := r.scoped().Where("year = ? AND is_eligible = true", year).Find(&results).Error
	return results, err
}
