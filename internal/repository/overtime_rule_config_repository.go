package repository

import (
	"errors"
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type OvertimeRuleConfigRepository struct {
	db    *gorm.DB
	orgID string
}

func NewOvertimeRuleConfigRepository(db *gorm.DB) *OvertimeRuleConfigRepository {
	return &OvertimeRuleConfigRepository{db: db}
}

func NewOvertimeRuleConfigRepositoryWithOrgID(db *gorm.DB, orgID string) *OvertimeRuleConfigRepository {
	return &OvertimeRuleConfigRepository{db: db, orgID: orgID}
}

func (r *OvertimeRuleConfigRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

func (r *OvertimeRuleConfigRepository) FindActiveAll() ([]database.OvertimeRuleConfig, error) {
	var configs []database.OvertimeRuleConfig
	err := r.scoped().Where("status = 'active'").Find(&configs).Error
	return configs, err
}

func (r *OvertimeRuleConfigRepository) FindByKey(ruleKey string) (*database.OvertimeRuleConfig, error) {
	var config database.OvertimeRuleConfig
	err := r.scoped().Where("rule_key = ? AND status = 'active'", ruleKey).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *OvertimeRuleConfigRepository) Upsert(config *database.OvertimeRuleConfig) error {
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, config.OrgID)
		if err != nil {
			return err
		}
		config.OrgID = merged
	}
	var existing database.OvertimeRuleConfig
	err := r.scoped().Where("rule_key = ?", config.RuleKey).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(config).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"rule_name":       config.RuleName,
		"rule_value_json": config.RuleValueJSON,
		"status":          config.Status,
		"effective_from":  config.EffectiveFrom,
		"effective_to":    config.EffectiveTo,
	}).Error
}
