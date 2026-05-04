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
	// 高风险工具：执行前必须显式传入 leave-code，并优先使用 dry-run 复核目标范围。
	leaveCode := flag.String("leave-code", "", "钉钉假期类型 leave_code（必填）")
	year := flag.Int("year", time.Now().Year(), "配额周期年份")
	quotaDays := flag.Float64("quota-days", 0, "发放天数（支持小数，默认 0 表示仅初始化记录不发放）")
	quotaHours := flag.Float64("quota-hours", -1, "发放小时数（优先级高于 -quota-days，-1 表示使用 -quota-days）")
	hoursPerDay := flag.Float64("hours-per-day", 8, "每天工时（用于天↔小时换算）")
	reason := flag.String("reason", "批量初始化假期配额", "变更原因（记录在钉钉操作日志）")
	dryRun := flag.Bool("dry-run", true, "仅列出员工，不实际调用钉钉接口")
	flag.Parse()

	if strings.TrimSpace(*leaveCode) == "" {
		log.Fatal("missing required -leave-code")
	}

	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("钉钉初始化失败: %v", err)
	}

	// 计算 1/100 单位的配额值
	var perHour float64
	if *quotaHours >= 0 {
		perHour = *quotaHours
	} else {
		perHour = *quotaDays * *hoursPerDay
	}
	quotaPerHour := int64(math.Round(perHour * 100))
	quotaPerDay := int64(math.Round(perHour / *hoursPerDay * 100))

	log.Printf("leave_code=%s  year=%d  quota=%.2f小时(%.2f天)  perHour×100=%d  perDay×100=%d",
		*leaveCode, *year, perHour, perHour / *hoursPerDay, quotaPerHour, quotaPerDay)

	// 获取全部员工
	log.Println("正在从钉钉同步员工列表...")
	users, err := dingtalk.SyncUsers()
	if err != nil {
		log.Fatalf("获取员工列表失败: %v", err)
	}
	log.Printf("共找到 %d 名员工", len(users))

	if *dryRun {
		for _, u := range users {
			fmt.Printf("  [dry-run] userID=%s  name=%s\n", u.UserID, u.Name)
		}
		log.Println("dry-run 结束，未实际写入")
		return
	}

	success, failed := 0, 0
	for _, u := range users {
		if u.UserID == "" {
			continue
		}
		err := dingtalk.InitVacationQuota(u.UserID, *leaveCode, *year, quotaPerDay, quotaPerHour, *reason)
		if err != nil {
			log.Printf("  FAIL  userID=%s  name=%s  err=%v", u.UserID, u.Name, err)
			failed++
		} else {
			log.Printf("  OK    userID=%s  name=%s", u.UserID, u.Name)
			success++
		}
	}

	log.Printf("完成：成功 %d，失败 %d，合计 %d", success, failed, success+failed)
	if failed > 0 {
		log.Fatalf("存在 %d 个失败，请检查日志", failed)
	}
}
