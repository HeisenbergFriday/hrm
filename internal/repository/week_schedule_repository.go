package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeekScheduleRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

type WeekScheduleScope struct {
	ScopeType string
	ScopeID   string
}

func NewWeekScheduleRepository(db *gorm.DB) *WeekScheduleRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &WeekScheduleRepository{db: db, orgID: orgID, orgErr: err}
}

func NewWeekScheduleRepositoryWithOrgID(db *gorm.DB, orgID string) *WeekScheduleRepository {
	normalized, err := RequireOrgID(orgID)
	return &WeekScheduleRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *WeekScheduleRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func rowsAffectedOrNotFound(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ===================== ?? CRUD =====================

func (r *WeekScheduleRepository) CreateRule(rule *database.WeekScheduleRule) error {
	if rule == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, rule.OrgID)
	if err != nil {
		return err
	}
	rule.OrgID = merged
	return r.db.Create(rule).Error
}

func (r *WeekScheduleRepository) UpdateRule(rule *database.WeekScheduleRule) error {
	if rule == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, rule.OrgID)
	if err != nil {
		return err
	}
	rule.OrgID = merged
	return rowsAffectedOrNotFound(r.db.Model(&database.WeekScheduleRule{}).
		Where("org_id = ? AND id = ?", orgID, rule.ID).Updates(rule))
}

func (r *WeekScheduleRepository) DeleteRule(id uint) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(r.db.Unscoped().Where("org_id = ? AND id = ?", orgID, id).
		Delete(&database.WeekScheduleRule{}))
}

func (r *WeekScheduleRepository) FindRuleByID(id uint) (*database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rule database.WeekScheduleRule
	err = r.db.Where("org_id = ? AND id = ?", orgID, id).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *WeekScheduleRepository) FindRuleByScope(scopeType, scopeID string) (*database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rule database.WeekScheduleRule
	err = r.db.Where("org_id = ? AND scope_type = ? AND scope_id = ? AND status = ?", orgID, scopeType, scopeID, "active").First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *WeekScheduleRepository) FindAllRules() ([]database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rules []database.WeekScheduleRule
	err = r.db.Where("org_id = ?", orgID).Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRules() ([]database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rules []database.WeekScheduleRule
	err = r.db.Where("org_id = ? AND status = ?", orgID, "active").Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRulesByScopes(scopes []WeekScheduleScope) ([]database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rules []database.WeekScheduleRule
	if len(scopes) == 0 {
		return rules, nil
	}
	query := r.db.Where("org_id = ? AND status = ?", orgID, "active")
	query = applyWeekScheduleScopeFilter(query, scopes)
	err = query.Order("id ASC").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRulesByUserIDs(userIDs []string) ([]database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rules []database.WeekScheduleRule
	err = r.db.Where("org_id = ? AND scope_type = ? AND scope_id IN ? AND status = ?", orgID, "user", userIDs, "active").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRulesWithShift() ([]database.WeekScheduleRule, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var rules []database.WeekScheduleRule
	err = r.db.Where("org_id = ? AND status = ? AND shift_id > 0", orgID, "active").Find(&rules).Error
	return rules, err
}

// ===================== ?? CRUD =====================

func (r *WeekScheduleRepository) CreateOverride(override *database.WeekScheduleOverride) error {
	if override == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, override.OrgID)
	if err != nil {
		return err
	}
	override.OrgID = merged
	return r.db.Create(override).Error
}

func (r *WeekScheduleRepository) UpdateOverride(override *database.WeekScheduleOverride) error {
	if override == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, override.OrgID)
	if err != nil {
		return err
	}
	override.OrgID = merged
	return rowsAffectedOrNotFound(r.db.Model(&database.WeekScheduleOverride{}).
		Where("org_id = ? AND id = ?", orgID, override.ID).Updates(override))
}

func (r *WeekScheduleRepository) DeleteOverride(id uint) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(r.db.Where("org_id = ? AND id = ?", orgID, id).
		Delete(&database.WeekScheduleOverride{}))
}

func (r *WeekScheduleRepository) FindOverrideByID(id uint) (*database.WeekScheduleOverride, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var override database.WeekScheduleOverride
	err = r.db.Where("org_id = ? AND id = ?", orgID, id).First(&override).Error
	if err != nil {
		return nil, err
	}
	return &override, nil
}

func (r *WeekScheduleRepository) FindOverride(scopeType, scopeID, weekStartDate string) (*database.WeekScheduleOverride, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var override database.WeekScheduleOverride
	err = r.db.Where("org_id = ? AND scope_type = ? AND scope_id = ? AND week_start_date = ?", orgID, scopeType, scopeID, weekStartDate).First(&override).Error
	if err != nil {
		return nil, err
	}
	return &override, nil
}

func (r *WeekScheduleRepository) FindOverridesByScope(scopeType, scopeID string) ([]database.WeekScheduleOverride, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var overrides []database.WeekScheduleOverride
	err = r.db.Where("org_id = ? AND scope_type = ? AND scope_id = ?", orgID, scopeType, scopeID).Order("week_start_date ASC").Find(&overrides).Error
	return overrides, err
}

func (r *WeekScheduleRepository) FindOverridesByDateRange(startDate, endDate string) ([]database.WeekScheduleOverride, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var overrides []database.WeekScheduleOverride
	err = r.db.Where("org_id = ? AND week_start_date >= ? AND week_start_date <= ?", orgID, startDate, endDate).Order("week_start_date ASC").Find(&overrides).Error
	return overrides, err
}

func (r *WeekScheduleRepository) FindOverridesByWeekStartDates(scopes []WeekScheduleScope, weekStartDates []string) ([]database.WeekScheduleOverride, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var overrides []database.WeekScheduleOverride
	if len(scopes) == 0 || len(weekStartDates) == 0 {
		return overrides, nil
	}
	query := r.db.Where("org_id = ? AND week_start_date IN ?", orgID, weekStartDates)
	query = applyWeekScheduleScopeFilter(query, scopes)
	err = query.Order("week_start_date ASC, id ASC").Find(&overrides).Error
	return overrides, err
}

// ===================== ???? =====================

func (r *WeekScheduleRepository) CreateSyncLog(log *database.WeekScheduleSyncLog) error {
	if log == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, log.OrgID)
	if err != nil {
		return err
	}
	log.OrgID = merged
	return r.db.Create(log).Error
}

