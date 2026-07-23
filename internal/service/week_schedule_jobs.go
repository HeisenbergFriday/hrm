package service

import (
	"fmt"
	"log"
	"os"
	"peopleops/internal/database"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// WeekScheduleJobScheduler runs recurring week-schedule notices.
// Friday auto reminder: text-only tip about whether Saturday works, based on company calendar.
// Enable with WEEK_SCHEDULE_FRIDAY_REMINDER_ENABLED=true.
// Time with WEEK_SCHEDULE_FRIDAY_REMINDER_HOUR / WEEK_SCHEDULE_FRIDAY_REMINDER_MINUTE (default 17:00 Asia/Shanghai).
// Recipients: WEEK_SCHEDULE_FRIDAY_REMINDER_USER_IDS=uid1,uid2 (local users.user_id within each org).
// When user IDs are empty, the job logs and skips send (safe default until configured).
type WeekScheduleJobScheduler struct {
	db *gorm.DB
}

func NewWeekScheduleJobScheduler(db *gorm.DB) *WeekScheduleJobScheduler {
	return &WeekScheduleJobScheduler{db: db}
}

func (s *WeekScheduleJobScheduler) Start() {
	if !weekScheduleFridayReminderEnabled() {
		log.Println("[WeekScheduleJobs] Friday reminder disabled (set WEEK_SCHEDULE_FRIDAY_REMINDER_ENABLED=true to enable)")
		return
	}
	go s.runFridayReminderLoop()
	hour, minute := weekScheduleFridayReminderClock()
	log.Printf("[WeekScheduleJobs] Friday reminder enabled at %02d:%02d (local)", hour, minute)
}

func (s *WeekScheduleJobScheduler) runFridayReminderLoop() {
	for {
		hour, minute := weekScheduleFridayReminderClock()
		next := nextFridayAt(time.Now(), hour, minute)
		log.Printf("[WeekScheduleJobs] next Friday reminder at %s", next.Format("2006-01-02 15:04:05"))
		time.Sleep(time.Until(next) + time.Second)
		s.runFridayReminderOnce(time.Now())
	}
}

// runFridayReminderOnce is exported via package for tests; production loop calls it.
func (s *WeekScheduleJobScheduler) runFridayReminderOnce(now time.Time) {
	if s == nil || s.db == nil {
		return
	}
	if now.Weekday() != time.Friday {
		// Allow manual/test invocation on non-Friday; still compute "tomorrow" relative to now.
		log.Printf("[WeekScheduleJobs] run on weekday=%s (expected Friday for production schedule)", now.Weekday())
	}

	userIDs := weekScheduleFridayReminderUserIDs()
	if len(userIDs) == 0 {
		log.Println("[WeekScheduleJobs] skip: WEEK_SCHEDULE_FRIDAY_REMINDER_USER_IDS is empty")
		return
	}

	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[WeekScheduleJobs] list orgs failed: %v", err)
		return
	}
	if len(orgs) == 0 {
		log.Println("[WeekScheduleJobs] skip: no active organizations")
		return
	}

	tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	for _, orgID := range orgs {
		if err := s.sendFridayReminderForOrg(orgID, userIDs, now, tomorrow); err != nil {
			log.Printf("[WeekScheduleJobs] org=%s friday reminder failed: %v", orgID, err)
		}
	}
}

func (s *WeekScheduleJobScheduler) sendFridayReminderForOrg(orgID string, userIDs []string, now, tomorrow time.Time) error {
	svc := NewWeekScheduleServiceWithOrgID(s.db, orgID)
	// Company-scope calendar (empty user/dept), same as page default for managers.
	weeks, err := svc.GetWeekCalendar("", "", 4, now.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("calendar: %w", err)
	}

	weekLabel, satWork, ok := resolveTomorrowSaturdayTip(weeks, tomorrow)
	if !ok {
		return fmt.Errorf("unable to resolve week type for %s", tomorrow.Format("2006-01-02"))
	}

	dateLabel := fmt.Sprintf("%d年%d月%d日", tomorrow.Year(), int(tomorrow.Month()), tomorrow.Day())
	title := "周末作息提醒"
	// Friday tip focuses on whether tomorrow (Saturday) is workday.
	workWord := "休息"
	if satWork {
		workWord = "上班"
	}
	content := fmt.Sprintf("本周%s，明天（%s，周六）%s，请大家注意。", weekLabel, dateLabel, workWord)

	// Reuse personal push pipeline but text-only: empty image not allowed, so send text directly via same recipient resolution.
	result, err := svc.SendPersonalTextNotice(userIDs, title, content)
	if err != nil {
		return err
	}
	log.Printf("[WeekScheduleJobs] org=%s friday reminder status=%s success=%d failed=%d skipped=%d msg=%s",
		orgID, result.Status, result.SuccessCount, result.FailedCount, result.SkippedCount, result.Message)
	return nil
}

func resolveTomorrowSaturdayTip(weeks []WeekInfo, tomorrow time.Time) (weekLabel string, saturdayWork bool, ok bool) {
	dateStr := tomorrow.Format("2006-01-02")
	for _, w := range weeks {
		ws, err1 := time.ParseInLocation("2006-01-02", w.WeekStart, tomorrow.Location())
		we, err2 := time.ParseInLocation("2006-01-02", w.WeekEnd, tomorrow.Location())
		if err1 != nil || err2 != nil {
			continue
		}
		if tomorrow.Before(ws) || tomorrow.After(we) {
			continue
		}
		if w.WeekType == "small" {
			weekLabel = "小周"
		} else {
			weekLabel = "大周"
		}
		// holiday override for Saturday
		for _, h := range w.Holidays {
			if h.Date == dateStr {
				if h.Type == "holiday" {
					return weekLabel, false, true
				}
				if h.Type == "workday" {
					return weekLabel, true, true
				}
			}
		}
		if tomorrow.Weekday() == time.Saturday {
			return weekLabel, w.SaturdayWork, true
		}
		// If invoked not for Saturday, still return week label and whether that day works.
		if tomorrow.Weekday() == time.Sunday {
			return weekLabel, false, true
		}
		return weekLabel, true, true
	}
	return "", false, false
}

func (s *WeekScheduleJobScheduler) listActiveOrgIDs() ([]string, error) {
	orgs, err := database.ListActiveOrganizations()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(orgs))
	for _, o := range orgs {
		if id := strings.TrimSpace(o.OrgID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func weekScheduleFridayReminderEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("WEEK_SCHEDULE_FRIDAY_REMINDER_ENABLED")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func weekScheduleFridayReminderClock() (hour, minute int) {
	hour, minute = 17, 0
	if raw := strings.TrimSpace(os.Getenv("WEEK_SCHEDULE_FRIDAY_REMINDER_HOUR")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 && n <= 23 {
			hour = n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("WEEK_SCHEDULE_FRIDAY_REMINDER_MINUTE")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 && n <= 59 {
			minute = n
		}
	}
	return hour, minute
}

func weekScheduleFridayReminderUserIDs() []string {
	raw := strings.TrimSpace(os.Getenv("WEEK_SCHEDULE_FRIDAY_REMINDER_USER_IDS"))
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func nextFridayAt(from time.Time, hour, minute int) time.Time {
	loc := from.Location()
	day := time.Date(from.Year(), from.Month(), from.Day(), hour, minute, 0, 0, loc)
	// days until Friday (5)
	offset := (int(time.Friday) - int(from.Weekday()) + 7) % 7
	candidate := day.AddDate(0, 0, offset)
	if !candidate.After(from) {
		candidate = candidate.AddDate(0, 0, 7)
	}
	return candidate
}
