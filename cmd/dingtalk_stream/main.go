package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/service"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/logger"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"gorm.io/gorm"
)

func main() {
	if err := config.Load(); err != nil {
		log.Printf("加载配置警告: %v", err)
	}

	db, err := openStreamDB()
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	connectionConfig, err := resolveStreamConnectionConfig(db)
	if err != nil {
		log.Fatalf("解析 Stream 连接配置失败: %v", err)
	}
	orgID := connectionConfig.OrgID

	streamService := service.NewDingTalkStreamService(db, orgID).
		WithLogPayload(truthyEnv("DINGTALK_STREAM_LOG_PAYLOAD"))
	groupService := service.NewWeekScheduleGroupServiceWithOrgID(db, orgID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.SetLogger(streamSDKLogger{})
	options := []client.ClientOption{
		client.WithAppCredential(client.NewAppCredentialConfig(connectionConfig.ClientID, connectionConfig.ClientSecret)),
		client.WithAutoReconnect(true),
	}
	if proxy := strings.TrimSpace(os.Getenv("DINGTALK_STREAM_PROXY")); proxy != "" {
		options = append(options, client.WithProxy(proxy))
	}

	streamClient := client.NewStreamClient(options...)
	streamClient.RegisterAllEventRouter(func(ctx context.Context, dataFrame *payload.DataFrame) (*payload.DataFrameResponse, error) {
		return streamService.HandleDataFrame(ctx, dataFrame)
	})
	streamClient.RegisterChatBotCallbackRouter(func(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		if data != nil {
			log.Printf(
				"收到钉钉机器人回调: org_id=%s conversation_type=%s msg_type=%s at_robot=%t command_match=%t",
				orgID,
				strings.TrimSpace(data.ConversationType),
				strings.TrimSpace(data.Msgtype),
				data.IsInAtList,
				strings.TrimSpace(data.Text.Content) == "绑定作息表",
			)
		}
		result, bindErr := groupService.HandleChatbotMessage(data)
		if bindErr != nil {
			log.Printf("钉钉群聊作息表绑定处理失败: org_id=%s err=%s", orgID, dingtalk.SafeErrorSummary(bindErr))
		}
		if result.Handled && strings.TrimSpace(result.Reply) != "" && data != nil && strings.TrimSpace(data.SessionWebhook) != "" {
			if replyErr := chatbot.NewChatbotReplier().SimpleReplyText(ctx, data.SessionWebhook, []byte(result.Reply)); replyErr != nil {
				// SessionWebhook contains a credential-like token; never include it or the raw error in logs.
				log.Printf("钉钉群聊作息表绑定结果回复失败: org_id=%s", orgID)
			}
		}
		return []byte(""), nil
	})

	log.Printf("正在连接钉钉 Stream，org_id=%s，等待建立长连接...", orgID)
	if err := streamClient.Start(ctx); err != nil {
		log.Fatalf("钉钉 Stream 连接失败: %v", err)
	}
	defer streamClient.Close()

	log.Printf("钉钉 Stream 已连接。审批事件将增量同步，群聊可通过 @机器人发送“绑定作息表”完成绑定。")
	<-ctx.Done()
	log.Printf("收到退出信号，正在关闭钉钉 Stream 连接")
}

func openStreamDB() (*gorm.DB, error) {
	if err := database.Init(); err != nil {
		return nil, err
	}
	if database.DB == nil {
		return nil, fmt.Errorf("database.DB is nil after init")
	}
	return database.DB, nil
}

type streamOrgSource interface {
	GetActiveOrg(orgID string) (*database.Organization, error)
	ListActiveByAppKey(appKey string) ([]database.Organization, error)
}

type gormStreamOrgSource struct {
	db *gorm.DB
}

type streamConnectionConfig struct {
	OrgID        string
	ClientID     string
	ClientSecret string
}

func (s gormStreamOrgSource) GetActiveOrg(orgID string) (*database.Organization, error) {
	var org database.Organization
	err := s.db.Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, "active").First(&org).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func (s gormStreamOrgSource) ListActiveByAppKey(appKey string) ([]database.Organization, error) {
	var orgs []database.Organization
	// 使用模型字段匹配，确保落到库表真实列 ding_talk_app_key，
	// 避免手写 dingtalk_app_key 这类错误列名导致启动 fail-closed。
	err := s.db.
		Where("status = ? AND deleted_at IS NULL", "active").
		Where(&database.Organization{DingTalkAppKey: appKey}).
		Find(&orgs).Error
	return orgs, err
}

func resolveStreamConnectionConfig(db *gorm.DB) (streamConnectionConfig, error) {
	if db == nil {
		return streamConnectionConfig{}, fmt.Errorf("database is required to resolve stream connection config")
	}
	return resolveStreamConnectionConfigWithSource(
		gormStreamOrgSource{db: db},
		strings.TrimSpace(os.Getenv("DINGTALK_STREAM_ORG_ID")),
		strings.TrimSpace(os.Getenv("DINGTALK_APP_KEY")),
		strings.TrimSpace(os.Getenv("DINGTALK_APP_SECRET")),
	)
}

// resolveStreamConnectionConfigWithSource 保证组织与 Stream 凭据来自同一配置源：
// 1) 显式指定组织时，直接使用该 active 组织持久化的 AppKey/Secret；
// 2) 未指定组织时，继续使用全局环境凭据，并要求 AppKey 唯一匹配 active 组织；
// 3) 任一必要配置缺失都 fail-closed，禁止静默回退 default 或混用其他组织凭据。
func resolveStreamConnectionConfigWithSource(
	src streamOrgSource,
	explicitOrg string,
	envClientID string,
	envClientSecret string,
) (streamConnectionConfig, error) {
	if src == nil {
		return streamConnectionConfig{}, fmt.Errorf("organization source is required")
	}

	if explicit := strings.TrimSpace(explicitOrg); explicit != "" {
		orgID := database.NormalizeOrganizationID(explicit)
		org, err := src.GetActiveOrg(orgID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return streamConnectionConfig{}, fmt.Errorf("organization %s not found or inactive", orgID)
			}
			return streamConnectionConfig{}, err
		}
		clientID := strings.TrimSpace(org.DingTalkAppKey)
		clientSecret := strings.TrimSpace(org.DingTalkSecret)
		if clientID == "" {
			return streamConnectionConfig{}, fmt.Errorf("organization %s has empty dingtalk app key", orgID)
		}
		if clientSecret == "" {
			return streamConnectionConfig{}, fmt.Errorf("organization %s has empty dingtalk secret", orgID)
		}
		return streamConnectionConfig{OrgID: orgID, ClientID: clientID, ClientSecret: clientSecret}, nil
	}

	clientID := strings.TrimSpace(envClientID)
	clientSecret := strings.TrimSpace(envClientSecret)
	if clientID == "" || clientSecret == "" {
		return streamConnectionConfig{}, fmt.Errorf("DINGTALK_APP_KEY and DINGTALK_APP_SECRET are required when DINGTALK_STREAM_ORG_ID is empty")
	}
	orgID, err := resolveStreamOrgIDWithSource(src, "", clientID)
	if err != nil {
		return streamConnectionConfig{}, err
	}
	return streamConnectionConfig{OrgID: orgID, ClientID: clientID, ClientSecret: clientSecret}, nil
}

