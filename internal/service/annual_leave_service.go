package service

import (
	"encoding/json"
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"time"

	"gorm.io/gorm"
)

type AnnualLeaveService struct {
	db              *gorm.DB
	eligibilityRepo *repository.AnnualLeaveEligibilityRepository
	ruleRepo        *repository.LeaveRuleConfigRepository
}

func NewAnnualLeaveService(db *gorm.DB) *AnnualLeaveService {
	return &AnnualLeaveService{
		db:              db,
		eligibilityRepo: repository.NewAnnualLeaveEligibilityRepository(db),
		ruleRepo:        repository.NewLeaveRuleConfigRepository(db),
	}
}

type EligibilityResult struct {
	UserID                   string `json:"user_id"`
	Year                     int    `json:"year"`
	Quarter                  int    `json:"quarter"`
	EntryDate                string `json:"entry_date"`
	ConfirmationDate         string `json:"confirmation_date"`
	IsEligible               bool   `json:"is_eligible"`
	EligibleStartDate        string `json:"eligible_start_date"`
	EligibleEndDate          string `json:"eligible_end_date"`
	RetroactiveSourceQuarter int    `json:"retroactive_source_quarter"`
	CalcReason               string `json:"calc_reason"`
}

// quarterStart 返回某年某季度的第一天
func quarterStart(year, quarter int) time.Time {
	month := time.Month((quarter-1)*3 + 1)
	return time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
}

// quarterEnd 返回某年某季度的最后一天
func quarterEnd(year, quarter int) time.Time {
	return quarterStart(year, quarter).AddDate(0, 3, -1)
}

func (s *AnnualLeaveService) RecalculateEligibility(userID string, year int) error {
	var profile database.EmployeeProfile
	if err := s.db.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		return fmt.Errorf("员工档案不存在: %w", err)
	}

	retroactiveEnabled := s.isRetroactiveEnabled()

	for q := 1; q <= 4; q++ {
		elig := s.calcQuarterEligibility(&profile, year, q, retroactiveEnabled)
		if err := s.eligibilityRepo.Upsert(elig); err != nil {
			return fmt.Errorf("保存季度%d资格失败: %w", q, err)
		}
	}
	return nil
}

func (s *AnnualLeaveService) RecalculateEligibilityBatch(year int, userIDs []string) error {
	for _, uid := range userIDs {
		if err := s.RecalculateEligibility(uid, year); err != nil {
			return err
		}
	}
	return nil
}

func (s *AnnualLeaveService) GetEligibility(userID string, year int) ([]EligibilityResult, error) {
	calcErr := s.RecalculateEligibility(userID, year)
	rows, err := s.eligibilityRepo.FindByUserYear(userID, year)
	if err != nil {
		return nil, err
	}
	if calcErr != nil && len(rows) == 0 {
		return nil, calcErr
	}
	var results []EligibilityResult
	for _, r := range rows {
		results = append(results, EligibilityResult{
			UserID:                   r.UserID,
			Year:                     r.Year,
			Quarter:                  r.Quarter,
			EntryDate:                r.EntryDate,
			ConfirmationDate:         r.ConfirmationDate,
			IsEligible:               r.IsEligible,
			EligibleStartDate:        r.EligibleStartDate,
			EligibleEndDate:          r.EligibleEndDate,
			RetroactiveSourceQuarter: r.RetroactiveSourceQuarter,
			CalcReason:               r.CalcReason,
		})
	}
	return results, nil
}

func (s *AnnualLeaveService) calcQuarterEligibility(profile *database.EmployeeProfile, year, quarter int, retroactiveEnabled bool) *database.AnnualLeaveEligibility {
	qStart := quarterStart(year, quarter)
	qEnd := quarterEnd(year, quarter)
	confirmationDate := profile.ActualRegularDate
	if confirmationDate == "" {
		confirmationDate = profile.ProbationEndDate
	}

	e := &database.AnnualLeaveEligibility{
		UserID:            profile.UserID,
		Year:              year,
		Quarter:           quarter,
		EntryDate:         profile.EntryDate,
		ConfirmationDate:  confirmationDate,
		CalcVersion:       "v1",
		EligibleStartDate: qStart.Format("2006-01-02"),
		EligibleEndDate:   qEnd.Format("2006-01-02"),
	}

	if profile.EntryDate == "" {
		e.IsEligible = false
		e.CalcReason = "缺少入职日期"
		return e
	}

	entryDate, err := time.Parse("2006-01-02", profile.EntryDate)
	if err != nil {
		e.IsEligible = false
		e.CalcReason = "入职日期格式错误"
		return e
	}

	// 入职日期必须在季度结束前
	if entryDate.After(qEnd) {
		e.IsEligible = false
		e.CalcReason = "入职日期晚于本季度"
		return e
	}

	// 无实际转正日期视为已转正
	if confirmationDate == "" {
		e.IsEligible = true
		e.CalcReason = "无试用期记录，默认已转正"
		return e
	}

	confirmDate, err := time.Parse("2006-01-02", confirmationDate)
	if err != nil {
		e.IsEligible = false
		e.CalcReason = "试用期结束日期格式错误"
		return e
	}

	// 在本季度内已转正
	if !confirmDate.After(qEnd) {
		e.IsEligible = true
		e.CalcReason = "本季度内已转正"
		return e
	}

	// 追溯逻辑：若转正在后续季度，且启用了追溯，前序季度也可追溯资格
	if retroactiveEnabled {
		confirmQuarter := (int(confirmDate.Month())-1)/3 + 1
		confirmYear := confirmDate.Year()
		if confirmYear == year && confirmQuarter > quarter {
			e.IsEligible = true
			e.RetroactiveSourceQuarter = confirmQuarter
			e.CalcReason = fmt.Sprintf("Q%d转正，追溯Q%d资格", confirmQuarter, quarter)
			return e
		}
	}

	e.IsEligible = false
	e.CalcReason = "本季度末尚未转正"
	return e
}

func (s *AnnualLeaveService) isRetroactiveEnabled() bool {
	cfg, err := s.ruleRepo.FindByKey("eligibility.retroactive_confirmation")
	if err != nil {
		return true // 默认开启
	}
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(cfg.RuleValueJSON), &v); err != nil {
		return true
	}
	enabled, _ := v["enabled"].(bool)
	return enabled
}
