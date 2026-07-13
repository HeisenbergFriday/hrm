package main

import (
	"flag"
	"fmt"
	"log"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func main() {
	idsRaw := flag.String("ids", "", "comma-separated overtime_match_results ids")
	orgIDFlag := flag.String("org-id", "", "required organization id; all selected ids must belong to this org")
	dryRun := flag.Bool("dry-run", true, "print the planned resync records without writing to DingTalk or database")
	flag.Parse()

	orgID := strings.TrimSpace(*orgIDFlag)
	if orgID == "" {
		log.Fatal("missing -org-id")
	}
	if strings.TrimSpace(*idsRaw) == "" {
		log.Fatal("missing -ids")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("load env failed: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}

	ids, err := parseIDs(*idsRaw)
	if err != nil {
		log.Fatal(err)
	}

	if *dryRun {
		for _, id := range ids {
			var match database.OvertimeMatchResult
			if err := database.DB.Where("org_id = ?", orgID).First(&match, id).Error; err != nil {
				log.Printf("  FAIL  load org=%s id=%d  err=%v", orgID, id, err)
				continue
			}
			fmt.Printf("  [dry-run] org=%s  id=%d  userID=%s  date=%s  minutes=%d  status=%s\n",
				match.OrgID, match.ID, match.UserID, match.WorkDate, match.EffectiveOvertimeMinutes, match.DingtalkSyncStatus)
		}
		log.Println("dry-run finished without writes")
		return
	}

	dingCfg, ok := dingtalk.GetAppConfigForOrg(orgID)
	if !ok {
		log.Fatalf("dingtalk app config not found for org_id=%s", orgID)
	}
	if err := dingtalk.InitWithConfig(dingCfg); err != nil {
		log.Fatalf("dingtalk init failed for org_id=%s: %v", orgID, err)
	}

	for _, id := range ids {
		if err := resyncOne(database.DB, dingCfg, orgID, id); err != nil {
			log.Fatalf("resync org=%s id=%d failed: %v", orgID, id, err)
		}
	}
}

func parseIDs(raw string) ([]uint, error) {
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid id %q", part)
		}
		ids = append(ids, uint(id))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no valid ids")
	}
	return ids, nil
}

func resyncOne(db *gorm.DB, dingCfg dingtalk.AppConfig, orgID string, id uint) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("org id is required")
	}

	var match database.OvertimeMatchResult
	if err := db.Where("org_id = ?", orgID).First(&match, id).Error; err != nil {
		return err
	}
	if match.EffectiveOvertimeMinutes <= 0 {
		return fmt.Errorf("record has no effective overtime minutes")
	}
	reason := fmt.Sprintf("休息日加班调休 %s %d分钟", match.WorkDate, match.EffectiveOvertimeMinutes)
	if err := dingtalk.UpdateCompensatoryLeaveQuotaForConfig(dingCfg, match.UserID, match.EffectiveOvertimeMinutes, match.WorkDate, reason); err != nil {
		_ = db.Model(&database.OvertimeMatchResult{}).Where("org_id = ? AND id = ?", orgID, match.ID).Updates(map[string]interface{}{
			"dingtalk_sync_status": "failed",
			"dingtalk_sync_error":  err.Error(),
		}).Error
		return err
	}
	return db.Model(&database.OvertimeMatchResult{}).Where("org_id = ? AND id = ?", orgID, match.ID).Updates(map[string]interface{}{
		"dingtalk_sync_status":     "success",
		"dingtalk_sync_request_id": fmt.Sprintf("manual-resync:%d", match.ID),
		"dingtalk_sync_error":      "",
		"match_status":             "synced",
	}).Error
}
