package repository

import (
	"errors"
	"peopleops/internal/database"
	"strings"

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
	// Fail-closed: empty org must never return an unfiltered db session.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
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
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, config.OrgID)
	if err != nil {
		return err
	}
	config.OrgID = merged
	var existing database.LeaveRuleConfig
	err = r.scoped().Where("rule_key = ?", config.RuleKey).First(&existing).Error
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
