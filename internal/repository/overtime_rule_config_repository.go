package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type OvertimeRuleConfigRepository struct {
	db *gorm.DB
}

func NewOvertimeRuleConfigRepository(db *gorm.DB) *OvertimeRuleConfigRepository {
	return &OvertimeRuleConfigRepository{db: db}
}

func (r *OvertimeRuleConfigRepository) FindActiveAll() ([]database.OvertimeRuleConfig, error) {
	var configs []database.OvertimeRuleConfig
	err := r.db.Where("status = 'active'").Find(&configs).Error
	return configs, err
}

func (r *OvertimeRuleConfigRepository) FindByKey(ruleKey string) (*database.OvertimeRuleConfig, error) {
	var config database.OvertimeRuleConfig
	err := r.db.Where("rule_key = ? AND status = 'active'", ruleKey).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *OvertimeRuleConfigRepository) Upsert(config *database.OvertimeRuleConfig) error {
	var existing database.OvertimeRuleConfig
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
