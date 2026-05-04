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
	// 高风险余额修正工具：默认 dry-run，正式执行前必须显式关闭 dry-run 并确认 user/year/minutes。
	userID := flag.String("user", "", "DingTalk user id")
	minutes := flag.Int("minutes", -1, "target total balance in minutes")
	year := flag.Int("year", 0, "quota cycle year")
	reason := flag.String("reason", "fix compensatory leave balance", "DingTalk balance change reason")
	dryRun := flag.Bool("dry-run", true, "print the planned balance change without writing to DingTalk")
	flag.Parse()

	if strings.TrimSpace(*userID) == "" {
		log.Fatal("missing -user")
	}
	if *minutes < 0 {
		log.Fatal("missing or invalid -minutes")
	}
	if *year <= 0 {
		log.Fatal("missing or invalid -year")
	}
	if *dryRun {
		fmt.Printf("[dry-run] would set compensatory leave balance: user=%s year=%d minutes=%d reason=%q\n", *userID, *year, *minutes, *reason)
		return
	}
	if err := config.Load(); err != nil {
		log.Fatalf("load env failed: %v", err)
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("dingtalk init failed: %v", err)
	}
	if err := dingtalk.SetCompensatoryLeaveQuota(*userID, *year, *minutes, *reason); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("set compensatory leave balance success: user=%s year=%d minutes=%d\n", *userID, *year, *minutes)
}
