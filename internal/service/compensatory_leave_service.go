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
	UserID         string  `json:"user_id"`
	BalanceMinutes int     `json:"balance_minutes"`
	BalanceHours   float64 `json:"balance_hours"`
}

func (s *CompensatoryLeaveService) GetBalance(userID string) (BalanceResult, error) {
	balance, err := s.ledgerRepo.GetBalance(userID)
	if err != nil {
		return BalanceResult{}, err
	}
	return BalanceResult{
		UserID:         userID,
		BalanceMinutes: balance,
		BalanceHours:   float64(balance) / 60.0,
	}, nil
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
