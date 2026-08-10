package repository

import (
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ApprovalRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewApprovalRepository(db *gorm.DB) *ApprovalRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &ApprovalRepository{db: db, orgID: orgID, orgErr: err}
}

func NewApprovalRepositoryWithOrgID(db *gorm.DB, orgID string) *ApprovalRepository {
	normalized, err := RequireOrgID(orgID)
	return &ApprovalRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ApprovalRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *ApprovalRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		// Fail closed: never return unscoped tenant rows.
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *ApprovalRepository) Create(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, approval.OrgID)
	if err != nil {
		return err
	}
	approval.OrgID = merged
	return r.createApproval(approval)
}

// UpsertByOrgProcessID creates or updates an approval by (org_id, process_id).
// Existing Title/ApplicantID/ApplicantName/CreateTime are preserved when incoming values are empty.
func (r *ApprovalRepository) UpsertByOrgProcessID(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, approval.OrgID)
	if err != nil {
		return err
	}
	approval.OrgID = merged
	approval.ProcessID = strings.TrimSpace(approval.ProcessID)
	if approval.ProcessID == "" {
		return gorm.ErrInvalidData
	}

	var existing database.Approval
	err = r.db.Where("org_id = ? AND process_id = ?", approval.OrgID, approval.ProcessID).First(&existing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.createApproval(approval)
		}
		return err
	}

	if approval.Title != "" {
		existing.Title = approval.Title
	}
	if approval.ApplicantID != "" {
		existing.ApplicantID = approval.ApplicantID
	}
	if incomingName := strings.TrimSpace(approval.ApplicantName); incomingName != "" {
		existingName := strings.TrimSpace(existing.ApplicantName)
		incomingIsFallback := incomingName == strings.TrimSpace(approval.ApplicantID)
		existingIsFallback := existingName == "" || existingName == strings.TrimSpace(existing.ApplicantID)
		if !incomingIsFallback || existingIsFallback {
			existing.ApplicantName = incomingName
		}
	}
	if !approval.CreateTime.IsZero() {
		existing.CreateTime = approval.CreateTime
	}
	if approval.Status != "" {
		existing.Status = approval.Status
	}
	if !approval.FinishTime.IsZero() {
		existing.FinishTime = approval.FinishTime
	}
	if approval.Content != nil {
		existing.Content = approval.Content
	}
	if approval.Extension != nil {
		existing.Extension = mergeApprovalExtension(existing.Extension, approval.Extension)
	}
	return r.db.Save(&existing).Error
}

// createApproval inserts a new approval row.
// RUNNING instances often have empty finish_time; omit the zero time.Time so MySQL
// does not reject '0000-00-00' under strict datetime mode.
func (r *ApprovalRepository) createApproval(approval *database.Approval) error {
	if approval == nil {
		return gorm.ErrInvalidData
	}
	tx := r.db
	if approval.FinishTime.IsZero() {
		tx = tx.Omit("FinishTime")
	}
	return tx.Create(approval).Error
}

