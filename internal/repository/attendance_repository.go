package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AttendanceRepository struct {
	db    *gorm.DB
	orgID string
}

func NewAttendanceRepository(db *gorm.DB) *AttendanceRepository {
	return &AttendanceRepository{db: db}
}

// NewAttendanceRepositoryWithOrgID 构造带 org 隔离的考勤仓储；Upsert/Create 会强制
// record.OrgID 与仓储绑定组织一致，禁止落 "default" 或跨组织写入。
func NewAttendanceRepositoryWithOrgID(db *gorm.DB, orgID string) *AttendanceRepository {
	return &AttendanceRepository{db: db, orgID: orgID}
}

func (r *AttendanceRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

func (r *AttendanceRepository) Create(record *database.Attendance) error {
	if record == nil {
		return gorm.ErrInvalidData
	}
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, record.OrgID)
		if err != nil {
			return err
		}
		record.OrgID = merged
	}
	return r.db.Create(record).Error
}

func (r *AttendanceRepository) Upsert(record *database.Attendance) error {
	if record == nil {
		return gorm.ErrInvalidData
	}
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, record.OrgID)
		if err != nil {
			return err
		}
		record.OrgID = merged
	} else if record.OrgID == "" {
		// 迁移期兼容：无租户上下文构造的旧调用会继续使用 default 占位。
		// 新代码请使用 NewAttendanceRepositoryWithOrgID。
		record.OrgID = "default"
	}
	var existing database.Attendance
	err := r.db.
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
	var record database.Attendance
	err := r.scoped().First(&record, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *AttendanceRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	var records []database.Attendance
	var total int64

	query := r.db.Model(&database.Attendance{})

	orgID := ""
	if v, ok := filters["org_id"]; ok && v != "" {
		orgID = v
		query = query.Where("attendances.org_id = ?", v)
	}

	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("attendances.user_id = ?", v)
	}
	if userIDs := csvFilterValues(filters["user_ids"]); len(userIDs) > 0 {
		query = query.Where("attendances.user_id IN ?", userIDs)
	}
	if v, ok := filters["department_id"]; ok && v != "" {
		if orgID != "" {
			query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id = ? AND deleted_at IS NULL)", orgID, v)
		} else {
			query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE department_id = ? AND deleted_at IS NULL)", v)
		}
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		if orgID != "" {
			query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)", orgID, departmentIDs)
		} else {
			query = query.Where("attendances.user_id IN (SELECT user_id FROM users WHERE department_id IN ? AND deleted_at IS NULL)", departmentIDs)
		}
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("check_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
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
	db *gorm.DB
}

func NewAttendanceExportRepository(db *gorm.DB) *AttendanceExportRepository {
	return &AttendanceExportRepository{db: db}
}

func (r *AttendanceExportRepository) Create(export *database.AttendanceExport) error {
	return r.db.Create(export).Error
}

func (r *AttendanceExportRepository) FindAll(page, pageSize int) ([]database.AttendanceExport, int64, error) {
	var exports []database.AttendanceExport
	var total int64

	if err := r.db.Model(&database.AttendanceExport{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := r.db.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&exports).Error; err != nil {
		return nil, 0, err
	}

	return exports, total, nil
}
