package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCrossOrgSyncRunning = errors.New("cross-organization sync already running")

type SourceEmployeeSnapshot struct {
	User           database.User
	Profile        *database.EmployeeProfile
	DepartmentName string
	SourceUpdated  time.Time
}

type BusinessMirrorRow struct {
	EntityType   string
	SourceKey    string
	SourceUserID string
	Payload      map[string]interface{}
	UpdatedAt    *time.Time
}

// EmployeeCenterRow is a read-only normalized employee row used by the
// 人事数据中心. SourceOrgID is the owning organization of the row; rows from
// the current organization are local data and rows from linked organizations
// are mirror snapshots.
type EmployeeCenterRow struct {
	ID             uint
	SourceOrgID    string
	SourceUserID   string
	EmployeeID     string
	Name           string
	Email          string
	Mobile         string
	DepartmentName string
	Position       string
	Status         string
	LastSyncedAt   time.Time
	SourceUpdated  *time.Time
	IsMirror       bool
}

type BusinessCenterRow struct {
	ID            uint
	SourceOrgID   string
	EntityType    string
	SourceKey     string
	SourceUserID  string
	Payload       map[string]interface{}
	SourceUpdated *time.Time
	LastSyncedAt  time.Time
	SyncStatus    string
	IsMirror      bool
}

type CrossOrganizationSyncRepository struct {
	db *gorm.DB
}

func NewCrossOrganizationSyncRepository(db *gorm.DB) *CrossOrganizationSyncRepository {
	return &CrossOrganizationSyncRepository{db: db}
}

func requireSyncOrgPair(sourceOrgID, targetOrgID string) (string, string, error) {
	sourceOrgID, err := RequireOrgID(sourceOrgID)
	if err != nil {
		return "", "", err
	}
	targetOrgID, err = RequireOrgID(targetOrgID)
	if err != nil {
		return "", "", err
	}
	if sourceOrgID == targetOrgID {
		return "", "", fmt.Errorf("source and target organization must differ")
	}
	return sourceOrgID, targetOrgID, nil
}

func (r *CrossOrganizationSyncRepository) CreateOrUpdateLink(ctx context.Context, link *database.OrganizationSyncLink) error {
	if r == nil || r.db == nil || link == nil {
		return gorm.ErrInvalidData
	}
	source, target, err := requireSyncOrgPair(link.SourceOrgID, link.TargetOrgID)
	if err != nil {
		return err
	}
	link.SourceOrgID, link.TargetOrgID = source, target
	if strings.TrimSpace(link.Status) == "" {
		link.Status = "active"
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source_org_id"}, {Name: "target_org_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "employee_sync", "business_sync", "business_scopes", "updated_at"}),
	}).Create(link).Error
}

func (r *CrossOrganizationSyncRepository) FindLink(ctx context.Context, sourceOrgID, targetOrgID string) (*database.OrganizationSyncLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	source, target, err := requireSyncOrgPair(sourceOrgID, targetOrgID)
	if err != nil {
		return nil, err
	}
	var link database.OrganizationSyncLink
	err = r.db.WithContext(ctx).
		Where("source_org_id = ? AND target_org_id = ? AND deleted_at IS NULL", source, target).
		First(&link).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *CrossOrganizationSyncRepository) ListLinks(ctx context.Context, sourceOrgID string) ([]database.OrganizationSyncLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sourceOrgID, err := RequireOrgID(sourceOrgID)
	if err != nil {
		return nil, err
	}
	var links []database.OrganizationSyncLink
	err = r.db.WithContext(ctx).
		Where("source_org_id = ? AND deleted_at IS NULL", sourceOrgID).
		Order("id ASC").Find(&links).Error
	return links, err
}

// ListTargetLinks lists configured inbound mirror links for a target org. The
// target is always supplied by the authenticated tenant context; callers
// cannot use this method to select an arbitrary source outside a saved link.
func (r *CrossOrganizationSyncRepository) ListTargetLinks(ctx context.Context, targetOrgID string) ([]database.OrganizationSyncLink, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	var links []database.OrganizationSyncLink
	err = r.db.WithContext(ctx).
		Where("target_org_id = ? AND deleted_at IS NULL", targetOrgID).
		Order("id ASC").Find(&links).Error
	return links, err
}

