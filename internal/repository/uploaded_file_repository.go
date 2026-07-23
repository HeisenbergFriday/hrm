package repository

import (
	"errors"
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
)

// UploadedFileRepository 按组织隔离的上传文件元数据仓储。
// 必须通过 NewUploadedFileRepositoryWithOrgID 构造；空 org 直接拒绝。
type UploadedFileRepository struct {
	db    *gorm.DB
	orgID string
}

// NewUploadedFileRepositoryWithOrgID 构造严格 tenant 仓储；orgID 为空返回错误。
func NewUploadedFileRepositoryWithOrgID(db *gorm.DB, orgID string) (*UploadedFileRepository, error) {
	normalized, err := RequireOrgID(orgID)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("repository: db required")
	}
	return &UploadedFileRepository{db: db, orgID: normalized}, nil
}

func (r *UploadedFileRepository) scoped() *gorm.DB {
	return r.db.Model(&database.UploadedFile{}).Where("org_id = ?", r.orgID)
}

// Create 写入文件元数据；强制 org_id 与仓储绑定组织一致。
func (r *UploadedFileRepository) Create(file *database.UploadedFile) error {
	if r == nil || r.db == nil {
		return errors.New("repository: uploaded file repository not initialized")
	}
	if file == nil {
		return gorm.ErrInvalidData
	}
	merged, err := EnsureSameOrg(r.orgID, file.OrgID)
	if err != nil {
		return err
	}
	file.OrgID = merged
	file.UploaderUserID = strings.TrimSpace(file.UploaderUserID)
	file.StoredName = strings.TrimSpace(file.StoredName)
	file.OriginalName = strings.TrimSpace(file.OriginalName)
	if file.StoredName == "" || file.OriginalName == "" {
		return gorm.ErrInvalidData
	}
	return r.db.Create(file).Error
}

// FindByID 按主键在当前组织内查找；跨组织或不存在时返回 gorm.ErrRecordNotFound。
func (r *UploadedFileRepository) FindByID(id uint) (*database.UploadedFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository: uploaded file repository not initialized")
	}
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var file database.UploadedFile
	err := r.scoped().Where("id = ?", id).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// FindByStoredName 按磁盘文件名在当前组织内查找（迁移/运维用）。
func (r *UploadedFileRepository) FindByStoredName(storedName string) (*database.UploadedFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository: uploaded file repository not initialized")
	}
	storedName = strings.TrimSpace(storedName)
	if storedName == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var file database.UploadedFile
	err := r.scoped().Where("stored_name = ?", storedName).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// OrgID 返回仓储绑定的组织标识。
func (r *UploadedFileRepository) OrgID() string {
	if r == nil {
		return ""
	}
	return r.orgID
}
