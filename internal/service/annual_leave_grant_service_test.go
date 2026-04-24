package service

import (
	"math"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupGrantTestDB(t *testing.T) *gorm.DB {
	t.Setenv("DINGTALK_LEAVE_SYNC_ENABLED", "false")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试DB失败: %v", err)
	}
	if err := db.AutoMigrate(
		&database.EmployeeProfile{},
		&database.AnnualLeaveEligibility{},
		&database.AnnualLeaveGrant{},
		&database.LeaveRuleConfig{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	return db
}

func seedEligibility(db *gorm.DB, userID string, year, quarter int, eligible bool) database.AnnualLeaveEligibility {
	e := database.AnnualLeaveEligibility{
		UserID:           userID,
		Year:             year,
		Quarter:          quarter,
		EntryDate:        "2020-01-01",
		ConfirmationDate: "2020-04-01",
		IsEligible:       eligible,
		CalcVersion:      "v1",
	}
	if err := db.Create(&e).Error; err != nil {
		panic(err)
	}
	return e
}

func TestGrantWorkingYearsMapping(t *testing.T) {
	db := setupGrantTestDB(t)
	seedEligibility(db, "u1", 2026, 1, true)
	db.Create(&database.EmployeeProfile{
		UserID:        "u1",
		EmployeeID:    "u1",
		EntryDate:     "2020-01-01",
		ProfileStatus: "active",
	})

	svc := NewAnnualLeaveGrantService(db)
	svc.nowFn = func() time.Time {
		return time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	}

	if err := svc.GrantForUser("u1", 2026, 1); err != nil {
		t.Fatalf("季度发放失败: %v", err)
	}

	records, err := svc.GetGrantLedger("u1", 2026)
	if err != nil {
		t.Fatalf("获取台账失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("应有1条发放记录，实际: %d", len(records))
	}
	if records[0].GrantedDays != 2.5 {
		t.Fatalf("季度发放天数应为2.5，实际: %.1f", records[0].GrantedDays)
	}
	if records[0].BaseDays != 10 {
		t.Fatalf("基础年假应为10天，实际: %.1f", records[0].BaseDays)
	}
}

func TestGrantRetroactiveGrantCreatedOnce(t *testing.T) {
	db := setupGrantTestDB(t)
	e := seedEligibility(db, "u2", 2026, 2, true)
	e.RetroactiveSourceQuarter = 3
	db.Save(&e)
	db.Create(&database.EmployeeProfile{
		UserID:            "u2",
		EmployeeID:        "u2",
		EntryDate:         "2026-04-01",
		ActualRegularDate: "2026-07-15",
		ProfileStatus:     "active",
	})

	svc := NewAnnualLeaveGrantService(db)
	if err := svc.RegrantForEligibilityChange("u2", 2026); err != nil {
		t.Fatalf("第一次追溯发放失败: %v", err)
	}
	if err := svc.RegrantForEligibilityChange("u2", 2026); err != nil {
		t.Fatalf("第二次追溯发放失败: %v", err)
	}

	records, err := svc.GetGrantLedger("u2", 2026)
	if err != nil {
		t.Fatalf("获取台账失败: %v", err)
	}
	retroCount := 0
	for _, r := range records {
		if r.GrantType == "retroactive" {
			retroCount++
		}
	}
	if retroCount != 1 {
		t.Fatalf("追溯记录应只有1条，实际: %d", retroCount)
	}
}

func TestGrantIdempotentNormalGrant(t *testing.T) {
	db := setupGrantTestDB(t)
	seedEligibility(db, "u3", 2026, 1, true)
	db.Create(&database.EmployeeProfile{
		UserID:        "u3",
		EmployeeID:    "u3",
		EntryDate:     "2022-01-01",
		ProfileStatus: "active",
	})

	svc := NewAnnualLeaveGrantService(db)
	if err := svc.GrantForUser("u3", 2026, 1); err != nil {
		t.Fatalf("第一次发放失败: %v", err)
	}
	if err := svc.GrantForUser("u3", 2026, 1); err != nil {
		t.Fatalf("第二次发放失败: %v", err)
	}

	records, err := svc.GetGrantLedger("u3", 2026)
	if err != nil {
		t.Fatalf("获取台账失败: %v", err)
	}
	normalCount := 0
	for _, r := range records {
		if r.GrantType == "normal" {
			normalCount++
		}
	}
	if normalCount != 1 {
		t.Fatalf("正常发放记录应只有1条，实际: %d", normalCount)
	}
}

func TestGrantLedgerReturnsUsedDaysAndCurrentWorkingYears(t *testing.T) {
	db := setupGrantTestDB(t)
	db.Create(&database.EmployeeProfile{
		UserID:        "u4",
		EmployeeID:    "u4",
		EntryDate:     "2021-11-15",
		ProfileStatus: "active",
	})
	db.Create(&database.AnnualLeaveGrant{
		UserID:        "u4",
		Year:          2026,
		Quarter:       1,
		WorkingYears:  4.1305,
		BaseDays:      10,
		GrantedDays:   2.5,
		UsedDays:      1.25,
		RemainingDays: 1.25,
		GrantType:     "normal",
		Remark:        "Q1正常发放，工龄4.1年",
	})

	svc := NewAnnualLeaveGrantService(db)
	now := time.Date(2026, 4, 23, 9, 0, 0, 0, time.Local)
	svc.nowFn = func() time.Time { return now }

	records, err := svc.GetGrantLedger("u4", 2026)
	if err != nil {
		t.Fatalf("获取台账失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("应返回1条发放记录，实际: %d", len(records))
	}
	if records[0].UsedDays != 1.25 {
		t.Fatalf("已用天数应为1.25，实际: %.2f", records[0].UsedDays)
	}
	if records[0].BaseDays != 10 {
		t.Fatalf("基础天数应为10，实际: %.1f", records[0].BaseDays)
	}

	expectedYears := now.Sub(time.Date(2021, 11, 15, 0, 0, 0, 0, time.Local)).Hours() / 24 / 365.0
	if math.Abs(records[0].WorkingYears-expectedYears) > 0.01 {
		t.Fatalf("工龄应按当前日期口径重算，期望 %.4f，实际 %.4f", expectedYears, records[0].WorkingYears)
	}
	if records[0].Remark != "Q1正常发放，工龄4.4年" {
		t.Fatalf("备注应同步为当前工龄展示，实际: %q", records[0].Remark)
	}
}
