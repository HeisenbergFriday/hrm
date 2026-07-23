package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AttendanceRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

// NewAttendanceRepository binds org from the DB request/tenant context.
// Empty context fails closed (no default fallback, no filters["org_id"] trust).
func NewAttendanceRepository(db *gorm.DB) *AttendanceRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &AttendanceRepository{db: db, orgID: orgID, orgErr: err}
}

// NewAttendanceRepositoryWithOrgID 构造带 org 隔离的考勤仓储；Upsert/Create 会强制
// record.OrgID 与仓储绑定组织一致，禁止落 "default" 或跨组织写入。
func NewAttendanceRepositoryWithOrgID(db *gorm.DB, orgID string) *AttendanceRepository {
	normalized, err := RequireOrgID(orgID)
	return &AttendanceRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *AttendanceRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *AttendanceRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		// Fail closed: never return unscoped tenant rows.
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *AttendanceRepository) Create(record *database.Attendance) error {
	if record == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, record.OrgID)
	if err != nil {
		return err
	}
	record.OrgID = merged
	return r.db.Create(record).Error
}

func (r *AttendanceRepository) Upsert(record *database.Attendance) error {
	if record == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, record.OrgID)
	if err != nil {
		return err
	}
	record.OrgID = merged
	var existing database.Attendance
	err = r.db.
		Where("org_id = ? AND user_id = ? AND check_time = ? AND check_type = ?", record.OrgID, record.UserID, record.CheckTime, record.CheckType).
		First(&existing).Error
	if err == nil {
		existing.UserName = record.UserName
		existing.Location = record.Location
		existing.Extension = record.Extension
		return r.db.Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return r.db.Create(record).Error
}

func (r *AttendanceRepository) FindByID(id string) (*database.Attendance, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.Attendance
	err = r.db.Where("org_id = ? AND id = ?", orgID, id).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *AttendanceRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, 0, err
	}

	var records []database.Attendance
	var total int64

	// Tenant comes only from bound repository / trusted DB context — never filters["org_id"].
	query := r.db.Model(&database.Attendance{}).Where("attendances.org_id = ?", orgID)

	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("attendances.user_id = ?", v)
	}
	if userIDs := csvFilterValues(filters["user_ids"]); len(userIDs) > 0 {
		query = query.Where("attendances.user_id IN ?", userIDs)
	}
	if v, ok := filters["department_id"]; ok && v != "" {
		query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id = ? AND deleted_at IS NULL)", orgID, v)
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)", orgID, departmentIDs)
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, parseErr := time.Parse("2006-01-02", v)
		if parseErr == nil {
			query = query.Where("check_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, parseErr := time.Parse("2006-01-02", v)
		if parseErr == nil {
			query = query.Where("check_time < ?", t.AddDate(0, 0, 1))
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("check_time DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// AttendanceExport Repository

type AttendanceExportRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewAttendanceExportRepository(db *gorm.DB) *AttendanceExportRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &AttendanceExportRepository{db: db, orgID: orgID, orgErr: err}
}

func NewAttendanceExportRepositoryWithOrgID(db *gorm.DB, orgID string) *AttendanceExportRepository {
	normalized, err := RequireOrgID(orgID)
	return &AttendanceExportRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *AttendanceExportRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *AttendanceExportRepository) Create(export *database.AttendanceExport) error {
	if export == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, export.OrgID)
	if err != nil {
		return err
	}
	export.OrgID = merged
	return r.db.Create(export).Error
}

func (r *AttendanceExportRepository) FindAll(page, pageSize int) ([]database.AttendanceExport, int64, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, 0, err
	}
	var exports []database.AttendanceExport
	var total int64

	query := r.db.Model(&database.AttendanceExport{}).Where("org_id = ?", orgID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&exports).Error; err != nil {
		return nil, 0, err
	}

	return exports, total, nil
}
