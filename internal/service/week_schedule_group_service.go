package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/requestmeta"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	WeekScheduleGroupPushPermission = "week_schedule_group_push"
	weekScheduleGroupPushWindow     = 10 * time.Minute
	weekScheduleImageTTL            = 10 * time.Minute
	weekScheduleImageMaxEntries     = 128
)

var (
	ErrWeekScheduleGroupMissingOrg = errors.New("week schedule group: organization is required")
	ErrWeekScheduleGroupNotFound   = errors.New("week schedule group: target not found")
	ErrWeekScheduleGroupDuplicate  = errors.New("week schedule group: duplicate push blocked")
	ErrWeekScheduleGroupForbidden  = errors.New("week schedule group: permission denied")
	ErrWeekScheduleGroupImageURL   = errors.New("week schedule group: HTTPS app home URL is required")
)

type WeekScheduleGroupSender func(orgID, openConversationID, title, content, imageURL string) (*dingtalk.GroupMessageAcceptance, error)

type WeekScheduleGroupPushInput struct {
	GroupTargetID  uint
	OperatorUserID string
	OperatorName   string
	Title          string
	Content        string
	Month          string
	Image          []byte
	ContentType    string
}

type WeekScheduleGroupPushResult struct {
	Status            string    `json:"status"`
	Message           string    `json:"message"`
	LogID             uint      `json:"log_id"`
	GroupTargetID     uint      `json:"group_target_id"`
	GroupName         string    `json:"group_name"`
	DingTalkRequestID string    `json:"dingtalk_request_id,omitempty"`
	ImageExpiresAt    time.Time `json:"image_expires_at"`
}

type ChatbotBindResult struct {
	Handled bool
	Reply   string
}

type WeekScheduleGroupService struct {
	db            *gorm.DB
	orgID         string
	now           func() time.Time
	imageStore    *temporaryScheduleImageStore
	buildAppURL   func(orgID, path string) string
	sendGroup     WeekScheduleGroupSender
	authorizeBind func(userID string) (bool, error)
}

func NewWeekScheduleGroupServiceWithOrgID(db *gorm.DB, orgID string) *WeekScheduleGroupService {
	orgID = strings.TrimSpace(orgID)
	if orgID != "" {
		orgID = database.NormalizeOrganizationID(orgID)
		ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: orgID})
		ctx = requestmeta.WithTenant(ctx, orgID)
		db = db.Session(&gorm.Session{NewDB: true}).WithContext(ctx)
	}
	svc := &WeekScheduleGroupService{
		db:          db,
		orgID:       orgID,
		now:         time.Now,
		imageStore:  defaultTemporaryScheduleImageStore,
		buildAppURL: dingtalk.BuildAppURLForOrg,
		sendGroup:   dingtalk.SendGroupScheduleMarkdownForOrg,
	}
	svc.authorizeBind = func(userID string) (bool, error) {
		if svc.orgID == "" {
			return false, ErrWeekScheduleGroupMissingOrg
		}
		return NewPermissionServiceWithOrgID(svc.db, svc.orgID).
			HasAnyPermission(userID, WeekScheduleGroupPushPermission, "attendance_manage")
	}
	return svc
}

func (s *WeekScheduleGroupService) requireOrgID() (string, error) {
	if s == nil || s.db == nil || strings.TrimSpace(s.orgID) == "" {
		return "", ErrWeekScheduleGroupMissingOrg
	}
	return s.orgID, nil
}

func (s *WeekScheduleGroupService) HandleChatbotMessage(data *chatbot.BotCallbackDataModel) (ChatbotBindResult, error) {
	if data == nil || strings.TrimSpace(data.Text.Content) != "绑定作息表" || !data.IsInAtList {
		return ChatbotBindResult{}, nil
	}
	result := ChatbotBindResult{Handled: true}
	orgID, err := s.requireOrgID()
	if err != nil {
		result.Reply = "绑定失败：未识别当前组织，请联系管理员检查机器人配置。"
		return result, err
	}
	conversationID := strings.TrimSpace(data.ConversationId)
	groupName := strings.TrimSpace(data.ConversationTitle)
	if conversationID == "" || groupName == "" || strings.TrimSpace(data.ConversationType) == "1" {
		result.Reply = "绑定失败：请在机器人已加入的钉钉群聊中发送“绑定作息表”。"
		return result, ErrWeekScheduleGroupNotFound
	}
	senderStaffID := strings.TrimSpace(data.SenderStaffId)
	if senderStaffID == "" {
		result.Reply = "绑定失败：无法识别操作人，请确认钉钉账号已同步到系统。"
		return result, ErrWeekScheduleGroupForbidden
	}

	var user database.User
	err = s.db.Where(
		"org_id = ? AND status = ? AND deleted_at IS NULL AND (user_id = ? OR ding_talk_user_id = ?)",
		orgID, "active", senderStaffID, senderStaffID,
	).First(&user).Error
	if err != nil {
		result.Reply = "绑定失败：当前钉钉账号未映射到本组织的在职员工。"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, ErrWeekScheduleGroupForbidden
		}
		return result, err
	}
	allowed, err := s.authorizeBind(user.UserID)
	if err != nil {
		result.Reply = "绑定失败：权限校验异常，请稍后重试。"
		return result, err
	}
	if !allowed {
		result.Reply = "绑定失败：你缺少作息表群聊推送权限，请联系管理员添加。"
		return result, ErrWeekScheduleGroupForbidden
	}

	now := s.now()
	target := database.WeekScheduleGroupTarget{
		OrgID:              orgID,
		OpenConversationID: conversationID,
		GroupName:          groupName,
		Status:             "active",
		BoundByUserID:      user.UserID,
		BoundByUserName:    user.Name,
		BoundAt:            now,
	}
	err = s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "open_conversation_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"group_name":           groupName,
			"status":               "active",
			"bound_by_user_id":     user.UserID,
			"bound_by_user_name":   user.Name,
			"bound_at":             now,
			"unbound_by_user_id":   "",
			"unbound_by_user_name": "",
			"unbound_at":           nil,
			"updated_at":           now,
		}),
	}).Create(&target).Error
	if err != nil {
		result.Reply = "绑定失败：保存群聊信息失败，请稍后重试。"
		return result, err
	}
	result.Reply = fmt.Sprintf("已绑定作息表群聊：%s。今后可在系统中选择该群推送月作息表。", groupName)
	return result, nil
}

