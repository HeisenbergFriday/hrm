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
	// 高风险重同步工具：默认 dry-run，正式执行前必须显式关闭 dry-run 并确认目标记录 ID。
	idsRaw := flag.String("ids", "", "comma-separated overtime_match_results ids")
	dryRun := flag.Bool("dry-run", true, "print the planned resync records without writing to DingTalk or database")
	flag.Parse()
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
			if err := database.DB.First(&match, id).Error; err != nil {
				log.Printf("  FAIL  load id=%d  err=%v", id, err)
				continue
			}
			fmt.Printf("  [dry-run] id=%d  userID=%s  date=%s  minutes=%d  status=%s\n",
				match.ID, match.UserID, match.WorkDate, match.EffectiveOvertimeMinutes, match.DingtalkSyncStatus)
		}
		log.Println("dry-run 结束，未实际写入")
		return
	}

	if err := dingtalk.Init(); err != nil {
		log.Fatalf("dingtalk init failed: %v", err)
	}

	for _, id := range ids {
		if err := resyncOne(database.DB, id); err != nil {
			log.Fatalf("resync id=%d failed: %v", id, err)
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

func resyncOne(db *gorm.DB, id uint) error {
	var match database.OvertimeMatchResult
	if err := db.First(&match, id).Error; err != nil {
		return err
	}
	if match.EffectiveOvertimeMinutes <= 0 {
		return fmt.Errorf("record has no effective overtime minutes")
	}
	reason := fmt.Sprintf("休息日加班调休 %s %d分钟", match.WorkDate, match.EffectiveOvertimeMinutes)
	if err := dingtalk.UpdateCompensatoryLeaveQuota(match.UserID, match.EffectiveOvertimeMinutes, match.WorkDate, reason); err != nil {
		_ = db.Model(&database.OvertimeMatchResult{}).Where("id = ?", match.ID).Updates(map[string]interface{}{
			"dingtalk_sync_status": "failed",
			"dingtalk_sync_error":  err.Error(),
		}).Error
		return err
	}
	return db.Model(&database.OvertimeMatchResult{}).Where("id = ?", match.ID).Updates(map[string]interface{}{
		"dingtalk_sync_status":     "success",
		"dingtalk_sync_request_id": fmt.Sprintf("manual-resync:%d", match.ID),
		"dingtalk_sync_error":      "",
		"match_status":             "synced",
	}).Error
}
