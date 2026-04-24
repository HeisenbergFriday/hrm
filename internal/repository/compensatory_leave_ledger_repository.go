package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type CompensatoryLeaveLedgerRepository struct {
	db *gorm.DB
}

func NewCompensatoryLeaveLedgerRepository(db *gorm.DB) *CompensatoryLeaveLedgerRepository {
	return &CompensatoryLeaveLedgerRepository{db: db}
}

func (r *CompensatoryLeaveLedgerRepository) FindByUser(userID string) ([]database.CompensatoryLeaveLedger, error) {
	var results []database.CompensatoryLeaveLedger
	err := r.db.Where("user_id = ?", userID).Order("created_at asc").Find(&results).Error
	return results, err
}

func (r *CompensatoryLeaveLedgerRepository) FindBySourceMatch(matchID uint) (*database.CompensatoryLeaveLedger, error) {
	var result database.CompensatoryLeaveLedger
	err := r.db.Where("source_match_id = ? AND ledger_type = 'credit'", matchID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *CompensatoryLeaveLedgerRepository) GetBalance(userID string) (int, error) {
	var balance int
	err := r.db.Model(&database.CompensatoryLeaveLedger{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").
		Scan(&balance).Error
	return balance, err
}

func (r *CompensatoryLeaveLedgerRepository) Create(ledger *database.CompensatoryLeaveLedger) error {
	return r.db.Create(ledger).Error
}

func (r *CompensatoryLeaveLedgerRepository) ExistsBySourceMatch(matchID uint) (bool, error) {
	var count int64
	err := r.db.Model(&database.CompensatoryLeaveLedger{}).
		Where("source_match_id = ? AND ledger_type = 'credit'", matchID).Count(&count).Error
	return count > 0, err
}
