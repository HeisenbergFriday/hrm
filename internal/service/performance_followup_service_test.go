package service

import (
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestPerformanceFollowupAllowed(t *testing.T) {
	allowedStatuses := []string{"result_publish", "result_confirmed", "locked", "archived", "interview", "appeal"}
	for _, status := range allowedStatuses {
		if !performanceFollowupAllowed(&database.PerformanceActivity{Status: status}) {
			t.Fatalf("performanceFollowupAllowed(%q) = false, want true", status)
		}
	}

	blockedStatuses := []string{"draft", "target_setting", "self_evaluation", "manager_evaluation", "hr_review", ""}
	for _, status := range blockedStatuses {
		if performanceFollowupAllowed(&database.PerformanceActivity{Status: status}) {
			t.Fatalf("performanceFollowupAllowed(%q) = true, want false", status)
		}
	}
	if performanceFollowupAllowed(nil) {
		t.Fatal("performanceFollowupAllowed(nil) = true, want false")
	}
}

func TestNormalizePerformanceFollowupStatuses(t *testing.T) {
	if got := normalizePerformanceInterviewStatus(PerformanceInterviewStatusScheduled); got != PerformanceInterviewStatusScheduled {
		t.Fatalf("normalizePerformanceInterviewStatus() = %q", got)
	}
	if got := normalizePerformanceInterviewStatus("bad-status"); got != "" {
		t.Fatalf("invalid interview status normalized to %q, want empty", got)
	}
	if got := normalizePerformanceAppealStatus(PerformanceAppealStatusProcessing); got != PerformanceAppealStatusProcessing {
		t.Fatalf("normalizePerformanceAppealStatus() = %q", got)
	}
	if got := normalizePerformanceAppealStatus("bad-status"); got != "" {
		t.Fatalf("invalid appeal status normalized to %q, want empty", got)
	}
}

func TestApplyInterviewPayloadStatusDefaultsAndCompletedAt(t *testing.T) {
	scheduledAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.Local)
	record := &database.PerformanceInterviewRecord{}
	applyInterviewPayload(record, PerformanceInterviewPayload{ScheduledAt: &scheduledAt}, true)
	if record.Status != PerformanceInterviewStatusScheduled {
		t.Fatalf("new scheduled interview status = %q, want scheduled", record.Status)
	}
	if record.CompletedAt != nil {
		t.Fatalf("scheduled interview completed_at = %v, want nil", record.CompletedAt)
	}

	applyInterviewPayload(record, PerformanceInterviewPayload{Status: PerformanceInterviewStatusCompleted}, false)
	if record.CompletedAt == nil {
		t.Fatal("completed interview should set completed_at")
	}

	applyInterviewPayload(record, PerformanceInterviewPayload{Status: PerformanceInterviewStatusCancelled}, false)
	if record.CompletedAt != nil {
		t.Fatalf("cancelled interview completed_at = %v, want nil", record.CompletedAt)
	}
}

func TestNotifyInterviewChangedSendsEmployeeAndInterviewer(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://peopleops.example/app")
	originalSender := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = originalSender })

	type sentNotice struct {
		userID      string
		title       string
		content     string
		actionTitle string
		actionURL   string
	}
	sent := make([]sentNotice, 0)
	sendPerformanceActionCardToUser = func(userID, title, content, actionTitle, actionURL string) error {
		sent = append(sent, sentNotice{
			userID:      userID,
			title:       title,
			content:     content,
			actionTitle: actionTitle,
			actionURL:   actionURL,
		})
		return nil
	}

	scheduledAt := time.Date(2026, 6, 30, 10, 30, 0, 0, time.Local)
	svc := &PerformanceFollowupService{}
	svc.notifyInterviewChanged(&database.PerformanceInterviewRecord{
		ActivityID:      "activity 1",
		ActivityName:    "2026 Q2 绩效",
		ParticipantID:   12,
		EmployeeID:      "employee-1",
		EmployeeName:    "列德",
		FinalLevel:      "B",
		Status:          PerformanceInterviewStatusScheduled,
		InterviewerID:   "manager-1",
		InterviewerName: "主管",
		ScheduledAt:     &scheduledAt,
		Location:        "会议室 A",
	})

	if len(sent) != 2 {
		t.Fatalf("sent notice count = %d, want 2: %#v", len(sent), sent)
	}
	if sent[0].userID != "employee-1" || sent[0].title != "绩效面谈通知" {
		t.Fatalf("employee notice = %#v", sent[0])
	}
	if !strings.Contains(sent[0].content, "面谈时间：2026-06-30 10:30") ||
		!strings.Contains(sent[0].actionURL, "/performance-result/activity%201/12") {
		t.Fatalf("employee notice content/url = %#v", sent[0])
	}
	if sent[1].userID != "manager-1" || sent[1].title != "绩效面谈任务提醒" {
		t.Fatalf("interviewer notice = %#v", sent[1])
	}
	if !strings.Contains(sent[1].actionURL, "/performance-interviews?activity_id=activity+1") {
		t.Fatalf("interviewer actionURL = %q", sent[1].actionURL)
	}
}

func TestNotifyAppealStatusChangedSendsEmployee(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://peopleops.example/app")
	originalSender := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = originalSender })

	var gotUserID, gotTitle, gotContent, gotURL string
	sendPerformanceActionCardToUser = func(userID, title, content, actionTitle, actionURL string) error {
		gotUserID = userID
		gotTitle = title
		gotContent = content
		gotURL = actionURL
		return nil
	}

	svc := &PerformanceFollowupService{}
	svc.notifyAppealStatusChanged(&database.PerformanceAppealRecord{
		ActivityID:    "activity-1",
		ActivityName:  "2026 Q2 绩效",
		ParticipantID: 18,
		EmployeeID:    "employee-1",
		Status:        PerformanceAppealStatusResolved,
		HandlerName:   "HR",
		HandleComment: "已复核并完成处理",
	})

	if gotUserID != "employee-1" || gotTitle != "绩效申诉处理通知" {
		t.Fatalf("appeal notice target/title = (%q, %q)", gotUserID, gotTitle)
	}
	if !strings.Contains(gotContent, "申诉状态：已完成") ||
		!strings.Contains(gotContent, "处理意见：已复核并完成处理") ||
		!strings.Contains(gotURL, "/performance-result/activity-1/18") {
		t.Fatalf("appeal notice content/url = (%q, %q)", gotContent, gotURL)
	}
}

func TestPerformanceFollowupScopeAllowsRecord(t *testing.T) {
	if !performanceFollowupScopeAllowsRecord(nil, "employee-1", "dept-1") {
		t.Fatal("nil scope should allow record")
	}
	if !performanceFollowupScopeAllowsRecord(&OrgDataScope{Mode: "department", DepartmentIDs: []string{"dept-1"}}, "employee-1", "dept-1") {
		t.Fatal("matching department scope should allow record")
	}
	if performanceFollowupScopeAllowsRecord(&OrgDataScope{Mode: "department", DepartmentIDs: []string{"dept-2"}}, "employee-1", "dept-1") {
		t.Fatal("different department scope should reject record")
	}
	if !performanceFollowupScopeAllowsRecord(&OrgDataScope{Mode: "self", UserIDs: []string{"employee-1"}}, "employee-1", "dept-1") {
		t.Fatal("matching self scope should allow record")
	}
	if performanceFollowupScopeAllowsRecord(&OrgDataScope{Mode: "self", UserIDs: []string{"employee-2"}}, "employee-1", "dept-1") {
		t.Fatal("different self scope should reject record")
	}
}