func (r *CrossOrganizationSyncRepository) StartRun(ctx context.Context, sourceOrgID, targetOrgID, requestID, triggeredBy string) (*database.OrganizationSyncRun, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	source, target, err := requireSyncOrgPair(sourceOrgID, targetOrgID)
	if err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request id is required")
	}
	var run database.OrganizationSyncRun
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.OrganizationSyncRun{}).
			Where("source_org_id = ? AND target_org_id = ? AND status = ?", source, target, "running").
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrCrossOrgSyncRunning
		}
		now := time.Now()
		run = database.OrganizationSyncRun{
			RequestID: requestID, SourceOrgID: source, TargetOrgID: target,
			Status: "running", TriggeredBy: strings.TrimSpace(triggeredBy), StartedAt: now,
		}
		return tx.Create(&run).Error
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *CrossOrganizationSyncRepository) FinishRun(ctx context.Context, runID uint, status string, employeeCount, businessCount, failureCount int, message string) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	status = strings.TrimSpace(status)
	if status != "success" && status != "partial" && status != "failed" {
		return fmt.Errorf("invalid sync terminal status %q", status)
	}
	now := time.Now()
	return r.db.WithContext(ctx).Model(&database.OrganizationSyncRun{}).
		Where("id = ?", runID).
		Updates(map[string]interface{}{
			"status": status, "employee_count": employeeCount, "business_count": businessCount,
			"failure_count": failureCount, "error_message": strings.TrimSpace(message), "finished_at": &now,
		}).Error
}

func (r *CrossOrganizationSyncRepository) FindRun(ctx context.Context, sourceOrgID, requestID string) (*database.OrganizationSyncRun, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sourceOrgID, err := RequireOrgID(sourceOrgID)
	if err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var run database.OrganizationSyncRun
	err = r.db.WithContext(ctx).Where("source_org_id = ? AND request_id = ?", sourceOrgID, requestID).First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *CrossOrganizationSyncRepository) FindRunByTarget(ctx context.Context, targetOrgID, requestID string) (*database.OrganizationSyncRun, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var run database.OrganizationSyncRun
	err = r.db.WithContext(ctx).Where("target_org_id = ? AND request_id = ?", targetOrgID, requestID).First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *CrossOrganizationSyncRepository) ListSourceEmployees(ctx context.Context, sourceOrgID string) ([]SourceEmployeeSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sourceOrgID, err := RequireOrgID(sourceOrgID)
	if err != nil {
		return nil, err
	}
	var users []database.User
	if err := r.db.WithContext(ctx).Where("org_id = ? AND deleted_at IS NULL", sourceOrgID).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return []SourceEmployeeSnapshot{}, nil
	}
	userIDs := make([]string, 0, len(users))
	deptIDs := make([]string, 0, len(users))
	seenDept := map[string]struct{}{}
	for _, user := range users {
		userIDs = append(userIDs, user.UserID)
		if dept := strings.TrimSpace(user.DepartmentID); dept != "" {
			if _, ok := seenDept[dept]; !ok {
				seenDept[dept] = struct{}{}
				deptIDs = append(deptIDs, dept)
			}
		}
	}
	var profiles []database.EmployeeProfile
	if err := r.db.WithContext(ctx).Where("org_id = ? AND user_id IN ? AND deleted_at IS NULL", sourceOrgID, userIDs).Find(&profiles).Error; err != nil {
		return nil, err
	}
	profileByUser := make(map[string]*database.EmployeeProfile, len(profiles))
	for i := range profiles {
		profile := profiles[i]
		profileByUser[profile.UserID] = &profile
	}
	deptNames := map[string]string{}
	if len(deptIDs) > 0 {
		var departments []database.Department
		if err := r.db.WithContext(ctx).Where("org_id = ? AND department_id IN ? AND deleted_at IS NULL", sourceOrgID, deptIDs).Find(&departments).Error; err != nil {
			return nil, err
		}
		for _, dept := range departments {
			deptNames[dept.DepartmentID] = dept.Name
		}
	}
	result := make([]SourceEmployeeSnapshot, 0, len(users))
	for _, user := range users {
		updated := user.UpdatedAt
		if profile := profileByUser[user.UserID]; profile != nil && profile.UpdatedAt.After(updated) {
			updated = profile.UpdatedAt
		}
		result = append(result, SourceEmployeeSnapshot{
			User: user, Profile: profileByUser[user.UserID], DepartmentName: deptNames[user.DepartmentID], SourceUpdated: updated,
		})
	}
	return result, nil
}

type businessMirrorTableSpec struct {
	table      string
	softDelete bool
}

var businessMirrorTables = map[string]businessMirrorTableSpec{
	"attendance":              {table: "attendances", softDelete: true},
	"approval":                {table: "approvals", softDelete: true},
	"annual_leave_grant":      {table: "annual_leave_grants"},
	"overtime_match":          {table: "overtime_match_results", softDelete: true},
	"performance_activity":    {table: "performance_activities", softDelete: true},
	"performance_participant": {table: "performance_participants", softDelete: true},
}

