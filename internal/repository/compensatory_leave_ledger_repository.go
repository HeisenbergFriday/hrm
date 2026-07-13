package repository

import (
	"fmt"
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type CompensatoryLeaveLedgerRepository struct {
	db    *gorm.DB
	orgID string
}

func NewCompensatoryLeaveLedgerRepository(db *gorm.DB) *CompensatoryLeaveLedgerRepository {
	return &CompensatoryLeaveLedgerRepository{db: db}
}

func NewCompensatoryLeaveLedgerRepositoryWithOrgID(db *gorm.DB, orgID string) *CompensatoryLeaveLedgerRepository {
	return &CompensatoryLeaveLedgerRepository{db: db, orgID: orgID}
}

func (r *CompensatoryLeaveLedgerRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

func (r *CompensatoryLeaveLedgerRepository) FindByUser(userID string) ([]database.CompensatoryLeaveLedger, error) {
	var results []database.CompensatoryLeaveLedger
	err := r.scoped().Where("user_id = ?", userID).Order("created_at asc").Find(&results).Error
	return results, err
}

func (r *CompensatoryLeaveLedgerRepository) FindBySourceMatch(matchID uint) (*database.CompensatoryLeaveLedger, error) {
	var result database.CompensatoryLeaveLedger
	err := r.scoped().Where("source_match_id = ? AND ledger_type = 'credit'", matchID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CompensatoryLeaveLedgerRepository) FindBySourceMatchKey(matchRef string, matchID uint) (*database.CompensatoryLeaveLedger, error) {
	var result database.CompensatoryLeaveLedger
	query := r.scoped().Where("ledger_type = 'credit'")
	if matchRef != "" {
		query = query.Where("source_match_ref = ?", matchRef)
	} else {
		query = query.Where("source_match_id = ?", matchID)
	}
	if err := query.First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CompensatoryLeaveLedgerRepository) GetBalance(userID string) (int, error) {
	var balance int
	err := r.scoped().Model(&database.CompensatoryLeaveLedger{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").
		Scan(&balance).Error
	return balance, err
}

func (r *CompensatoryLeaveLedgerRepository) GetBalanceByUserYearAndSource(userID string, year int, sourceType string) (int, error) {
	var balance int
	startDate := fmt.Sprintf("%04d-01-01", year)
	endDate := fmt.Sprintf("%04d-12-31", year)
	query := r.scoped().Model(&database.CompensatoryLeaveLedger{}).
		Where("user_id = ? AND effective_date >= ? AND effective_date <= ?", userID, startDate, endDate)
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	err := query.Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").Scan(&balance).Error
	return balance, err
}

func (r *CompensatoryLeaveLedgerRepository) Create(ledger *database.CompensatoryLeaveLedger) error {
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, ledger.OrgID)
		if err != nil {
			return err
		}
		ledger.OrgID = merged
	}
	return r.db.Create(ledger).Error
}

func (r *CompensatoryLeaveLedgerRepository) ExistsBySourceMatch(matchID uint) (bool, error) {
	var count int64
	err := r.scoped().Model(&database.CompensatoryLeaveLedger{}).
		Where("source_match_id = ? AND ledger_type = 'credit'", matchID).Count(&count).Error
	return count > 0, err
}

func (r *CompensatoryLeaveLedgerRepository) ExistsBySourceMatchKey(matchRef string, matchID uint) (bool, error) {
	var count int64
	query := r.scoped().Model(&database.CompensatoryLeaveLedger{}).
		Where("ledger_type = 'credit'")
	if matchRef != "" {
		query = query.Where("source_match_ref = ?", matchRef)
	} else {
		query = query.Where("source_match_id = ?", matchID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
