package service

import (
	"encoding/json"
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CompensatoryLeaveService struct {
	db         *gorm.DB
	ledgerRepo *repository.CompensatoryLeaveLedgerRepository
	matchRepo  *repository.OvertimeMatchResultRepository
	orgID      string
}

func NewCompensatoryLeaveService(db *gorm.DB) *CompensatoryLeaveService {
	return &CompensatoryLeaveService{
		db:         db,
		ledgerRepo: repository.NewCompensatoryLeaveLedgerRepository(db),
		matchRepo:  repository.NewOvertimeMatchResultRepository(db),
	}
}

func NewCompensatoryLeaveServiceWithOrgID(db *gorm.DB, orgID string) *CompensatoryLeaveService {
	return &CompensatoryLeaveService{
		db:         db,
		ledgerRepo: repository.NewCompensatoryLeaveLedgerRepositoryWithOrgID(db, orgID),
		matchRepo:  repository.NewOvertimeMatchResultRepositoryWithOrgID(db, orgID),
		orgID:      orgID,
	}
}

type BalanceResult struct {
	UserID                   string     `json:"user_id"`
	BalanceMinutes           int        `json:"balance_minutes"`
	BalanceHours             float64    `json:"balance_hours"`
	DingTalkRemainingMinutes int        `json:"dingtalk_remaining_minutes"`
	DingTalkSyncedAt         *time.Time `json:"dingtalk_synced_at,omitempty"`
}

type CompensatoryMutationResult struct {
	Changed     bool
	Status      string
	OperationNo int
}

func (s *CompensatoryLeaveService) GetBalance(userID string) (BalanceResult, error) {
	balance, err := s.ledgerRepo.GetBalance(userID)
	if err != nil {
		return BalanceResult{}, err
	}
	result := BalanceResult{
		UserID:         userID,
		BalanceMinutes: balance,
		BalanceHours:   float64(balance) / 60.0,
	}
	if strings.TrimSpace(s.orgID) != "" {
		var snapshot database.LeaveBalanceSnapshot
		if err := s.db.Where("org_id = ? AND user_id = ? AND year = ? AND leave_type = ? AND sync_status = ?", s.orgID, userID, time.Now().Year(), "compensatory", "success").First(&snapshot).Error; err == nil {
			result.DingTalkRemainingMinutes = snapshot.RemainingMinutes
			result.DingTalkSyncedAt = &snapshot.SyncedAt
		}
	}
	return result, nil
}

func (s *CompensatoryLeaveService) GetOvertimeBalanceByYear(userID string, year int) (int, error) {
	return s.ledgerRepo.GetBalanceByUserYearAndSource(userID, year, "overtime")
}

func (s *CompensatoryLeaveService) CreditFromOvertime(matchID uint) error {
	return s.mutateOvertimeCredit(matchID, false)
}

func (s *CompensatoryLeaveService) RollbackCredit(matchID uint) error {
	return s.mutateOvertimeCredit(matchID, true)
}

// ConsumeForApproval debits locally available compensatory time for one
// approved leave request. The approval reference is the idempotency key.
func (s *CompensatoryLeaveService) ConsumeForApproval(userID string, minutes int, approvalRef, effectiveDate, remark string) (CompensatoryMutationResult, error) {
	if strings.TrimSpace(s.orgID) == "" {
		return CompensatoryMutationResult{}, fmt.Errorf("org_id required for compensatory leave mutation")
	}
	if strings.TrimSpace(userID) == "" || minutes <= 0 || strings.TrimSpace(approvalRef) == "" {
		return CompensatoryMutationResult{}, fmt.Errorf("user_id, minutes and approval_ref are required")
	}
	if effectiveDate == "" {
		effectiveDate = time.Now().Format("2006-01-02")
	}
	result := CompensatoryMutationResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing database.CompensatoryLeaveLedger
		if err := tx.Where("org_id = ? AND user_id = ? AND source_type = ? AND source_match_ref = ? AND ledger_type = ?", s.orgID, userID, "leave", approvalRef, "debit").First(&existing).Error; err == nil {
			result = CompensatoryMutationResult{Changed: false, Status: "applied"}
			return nil
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		var current int
		if err := tx.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND user_id = ?", s.orgID, userID).
			Clauses(clause.Locking{Strength: "UPDATE"}).Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").Scan(&current).Error; err != nil {
			return err
		}
		if current < minutes {
			return fmt.Errorf("调休余额不足：可用%d分钟，请求%d分钟", current, minutes)
		}
		entry := database.CompensatoryLeaveLedger{
			OrgID: s.orgID, UserID: userID, SourceType: "leave", SourceMatchRef: approvalRef,
			DebitMinutes: minutes, BalanceMinutes: current - minutes, LedgerType: "debit",
			EffectiveDate: effectiveDate, Remark: remark,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		result = CompensatoryMutationResult{Changed: true, Status: "applied", OperationNo: 1}
		return nil
	})
	return result, err
}