func (r *CrossOrganizationSyncRepository) ListBusinessRows(ctx context.Context, sourceOrgID, entityType string) ([]BusinessMirrorRow, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	sourceOrgID, err := RequireOrgID(sourceOrgID)
	if err != nil {
		return nil, err
	}
	entityType = strings.TrimSpace(entityType)
	spec, ok := businessMirrorTables[entityType]
	if !ok {
		return nil, fmt.Errorf("unsupported business mirror entity %q", entityType)
	}
	var rows []map[string]interface{}
	query := r.db.WithContext(ctx).Table(spec.table).Where("org_id = ?", sourceOrgID)
	if spec.softDelete {
		query = query.Where("deleted_at IS NULL")
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]BusinessMirrorRow, 0, len(rows))
	for _, row := range rows {
		sourceKey := strings.TrimSpace(fmt.Sprint(row["id"]))
		if sourceKey == "" || sourceKey == "<nil>" {
			continue
		}
		userID := firstMapString(row, "user_id", "applicant_id", "employee_id")
		updated := mapTime(row["updated_at"])
		result = append(result, BusinessMirrorRow{EntityType: entityType, SourceKey: sourceKey, SourceUserID: userID, Payload: row, UpdatedAt: updated})
	}
	return result, nil
}

func (r *CrossOrganizationSyncRepository) UpsertEmployeeMirror(ctx context.Context, mirror *database.OrganizationEmployeeMirror) error {
	if r == nil || r.db == nil || mirror == nil {
		return gorm.ErrInvalidData
	}
	target, err := RequireOrgID(mirror.TargetOrgID)
	if err != nil {
		return err
	}
	source, err := RequireOrgID(mirror.SourceOrgID)
	if err != nil {
		return err
	}
	if target == source || strings.TrimSpace(mirror.SourceUserID) == "" {
		return fmt.Errorf("invalid employee mirror identity")
	}
	mirror.TargetOrgID, mirror.SourceOrgID = target, source
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "target_org_id"}, {Name: "source_org_id"}, {Name: "source_user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"source_ding_talk_id", "employee_id", "name", "email", "mobile", "department_id", "department_name", "position", "status", "profile", "source_updated_at", "last_synced_at", "last_sync_run_id", "sync_status", "sync_error", "updated_at"}),
	}).Create(mirror).Error
}

func (r *CrossOrganizationSyncRepository) UpsertBusinessMirror(ctx context.Context, mirror *database.OrganizationBusinessMirror) error {
	if r == nil || r.db == nil || mirror == nil {
		return gorm.ErrInvalidData
	}
	target, err := RequireOrgID(mirror.TargetOrgID)
	if err != nil {
		return err
	}
	source, err := RequireOrgID(mirror.SourceOrgID)
	if err != nil {
		return err
	}
	if target == source || strings.TrimSpace(mirror.EntityType) == "" || strings.TrimSpace(mirror.SourceKey) == "" {
		return fmt.Errorf("invalid business mirror identity")
	}
	mirror.TargetOrgID, mirror.SourceOrgID = target, source
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "target_org_id"}, {Name: "source_org_id"}, {Name: "entity_type"}, {Name: "source_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"source_user_id", "payload", "source_updated_at", "last_synced_at", "last_sync_run_id", "sync_status", "sync_error", "updated_at"}),
	}).Create(mirror).Error
}

func (r *CrossOrganizationSyncRepository) ListEmployeeMirrors(ctx context.Context, targetOrgID string, limit int) ([]database.OrganizationEmployeeMirror, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var mirrors []database.OrganizationEmployeeMirror
	err = r.db.WithContext(ctx).Where("target_org_id = ?", targetOrgID).Order("id ASC").Limit(limit).Find(&mirrors).Error
	return mirrors, err
}

func (r *CrossOrganizationSyncRepository) ListBusinessMirrors(ctx context.Context, targetOrgID, entityType string, limit int) ([]database.OrganizationBusinessMirror, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := r.db.WithContext(ctx).Where("target_org_id = ?", targetOrgID)
	if entityType = strings.TrimSpace(entityType); entityType != "" {
		if _, ok := businessMirrorTables[entityType]; !ok {
			return nil, fmt.Errorf("unsupported business mirror entity %q", entityType)
		}
		query = query.Where("entity_type = ?", entityType)
	}
	var mirrors []database.OrganizationBusinessMirror
	err = query.Order("id ASC").Limit(limit).Find(&mirrors).Error
	return mirrors, err
}

