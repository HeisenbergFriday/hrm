package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AttendanceRepository struct {
	db *gorm.DB
}

func NewAttendanceRepository(db *gorm.DB) *AttendanceRepository {
	return &AttendanceRepository{db: db}
}

func (r *AttendanceRepository) Create(record *database.Attendance) error {
	return r.db.Create(record).Error
}

func (r *AttendanceRepository) FindByID(id string) (*database.Attendance, error) {
	var record database.Attendance
	err := r.db.First(&record, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *AttendanceRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Attendance, int64, error) {
	var records []database.Attendance
	var total int64

	query := r.db.Model(&database.Attendance{})

	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("user_id = ?", v)
	}
	if v, ok := filters["department_id"]; ok && v != "" {
		// 通过子查询找到该部门下所有用户的 user_id
		query = query.Where("user_id IN (SELECT user_id FROM users WHERE department_id = ? AND deleted_at IS NULL)", v)
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