func (s *WeekScheduleGroupService) ListTargets() ([]database.WeekScheduleGroupTarget, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var targets []database.WeekScheduleGroupTarget
	err = s.db.Where("org_id = ? AND status = ?", orgID, "active").Order("group_name ASC, id ASC").Find(&targets).Error
	return targets, err
}

func (s *WeekScheduleGroupService) UnbindTarget(targetID uint, operatorUserID, operatorName string) error {
	orgID, err := s.requireOrgID()
	if err != nil {
		return err
	}
	if targetID == 0 {
		return ErrWeekScheduleGroupNotFound
	}
	now := s.now()
	result := s.db.Model(&database.WeekScheduleGroupTarget{}).
		Where("org_id = ? AND id = ? AND status = ?", orgID, targetID, "active").
		Updates(map[string]interface{}{
			"status":               "unbound",
			"unbound_by_user_id":   strings.TrimSpace(operatorUserID),
			"unbound_by_user_name": strings.TrimSpace(operatorName),
			"unbound_at":           now,
			"updated_at":           now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWeekScheduleGroupNotFound
	}
	return nil
}

func (s *WeekScheduleGroupService) Push(input WeekScheduleGroupPushInput) (*WeekScheduleGroupPushResult, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	input.Month = strings.TrimSpace(input.Month)
	if parsed, parseErr := time.Parse("2006-01", input.Month); parseErr != nil || parsed.Format("2006-01") != input.Month {
		return nil, fmt.Errorf("month must use YYYY-MM format")
	}
	if input.GroupTargetID == 0 {
		return nil, ErrWeekScheduleGroupNotFound
	}
	if len(input.Image) == 0 {
		return nil, errors.New("image is empty")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	if len([]rune(input.Title)) > 200 || len([]rune(input.Content)) > 4000 {
		return nil, errors.New("title or content is too long")
	}

	var target database.WeekScheduleGroupTarget
	var pushLog database.WeekScheduleGroupPushLog
	now := s.now()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND id = ? AND status = ?", orgID, input.GroupTargetID, "active").
			First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWeekScheduleGroupNotFound
			}
			return err
		}
		var recent int64
		if err := tx.Model(&database.WeekScheduleGroupPushLog{}).
			Where("org_id = ? AND group_target_id = ? AND month = ? AND status IN ? AND created_at >= ?",
				orgID, target.ID, input.Month, []string{"processing", "submitted"}, now.Add(-weekScheduleGroupPushWindow)).
			Count(&recent).Error; err != nil {
			return err
		}
		if recent > 0 {
			return ErrWeekScheduleGroupDuplicate
		}
		pushLog = database.WeekScheduleGroupPushLog{
			OrgID:            orgID,
			OperatorUserID:   strings.TrimSpace(input.OperatorUserID),
			OperatorUserName: strings.TrimSpace(input.OperatorName),
			Month:            input.Month,
			GroupTargetID:    target.ID,
			GroupName:        target.GroupName,
			Status:           "processing",
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		return tx.Create(&pushLog).Error
	})
	if err != nil {
		return nil, err
	}

	token, expiresAt, err := s.imageStore.Put(input.Image, input.ContentType, now)
	if err != nil {
		_ = s.finishPushLog(&pushLog, "failed", "", "IMAGE_STORE_FAILED", "临时图片保存失败")
		return nil, err
	}
	imageURL, err := s.temporaryImageURL(token)
	if err != nil {
		_ = s.finishPushLog(&pushLog, "failed", "", "IMAGE_URL_CONFIG_MISSING", "作息表临时图片地址必须使用 HTTPS")
		return nil, err
	}

	acceptance, err := s.sendGroup(orgID, target.OpenConversationID, input.Title, input.Content, imageURL)
	if err != nil {
		code := dingtalk.SyncErrorCode(err)
		summary := dingtalk.SyncErrorSafeMessage(err)
		if summary == "" {
			summary = "群消息提交失败"
		}
		status := "failed"
		if code == dingtalk.ErrorCodeGroupUnavailable || code == dingtalk.ErrorCodeGroupRejected {
			status = "rejected"
		}
		_ = s.finishPushLog(&pushLog, status, "", code, summary)
		return nil, err
	}
	requestID := ""
	if acceptance != nil {
		requestID = strings.TrimSpace(acceptance.RequestID)
	}
	if err := s.finishPushLog(&pushLog, "submitted", requestID, "", ""); err != nil {
		return nil, err
	}
	return &WeekScheduleGroupPushResult{
		Status:            "submitted",
		Message:           "群消息已提交钉钉处理",
		LogID:             pushLog.ID,
		GroupTargetID:     target.ID,
		GroupName:         target.GroupName,
		DingTalkRequestID: requestID,
		ImageExpiresAt:    expiresAt,
	}, nil
}

