package main

import (
	"flag"
	"fmt"
	"log"
	"peopleops/internal/config"
	"peopleops/internal/dingtalk"
	"strings"
)

func main() {
	orgIDFlag := flag.String("org-id", "", "required organization id; DingTalk writes use this org app config")
	userID := flag.String("user", "", "DingTalk user id")
	minutes := flag.Int("minutes", -1, "target total balance in minutes")
	year := flag.Int("year", 0, "quota cycle year")
	reason := flag.String("reason", "fix compensatory leave balance", "DingTalk balance change reason")
	dryRun := flag.Bool("dry-run", true, "print the planned balance change without writing to DingTalk")
	flag.Parse()

	orgID := strings.TrimSpace(*orgIDFlag)
	if orgID == "" {
		log.Fatal("missing -org-id")
	}
	if strings.TrimSpace(*userID) == "" {
		log.Fatal("missing -user")
	}
	if *minutes < 0 {
		log.Fatal("missing or invalid -minutes")
	}
	if *year <= 0 {
		log.Fatal("missing or invalid -year")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("load env failed: %v", err)
	}
	dingCfg, ok := dingtalk.GetAppConfigForOrg(orgID)
	if !ok {
		log.Fatalf("dingtalk app config not found for org_id=%s", orgID)
	}

	if *dryRun {
		fmt.Printf("[dry-run] would set compensatory leave balance: org=%s user=%s year=%d minutes=%d reason=%q\n", orgID, *userID, *year, *minutes, *reason)
		return
	}
	if err := dingtalk.InitWithConfig(dingCfg); err != nil {
		log.Fatalf("dingtalk init failed for org_id=%s: %v", orgID, err)
	}
	if err := dingtalk.SetCompensatoryLeaveQuotaForConfig(dingCfg, *userID, *year, *minutes, *reason); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("set compensatory leave balance success: org=%s user=%s year=%d minutes=%d\n", orgID, *userID, *year, *minutes)
}
