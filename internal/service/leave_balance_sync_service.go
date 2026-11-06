package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LeaveBalanceSyncService struct {
	db          *gorm.DB
	orgID       string
	snapshot    *repository.LeaveBalanceSnapshotRepository
	fetchQuota  func(string, string, string, int) (dingtalk.VacationQuotaSnapshot, error)
	resolveCode func(string, string) (string, error)
}

type LeaveBalanceSyncResult struct {
	Year      int      `json:"year"`
	Processed int      `json:"processed"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

func NewLeaveBalanceSyncServiceWithOrgID(db *gorm.DB, orgID string) *LeaveBalanceSyncService {
	svc := &LeaveBalanceSyncService{
		db:       db,
		orgID:    strings.TrimSpace(orgID),
		snapshot: repository.NewLeaveBalanceSnapshotRepositoryWithOrgID(db, orgID),
	}
	svc.fetchQuota = dingtalk.GetVacationQuotaForOrg
	svc.resolveCode = func(orgID, leaveType string) (string, error) {
		if leaveType == "annual" {
			return dingtalk.ResolveAnnualLeaveCodeForOrg(orgID)
		}
		return dingtalk.ResolveCompensatoryLeaveCodeForOrg(orgID)
	}
	return svc
}

func (s *LeaveBalanceSyncService) SyncUserYear(userID string, year int) (*LeaveBalanceSyncResult, error) {
	if strings.TrimSpace(s.orgID) == "" || strings.TrimSpace(userID) == "" || year <= 0 {
		return nil, fmt.Errorf("org_id, user_id and year are required")
	}
	dingTalkUserID, err := s.resolveDingTalkUserID(userID)
	if err != nil {
		return nil, err
	}
	result := &LeaveBalanceSyncResult{Year: year}
	for _, leaveType := range []string{"annual", "compensatory"} {
		result.Processed++
		code, err := s.resolveCode(s.orgID, leaveType)
		if err == nil {
			var quota dingtalk.VacationQuotaSnapshot
			quota, err = s.fetchQuota(s.orgID, dingTalkUserID, code, year)
			if err == nil {
				now := time.Now()
				snapshot := &database.LeaveBalanceSnapshot{
					OrgID: s.orgID, UserID: userID, Year: year, LeaveType: leaveType,
					LeaveCode: code, TotalMinutes: quota.TotalMinutes, UsedMinutes: quota.UsedMinutes,
					RemainingMinutes: quota.RemainingMinutes, SyncStatus: "success",
					SourceUpdatedAt: quota.SourceUpdatedAt, SyncedAt: now,
				}
				err = s.snapshot.Upsert(snapshot)
			}
		}
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", userID, leaveType, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

func (s *LeaveBalanceSyncService) resolveDingTalkUserID(userID string) (string, error) {
	var user database.User
	err := s.db.Where("org_id = ? AND (user_id = ? OR ding_talk_user_id = ?)", s.orgID, userID, userID).First(&user).Error
	if err != nil {
		return "", fmt.Errorf("load user for DingTalk balance sync: %w", err)
	}
	dingTalkUserID := strings.TrimSpace(user.DingTalkUserID)
	if dingTalkUserID == "" {
		return "", fmt.Errorf("user %q has no DingTalk user id", userID)
	}
	return dingTalkUserID, nil
}

func (s *LeaveBalanceSyncService) SyncUsersYear(userIDs []string, year int) (*LeaveBalanceSyncResult, error) {
	if strings.TrimSpace(s.orgID) == "" || year <= 0 {
		return nil, fmt.Errorf("org_id and year are required")
	}
	result := &LeaveBalanceSyncResult{Year: year}
	for _, userID := range userIDs {
		userResult, err := s.SyncUserYear(userID, year)
		if err != nil {
			return result, err
		}
		result.Processed += userResult.Processed
		result.Succeeded += userResult.Succeeded
		result.Failed += userResult.Failed
		result.Errors = append(result.Errors, userResult.Errors...)
	}
	return result, nil
}

func (s *LeaveBalanceSyncService) SyncAllUsersYear(year int) (*LeaveBalanceSyncResult, error) {
	var users []database.User
	if err := s.db.Where("org_id = ? AND status = ?", s.orgID, "active").Find(&users).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.UserID) != "" {
			ids = append(ids, user.UserID)
		}
	}
	return s.SyncUsersYear(ids, year)
}

func (s *LeaveBalanceSyncService) GetSnapshots(userID string, year int) ([]database.LeaveBalanceSnapshot, error) {
	return s.snapshot.FindByUserYear(userID, year)
}