// ListLocalEmployeeCenterRows returns the current organization's employees
// using the same user/profile source as the organization roster. It does not
// include mirrored rows.
func (r *CrossOrganizationSyncRepository) ListLocalEmployeeCenterRows(ctx context.Context, orgID string) ([]EmployeeCenterRow, error) {
	snapshots, err := r.ListSourceEmployees(ctx, orgID)
	if err != nil {
		return nil, err
	}
	rows := make([]EmployeeCenterRow, 0, len(snapshots))
	for _, item := range snapshots {
		row := EmployeeCenterRow{ID: item.User.ID, SourceOrgID: orgID, SourceUserID: item.User.UserID, Name: item.User.Name, Email: item.User.Email, Mobile: item.User.Mobile, Position: item.User.Position, Status: item.User.Status, IsMirror: false}
		row.DepartmentName = item.DepartmentName
		row.LastSyncedAt = item.User.UpdatedAt
		if item.Profile != nil {
			row.EmployeeID = item.Profile.EmployeeID
			if item.Profile.UpdatedAt.After(row.LastSyncedAt) {
				row.LastSyncedAt = item.Profile.UpdatedAt
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *CrossOrganizationSyncRepository) ListAllEmployeeMirrors(ctx context.Context, targetOrgID string) ([]database.OrganizationEmployeeMirror, error) {
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	var mirrors []database.OrganizationEmployeeMirror
	err = r.db.WithContext(ctx).Where("target_org_id = ?", targetOrgID).Order("id ASC").Find(&mirrors).Error
	return mirrors, err
}

func (r *CrossOrganizationSyncRepository) ListAllBusinessMirrors(ctx context.Context, targetOrgID, entityType string) ([]database.OrganizationBusinessMirror, error) {
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx).Where("target_org_id = ?", targetOrgID)
	if entityType = strings.TrimSpace(entityType); entityType != "" {
		if _, ok := businessMirrorTables[entityType]; !ok {
			return nil, fmt.Errorf("unsupported business mirror entity %q", entityType)
		}
		query = query.Where("entity_type = ?", entityType)
	}
	var mirrors []database.OrganizationBusinessMirror
	err = query.Order("id ASC").Find(&mirrors).Error
	return mirrors, err
}

func (r *CrossOrganizationSyncRepository) CountLocalEmployees(ctx context.Context, orgID string) (total, active int64, err error) {
	orgID, err = RequireOrgID(orgID)
	if err != nil {
		return 0, 0, err
	}
	query := r.db.WithContext(ctx).Model(&database.User{}).Where("org_id = ? AND deleted_at IS NULL", orgID)
	if err = query.Count(&total).Error; err != nil {
		return 0, 0, err
	}
	err = query.Where("status = ?", "active").Count(&active).Error
	return total, active, err
}

func (r *CrossOrganizationSyncRepository) CountEmployeeMirrors(ctx context.Context, targetOrgID string) (total, active int64, err error) {
	targetOrgID, err = RequireOrgID(targetOrgID)
	if err != nil {
		return 0, 0, err
	}
	query := r.db.WithContext(ctx).Model(&database.OrganizationEmployeeMirror{}).Where("target_org_id = ?", targetOrgID)
	if err = query.Count(&total).Error; err != nil {
		return 0, 0, err
	}
	err = query.Where("status = ?", "active").Count(&active).Error
	return total, active, err
}

func (r *CrossOrganizationSyncRepository) CountLocalBusinessRows(ctx context.Context, orgID string) (int64, error) {
	orgID, err := RequireOrgID(orgID)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, spec := range businessMirrorTables {
		query := r.db.WithContext(ctx).Table(spec.table).Where("org_id = ?", orgID)
		if spec.softDelete {
			query = query.Where("deleted_at IS NULL")
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (r *CrossOrganizationSyncRepository) CountBusinessMirrors(ctx context.Context, targetOrgID string) (int64, error) {
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return 0, err
	}
	var total int64
	err = r.db.WithContext(ctx).Model(&database.OrganizationBusinessMirror{}).Where("target_org_id = ?", targetOrgID).Count(&total).Error
	return total, err
}

func (r *CrossOrganizationSyncRepository) LatestRunForTarget(ctx context.Context, targetOrgID string) (*database.OrganizationSyncRun, error) {
	targetOrgID, err := RequireOrgID(targetOrgID)
	if err != nil {
		return nil, err
	}
	var run database.OrganizationSyncRun
	err = r.db.WithContext(ctx).Where("target_org_id = ?", targetOrgID).Order("started_at DESC, id DESC").First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func firstMapString(row map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(row[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func mapTime(value interface{}) *time.Time {
	switch typed := value.(type) {
	case time.Time:
		return &typed
	case *time.Time:
		return typed
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, typed); err == nil {
			return &parsed
		}
	}
	return nil
}