func mergeApprovalExtension(base, patch map[string]interface{}) map[string]interface{} {
	if base == nil && patch == nil {
		return nil
	}
	out := make(map[string]interface{}, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

func (r *ApprovalRepository) FindByID(id string) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().First(&approval, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindByUintID(id uint) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().First(&approval, id).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindByProcessID(processID string) (*database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	var approval database.Approval
	err := r.scoped().Where("process_id = ?", strings.TrimSpace(processID)).First(&approval).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Approval, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	var approvals []database.Approval
	var total int64

	query := r.scoped().Model(&database.Approval{})

	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("status = ?", v)
	}
	if v, ok := filters["template_id"]; ok && v != "" {
		query = query.Where("extension->>'$.template_id' = ? OR extension LIKE ?", v, "%"+v+"%")
	}
	if v, ok := filters["applicant_id"]; ok && v != "" {
		query = query.Where("applicant_id = ?", v)
	}
	if v, ok := filters["title"]; ok && v != "" {
		query = query.Where("title LIKE ?", "%"+v+"%")
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(v), dingtalk.ApprovalBusinessLocation())
		if err == nil {
			query = query.Where("create_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(v), dingtalk.ApprovalBusinessLocation())
		if err == nil {
			query = query.Where("create_time < ?", t.AddDate(0, 0, 1))
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("create_time DESC").Offset(offset).Limit(pageSize).Find(&approvals).Error; err != nil {
		return nil, 0, err
	}

	return approvals, total, nil
}

// FindAllForStats returns the complete, narrow projection required for server-side
// aggregation. It intentionally has no page limit, so totals cannot truncate at 10,000.
func (r *ApprovalRepository) FindAllForStats(filters map[string]string, location *time.Location) ([]database.Approval, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, err
	}
	query := r.scoped().Model(&database.Approval{}).Select("status", "extension")
	if v := strings.TrimSpace(filters["template_id"]); v != "" {
		query = query.Where("extension->>'$.process_code' = ? OR extension->>'$.template_id' = ? OR extension LIKE ?", v, v, "%"+v+"%")
	}
	if v := strings.TrimSpace(filters["start_date"]); v != "" {
		start, err := time.ParseInLocation("2006-01-02", v, location)
		if err != nil {
			return nil, err
		}
		query = query.Where("create_time >= ?", start)
	}
	if v := strings.TrimSpace(filters["end_date"]); v != "" {
		end, err := time.ParseInLocation("2006-01-02", v, location)
		if err != nil {
			return nil, err
		}
		query = query.Where("create_time < ?", end.AddDate(0, 0, 1))
	}
	var approvals []database.Approval
	if err := query.Find(&approvals).Error; err != nil {
		return nil, err
	}
	return approvals, nil
}

// FindAllByTitleKeywords 与 FindAll 逻辑一致，只是先按标题关键字命中/排除过滤。
// include=true 时匹配 title LIKE 任一关键字（用于具体分类）；
// include=false 时排除所有关键字（用于 "other" 分类）。
// keywords 为空且 include=true 时返回空结果集，避免退化为无条件查询。
func (r *ApprovalRepository) FindAllByTitleKeywords(page, pageSize int, keywords []string, include bool, filters map[string]string) ([]database.Approval, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	if include && len(keywords) == 0 {
		return []database.Approval{}, 0, nil
	}
	var approvals []database.Approval
	var total int64

	query := r.scoped().Model(&database.Approval{})

	if len(keywords) > 0 {
		clauses := make([]string, 0, len(keywords))
		args := make([]interface{}, 0, len(keywords))
		for _, kw := range keywords {
			if strings.TrimSpace(kw) == "" {
				continue
			}
			clauses = append(clauses, "title LIKE ?")
			args = append(args, "%"+kw+"%")
		}
		if len(clauses) > 0 {
			combined := strings.Join(clauses, " OR ")
			if include {
				query = query.Where(combined, args...)
			} else {
				query = query.Where("NOT ("+combined+")", args...)
			}
		}
	}

	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("status = ?", v)
	}
	if v, ok := filters["applicant_id"]; ok && v != "" {
		query = query.Where("applicant_id = ?", v)
	}
	if v, ok := filters["title"]; ok && v != "" {
		query = query.Where("title LIKE ?", "%"+v+"%")
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(v), dingtalk.ApprovalBusinessLocation())
		if err == nil {
			query = query.Where("create_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(v), dingtalk.ApprovalBusinessLocation())
		if err == nil {
			query = query.Where("create_time < ?", t.AddDate(0, 0, 1))
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("create_time DESC").Offset(offset).Limit(pageSize).Find(&approvals).Error; err != nil {
		return nil, 0, err
	}

	return approvals, total, nil
}

// ApprovalTemplate Repository

type ApprovalTemplateRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewApprovalTemplateRepository(db *gorm.DB) *ApprovalTemplateRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &ApprovalTemplateRepository{db: db, orgID: orgID, orgErr: err}
}

func NewApprovalTemplateRepositoryWithOrgID(db *gorm.DB, orgID string) *ApprovalTemplateRepository {
	normalized, err := RequireOrgID(orgID)
	return &ApprovalTemplateRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ApprovalTemplateRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *ApprovalTemplateRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *ApprovalTemplateRepository) Create(template *database.ApprovalTemplate) error {
	if template == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, template.OrgID)
	if err != nil {
		return err
	}
	template.OrgID = merged
	return r.db.Create(template).Error
}

func (r *ApprovalTemplateRepository) FindAll() ([]database.ApprovalTemplate, int64, error) {
	if _, err := r.requireOrgID(); err != nil {
		return nil, 0, err
	}
	var templates []database.ApprovalTemplate
	var total int64

	if err := r.scoped().Model(&database.ApprovalTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.scoped().Find(&templates).Error; err != nil {
		return nil, 0, err
	}

	return templates, total, nil
}
