package repository

import (
	"fmt"
	"math"
	"peopleops/internal/database"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func performanceRepositoryDB(db *gorm.DB, orgID string) *gorm.DB {
	if db == nil || strings.TrimSpace(orgID) != "" || db.Error != nil {
		return db
	}
	tx := db.Session(&gorm.Session{NewDB: true})
	_ = tx.AddError(ErrMissingOrgID)
	return tx
}

type PerformanceActivityRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceActivityRepository(db *gorm.DB) *PerformanceActivityRepository {
	return &PerformanceActivityRepository{db: db}
}

func NewPerformanceActivityRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceActivityRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceActivityRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceActivityRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceActivityRepository) Create(a *database.PerformanceActivity) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, a.OrgID)
	if err != nil {
		return err
	}
	a.OrgID = merged
	return r.scoped().Create(a).Error
}

func (r *PerformanceActivityRepository) GetByID(activityID string) (*database.PerformanceActivity, error) {
	var a database.PerformanceActivity
	if err := r.scoped().Where("id = ?", activityID).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *PerformanceActivityRepository) Update(a *database.PerformanceActivity) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, a.OrgID)
	if err != nil {
		return err
	}
	a.OrgID = merged
	return r.scoped().Save(a).Error
}

func (r *PerformanceActivityRepository) UpdateStatus(activityID, status, updatedBy string) error {
	return r.scoped().Model(&database.PerformanceActivity{}).Where("id = ?", activityID).Updates(map[string]interface{}{"status": status, "updated_by": updatedBy}).Error
}

