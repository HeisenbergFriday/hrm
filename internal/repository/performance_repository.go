package repository

import (
	"peopleops/internal/database"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerformanceActivityRepository struct{ db *gorm.DB }

func NewPerformanceActivityRepository(db *gorm.DB) *PerformanceActivityRepository {
	return &PerformanceActivityRepository{db: db}
}

func (r *PerformanceActivityRepository) Create(a *database.PerformanceActivity) error {
	return r.db.Create(a).Error
}

func (r *PerformanceActivityRepository) GetByID(activityID string) (*database.PerformanceActivity, error) {
	var a database.PerformanceActivity
	if err := r.db.Where("id = ?", activityID).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *PerformanceActivityRepository) Update(a *database.PerformanceActivity) error {
	return r.db.Save(a).Error
}

func (r *PerformanceActivityRepository) UpdateStatus(activityID, status, updatedBy string) error {
	return r.db.Model(&database.PerformanceActivity{}).Where("id = ?", activityID).Updates(map[string]interface{}{"status": status, "updated_by": updatedBy}).Error
}

func (r *PerformanceActivityRepository) FindAll(page, pageSize int, status, keyword, startDate, endDate string) ([]database.PerformanceActivity, int64, error) {
	var items []database.PerformanceActivity
	var total int64

	query := r.db.Model(&database.PerformanceActivity{})
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

type PerformanceDistributionRuleRepository struct{ db *gorm.DB }

func NewPerformanceDistributionRuleRepository(db *gorm.DB) *PerformanceDistributionRuleRepository {
	return &PerformanceDistributionRuleRepository{db: db}
}

func (r *PerformanceDistributionRuleRepository) ReplaceForActivity(activityID string, rules []database.PerformanceDistributionRule) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("activity_id = ?", activityID).Delete(&database.PerformanceDistributionRule{}).Error; err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		for i := range rules {
			rules[i].ActivityID = activityID
		}
		return tx.Create(&rules).Error
	})
}

