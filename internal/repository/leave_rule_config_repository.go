package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type LeaveRuleConfigRepository struct {
	db *gorm.DB
}

func NewLeaveRuleConfigRepository(db *gorm.DB) *LeaveRuleConfigRepository {
	return &LeaveRuleConfigRepository{db: db}
}

func (r *LeaveRuleConfigRepository) FindActiveByType(ruleType string) ([]database.LeaveRuleConfig, error) {
	var configs []database.LeaveRuleConfig
	err := r.db.Where("rule_type = ? AND status = 'active'", ruleType).Find(&configs).Error
	return configs, err
}

func (r *LeaveRuleConfigRepository) FindByKey(ruleKey string) (*database.LeaveRuleConfig, error) {
	var config database.LeaveRuleConfig
	err := r.db.Where("rule_key = ?", ruleKey).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *LeaveRuleConfigRepository) Upsert(config *database.LeaveRuleConfig) error {
	var existing database.LeaveRuleConfig
	err := r.db.Where("rule_key = ?", config.RuleKey).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(config).Error
	}
	if err != nil {
		return err
	}
	config.ID = existing.ID
	config.CreatedAt = existing.CreatedAt
	return r.db.Save(config).Error
}
