package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/service"
)

func main() {
	activityIDFlag := flag.String("activity", "", "performance activity id")
	activityNameFlag := flag.String("activity-name", "", "performance activity name, used only when -activity is empty")
	nowFlag := flag.String("now", "", "run date in YYYY-MM-DD; defaults to today")
	includeCurrentDay := flag.Bool("include-current-day", false, "temporarily include today's days-before-deadline as an automatic reminder round")
	confirmSend := flag.Bool("confirm-send", false, "required to actually send DingTalk reminders")
	maxRecipients := flag.Int("max-recipients", 10, "refuse to send when active participants exceed this number")
	flag.Parse()

	if !*confirmSend {
		log.Fatal("missing -confirm-send; refuse to send DingTalk reminders without explicit confirmation")
	}
	if strings.TrimSpace(*activityIDFlag) == "" && strings.TrimSpace(*activityNameFlag) == "" {
		log.Fatal("missing -activity or -activity-name")
	}
	now, err := parseRunDate(*nowFlag)
	if err != nil {
		log.Fatal(err)
	}

	if err := config.Load(); err != nil {
		log.Printf("load env warning: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("dingtalk init failed: %v", err)
	}

	activity, err := resolveActivity(strings.TrimSpace(*activityIDFlag), strings.TrimSpace(*activityNameFlag))
	if err != nil {
		log.Fatal(err)
	}
	activityID := fmt.Sprintf("%d", activity.ID)
	recipientCount, err := countActiveParticipants(activityID)
	if err != nil {
		log.Fatal(err)
	}
	if *maxRecipients >= 0 && recipientCount > int64(*maxRecipients) {
		log.Fatalf("activity %s has %d active participants, above -max-recipients=%d", activityID, recipientCount, *maxRecipients)
	}

	result, err := service.NewPerformanceService(database.DB).SendDueSelfEvalAutoReminderForActivity(activityID, now, service.SelfEvalAutoReminderRunOptions{
		IncludeCurrentDay: *includeCurrentDay,
	})
	if err != nil {
		log.Fatalf("run self eval auto reminder failed: %v", err)
	}
	log.Printf(
		"self eval auto reminder result: activity_id=%s activity_name=%q date=%s scanned=%d matched=%d candidates=%d sent=%d skipped=%d already_sent=%d failed=%d",
		activityID,
		activity.Name,
		now.Format("2006-01-02"),
		result.ActivitiesScanned,
		result.ActivitiesMatched,
		result.Candidates,
		result.Sent,
		result.Skipped,
		result.AlreadySent,
		result.Failed,
	)
}

func parseRunDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now(), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid -now %q, expected YYYY-MM-DD", raw)
	}
	return parsed, nil
}

func resolveActivity(activityID string, activityName string) (*database.PerformanceActivity, error) {
	var activity database.PerformanceActivity
	query := database.DB.Where("deleted_at IS NULL")
	if activityID != "" {
		query = query.Where("id = ?", activityID)
	} else {
		query = query.Where("name = ?", activityName).Order("id DESC")
	}
	if err := query.First(&activity).Error; err != nil {
		return nil, fmt.Errorf("load activity failed: %w", err)
	}
	return &activity, nil
}

func countActiveParticipants(activityID string) (int64, error) {
	var count int64
	err := database.DB.Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND deleted_at IS NULL", activityID).
		Count(&count).Error
	return count, err
}