func (r *PerformanceActivityRepository) FindAll(page, pageSize int, status, keyword, startDate, endDate string, departmentIDs []string) ([]database.PerformanceActivity, int64, error) {
	var items []database.PerformanceActivity
	var total int64

	query := r.scoped().Model(&database.PerformanceActivity{})
	query = query.Where("deleted_at IS NULL")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if startDate != "" {
		query = query.Where("start_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("end_date <= ?", endDate)
	}
	// 部门隔离：只显示包含可见部门参与人的活动（子查询必须带 org_id）
	if len(departmentIDs) > 0 {
		orgID := strings.TrimSpace(r.orgID)
		if orgID == "" {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("id IN (SELECT DISTINCT activity_id FROM performance_participants WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)", orgID, departmentIDs)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// FindAllByUserID 查询用户参与的绩效活动（普通员工场景）
func (r *PerformanceActivityRepository) FindAllByUserID(page, pageSize int, status, keyword, startDate, endDate string, userIDs []string) ([]database.PerformanceActivity, int64, error) {
	var items []database.PerformanceActivity
	var total int64

	query := r.scoped().Model(&database.PerformanceActivity{})
	query = query.Where("deleted_at IS NULL")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if startDate != "" {
		query = query.Where("start_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("end_date <= ?", endDate)
	}
	// 只显示用户参与的活动（子查询必须带 org_id）
	if len(userIDs) > 0 {
		orgID := strings.TrimSpace(r.orgID)
		if orgID == "" {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("id IN (SELECT DISTINCT activity_id FROM performance_participants WHERE org_id = ? AND employee_id IN ? AND deleted_at IS NULL)", orgID, userIDs)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type PerformanceDistributionRuleRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceDistributionRuleRepository(db *gorm.DB) *PerformanceDistributionRuleRepository {
	return &PerformanceDistributionRuleRepository{db: db}
}

func NewPerformanceDistributionRuleRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceDistributionRuleRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceDistributionRuleRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceDistributionRuleRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceDistributionRuleRepository) ReplaceForActivity(activityID string, rules []database.PerformanceDistributionRule) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		deleteQuery := tx.Where("activity_id = ?", activityID)
		if orgID != "" {
			deleteQuery = deleteQuery.Where("org_id = ?", orgID)
		}
		if err := deleteQuery.Delete(&database.PerformanceDistributionRule{}).Error; err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		for i := range rules {
			rules[i].ActivityID = activityID
			if orgID != "" {
				if strings.TrimSpace(rules[i].OrgID) != "" && strings.TrimSpace(rules[i].OrgID) != orgID {
					return ErrOrgMismatch
				}
				rules[i].OrgID = orgID
			}
		}
		return tx.Create(&rules).Error
	})
}

func (r *PerformanceDistributionRuleRepository) ListByActivity(activityID string) ([]database.PerformanceDistributionRule, error) {
	var rules []database.PerformanceDistributionRule
	if err := r.scoped().Where("activity_id = ? AND deleted_at IS NULL", activityID).Order("level ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

type PerformanceTemplateRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceTemplateRepository(db *gorm.DB) *PerformanceTemplateRepository {
	return &PerformanceTemplateRepository{db: db}
}

func NewPerformanceTemplateRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceTemplateRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceTemplateRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceTemplateRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceTemplateRepository) Create(template *database.PerformanceTemplate, sections []database.PerformanceTemplateSection, items []database.PerformanceTemplateItem, sectionItemCounts []int) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		merged, err := EnsureSameOrg(orgID, template.OrgID)
		if err != nil {
			return err
		}
		template.OrgID = merged
		if err := tx.Create(template).Error; err != nil {
			return err
		}

		itemOffset := 0
		for i := range sections {
			sections[i].TemplateID = template.ID
			merged, err := EnsureSameOrg(orgID, sections[i].OrgID)
			if err != nil {
				return err
			}
			sections[i].OrgID = merged
			if err := tx.Create(&sections[i]).Error; err != nil {
				return err
			}

			count := 0
			if i < len(sectionItemCounts) {
				count = sectionItemCounts[i]
			}
			for j := itemOffset; j < itemOffset+count && j < len(items); j++ {
				items[j].SectionID = sections[i].ID
				merged, err := EnsureSameOrg(orgID, items[j].OrgID)
				if err != nil {
					return err
				}
				items[j].OrgID = merged
				if err := tx.Create(&items[j]).Error; err != nil {
					return err
				}
			}
			itemOffset += count
		}
		return nil
	})
}

func (r *PerformanceTemplateRepository) GetByID(templateID uint) (*database.PerformanceTemplate, []database.PerformanceTemplateSection, []database.PerformanceTemplateItem, error) {
	var template database.PerformanceTemplate
	if err := r.scoped().Where("id = ? AND deleted_at IS NULL", templateID).First(&template).Error; err != nil {
		return nil, nil, nil, err
	}

	var sections []database.PerformanceTemplateSection
	if err := r.scoped().Where("template_id = ? AND deleted_at IS NULL", templateID).Order("sort_order ASC").Find(&sections).Error; err != nil {
		return nil, nil, nil, err
	}

	var items []database.PerformanceTemplateItem
	if len(sections) > 0 {
		sectionIDs := make([]uint, len(sections))
		for i, section := range sections {
			sectionIDs[i] = section.ID
		}
		if err := r.scoped().Where("section_id IN ? AND deleted_at IS NULL", sectionIDs).Order("sort_order ASC").Find(&items).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	return &template, sections, items, nil
}

func (r *PerformanceTemplateRepository) FindAll(page, pageSize int, status string) ([]database.PerformanceTemplate, int64, error) {
	var items []database.PerformanceTemplate
	var total int64

	query := r.scoped().Model(&database.PerformanceTemplate{}).Where("deleted_at IS NULL")
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PerformanceTemplateRepository) Update(template *database.PerformanceTemplate, sections []database.PerformanceTemplateSection, items []database.PerformanceTemplateItem, structuralChange bool, sectionItemCounts []int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		if orgID != "" && strings.TrimSpace(template.OrgID) == "" {
			template.OrgID = orgID
		}
		if orgID != "" && strings.TrimSpace(template.OrgID) != orgID {
			return ErrOrgMismatch
		}
		if err := tx.Save(template).Error; err != nil {
			return err
		}
		if !structuralChange {
			return nil
		}

		itemDelete := tx.Where("section_id IN (SELECT id FROM performance_template_sections WHERE template_id = ?)", template.ID)
		sectionDelete := tx.Where("template_id = ?", template.ID)
		if orgID != "" {
			itemDelete = itemDelete.Where("org_id = ?", orgID)
			sectionDelete = sectionDelete.Where("org_id = ?", orgID)
		}
		if err := itemDelete.Delete(&database.PerformanceTemplateItem{}).Error; err != nil {
			return err
		}
		if err := sectionDelete.Delete(&database.PerformanceTemplateSection{}).Error; err != nil {
			return err
		}

		itemOffset := 0
		for i := range sections {
			sections[i].TemplateID = template.ID
			if orgID != "" {
				sections[i].OrgID = orgID
			}
			if err := tx.Create(&sections[i]).Error; err != nil {
				return err
			}

			count := 0
			if i < len(sectionItemCounts) {
				count = sectionItemCounts[i]
			}
			for j := itemOffset; j < itemOffset+count && j < len(items); j++ {
				items[j].SectionID = sections[i].ID
				if orgID != "" {
					items[j].OrgID = orgID
				}
				if err := tx.Create(&items[j]).Error; err != nil {
					return err
				}
			}
			itemOffset += count
		}
		return nil
	})
}

func (r *PerformanceTemplateRepository) IsReferencedByActivity(templateID uint) (bool, error) {
	if r.db != nil && r.db.Error != nil {
		return false, r.db.Error
	}
	if !r.db.Migrator().HasColumn(&database.PerformanceActivity{}, "template_id") {
		return false, nil
	}
	var count int64
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return false, ErrMissingOrgID
	}
	query := r.db.Table("performance_activities").Where("template_id = ? AND org_id = ? AND deleted_at IS NULL", templateID, orgID)
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type PerformanceParticipantRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceParticipantRepository(db *gorm.DB) *PerformanceParticipantRepository {
	return &PerformanceParticipantRepository{db: db}
}

func NewPerformanceParticipantRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceParticipantRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceParticipantRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceParticipantRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceParticipantRepository) GetByID(participantID string) (*database.PerformanceParticipant, error) {
	var p database.PerformanceParticipant
	if err := r.scoped().Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PerformanceParticipantRepository) FindAll(activityID string, page, pageSize int, departmentID, managerID, status, employeeKeyword string, visibleDepartmentIDs []string, visibleUserIDs []string) ([]database.PerformanceParticipant, int64, error) {
	var items []database.PerformanceParticipant
	var total int64

	query := r.scoped().Model(&database.PerformanceParticipant{}).Where("activity_id = ? AND deleted_at IS NULL", activityID)
	if status == "" {
		query = query.Where("status NOT IN ?", []string{"inactive", "removed_from_scope"})
	}
	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if managerID != "" {
		query = query.Where("manager_id = ?", managerID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if employeeKeyword != "" {
		like := "%" + strings.TrimSpace(employeeKeyword) + "%"
		query = query.Where("employee_name LIKE ? OR employee_id LIKE ?", like, like)
	}
	// 部门隔离：只显示可见部门的参与人
	if len(visibleDepartmentIDs) > 0 {
		query = query.Where("department_id IN ?", visibleDepartmentIDs)
	}
	// 自我隔离：只显示自己的参与记录
	if len(visibleUserIDs) > 0 {
		query = query.Where("employee_id IN ?", visibleUserIDs)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PerformanceParticipantRepository) CountByActivityAndStatus(activityID string, status string) (int64, error) {
	var count int64
	if err := r.scoped().Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND status = ? AND deleted_at IS NULL", activityID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

type PerformanceReviewVersionRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceReviewVersionRepository(db *gorm.DB) *PerformanceReviewVersionRepository {
	return &PerformanceReviewVersionRepository{db: db}
}

func NewPerformanceReviewVersionRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceReviewVersionRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceReviewVersionRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceReviewVersionRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceReviewVersionRepository) CreateSelfEvaluationVersion(participantID string, score float64, level, summary string, attachments []string, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		var p database.PerformanceParticipant
		participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID)
		if orgID != "" {
			participantQuery = participantQuery.Where("org_id = ?", orgID)
		}
		if err := participantQuery.First(&p).Error; err != nil {
			return err
		}
		if orgID == "" {
			orgID = strings.TrimSpace(p.OrgID)
		}
		if p.IsLocked {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交自评")
		}

		version = &database.PerformanceReviewVersion{
			OrgID:               orgID,
			ParticipantID:       p.ID,
			ActivityID:          p.ActivityID,
			ReviewType:          "self",
			SelfScore:           score,
			SelfLevel:           level,
			SelfSummary:         summary,
			SelfAttachmentsJSON: attachments,
			SuggestedLevel:      p.SuggestedLevel,
			FinalLevel:          p.FinalLevel,
			CreatedBy:           userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}

		return tx.Model(&p).Updates(map[string]interface{}{
			"self_score":   score,
			"self_level":   level,
			"self_summary": summary,
			"status":       nextParticipantStatusAfterSelfEvaluation(p.Status),
			"updated_by":   userID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) CreateManagerEvaluationVersion(participantID string, score float64, suggestedLevel, comment string, items []struct {
	ItemKey   string
	ItemScore float64
	ItemValue string
}, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		var p database.PerformanceParticipant
		participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID)
		if orgID != "" {
			participantQuery = participantQuery.Where("org_id = ?", orgID)
		}
		if err := participantQuery.First(&p).Error; err != nil {
			return err
		}
		if orgID == "" {
			orgID = strings.TrimSpace(p.OrgID)
		}
		if p.IsLocked {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交主管评分")
		}

		version = &database.PerformanceReviewVersion{
			OrgID:               orgID,
			ParticipantID:       p.ID,
			ActivityID:          p.ActivityID,
			ReviewType:          "manager",
			ManagerScore:        score,
			SuggestedLevel:      suggestedLevel,
			ManagerComment:      comment,
			EvaluationItemsJSON: items,
			FinalLevel:          ensureFinalLevel(suggestedLevel, score),
			CreatedBy:           userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}

		return tx.Model(&p).Updates(map[string]interface{}{
			"manager_score":   score,
			"suggested_level": suggestedLevel,
			"manager_comment": comment,
			"status":          nextParticipantStatusAfterManagerEvaluation(p.Status),
			"updated_by":      userID,
			"final_level":     ensureFinalLevel(suggestedLevel, score),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) BatchCreateManagerEvaluationVersions(activityID string, evaluations []struct {
	ParticipantID   uint
	ManagerScore    float64
	SuggestedLevel  string
	ManagerComment  string
	EvaluationItems []struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}
}, userID string) ([]database.PerformanceReviewVersion, error) {
	versions := make([]database.PerformanceReviewVersion, 0, len(evaluations))
	err := r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		for _, e := range evaluations {
			var p database.PerformanceParticipant
			participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND activity_id = ? AND deleted_at IS NULL", e.ParticipantID, activityID)
			if orgID != "" {
				participantQuery = participantQuery.Where("org_id = ?", orgID)
			}
			if err := participantQuery.First(&p).Error; err != nil {
				return err
			}
			versionOrgID := orgID
			if versionOrgID == "" {
				versionOrgID = strings.TrimSpace(p.OrgID)
			}
			if p.IsLocked {
				return fmt.Errorf("参与人 %d 的绩效结果已锁定，无法提交主管评分", e.ParticipantID)
			}
			v := database.PerformanceReviewVersion{
				OrgID:               versionOrgID,
				ParticipantID:       e.ParticipantID,
				ActivityID:          activityID,
				ReviewType:          "manager",
				ManagerScore:        e.ManagerScore,
				SuggestedLevel:      e.SuggestedLevel,
				ManagerComment:      e.ManagerComment,
				FinalLevel:          ensureFinalLevel(e.SuggestedLevel, e.ManagerScore),
				CreatedBy:           userID,
				EvaluationItemsJSON: e.EvaluationItems,
			}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			// 没有逐项评分时，按权重分摊总分到各指标
			if len(e.EvaluationItems) == 0 {
				var records []database.PerformanceGoalRecord
				recordQuery := tx.Where("participant_id = ? AND deleted_at IS NULL AND section_type != ? AND (goal_phase = ? OR goal_phase = '' OR goal_phase IS NULL)",
					e.ParticipantID, "bonus_penalty", "review")
				if versionOrgID != "" {
					recordQuery = recordQuery.Where("org_id = ?", versionOrgID)
				}
				if err := recordQuery.Find(&records).Error; err == nil && len(records) > 0 {
					totalWeight := 0.0
					for _, r := range records {
						totalWeight += r.Weight
					}
					if totalWeight > 0 {
						for _, r := range records {
							newScore := math.Round(e.ManagerScore*(r.Weight/totalWeight)*100) / 100
							updateRecord := tx.Model(&database.PerformanceGoalRecord{}).Where("id = ?", r.ID)
							if versionOrgID != "" {
								updateRecord = updateRecord.Where("org_id = ?", versionOrgID)
							}
							if err := updateRecord.Update("manager_score", newScore).Error; err != nil {
								return err
							}
						}
					}
				}
			}

			updateParticipant := tx.Model(&database.PerformanceParticipant{}).Where("id = ?", e.ParticipantID)
			if versionOrgID != "" {
				updateParticipant = updateParticipant.Where("org_id = ?", versionOrgID)
			}
			if err := updateParticipant.Updates(map[string]interface{}{
				"manager_score":   e.ManagerScore,
				"suggested_level": e.SuggestedLevel,
				"manager_comment": e.ManagerComment,
				"final_level":     ensureFinalLevel(e.SuggestedLevel, e.ManagerScore),
				"status":          nextParticipantStatusAfterManagerEvaluation(p.Status),
				"updated_by":      userID,
			}).Error; err != nil {
				return err
			}
			versions = append(versions, v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *PerformanceReviewVersionRepository) AdjustFinalLevel(participantID, finalLevel, reason, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		var p database.PerformanceParticipant
		participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID)
		if orgID != "" {
			participantQuery = participantQuery.Where("org_id = ?", orgID)
		}
		if err := participantQuery.First(&p).Error; err != nil {
			return err
		}
		if orgID == "" {
			orgID = strings.TrimSpace(p.OrgID)
		}
		version = &database.PerformanceReviewVersion{
			OrgID:         orgID,
			ParticipantID: p.ID,
			ActivityID:    p.ActivityID,
			ReviewType:    "adjust_final_level",
			FinalLevel:    finalLevel,
			AdjustReason:  reason,
			CreatedBy:     userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return tx.Model(&p).Updates(map[string]interface{}{
			"final_level":   finalLevel,
			"adjust_reason": reason,
			"updated_by":    userID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) ConfirmResult(participantID, confirmComment, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		orgID := strings.TrimSpace(r.orgID)
		var p database.PerformanceParticipant
		participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID)
		if orgID != "" {
			participantQuery = participantQuery.Where("org_id = ?", orgID)
		}
		if err := participantQuery.First(&p).Error; err != nil {
			return err
		}
		if orgID == "" {
			orgID = strings.TrimSpace(p.OrgID)
		}
		version = &database.PerformanceReviewVersion{
			OrgID:          orgID,
			ParticipantID:  p.ID,
			ActivityID:     p.ActivityID,
			ReviewType:     "confirm_result",
			FinalLevel:     p.FinalLevel,
			ConfirmComment: confirmComment,
			CreatedBy:      userID,
		}
		confirmedAt := timeNow()
		version.ConfirmedAt = &confirmedAt
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return tx.Model(&p).Updates(map[string]interface{}{
			"status":       "locked",
			"confirmed_at": confirmedAt,
			"confirmed_by": userID,
			"is_locked":    true,
			"locked_at":    confirmedAt,
			"locked_by":    userID,
			"updated_by":   userID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) ListByParticipant(participantID string) ([]database.PerformanceReviewVersion, error) {
	var versions []database.PerformanceReviewVersion
	if err := r.scoped().Where("participant_id = ? AND deleted_at IS NULL", participantID).Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *PerformanceReviewVersionRepository) getParticipantLocked(participantID string) (*database.PerformanceParticipant, error) {
	var p database.PerformanceParticipant
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	query := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND org_id = ? AND deleted_at IS NULL", participantID, orgID)
	if err := query.First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

type PerformanceRelationshipChangeLogRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceRelationshipChangeLogRepository(db *gorm.DB) *PerformanceRelationshipChangeLogRepository {
	return &PerformanceRelationshipChangeLogRepository{db: db}
}

func NewPerformanceRelationshipChangeLogRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceRelationshipChangeLogRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceRelationshipChangeLogRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceRelationshipChangeLogRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceRelationshipChangeLogRepository) ListByParticipant(participantID string) ([]database.PerformanceRelationshipChangeLog, error) {
	var logs []database.PerformanceRelationshipChangeLog
	if err := r.scoped().Where("participant_id = ? AND deleted_at IS NULL", participantID).Order("changed_at DESC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceRelationshipChangeLogRepository) ListByActivity(activityID string) ([]database.PerformanceRelationshipChangeLog, error) {
	var logs []database.PerformanceRelationshipChangeLog
	if err := r.scoped().Where("activity_id = ? AND deleted_at IS NULL", activityID).Order("changed_at DESC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func timeNow() time.Time { return time.Now() }

func nextParticipantStatusAfterSelfEvaluation(current string) string {
	switch current {
	case "manager_submitted", "result_confirmed":
		return current
	default:
		return "self_submitted"
	}
}

func nextParticipantStatusAfterManagerEvaluation(current string) string {
	if current == "result_confirmed" {
		return current
	}
	return "manager_submitted"
}

func ensureFinalLevel(level string, score float64) string {
	if strings.TrimSpace(level) != "" {
		return strings.TrimSpace(level)
	}
	if score >= 100 {
		return "S"
	}
	if score >= 90 {
		return "A"
	}
	if score >= 80 {
		return "B"
	}
	if score >= 60 {
		return "C"
	}
	return "D"
}