func (s *WeekScheduleGroupService) temporaryImageURL(token string) (string, error) {
	base := strings.TrimSpace(s.buildAppURL(s.orgID, "/api/v1/week-schedule/group-image"))
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", ErrWeekScheduleGroupImageURL
	}
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (s *WeekScheduleGroupService) finishPushLog(log *database.WeekScheduleGroupPushLog, status, requestID, code, summary string) error {
	if log == nil || log.ID == 0 {
		return errors.New("week schedule group push log is missing")
	}
	result := s.db.Model(&database.WeekScheduleGroupPushLog{}).
		Where("org_id = ? AND id = ?", s.orgID, log.ID).
		UpdateColumns(map[string]interface{}{
			"status":               status,
			"ding_talk_request_id": strings.TrimSpace(requestID),
			"error_code":           strings.TrimSpace(code),
			"error_summary":        truncateSafeSummary(summary),
			"updated_at":           s.now(),
		})
	if result.Error != nil {
		return fmt.Errorf("update week schedule group push log: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("update week schedule group push log: target not found")
	}
	log.Status = status
	log.DingTalkRequestID = strings.TrimSpace(requestID)
	log.ErrorCode = strings.TrimSpace(code)
	log.ErrorSummary = truncateSafeSummary(summary)
	return nil
}

func truncateSafeSummary(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > 240 {
		return string(runes[:240]) + "…"
	}
	return value
}

type temporaryScheduleImage struct {
	Content     []byte
	ContentType string
	ExpiresAt   time.Time
}

type temporaryScheduleImageStore struct {
	mu      sync.Mutex
	items   map[[sha256.Size]byte]temporaryScheduleImage
	ttl     time.Duration
	maxSize int
}

var defaultTemporaryScheduleImageStore = newTemporaryScheduleImageStore(weekScheduleImageTTL, weekScheduleImageMaxEntries)

func newTemporaryScheduleImageStore(ttl time.Duration, maxSize int) *temporaryScheduleImageStore {
	return &temporaryScheduleImageStore{items: make(map[[sha256.Size]byte]temporaryScheduleImage), ttl: ttl, maxSize: maxSize}
}

func (s *temporaryScheduleImageStore) Put(content []byte, contentType string, now time.Time) (string, time.Time, error) {
	if s == nil || len(content) == 0 {
		return "", time.Time{}, errors.New("temporary image is empty")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	key := sha256.Sum256([]byte(token))
	expiresAt := now.Add(s.ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	if s.maxSize > 0 && len(s.items) >= s.maxSize {
		return "", time.Time{}, errors.New("temporary image capacity reached")
	}
	s.items[key] = temporaryScheduleImage{
		Content:     append([]byte(nil), content...),
		ContentType: strings.TrimSpace(contentType),
		ExpiresAt:   expiresAt,
	}
	return token, expiresAt, nil
}

func (s *temporaryScheduleImageStore) Get(token string, now time.Time) (temporaryScheduleImage, bool) {
	if s == nil || strings.TrimSpace(token) == "" {
		return temporaryScheduleImage{}, false
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(token)))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	item, ok := s.items[key]
	if !ok || !now.Before(item.ExpiresAt) {
		return temporaryScheduleImage{}, false
	}
	item.Content = append([]byte(nil), item.Content...)
	return item, true
}

func (s *temporaryScheduleImageStore) cleanupLocked(now time.Time) {
	for key, item := range s.items {
		if !now.Before(item.ExpiresAt) {
			delete(s.items, key)
		}
	}
}

// LoadTemporaryWeekScheduleImage is used by the public, token-gated image route.
func LoadTemporaryWeekScheduleImage(token string, now time.Time) ([]byte, string, time.Time, bool) {
	item, ok := defaultTemporaryScheduleImageStore.Get(token, now)
	if !ok {
		return nil, "", time.Time{}, false
	}
	return item.Content, item.ContentType, item.ExpiresAt, true
}
