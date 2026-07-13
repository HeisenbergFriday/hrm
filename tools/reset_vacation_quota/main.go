package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"peopleops/internal/config"
	"peopleops/internal/dingtalk"
)

func main() {
	orgIDFlag := flag.String("org-id", "", "required organization id; DingTalk writes use this org app config")
	leaveCode := flag.String("leave-code", "", "DingTalk leave type leave_code")
	year := flag.Int("year", time.Now().Year(), "quota cycle year")
	quotaDays := flag.Float64("quota-days", 0, "grant days; 0 only initializes records")
	quotaHours := flag.Float64("quota-hours", -1, "grant hours; takes precedence over -quota-days when >= 0")
	hoursPerDay := flag.Float64("hours-per-day", 8, "working hours per day")
	reason := flag.String("reason", "batch initialize leave quota", "DingTalk quota change reason")
	dryRun := flag.Bool("dry-run", true, "list employees without writing to DingTalk")
	flag.Parse()

	orgID := strings.TrimSpace(*orgIDFlag)
	if orgID == "" {
		log.Fatal("missing -org-id")
	}
	if strings.TrimSpace(*leaveCode) == "" {
		log.Fatal("missing required -leave-code")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	dingCfg, ok := dingtalk.GetAppConfigForOrg(orgID)
	if !ok {
		log.Fatalf("dingtalk app config not found for org_id=%s", orgID)
	}
	if err := dingtalk.InitWithConfig(dingCfg); err != nil {
		log.Fatalf("dingtalk init failed for org_id=%s: %v", orgID, err)
	}

	var perHour float64
	if *quotaHours >= 0 {
		perHour = *quotaHours
	} else {
		perHour = *quotaDays * *hoursPerDay
	}
	quotaPerHour := int64(math.Round(perHour * 100))
	quotaPerDay := int64(math.Round(perHour / *hoursPerDay * 100))

	log.Printf("org=%s leave_code=%s year=%d quota=%.2f hours (%.2f days) perHour_x100=%d perDay_x100=%d",
		orgID, *leaveCode, *year, perHour, perHour / *hoursPerDay, quotaPerHour, quotaPerDay)

	log.Printf("syncing DingTalk users for org=%s...", orgID)
	users, err := dingtalk.SyncUsers()
	if err != nil {
		log.Fatalf("sync users failed for org=%s: %v", orgID, err)
	}
	log.Printf("found %d employees for org=%s", len(users), orgID)

	if *dryRun {
		for _, u := range users {
			fmt.Printf("  [dry-run] org=%s userID=%s name=%s\n", orgID, u.UserID, u.Name)
		}
		log.Println("dry-run finished without writes")
		return
	}

	success, failed := 0, 0
	for _, u := range users {
		if u.UserID == "" {
			continue
		}
		err := dingtalk.InitVacationQuotaForConfig(dingCfg, u.UserID, *leaveCode, *year, quotaPerDay, quotaPerHour, *reason)
		if err != nil {
			log.Printf("  FAIL  org=%s userID=%s name=%s err=%v", orgID, u.UserID, u.Name, err)
			failed++
		} else {
			log.Printf("  OK    org=%s userID=%s name=%s", orgID, u.UserID, u.Name)
			success++
		}
	}

	log.Printf("done: success=%d failed=%d total=%d", success, failed, success+failed)
	if failed > 0 {
		log.Fatalf("%d records failed; check logs", failed)
	}
}
