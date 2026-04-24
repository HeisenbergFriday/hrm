package service

import (
	"peopleops/internal/database"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupLeaveTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试DB失败: %v", err)
	}
	db.AutoMigrate(
		&database.EmployeeProfile{},
		&database.AnnualLeaveEligibility{},
		&database.LeaveRuleConfig{},
	)
	return db
}

func makeProfile(userID, entryDate, probationEndDate string) database.EmployeeProfile {
	return database.EmployeeProfile{
		UserID:           userID,
		EmployeeID:       userID,
		EntryDate:        entryDate,
		ProbationEndDate: probationEndDate,
		ProfileStatus:    "active",
	}
}

// 场景1：员工年初入职，Q1内转正
func TestEligibility_ConfirmedInQ1(t *testing.T) {
	db := setupLeaveTestDB(t)
	profile := makeProfile("u1", "2026-01-10", "2026-03-10")
	db.Create(&profile)

	svc := NewAnnualLeaveService(db)
	if err := svc.RecalculateEligibility("u1", 2026); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	results, _ := svc.GetEligibility("u1", 2026)
	for _, r := range results {
		if !r.IsEligible {
			t.Errorf("Q%d 应有资格，实际无资格，原因: %s", r.Quarter, r.CalcReason)
		}
	}
}

// 场景2：Q3转正，Q2应追溯
func TestEligibility_RetroactiveQ2WhenConfirmedQ3(t *testing.T) {
	db := setupLeaveTestDB(t)
	// 开启追溯规则
	db.Create(&database.LeaveRuleConfig{
		RuleType: "eligibility", RuleKey: "eligibility.retroactive_confirmation",
		RuleName: "追溯", RuleValueJSON: `{"enabled":true}`, Status: "active",
	})
	profile := makeProfile("u2", "2026-01-10", "2026-07-15")
	db.Create(&profile)

	svc := NewAnnualLeaveService(db)
	svc.RecalculateEligibility("u2", 2026)
	results, _ := svc.GetEligibility("u2", 2026)

	q2 := findQuarter(results, 2)
	if q2 == nil || !q2.IsEligible {
		t.Error("Q2 应因Q3转正而追溯有资格")
	}
	if q2 != nil && q2.RetroactiveSourceQuarter != 3 {
		t.Errorf("追溯来源季度应为3，实际: %d", q2.RetroactiveSourceQuarter)
	}
}

func TestEligibility_UsesActualRegularDateFirst(t *testing.T) {
	db := setupLeaveTestDB(t)
	profile := makeProfile("u_actual", "2026-01-10", "2026-07-15")
	profile.ActualRegularDate = "2026-03-10"
	db.Create(&profile)

	svc := NewAnnualLeaveService(db)
	if err := svc.RecalculateEligibility("u_actual", 2026); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	results, _ := svc.GetEligibility("u_actual", 2026)
	q1 := findQuarter(results, 1)
	if q1 == nil || !q1.IsEligible {
		t.Fatalf("Q1 应按实际转正日期获得资格")
	}
	if q1.ConfirmationDate != "2026-03-10" {
		t.Fatalf("expected actual regular date, got %s", q1.ConfirmationDate)
	}
}

// 场景3：缺少转正日期
func TestEligibility_MissingConfirmationDate(t *testing.T) {
	db := setupLeaveTestDB(t)
	profile := makeProfile("u3", "2026-01-10", "")
	db.Create(&profile)

	svc := NewAnnualLeaveService(db)
	svc.RecalculateEligibility("u3", 2026)
	results, _ := svc.GetEligibility("u3", 2026)
	for _, r := range results {
		if !r.IsEligible {
			t.Errorf("无试用期记录时Q%d应默认有资格", r.Quarter)
		}
	}
}

// 场景4：入职日期变更后重算结果更新
func TestEligibility_EntryDateChangeUpdatesResult(t *testing.T) {
	db := setupLeaveTestDB(t)
	profile := makeProfile("u4", "2027-06-01", "2027-09-01")
	db.Create(&profile)

	svc := NewAnnualLeaveService(db)
	svc.RecalculateEligibility("u4", 2026)
	results, _ := svc.GetEligibility("u4", 2026)
	for _, r := range results {
		if r.IsEligible {
			t.Errorf("入职日期在2027年，2026年Q%d不应有资格", r.Quarter)
		}
	}

	// 修改入职日期
	db.Model(&database.EmployeeProfile{}).Where("user_id = ?", "u4").Update("entry_date", "2026-01-01")
	db.Model(&database.EmployeeProfile{}).Where("user_id = ?", "u4").Update("probation_end_date", "2026-03-01")
	svc.RecalculateEligibility("u4", 2026)
	results, _ = svc.GetEligibility("u4", 2026)
	hasEligible := false
	for _, r := range results {
		if r.IsEligible {
			hasEligible = true
		}
	}
	if !hasEligible {
		t.Error("修改入职日期后应有季度有资格")
	}
}

func findQuarter(results []EligibilityResult, q int) *EligibilityResult {
	for i := range results {
		if results[i].Quarter == q {
			return &results[i]
		}
	}
	return nil
}

// 辅助：确保 time 包被使用
var _ = time.Now
