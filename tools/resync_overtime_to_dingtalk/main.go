package main

// 用法（在项目根目录执行）：
//
//	# 预览：列出将要同步的记录，不实际调用钉钉
//	go run ./tools/resync_overtime_to_dingtalk/ -dry-run
//
//	# 正式重放（高风险；需显式关闭 dry-run，并至少传入 user 或日期范围）
//	go run ./tools/resync_overtime_to_dingtalk/ -dry-run=false -user example-user-id
//
//	# 仅重放某个员工
//	go run ./tools/resync_overtime_to_dingtalk/ -user example-user-id
//
//	# 仅重放某段日期
//	go run ./tools/resync_overtime_to_dingtalk/ -start 2025-01-01 -end 2025-12-31

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
	// 高风险重放工具：默认 dry-run，正式执行前必须显式关闭 dry-run 并确认筛选条件。
	userID := flag.String("user", "", "只同步该员工（留空=全员）")
	start := flag.String("start", "", "工作日起始 YYYY-MM-DD（留空=不限）")
	end := flag.String("end", "", "工作日截止 YYYY-MM-DD（留空=不限）")
	dryRun := flag.Bool("dry-run", true, "仅打印计划，不实际写入钉钉")
	flag.Parse()

	if !*dryRun && strings.TrimSpace(*userID) == "" && strings.TrimSpace(*start) == "" && strings.TrimSpace(*end) == "" {
		log.Fatal("refuse to run without filter when -dry-run=false; pass -user or -start/-end")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("钉钉初始化失败: %v", err)
	}

	if strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID")) == "" {
		log.Fatal("DINGTALK_ADMIN_USER_ID 未配置")
	}

	db := database.DB
	query := db.Where("effective_overtime_minutes > 0 AND match_status IN ?",
		[]string{"matched", "synced", "dingtalk_sync_failed", "local_balance_failed"})
	if *userID != "" {
		query = query.Where("user_id = ?", *userID)
	}
	if *start != "" {
		query = query.Where("work_date >= ?", *start)
	}
	if *end != "" {
		query = query.Where("work_date <= ?", *end)
	}

	var records []database.OvertimeMatchResult
	if err := query.Order("user_id asc, work_date asc").Find(&records).Error; err != nil {
		log.Fatalf("查询匹配记录失败: %v", err)
	}

	log.Printf("共找到 %d 条需要重放的加班记录", len(records))

	if *dryRun {
		for _, r := range records {
			fmt.Printf("  [dry-run] userID=%-20s  date=%s  minutes=%d  prev_status=%s\n",
				r.UserID, r.WorkDate, r.EffectiveOvertimeMinutes, r.DingtalkSyncStatus)
		}
		log.Println("dry-run 结束，未实际写入")
		return
	}

	success, failed := 0, 0
	for _, r := range records {
		// 重置同步状态为 pending，确保钉钉侧 "already success" 跳过逻辑不生效
		if err := db.Model(&database.OvertimeMatchResult{}).Where("id = ?", r.ID).
			Update("dingtalk_sync_status", "pending").Error; err != nil {
			log.Printf("  FAIL  reset status id=%d: %v", r.ID, err)
			failed++
			continue
		}

		if err := pushToDingTalk(db, r); err != nil {
			log.Printf("  FAIL  userID=%s  date=%s  err=%v", r.UserID, r.WorkDate, err)
			failed++
		} else {
			log.Printf("  OK    userID=%s  date=%s  minutes=%d", r.UserID, r.WorkDate, r.EffectiveOvertimeMinutes)
			success++
		}
	}

	log.Printf("完成：成功 %d，失败 %d，合计 %d", success, failed, success+failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func pushToDingTalk(db *gorm.DB, r database.OvertimeMatchResult) error {
	reason := fmt.Sprintf("休息日加班调休 %s %d分钟", r.WorkDate, r.EffectiveOvertimeMinutes)
	if err := dingtalk.UpdateCompensatoryLeaveQuota(r.UserID, r.EffectiveOvertimeMinutes, r.WorkDate, reason); err != nil {
		_ = db.Model(&database.OvertimeMatchResult{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
			"dingtalk_sync_status": "failed",
			"dingtalk_sync_error":  err.Error(),
		})
		return err
	}

	requestID := fmt.Sprintf("resync:%s:%s:%d", r.UserID, r.WorkDate, r.ID)
	_ = db.Model(&database.OvertimeMatchResult{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
		"dingtalk_sync_status":     "success",
		"dingtalk_sync_request_id": requestID,
		"dingtalk_sync_error":      "",
		"match_status":             "synced",
	})
	return nil
}
