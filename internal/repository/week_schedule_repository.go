package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeekScheduleRepository struct {
	db *gorm.DB
}

func NewWeekScheduleRepository(db *gorm.DB) *WeekScheduleRepository {
	return &WeekScheduleRepository{db: db}
}

// ===================== 规则 CRUD =====================

func (r *WeekScheduleRepository) CreateRule(rule *database.WeekScheduleRule) error {
	return r.db.Create(rule).Error
}

func (r *WeekScheduleRepository) UpdateRule(rule *database.WeekScheduleRule) error {
	return r.db.Save(rule).Error
}

func (r *WeekScheduleRepository) DeleteRule(id uint) error {
	return r.db.Unscoped().Delete(&database.WeekScheduleRule{}, id).Error
}

func (r *WeekScheduleRepository) FindRuleByID(id uint) (*database.WeekScheduleRule, error) {
	var rule database.WeekScheduleRule
	err := r.db.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *WeekScheduleRepository) FindRuleByScope(scopeType, scopeID string) (*database.WeekScheduleRule, error) {
	var rule database.WeekScheduleRule
	err := r.db.Where("scope_type = ? AND scope_id = ? AND status = ?", scopeType, scopeID, "active").First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *WeekScheduleRepository) FindAllRules() ([]database.WeekScheduleRule, error) {
	var rules []database.WeekScheduleRule
	err := r.db.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRules() ([]database.WeekScheduleRule, error) {
	var rules []database.WeekScheduleRule
	err := r.db.Where("status = ?", "active").Order("created_at DESC").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRulesByUserIDs(userIDs []string) ([]database.WeekScheduleRule, error) {
	var rules []database.WeekScheduleRule
	err := r.db.Where("scope_type = ? AND scope_id IN ? AND status = ?", "user", userIDs, "active").Find(&rules).Error
	return rules, err
}

func (r *WeekScheduleRepository) FindActiveRulesWithShift() ([]database.WeekScheduleRule, error) {
	var rules []database.WeekScheduleRule
	err := r.db.Where("status = ? AND shift_id > 0", "active").Find(&rules).Error
	return rules, err
}

// ===================== 覆盖 CRUD =====================

func (r *WeekScheduleRepository) CreateOverride(override *database.WeekScheduleOverride) error {
	return r.db.Create(override).Error
}

func (r *WeekScheduleRepository) DeleteOverride(id uint) error {
	return r.db.Delete(&database.WeekScheduleOverride{}, id).Error
}

func (r *WeekScheduleRepository) FindOverrideByID(id uint) (*database.WeekScheduleOverride, error) {
	var override database.WeekScheduleOverride
	err := r.db.First(&override, id).Error
	if err != nil {
		return nil, err
	}
	return &override, nil
}

func (r *WeekScheduleRepository) FindOverride(scopeType, scopeID, weekStartDate string) (*database.WeekScheduleOverride, error) {
	var override database.WeekScheduleOverride
	err := r.db.Where("scope_type = ? AND scope_id = ? AND week_start_date = ?", scopeType, scopeID, weekStartDate).First(&override).Error
	if err != nil {
		return nil, err
	}
	return &override, nil
}

func (r *WeekScheduleRepository) FindOverridesByScope(scopeType, scopeID string) ([]database.WeekScheduleOverride, error) {
	var overrides []database.WeekScheduleOverride
	err := r.db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).Order("week_start_date ASC").Find(&overrides).Error
	return overrides, err
}

func (r *WeekScheduleRepository) FindOverridesByDateRange(startDate, endDate string) ([]database.WeekScheduleOverride, error) {
	var overrides []database.WeekScheduleOverride
	err := r.db.Where("week_start_date >= ? AND week_start_date <= ?", startDate, endDate).Order("week_start_date ASC").Find(&overrides).Error
	return overrides, err
}

// ===================== 同步日志 =====================

func (r *WeekScheduleRepository) CreateSyncLog(log *database.WeekScheduleSyncLog) error {
	return r.db.Create(log).Error
}

func (r *WeekScheduleRepository) FindSyncLogs(page, pageSize int) ([]database.WeekScheduleSyncLog, int64, error) {
	var logs []database.WeekScheduleSyncLog
	var total int64

	if err := r.db.Model(&database.WeekScheduleSyncLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := r.db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// ===================== 法定节假日 =====================

func (r *WeekScheduleRepository) CreateHoliday(holiday *database.StatutoryHoliday) error {
	return r.db.Create(holiday).Error
}

func (r *WeekScheduleRepository) UpsertHoliday(holiday *database.StatutoryHoliday) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "type", "year", "updated_at"}),
	}).Create(holiday).Error
}

func (r *WeekScheduleRepository) UpdateHoliday(holiday *database.StatutoryHoliday) error {
	return r.db.Save(holiday).Error
}

func (r *WeekScheduleRepository) DeleteHoliday(id uint) error {
	return r.db.Delete(&database.StatutoryHoliday{}, id).Error
}

func (r *WeekScheduleRepository) FindHolidayByDate(date string) (*database.StatutoryHoliday, error) {
	var holiday database.StatutoryHoliday
	err := r.db.Where("date = ?", date).First(&holiday).Error
	if err != nil {
		return nil, err
	}
	return &holiday, nil
}

func (r *WeekScheduleRepository) FindHolidaysByYear(year int) ([]database.StatutoryHoliday, error) {
	var holidays []database.StatutoryHoliday
	err := r.db.Where("year = ?", year).Order("date ASC").Find(&holidays).Error
	return holidays, err
}

func (r *WeekScheduleRepository) FindHolidaysByDateRange(startDate, endDate string) ([]database.StatutoryHoliday, error) {
	var holidays []database.StatutoryHoliday
	err := r.db.Where("date >= ? AND date <= ?", startDate, endDate).Order("date ASC").Find(&holidays).Error
	return holidays, err
}
