package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"gorm.io/gorm"
)

func main() {
	orgIDFlag := flag.String("org-id", "", "required organization id; all queried records must belong to this org")
	userID := flag.String("user", "", "only sync this employee user_id")
	start := flag.String("start", "", "work date start, YYYY-MM-DD")
	end := flag.String("end", "", "work date end, YYYY-MM-DD")
	dryRun := flag.Bool("dry-run", true, "print the planned replay without writing to DingTalk or database")
	flag.Parse()

	orgID := strings.TrimSpace(*orgIDFlag)
	if orgID == "" {
		log.Fatal("missing -org-id")
	}
	if !*dryRun && strings.TrimSpace(*userID) == "" && strings.TrimSpace(*start) == "" && strings.TrimSpace(*end) == "" {
		log.Fatal("refuse to run without filter when -dry-run=false; pass -user or -start/-end")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	dingCfg, ok := dingtalk.GetAppConfigForOrg(orgID)
	if !ok {
		log.Fatalf("dingtalk app config not found for org_id=%s", orgID)
	}
	if err := dingtalk.InitWithConfig(dingCfg); err != nil {
		log.Fatalf("dingtalk init failed for org_id=%s: %v", orgID, err)
	}

	if strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID")) == "" {
		log.Fatal("DINGTALK_ADMIN_USER_ID is not configured")
	}

	statuses := []string{"matched", "synced", "dingtalk_sync_failed", "local_balance_failed"}
	db := database.DB
	query := db.Where("org_id = ? AND effective_overtime_minutes > 0 AND match_status IN ?", orgID, statuses)
	if strings.TrimSpace(*userID) != "" {
		query = query.Where("user_id = ?", strings.TrimSpace(*userID))
	}
	if strings.TrimSpace(*start) != "" {
		query = query.Where("work_date >= ?", strings.TrimSpace(*start))
	}
	if strings.TrimSpace(*end) != "" {
		query = query.Where("work_date <= ?", strings.TrimSpace(*end))
	}

	var records []database.OvertimeMatchResult
	if err := query.Order("user_id asc, work_date asc").Find(&records).Error; err != nil {
		log.Fatalf("query overtime match records failed: %v", err)
	}

	log.Printf("found %d overtime records to replay for org=%s", len(records), orgID)

	if *dryRun {
		for _, r := range records {
			fmt.Printf("  [dry-run] org=%-12s  userID=%-20s  date=%s  minutes=%d  prev_status=%s\n",
				r.OrgID, r.UserID, r.WorkDate, r.EffectiveOvertimeMinutes, r.DingtalkSyncStatus)
		}
		log.Println("dry-run finished without writes")
		return
	}

	success, failed := 0, 0
	for _, r := range records {
		if err := db.Model(&database.OvertimeMatchResult{}).
			Where("org_id = ? AND id = ?", orgID, r.ID).
			Update("dingtalk_sync_status", "pending").Error; err != nil {
			log.Printf("  FAIL  reset status org=%s id=%d: %v", orgID, r.ID, err)
			failed++
			continue
		}

		if err := pushToDingTalk(db, dingCfg, orgID, r); err != nil {
			log.Printf("  FAIL  org=%s userID=%s date=%s err=%v", orgID, r.UserID, r.WorkDate, err)
			failed++
		} else {
			log.Printf("  OK    org=%s userID=%s date=%s minutes=%d", orgID, r.UserID, r.WorkDate, r.EffectiveOvertimeMinutes)
			success++
		}
	}

	log.Printf("done: success=%d failed=%d total=%d", success, failed, success+failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func pushToDingTalk(db *gorm.DB, dingCfg dingtalk.AppConfig, orgID string, r database.OvertimeMatchResult) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("org id is required")
	}
	reason := fmt.Sprintf("休息日加班调休 %s %d分钟", r.WorkDate, r.EffectiveOvertimeMinutes)
	if err := dingtalk.UpdateCompensatoryLeaveQuotaForConfig(dingCfg, r.UserID, r.EffectiveOvertimeMinutes, r.WorkDate, reason); err != nil {
		_ = db.Model(&database.OvertimeMatchResult{}).Where("org_id = ? AND id = ?", orgID, r.ID).Updates(map[string]interface{}{
			"dingtalk_sync_status": "failed",
			"dingtalk_sync_error":  err.Error(),
		})
		return err
	}

	requestID := fmt.Sprintf("resync:%s:%s:%d", r.UserID, r.WorkDate, r.ID)
	_ = db.Model(&database.OvertimeMatchResult{}).Where("org_id = ? AND id = ?", orgID, r.ID).Updates(map[string]interface{}{
		"dingtalk_sync_status":     "success",
		"dingtalk_sync_request_id": requestID,
		"dingtalk_sync_error":      "",
		"match_status":             "synced",
	})
	return nil
}
