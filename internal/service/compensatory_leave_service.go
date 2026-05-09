package service

import (
	"encoding/json"
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"time"

	"gorm.io/gorm"
)

type CompensatoryLeaveService struct {
	db         *gorm.DB
	ledgerRepo *repository.CompensatoryLeaveLedgerRepository
	matchRepo  *repository.OvertimeMatchResultRepository
}

func NewCompensatoryLeaveService(db *gorm.DB) *CompensatoryLeaveService {
	return &CompensatoryLeaveService{
		db:         db,
		ledgerRepo: repository.NewCompensatoryLeaveLedgerRepository(db),
		matchRepo:  repository.NewOvertimeMatchResultRepository(db),
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
	var m database.OvertimeMatchResult
	if err := s.db.First(&m, matchID).Error; err != nil {
		return fmt.Errorf("匹配记录不存在: %w", err)
	}

	exists, err := s.ledgerRepo.ExistsBySourceMatchKey(m.MatchRef, matchID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	currentBalance, _ := s.ledgerRepo.GetBalance(m.UserID)

	ledger := &database.CompensatoryLeaveLedger{
		UserID:         m.UserID,
		SourceType:     "overtime",
		SourceMatchID:  matchID,
		SourceMatchRef: m.MatchRef,
		CreditMinutes:  m.EffectiveOvertimeMinutes,
		DebitMinutes:   0,
		BalanceMinutes: currentBalance + m.EffectiveOvertimeMinutes,
		LedgerType:     "credit",
		EffectiveDate:  m.WorkDate,
		Remark:         fmt.Sprintf("加班审批%d匹配，获得%d分钟调休", m.ApprovalID, m.EffectiveOvertimeMinutes),
	}
	return s.ledgerRepo.Create(ledger)
}

func (s *CompensatoryLeaveService) RollbackCredit(matchID uint) error {
	var m database.OvertimeMatchResult
	if err := s.db.First(&m, matchID).Error; err != nil {
		return err
	}

	existing, err := s.ledgerRepo.FindBySourceMatchKey(m.MatchRef, matchID)
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return err
	}

	currentBalance, _ := s.ledgerRepo.GetBalance(m.UserID)

	rollback := &database.CompensatoryLeaveLedger{
		UserID:         m.UserID,
		SourceType:     "overtime",
		SourceMatchID:  matchID,
		SourceMatchRef: m.MatchRef,
		CreditMinutes:  0,
		DebitMinutes:   existing.CreditMinutes,
		BalanceMinutes: currentBalance - existing.CreditMinutes,
		LedgerType:     "rollback",
		EffectiveDate:  rollbackEffectiveDate(existing.EffectiveDate, m.WorkDate),
		Remark:         fmt.Sprintf("回滚匹配记录%d的调休积分", matchID),
	}
	return s.ledgerRepo.Create(rollback)
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

func rollbackEffectiveDate(existingDate, workDate string) string {
	if existingDate != "" {
		return existingDate
	}
	if workDate != "" {
		return workDate
	}
	return time.Now().Format("2006-01-02")
}
