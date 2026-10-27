package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LeaveBalanceSnapshotRepository struct {
	db    *gorm.DB
	orgID string
}

func NewLeaveBalanceSnapshotRepositoryWithOrgID(db *gorm.DB, orgID string) *LeaveBalanceSnapshotRepository {
	return &LeaveBalanceSnapshotRepository{db: db, orgID: strings.TrimSpace(orgID)}
}

func (r *LeaveBalanceSnapshotRepository) scoped() *gorm.DB {
	return r.db.Scopes(ScopeOrg(r.orgID, "org_id"))
}

func (r *LeaveBalanceSnapshotRepository) Upsert(snapshot *database.LeaveBalanceSnapshot) error {
	if strings.TrimSpace(r.orgID) == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(r.orgID, snapshot.OrgID)
	if err != nil {
		return err
	}
	snapshot.OrgID = merged
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "user_id"}, {Name: "year"}, {Name: "leave_type"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"leave_code", "total_minutes", "used_minutes", "remaining_minutes",
			"sync_status", "sync_error", "source_updated_at", "synced_at", "updated_at",
		}),
	}).Create(snapshot).Error
}

func (r *LeaveBalanceSnapshotRepository) FindByUserYearType(userID string, year int, leaveType string) (*database.LeaveBalanceSnapshot, error) {
	var snapshot database.LeaveBalanceSnapshot
	err := r.scoped().Where("user_id = ? AND year = ? AND leave_type = ?", userID, year, leaveType).First(&snapshot).Error
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (r *LeaveBalanceSnapshotRepository) FindByUserYear(userID string, year int) ([]database.LeaveBalanceSnapshot, error) {
	var snapshots []database.LeaveBalanceSnapshot
	err := r.scoped().Where("user_id = ? AND year = ?", userID, year).Order("leave_type asc").Find(&snapshots).Error
	return snapshots, err
}
