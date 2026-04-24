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
	UserID         string `json:"user_id"`
	BalanceMinutes int    `json:"balance_minutes"`
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

func (s *CompensatoryLeaveService) CreditFromOvertime(matchID uint) error {
	exists, err := s.ledgerRepo.ExistsBySourceMatch(matchID)
	if err != nil {
		return err
	}
	if exists {
		return nil // 幂等
	}

	// 通过matchID查找
	var m database.OvertimeMatchResult
	if err := s.db.First(&m, matchID).Error; err != nil {
		return fmt.Errorf("匹配记录不存在: %w", err)
	}

	currentBalance, _ := s.ledgerRepo.GetBalance(m.UserID)

	ledger := &database.CompensatoryLeaveLedger{
		UserID:         m.UserID,
		SourceType:     "overtime",
		SourceMatchID:  matchID,
		CreditMinutes:  m.QualifiedMinutes,
		DebitMinutes:   0,
		BalanceMinutes: currentBalance + m.QualifiedMinutes,
		LedgerType:     "credit",
		EffectiveDate:  m.ApprovalStartTime.Format("2006-01-02"),
		Remark:         fmt.Sprintf("加班审批%d匹配，获得%d分钟调休", m.ApprovalID, m.QualifiedMinutes),
	}
	return s.ledgerRepo.Create(ledger)
}

func (s *CompensatoryLeaveService) RollbackCredit(matchID uint) error {
	existing, err := s.ledgerRepo.FindBySourceMatch(matchID)
	if err == gorm.ErrRecordNotFound {
		return nil // 无需回滚
	}
	if err != nil {
		return err
	}

	var m database.OvertimeMatchResult
	if err := s.db.First(&m, matchID).Error; err != nil {
		return err
	}

	currentBalance, _ := s.ledgerRepo.GetBalance(m.UserID)

	rollback := &database.CompensatoryLeaveLedger{
		UserID:         m.UserID,
		SourceType:     "overtime",
		SourceMatchID:  matchID,
		CreditMinutes:  0,
		DebitMinutes:   existing.CreditMinutes,
		BalanceMinutes: currentBalance - existing.CreditMinutes,
		LedgerType:     "rollback",
		EffectiveDate:  time.Now().Format("2006-01-02"),
		Remark:         fmt.Sprintf("回滚匹配记录%d的调休积分", matchID),
	}
	return s.ledgerRepo.Create(rollback)
}

// parseJSON 辅助函数（供 overtime_matching_service 调用）
func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