// RollbackApproval restores a debit exactly once when a leave approval is
// later rejected/cancelled.
func (s *CompensatoryLeaveService) RollbackApproval(approvalRef, remark string) (CompensatoryMutationResult, error) {
	if strings.TrimSpace(s.orgID) == "" || strings.TrimSpace(approvalRef) == "" {
		return CompensatoryMutationResult{}, fmt.Errorf("org_id and approval_ref are required")
	}
	result := CompensatoryMutationResult{Status: "reversed"}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var debit database.CompensatoryLeaveLedger
		if err := tx.Where("org_id = ? AND source_type = ? AND source_match_ref = ? AND ledger_type = ?", s.orgID, "leave", approvalRef, "debit").First(&debit).Error; err == gorm.ErrRecordNotFound {
			result.Status = "skipped"
			return nil
		} else if err != nil {
			return err
		}
		var rollback database.CompensatoryLeaveLedger
		if err := tx.Where("org_id = ? AND source_type = ? AND source_match_ref = ? AND ledger_type = ?", s.orgID, "leave", approvalRef, "rollback").First(&rollback).Error; err == nil {
			result.Status = "skipped"
			return nil
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
		var current int
		if err := tx.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND user_id = ?", s.orgID, debit.UserID).
			Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").Scan(&current).Error; err != nil {
			return err
		}
		if err := tx.Create(&database.CompensatoryLeaveLedger{
			OrgID: s.orgID, UserID: debit.UserID, SourceType: "leave", SourceMatchRef: approvalRef,
			CreditMinutes: debit.DebitMinutes, BalanceMinutes: current + debit.DebitMinutes,
			LedgerType: "rollback", EffectiveDate: debit.EffectiveDate, Remark: remark,
		}).Error; err != nil {
			return err
		}
		result = CompensatoryMutationResult{Changed: true, Status: "reversed", OperationNo: 2}
		return nil
	})
	return result, err
}

func (s *CompensatoryLeaveService) mutateOvertimeCredit(matchID uint, rollback bool) error {
	orgID := strings.TrimSpace(s.orgID)
	if orgID == "" {
		return fmt.Errorf("org_id required for compensatory leave mutation")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var match database.OvertimeMatchResult
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ?", matchID, orgID).First(&match).Error; err != nil {
			return fmt.Errorf("匹配记录不存在: %w", err)
		}
		query := tx.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ?", orgID)
		if match.MatchRef != "" {
			query = query.Where("source_match_ref = ?", match.MatchRef)
		} else {
			query = query.Where("source_match_id = ?", matchID)
		}
		var sourceBalance int
		if err := query.Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").Scan(&sourceBalance).Error; err != nil {
			return err
		}
		if rollback && sourceBalance <= 0 {
			return nil
		}
		if !rollback && sourceBalance > 0 {
			return nil
		}
		if !rollback && sourceBalance < 0 {
			return fmt.Errorf("negative compensatory source balance for match %d", matchID)
		}
		var currentBalance int
		if err := tx.Model(&database.CompensatoryLeaveLedger{}).
			Where("org_id = ? AND user_id = ?", orgID, match.UserID).
			Select("COALESCE(SUM(credit_minutes) - SUM(debit_minutes), 0)").Scan(&currentBalance).Error; err != nil {
			return err
		}
		minutes := match.EffectiveOvertimeMinutes
		ledger := database.CompensatoryLeaveLedger{
			OrgID:          orgID,
			UserID:         match.UserID,
			SourceType:     "overtime",
			SourceMatchID:  matchID,
			SourceMatchRef: match.MatchRef,
			EffectiveDate:  match.WorkDate,
		}
		if rollback {
			ledger.DebitMinutes = sourceBalance
			ledger.BalanceMinutes = currentBalance - sourceBalance
			ledger.LedgerType = "rollback"
			ledger.Remark = fmt.Sprintf("回滚匹配记录%d的调休积分", matchID)
		} else {
			ledger.CreditMinutes = minutes
			ledger.BalanceMinutes = currentBalance + minutes
			ledger.LedgerType = "credit"
			ledger.Remark = fmt.Sprintf("加班审批%d匹配，获得%d分钟调休", match.ApprovalID, minutes)
		}
		return tx.Create(&ledger).Error
	})
}

// ManualCredit 手动发放调休
func (s *CompensatoryLeaveService) ManualCredit(userID string, minutes int, effectiveDate string, remark string) error {
	if minutes <= 0 {
		return fmt.Errorf("调休分钟数必须大于0")
	}
	if userID == "" {
		return fmt.Errorf("用户ID不能为空")
	}
	if effectiveDate == "" {
		effectiveDate = time.Now().Format("2006-01-02")
	}

	currentBalance, _ := s.ledgerRepo.GetBalance(userID)

	ledger := &database.CompensatoryLeaveLedger{
		UserID:         userID,
		SourceType:     "manual",
		SourceMatchID:  0,
		CreditMinutes:  minutes,
		DebitMinutes:   0,
		BalanceMinutes: currentBalance + minutes,
		LedgerType:     "credit",
		EffectiveDate:  effectiveDate,
		Remark:         remark,
	}
	return s.ledgerRepo.Create(ledger)
}

// parseJSON 辅助函数（供 overtime_matching_service 调用）
func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