func (r *PerformanceDistributionRuleRepository) ListByActivity(activityID string) ([]database.PerformanceDistributionRule, error) {
	var rules []database.PerformanceDistributionRule
	if err := r.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Order("level ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

type PerformanceTemplateRepository struct{ db *gorm.DB }

func NewPerformanceTemplateRepository(db *gorm.DB) *PerformanceTemplateRepository {
	return &PerformanceTemplateRepository{db: db}
}

func (r *PerformanceTemplateRepository) Create(template *database.PerformanceTemplate, sections []database.PerformanceTemplateSection, items []database.PerformanceTemplateItem, sectionItemCounts []int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(template).Error; err != nil {
			return err
		}

		itemOffset := 0
		for i := range sections {
			sections[i].TemplateID = template.ID
			if err := tx.Create(&sections[i]).Error; err != nil {
				return err
			}

			count := 0
			if i < len(sectionItemCounts) {
				count = sectionItemCounts[i]
			}
			for j := itemOffset; j < itemOffset+count && j < len(items); j++ {
				items[j].SectionID = sections[i].ID
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
	if err := r.db.Where("id = ? AND deleted_at IS NULL", templateID).First(&template).Error; err != nil {
		return nil, nil, nil, err
	}

	var sections []database.PerformanceTemplateSection
	if err := r.db.Where("template_id = ? AND deleted_at IS NULL", templateID).Order("sort_order ASC").Find(&sections).Error; err != nil {
		return nil, nil, nil, err
	}

	var items []database.PerformanceTemplateItem
	if len(sections) > 0 {
		sectionIDs := make([]uint, len(sections))
		for i, section := range sections {
			sectionIDs[i] = section.ID
		}
		if err := r.db.Where("section_id IN ? AND deleted_at IS NULL", sectionIDs).Order("sort_order ASC").Find(&items).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	return &template, sections, items, nil
}

func (r *PerformanceTemplateRepository) FindAll(page, pageSize int, status string) ([]database.PerformanceTemplate, int64, error) {
	var items []database.PerformanceTemplate
	var total int64

	query := r.db.Model(&database.PerformanceTemplate{}).Where("deleted_at IS NULL")
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
		if err := tx.Save(template).Error; err != nil {
			return err
		}
		if !structuralChange {
			return nil
		}

		if err := tx.Where("section_id IN (SELECT id FROM performance_template_sections WHERE template_id = ?)", template.ID).Delete(&database.PerformanceTemplateItem{}).Error; err != nil {
			return err
		}
		if err := tx.Where("template_id = ?", template.ID).Delete(&database.PerformanceTemplateSection{}).Error; err != nil {
			return err
		}

		itemOffset := 0
		for i := range sections {
			sections[i].TemplateID = template.ID
			if err := tx.Create(&sections[i]).Error; err != nil {
				return err
			}

			count := 0
			if i < len(sectionItemCounts) {
				count = sectionItemCounts[i]
			}
			for j := itemOffset; j < itemOffset+count && j < len(items); j++ {
				items[j].SectionID = sections[i].ID
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
	if !r.db.Migrator().HasColumn(&database.PerformanceActivity{}, "template_id") {
		return false, nil
	}
	var count int64
	if err := r.db.Table("performance_activities").Where("template_id = ? AND deleted_at IS NULL", templateID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type PerformanceParticipantRepository struct{ db *gorm.DB }

func NewPerformanceParticipantRepository(db *gorm.DB) *PerformanceParticipantRepository {
	return &PerformanceParticipantRepository{db: db}
}

func (r *PerformanceParticipantRepository) GetByID(participantID string) (*database.PerformanceParticipant, error) {
	var p database.PerformanceParticipant
	if err := r.db.Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PerformanceParticipantRepository) FindAll(activityID string, page, pageSize int, departmentID, managerID, status, employeeKeyword string) ([]database.PerformanceParticipant, int64, error) {
	var items []database.PerformanceParticipant
	var total int64

	query := r.db.Model(&database.PerformanceParticipant{}).Where("activity_id = ? AND deleted_at IS NULL", activityID)
	if status == "" {
		query = query.Where("status NOT IN ?", []string{"inactive", "removed_from_scope"})
	}
	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if managerID != "" {
		query = query.Where("manager_id = ? OR manager_id IS NULL", managerID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if employeeKeyword != "" {
		like := "%" + strings.TrimSpace(employeeKeyword) + "%"
		query = query.Where("employee_name LIKE ? OR employee_id LIKE ?", like, like)
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
	if err := r.db.Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND status = ? AND deleted_at IS NULL", activityID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

type PerformanceReviewVersionRepository struct{ db *gorm.DB }

func NewPerformanceReviewVersionRepository(db *gorm.DB) *PerformanceReviewVersionRepository {
	return &PerformanceReviewVersionRepository{db: db}
}

func (r *PerformanceReviewVersionRepository) CreateSelfEvaluationVersion(participantID string, score float64, level, summary string, attachments []string, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}

		version = &database.PerformanceReviewVersion{
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

		p.SelfScore = score
		p.SelfLevel = level
		p.SelfSummary = summary
		p.Status = nextParticipantStatusAfterSelfEvaluation(p.Status)
		p.UpdatedBy = userID
		return tx.Save(p).Error
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
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}

		version = &database.PerformanceReviewVersion{
			ParticipantID:       p.ID,
			ActivityID:          p.ActivityID,
			ReviewType:          "manager",
			ManagerScore:        score,
			SuggestedLevel:      suggestedLevel,
			ManagerComment:      comment,
			EvaluationItemsJSON: items,
			FinalLevel:          p.FinalLevel,
			CreatedBy:           userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}

		p.ManagerScore = score
		p.SuggestedLevel = suggestedLevel
		p.ManagerComment = comment
		p.Status = nextParticipantStatusAfterManagerEvaluation(p.Status)
		p.UpdatedBy = userID
		if p.FinalLevel == "" {
			p.FinalLevel = suggestedLevel
		}
		return tx.Save(p).Error
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
		for _, e := range evaluations {
			var p database.PerformanceParticipant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND activity_id = ? AND deleted_at IS NULL", e.ParticipantID, activityID).First(&p).Error; err != nil {
				return err
			}
			v := database.PerformanceReviewVersion{
				ParticipantID:       e.ParticipantID,
				ActivityID:          activityID,
				ReviewType:          "manager",
				ManagerScore:        e.ManagerScore,
				SuggestedLevel:      e.SuggestedLevel,
				ManagerComment:      e.ManagerComment,
				FinalLevel:          e.SuggestedLevel,
				CreatedBy:           userID,
				EvaluationItemsJSON: e.EvaluationItems,
			}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			if err := tx.Model(&database.PerformanceParticipant{}).Where("id = ?", e.ParticipantID).Updates(map[string]interface{}{
				"manager_score":   e.ManagerScore,
				"suggested_level": e.SuggestedLevel,
				"manager_comment": e.ManagerComment,
				"final_level":     e.SuggestedLevel,
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
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}
		version = &database.PerformanceReviewVersion{
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
		p.FinalLevel = finalLevel
		p.AdjustReason = reason
		p.UpdatedBy = userID
		return tx.Save(p).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) ConfirmResult(participantID, confirmComment, userID string) (*database.PerformanceReviewVersion, error) {
	var version *database.PerformanceReviewVersion
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}
		version = &database.PerformanceReviewVersion{
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
		p.Status = "result_confirmed"
		p.ConfirmedAt = version.ConfirmedAt
		p.ConfirmedBy = userID
		p.UpdatedBy = userID
		return tx.Save(p).Error
	})
	if err != nil {
		return nil, err
	}
	return version, nil
}

func (r *PerformanceReviewVersionRepository) ListByParticipant(participantID string) ([]database.PerformanceReviewVersion, error) {
	var versions []database.PerformanceReviewVersion
	if err := r.db.Where("participant_id = ? AND deleted_at IS NULL", participantID).Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *PerformanceReviewVersionRepository) getParticipantLocked(participantID string) (*database.PerformanceParticipant, error) {
	var p database.PerformanceParticipant
	if err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

type PerformanceRelationshipChangeLogRepository struct{ db *gorm.DB }

func NewPerformanceRelationshipChangeLogRepository(db *gorm.DB) *PerformanceRelationshipChangeLogRepository {
	return &PerformanceRelationshipChangeLogRepository{db: db}
}

func (r *PerformanceRelationshipChangeLogRepository) ListByParticipant(participantID string) ([]database.PerformanceRelationshipChangeLog, error) {
	var logs []database.PerformanceRelationshipChangeLog
	if err := r.db.Where("participant_id = ? AND deleted_at IS NULL", participantID).Order("changed_at DESC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceRelationshipChangeLogRepository) ListByActivity(activityID string) ([]database.PerformanceRelationshipChangeLog, error) {
	var logs []database.PerformanceRelationshipChangeLog
	if err := r.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Order("changed_at DESC").Find(&logs).Error; err != nil {
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
