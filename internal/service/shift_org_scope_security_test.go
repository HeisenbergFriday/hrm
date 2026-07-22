package service

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newShiftSecurityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return db
}

func TestDingTalkShiftOperationsRequireExplicitOrganization(t *testing.T) {
	db := newShiftSecurityTestDB(t)
	shiftService := NewShiftConfigService(db)
	weekService := NewWeekScheduleService(db)

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "get or create shift",
			run: func() error {
				_, err := shiftService.GetOrCreateShift("Security Test", "09:00", "18:00")
				return err
			},
		},
		{
			name: "apply and sync",
			run: func() error {
				_, err := shiftService.ApplyAndSync(&ApplyShiftConfigInput{ShiftID: 1, EndTime: "18:00"})
				return err
			},
		},
		{
			name: "preview",
			run: func() error {
				_, err := shiftService.Preview(&PreviewShiftConfigInput{})
				return err
			},
		},
		{
			name: "sync week schedule to DingTalk",
			run: func() error {
				_, err := weekService.SyncToDingTalk(1)
				return err
			},
		},
		{
			name: "sync week schedule from DingTalk",
			run: func() error {
				_, err := weekService.SyncFromDingTalk()
				return err
			},
		},
		{
			name: "sync week schedule conservatively",
			run: func() error {
				_, err := weekService.SyncFromDingTalkConservative()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "missing organization context") {
				t.Fatalf("error = %v, want missing organization context", err)
			}
		})
	}
}
