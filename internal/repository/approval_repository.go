package repository

import (
	"peopleops/internal/database"
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
	if approval.ApplicantName != "" {
		existing.ApplicantName = approval.ApplicantName
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
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("create_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
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