func resolveStreamOrgID(db *gorm.DB, clientID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database is required to resolve stream organization")
	}
	return resolveStreamOrgIDWithSource(gormStreamOrgSource{db: db}, strings.TrimSpace(os.Getenv("DINGTALK_STREAM_ORG_ID")), clientID)
}

// resolveStreamOrgIDWithSource 将 Stream 凭据严格绑定到唯一组织：
// 1) 显式 org 时，组织必须 active 且 dingtalk_app_key 与当前 clientID 完全一致；
// 2) 未显式设置时，按 active + app_key 精确匹配，恰好 1 条才允许启动；
// 3) 0 条或多条均 fail-closed，绝不静默回落 default。
func resolveStreamOrgIDWithSource(src streamOrgSource, explicitOrg, clientID string) (string, error) {
	if src == nil {
		return "", fmt.Errorf("organization source is required")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return "", fmt.Errorf("dingtalk app key is required to resolve stream organization")
	}

	if explicit := strings.TrimSpace(explicitOrg); explicit != "" {
		orgID := database.NormalizeOrganizationID(explicit)
		org, err := src.GetActiveOrg(orgID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return "", fmt.Errorf("organization %s not found or inactive", orgID)
			}
			return "", err
		}
		orgAppKey := strings.TrimSpace(org.DingTalkAppKey)
		if orgAppKey == "" {
			return "", fmt.Errorf("organization %s has empty dingtalk app key", orgID)
		}
		if orgAppKey != clientID {
			return "", fmt.Errorf(
				"organization %s app key mismatch with current stream credentials (expected configured key, got %s)",
				orgID,
				maskAppKey(clientID),
			)
		}
		return orgID, nil
	}

	orgs, err := src.ListActiveByAppKey(clientID)
	if err != nil {
		return "", err
	}
	switch len(orgs) {
	case 1:
		return database.NormalizeOrganizationID(orgs[0].OrgID), nil
	case 0:
		return "", fmt.Errorf(
			"no active organization matched current stream app key %s; set DINGTALK_STREAM_ORG_ID or configure organization.dingtalk_app_key",
			maskAppKey(clientID),
		)
	default:
		return "", fmt.Errorf(
			"stream app key %s matches %d active organizations; set DINGTALK_STREAM_ORG_ID explicitly",
			maskAppKey(clientID),
			len(orgs),
		)
	}
}

// maskAppKey avoids leaking full credentials in logs/errors.
func maskAppKey(appKey string) string {
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return "<empty>"
	}
	if len(appKey) <= 8 {
		return appKey[:1] + "***"
	}
	return appKey[:4] + "***" + appKey[len(appKey)-2:]
}

type streamSDKLogger struct{}

func (streamSDKLogger) Debugf(string, ...interface{}) {}

func (streamSDKLogger) Infof(format string, args ...interface{}) {
	log.Printf("[DingTalk Stream] INFO: %s", fmt.Sprintf(format, args...))
}

func (streamSDKLogger) Warningf(format string, args ...interface{}) {
	log.Printf("[DingTalk Stream] WARN: %s", fmt.Sprintf(format, args...))
}

func (streamSDKLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[DingTalk Stream] ERROR: %s", fmt.Sprintf(format, args...))
}

func (streamSDKLogger) Fatalf(format string, args ...interface{}) {
	log.Printf("[DingTalk Stream] FATAL: %s", fmt.Sprintf(format, args...))
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
