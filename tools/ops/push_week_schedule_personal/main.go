package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/service"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type exportDay struct {
	Date  string `json:"date"`
	Day   int    `json:"day"`
	State string `json:"state"`
}

type exportWeek struct {
	WeekStart    string      `json:"week_start"`
	WeekEnd      string      `json:"week_end"`
	WeekType     string      `json:"week_type"`
	SaturdayWork bool        `json:"saturday_work"`
	Cells        []exportDay `json:"cells"`
}

type exportPayload struct {
	Title   string       `json:"title"`
	Content string       `json:"content"`
	Month   string       `json:"month"`
	Weeks   []exportWeek `json:"weeks"`
}

func main() {
	if err := config.Load(); err != nil {
		log.Printf("load config warning: %v", err)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL empty")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	database.DB = db
	if err := dingtalk.Init(); err != nil {
		log.Printf("dingtalk init warning: %v", err)
	}

	names := []string{"郑凤仪", "吴列德"}
	var users []database.User
	if err := db.Where("name IN ? AND deleted_at IS NULL", names).Find(&users).Error; err != nil {
		log.Fatalf("query users: %v", err)
	}
	byOrg := map[string][]database.User{}
	for _, u := range users {
		byOrg[u.OrgID] = append(byOrg[u.OrgID], u)
	}
	orgID := ""
	var selected []database.User
	for oid, list := range byOrg {
		have := map[string]bool{}
		for _, u := range list {
			have[u.Name] = true
		}
		if have["郑凤仪"] && have["吴列德"] {
			orgID = oid
			selected = list
			break
		}
	}
	if orgID == "" {
		log.Fatal("org with both recipients not found")
	}
	picked := map[string]database.User{}
	for _, u := range selected {
		if prev, ok := picked[u.Name]; ok {
			if strings.TrimSpace(prev.DingTalkUserID) == "" && strings.TrimSpace(u.DingTalkUserID) != "" {
				picked[u.Name] = u
			}
			continue
		}
		picked[u.Name] = u
	}
	userIDs := make([]string, 0, 2)
	fmt.Printf("org_id=%s\n", orgID)
	for _, name := range names {
		u := picked[name]
		fmt.Printf("- name=%s user_id=%s ding_set=%v status=%s\n", u.Name, u.UserID, strings.TrimSpace(u.DingTalkUserID) != "", u.Status)
		userIDs = append(userIDs, u.UserID)
	}

	// Match the page screenshot month (2026-07). Company-scope calendar.
	month := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	svc := service.NewWeekScheduleServiceWithOrgID(db, orgID)
	weeks, err := svc.GetWeekCalendar("", "", 10, month.Format("2006-01-02"))
	if err != nil {
		log.Fatalf("GetWeekCalendar: %v", err)
	}

	monthKey := month.Format("2006-01")
	var exportWeeks []exportWeek
	for _, w := range weeks {
		ws, err := time.ParseInLocation("2006-01-02", w.WeekStart, time.Local)
		if err != nil {
			continue
		}
		inMonth := false
		cells := make([]exportDay, 7)
		for i := 0; i < 7; i++ {
			d := ws.AddDate(0, 0, i)
			dateStr := d.Format("2006-01-02")
			day := exportDay{Date: dateStr, Day: d.Day()}
			if d.Format("2006-01") != monthKey {
				day.State = "outside"
			} else {
				inMonth = true
				day.State = resolveDayState(d, w)
			}
			cells[i] = day
		}
		if !inMonth {
			continue
		}
		exportWeeks = append(exportWeeks, exportWeek{
			WeekStart:    w.WeekStart,
			WeekEnd:      w.WeekEnd,
			WeekType:     w.WeekType,
			SaturdayWork: w.SaturdayWork,
			Cells:        cells,
		})
	}
	if len(exportWeeks) == 0 {
		log.Fatal("no weeks in target month")
	}

	fmt.Printf("month weeks=%d\n", len(exportWeeks))
	for i, w := range exportWeeks {
		fmt.Printf("  week%d %s~%s type=%s sat_work=%v\n", i+1, w.WeekStart, w.WeekEnd, w.WeekType, w.SaturdayWork)
	}

	title, content := buildContent(month, exportWeeks)
	fmt.Printf("title=%s\ncontent:\n%s\n", title, content)

	payload := exportPayload{Title: title, Content: content, Month: monthKey, Weeks: exportWeeks}
	dir := "tools/ops/push_week_schedule_personal"
	_ = os.MkdirAll(dir, 0o755)
	jsonPath := filepath.Join(dir, "calendar.json")
	pngPath := filepath.Join(dir, "july2026_from_db.png")
	jb, _ := json.MarshalIndent(payload, "", "  ")
	if err := os.WriteFile(jsonPath, jb, 0o644); err != nil {
		log.Fatalf("write json: %v", err)
	}

	cmd := exec.Command("python", filepath.Join(dir, "render_calendar.py"), jsonPath, pngPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("render_calendar.py: %v", err)
	}
	imageBytes, err := os.ReadFile(pngPath)
	if err != nil {
		log.Fatalf("read png: %v", err)
	}
	fmt.Printf("png_bytes=%d\n", len(imageBytes))

	result, err := svc.PushPersonalScheduleImage(userIDs, title, content, imageBytes, "2026-07-schedule.png")
	if err != nil {
		log.Fatalf("push failed: %v", err)
	}
	fmt.Printf("status=%s message=%s total=%d success=%d failed=%d skipped=%d media_present=%v\n",
		result.Status, result.Message, result.Total, result.SuccessCount, result.FailedCount, result.SkippedCount, strings.TrimSpace(result.MediaID) != "")
	for _, r := range result.Recipients {
		fmt.Printf("recipient name=%s status=%s message=%s\n", r.Name, r.Status, r.Message)
	}
	if result.SuccessCount == 0 {
		os.Exit(2)
	}
}

func resolveDayState(d time.Time, w service.WeekInfo) string {
	dateStr := d.Format("2006-01-02")
	for _, h := range w.Holidays {
		if h.Date == dateStr {
			if h.Type == "holiday" {
				return "holiday"
			}
			return "work"
		}
	}
	if d.Weekday() == time.Sunday {
		return "rest"
	}
	if d.Weekday() == time.Saturday {
		if w.SaturdayWork {
			return "work"
		}
		return "rest"
	}
	return "work"
}

func buildContent(month time.Time, weeks []exportWeek) (string, string) {
	title := fmt.Sprintf("%d年%d月作息时间表", month.Year(), int(month.Month()))
	lines := []string{title + "，请查收。"}

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	// Spec: if today is not in selected month, only keep month title + generic tip.
	if today.Year() != month.Year() || today.Month() != month.Month() {
		return title, strings.Join(lines, "\n")
	}

	tomorrow := today.AddDate(0, 0, 1)
	weekLabel := ""
	for _, w := range weeks {
		ws, _ := time.ParseInLocation("2006-01-02", w.WeekStart, time.Local)
		we, _ := time.ParseInLocation("2006-01-02", w.WeekEnd, time.Local)
		if !today.Before(ws) && !today.After(we) {
			if w.WeekType == "small" {
				weekLabel = "小周"
			} else {
				weekLabel = "大周"
			}
			break
		}
	}

	found := false
	isWork := false
	for _, w := range weeks {
		for _, c := range w.Cells {
			if c.Date == tomorrow.Format("2006-01-02") && c.State != "outside" {
				found = true
				isWork = c.State == "work"
			}
		}
	}
	if found {
		dateLabel := fmt.Sprintf("%d年%d月%d日", tomorrow.Year(), int(tomorrow.Month()), tomorrow.Day())
		word := "休息"
		if isWork {
			word = "上班"
		}
		if weekLabel != "" {
			lines = append(lines, fmt.Sprintf("本周%s，明天（%s）%s，请大家注意。", weekLabel, dateLabel, word))
		} else {
			lines = append(lines, fmt.Sprintf("明天（%s）%s，请大家注意。", dateLabel, word))
		}
	} else if weekLabel != "" {
		lines = append(lines, fmt.Sprintf("本周%s。", weekLabel))
	}
	return title, strings.Join(lines, "\n")
}
