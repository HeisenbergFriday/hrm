package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"github.com/glebarez/sqlite"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openWeekScheduleGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:week-group-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.WeekScheduleGroupTarget{},
		&database.WeekScheduleGroupPushLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedWeekScheduleGroupUser(t *testing.T, db *gorm.DB, orgID, userID, dingID, name string) {
	t.Helper()
	if err := db.Create(&database.User{
		OrgID: orgID, UserID: userID, DingTalkUserID: dingID, Name: name,
		Email: userID + "@" + orgID + ".test", Mobile: orgID + "-" + userID,
		DepartmentID: "dept-1", Status: "active",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func bindMessage(conversationID, title, sender string) *chatbot.BotCallbackDataModel {
	return &chatbot.BotCallbackDataModel{
		ConversationId: conversationID, ConversationTitle: title, ConversationType: "2",
		SenderStaffId: sender, IsInAtList: true,
		Text: chatbot.BotCallbackDataTextModel{Content: "绑定作息表"},
	}
}

func TestWeekScheduleGroupStreamBindAndUnbind(t *testing.T) {
	db := openWeekScheduleGroupTestDB(t)
	seedWeekScheduleGroupUser(t, db, "org-a", "u-a", "ding-a", "张三")
	svc := NewWeekScheduleGroupServiceWithOrgID(db, "org-a")
	svc.authorizeBind = func(userID string) (bool, error) { return userID == "u-a", nil }

	result, err := svc.HandleChatbotMessage(bindMessage("cid-1", "考勤计算群", "ding-a"))
	if err != nil || !result.Handled || !strings.Contains(result.Reply, "已绑定") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	targets, err := svc.ListTargets()
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets=%#v err=%v", targets, err)
	}
	if targets[0].GroupName != "考勤计算群" || targets[0].BoundByUserID != "u-a" {
		t.Fatalf("target=%#v", targets[0])
	}
	encoded, _ := json.Marshal(targets[0])
	if strings.Contains(string(encoded), "cid-1") || strings.Contains(string(encoded), "org-a") {
		t.Fatalf("target JSON leaks server-only identifiers: %s", encoded)
	}
	if err := svc.UnbindTarget(targets[0].ID, "u-a", "张三"); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	targets, _ = svc.ListTargets()
	if len(targets) != 0 {
		t.Fatalf("active targets after unbind=%#v", targets)
	}
}

func TestWeekScheduleGroupStreamBindRequiresLocalUserAndPermission(t *testing.T) {
	db := openWeekScheduleGroupTestDB(t)
	seedWeekScheduleGroupUser(t, db, "org-a", "u-a", "ding-a", "张三")
	svc := NewWeekScheduleGroupServiceWithOrgID(db, "org-a")
	svc.authorizeBind = func(string) (bool, error) { return false, nil }

	result, err := svc.HandleChatbotMessage(bindMessage("cid-1", "群A", "ding-a"))
	if !errors.Is(err, ErrWeekScheduleGroupForbidden) || !strings.Contains(result.Reply, "缺少") {
		t.Fatalf("permission result=%#v err=%v", result, err)
	}
	result, err = svc.HandleChatbotMessage(bindMessage("cid-2", "群B", "unknown"))
	if !errors.Is(err, ErrWeekScheduleGroupForbidden) || !strings.Contains(result.Reply, "未映射") {
		t.Fatalf("mapping result=%#v err=%v", result, err)
	}
	var count int64
	_ = db.Model(&database.WeekScheduleGroupTarget{}).Count(&count).Error
	if count != 0 {
		t.Fatalf("target count=%d, want 0", count)
	}
}

func TestWeekScheduleGroupTargetsAreIsolatedByOrganization(t *testing.T) {
	db := openWeekScheduleGroupTestDB(t)
	seedWeekScheduleGroupUser(t, db, "org-a", "u-a", "ding-a", "A用户")
	seedWeekScheduleGroupUser(t, db, "org-b", "u-b", "ding-b", "B用户")
	for _, tc := range []struct{ org, user, ding, name string }{
		{"org-a", "u-a", "ding-a", "A群"},
		{"org-b", "u-b", "ding-b", "B群"},
	} {
		svc := NewWeekScheduleGroupServiceWithOrgID(db, tc.org)
		svc.authorizeBind = func(string) (bool, error) { return true, nil }
		if _, err := svc.HandleChatbotMessage(bindMessage("same-conversation", tc.name, tc.ding)); err != nil {
			t.Fatalf("bind %s: %v", tc.org, err)
		}
	}
	targetsA, _ := NewWeekScheduleGroupServiceWithOrgID(db, "org-a").ListTargets()
	targetsB, _ := NewWeekScheduleGroupServiceWithOrgID(db, "org-b").ListTargets()
	if len(targetsA) != 1 || targetsA[0].GroupName != "A群" || len(targetsB) != 1 || targetsB[0].GroupName != "B群" {
		t.Fatalf("targetsA=%#v targetsB=%#v", targetsA, targetsB)
	}
	if err := NewWeekScheduleGroupServiceWithOrgID(db, "org-a").UnbindTarget(targetsB[0].ID, "u-a", "A用户"); !errors.Is(err, ErrWeekScheduleGroupNotFound) {
		t.Fatalf("cross-org unbind err=%v", err)
	}
}

func TestWeekScheduleGroupPushSubmittedAndDuplicateBlocked(t *testing.T) {
	db := openWeekScheduleGroupTestDB(t)
	target := database.WeekScheduleGroupTarget{OrgID: "org-a", OpenConversationID: "cid-a", GroupName: "A群", Status: "active", BoundByUserID: "u-a", BoundByUserName: "A", BoundAt: time.Now()}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewWeekScheduleGroupServiceWithOrgID(db, "org-a")
	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	svc.imageStore = newTemporaryScheduleImageStore(10*time.Minute, 8)
	svc.buildAppURL = func(orgID, path string) string {
		if orgID != "org-a" {
			t.Fatalf("build URL org=%s", orgID)
		}
		return "https://hr.example" + path
	}
	var sentURL string
	svc.sendGroup = func(orgID, conversationID, title, content, imageURL string) (*dingtalk.GroupMessageAcceptance, error) {
		if orgID != "org-a" || conversationID != "cid-a" {
			t.Fatalf("send args org=%s conversation=%s", orgID, conversationID)
		}
		sentURL = imageURL
		return &dingtalk.GroupMessageAcceptance{RequestID: "req-1"}, nil
	}
	input := WeekScheduleGroupPushInput{GroupTargetID: target.ID, OperatorUserID: "u-a", OperatorName: "A", Month: "2026-07", Title: "七月作息表", Content: "请查收", Image: []byte("png"), ContentType: "image/png"}
	result, err := svc.Push(input)
	if err != nil || result.Status != "submitted" || result.DingTalkRequestID != "req-1" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if !strings.HasPrefix(sentURL, "https://hr.example/api/v1/week-schedule/group-image?token=") {
		t.Fatalf("temporary URL=%s", sentURL)
	}
	if _, err := svc.Push(input); !errors.Is(err, ErrWeekScheduleGroupDuplicate) {
		t.Fatalf("duplicate err=%v", err)
	}
	var log database.WeekScheduleGroupPushLog
	if err := db.Where("org_id = ?", "org-a").First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.Status != "submitted" || log.DingTalkRequestID != "req-1" || log.GroupName != "A群" {
		t.Fatalf("log=%#v", log)
	}
}

func TestWeekScheduleGroupPushRejectionIsSafeAndRetryable(t *testing.T) {
	db := openWeekScheduleGroupTestDB(t)
	target := database.WeekScheduleGroupTarget{OrgID: "org-a", OpenConversationID: "bad-cid", GroupName: "失效群", Status: "active", BoundByUserID: "u", BoundByUserName: "U", BoundAt: time.Now()}
	_ = db.Create(&target).Error
	svc := NewWeekScheduleGroupServiceWithOrgID(db, "org-a")
	svc.imageStore = newTemporaryScheduleImageStore(time.Minute, 8)
	svc.buildAppURL = func(string, string) string { return "https://hr.example/image" }
	svc.sendGroup = func(string, string, string, string, string) (*dingtalk.GroupMessageAcceptance, error) {
		return nil, &dingtalk.SyncError{Code: dingtalk.ErrorCodeGroupUnavailable, SafeMessage: "群聊无效或机器人未加入该群"}
	}
	input := WeekScheduleGroupPushInput{GroupTargetID: target.ID, OperatorUserID: "u", OperatorName: "U", Month: "2026-07", Image: []byte("png"), ContentType: "image/png"}
	if _, err := svc.Push(input); dingtalk.SyncErrorCode(err) != dingtalk.ErrorCodeGroupUnavailable {
		t.Fatalf("err=%v", err)
	}
	var log database.WeekScheduleGroupPushLog
	_ = db.First(&log).Error
	if log.Status != "rejected" || log.ErrorSummary != "群聊无效或机器人未加入该群" {
		t.Fatalf("log=%#v", log)
	}
	// Rejected attempts do not claim a false success and may be retried after fixing the group.
	if _, err := svc.Push(input); dingtalk.SyncErrorCode(err) != dingtalk.ErrorCodeGroupUnavailable {
		t.Fatalf("retry err=%v", err)
	}
}

func TestTemporaryScheduleImageExpires(t *testing.T) {
	store := newTemporaryScheduleImageStore(time.Minute, 2)
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	token, expiresAt, err := store.Put([]byte("image"), "image/png", now)
	if err != nil || token == "" || len(token) < 40 {
		t.Fatalf("token length=%d expiry=%s err=%v", len(token), expiresAt, err)
	}
	if item, ok := store.Get(token, now.Add(59*time.Second)); !ok || string(item.Content) != "image" {
		t.Fatalf("image before expiry ok=%v item=%#v", ok, item)
	}
	if _, ok := store.Get(token, expiresAt); ok {
		t.Fatal("image must expire at expiry time")
	}
	if _, ok := store.Get("invalid-token", now); ok {
		t.Fatal("invalid token must not resolve")
	}
}

func TestWeekScheduleGroupServiceFailsClosedWithoutOrganization(t *testing.T) {
	svc := NewWeekScheduleGroupServiceWithOrgID(openWeekScheduleGroupTestDB(t), "")
	if _, err := svc.ListTargets(); !errors.Is(err, ErrWeekScheduleGroupMissingOrg) {
		t.Fatalf("list err=%v", err)
	}
	if _, err := svc.Push(WeekScheduleGroupPushInput{}); !errors.Is(err, ErrWeekScheduleGroupMissingOrg) {
		t.Fatalf("push err=%v", err)
	}
}