func (r *WeekScheduleRepository) FindSyncLogs(page, pageSize int) ([]database.WeekScheduleSyncLog, int64, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, 0, err
	}
	var logs []database.WeekScheduleSyncLog
	var total int64
	query := r.db.Model(&database.WeekScheduleSyncLog{}).Where("org_id = ?", orgID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ===================== ????? =====================

func (r *WeekScheduleRepository) CreateHoliday(holiday *database.StatutoryHoliday) error {
	if holiday == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, holiday.OrgID)
	if err != nil {
		return err
	}
	holiday.OrgID = merged
	return r.db.Create(holiday).Error
}

func (r *WeekScheduleRepository) UpsertHoliday(holiday *database.StatutoryHoliday) error {
	if holiday == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, holiday.OrgID)
	if err != nil {
		return err
	}
	holiday.OrgID = merged
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "type", "year", "updated_at"}),
	}).Create(holiday).Error
}

func (r *WeekScheduleRepository) UpdateHoliday(holiday *database.StatutoryHoliday) error {
	if holiday == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, holiday.OrgID)
	if err != nil {
		return err
	}
	holiday.OrgID = merged
	return rowsAffectedOrNotFound(r.db.Model(&database.StatutoryHoliday{}).
		Where("org_id = ? AND id = ?", orgID, holiday.ID).Updates(holiday))
}

func (r *WeekScheduleRepository) DeleteHoliday(id uint) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(r.db.Where("org_id = ? AND id = ?", orgID, id).
		Delete(&database.StatutoryHoliday{}))
}

func (r *WeekScheduleRepository) FindHolidayByDate(date string) (*database.StatutoryHoliday, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var holiday database.StatutoryHoliday
	err = r.db.Where("org_id = ? AND date = ?", orgID, date).First(&holiday).Error
	if err != nil {
		return nil, err
	}
	return &holiday, nil
}

func (r *WeekScheduleRepository) FindHolidaysByYear(year int) ([]database.StatutoryHoliday, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var holidays []database.StatutoryHoliday
	err = r.db.Where("org_id = ? AND year = ?", orgID, year).Order("date ASC").Find(&holidays).Error
	return holidays, err
}

func (r *WeekScheduleRepository) FindHolidaysByYears(years []int) ([]database.StatutoryHoliday, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var holidays []database.StatutoryHoliday
	if len(years) == 0 {
		return holidays, nil
	}
	err = r.db.Where("org_id = ? AND year IN ?", orgID, years).Order("date ASC").Find(&holidays).Error
	return holidays, err
}

func (r *WeekScheduleRepository) FindHolidaysByDateRange(startDate, endDate string) ([]database.StatutoryHoliday, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var holidays []database.StatutoryHoliday
	err = r.db.Where("org_id = ? AND date >= ? AND date <= ?", orgID, startDate, endDate).Order("date ASC").Find(&holidays).Error
	return holidays, err
}

func applyWeekScheduleScopeFilter(query *gorm.DB, scopes []WeekScheduleScope) *gorm.DB {
	clauses := make([]string, 0, len(scopes))
	args := make([]interface{}, 0, len(scopes)*2)
	for _, scope := range scopes {
		clauses = append(clauses, "(scope_type = ? AND scope_id = ?)")
		args = append(args, scope.ScopeType, scope.ScopeID)
	}
	return query.Where(strings.Join(clauses, " OR "), args...)
}
