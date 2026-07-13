package repository

import (
	"errors"
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type LeaveRuleConfigRepository struct {
	db    *gorm.DB
	orgID string
}

func NewLeaveRuleConfigRepository(db *gorm.DB) *LeaveRuleConfigRepository {
	return &LeaveRuleConfigRepository{db: db}
}

func NewLeaveRuleConfigRepositoryWithOrgID(db *gorm.DB, orgID string) *LeaveRuleConfigRepository {
	return &LeaveRuleConfigRepository{db: db, orgID: orgID}
}

func (r *LeaveRuleConfigRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

func (r *LeaveRuleConfigRepository) FindActiveByType(ruleType string) ([]database.LeaveRuleConfig, error) {
	var configs []database.LeaveRuleConfig
	err := r.scoped().Where("rule_type = ? AND status = 'active'", ruleType).Find(&configs).Error
	return configs, err
}

func (r *LeaveRuleConfigRepository) FindByKey(ruleKey string) (*database.LeaveRuleConfig, error) {
	var config database.LeaveRuleConfig
	err := r.scoped().Where("rule_key = ?", ruleKey).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *LeaveRuleConfigRepository) Upsert(config *database.LeaveRuleConfig) error {
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, config.OrgID)
		if err != nil {
			return err
		}
		config.OrgID = merged
	}
	var existing database.LeaveRuleConfig
	err := r.scoped().Where("rule_key = ?", config.RuleKey).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(config).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"rule_type":       config.RuleType,
		"rule_name":       config.RuleName,
		"rule_value_json": config.RuleValueJSON,
		"status":          config.Status,
		"effective_from":  config.EffectiveFrom,
		"effective_to":    config.EffectiveTo,
	}).Error
}
