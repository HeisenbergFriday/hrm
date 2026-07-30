package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var forbiddenWeekScheduleGroupFields = []string{
	"org_id", "open_conversation_id", "openConversationId", "webhook", "session_webhook",
	"app_key", "app_secret", "secret", "robot_code", "robotCode",
}

func GetWeekScheduleGroupTargets(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	targets, err := service.NewWeekScheduleGroupServiceWithOrgID(middleware.RequestDB(c), orgID).ListTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取已绑定群聊失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"items": targets}})
}

func UnbindWeekScheduleGroupTarget(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	targetID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || targetID == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "群聊目标 ID 无效"})
		return
	}
	svc := service.NewWeekScheduleGroupServiceWithOrgID(middleware.RequestDB(c), orgID)
	err = svc.UnbindTarget(uint(targetID), c.GetString("userID"), c.GetString("userName"))
	if errors.Is(err, service.ErrWeekScheduleGroupNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "群聊不存在或已解绑"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "解绑群聊失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "解绑成功"})
}

func PushWeekScheduleToGroup(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	for _, field := range forbiddenWeekScheduleGroupFields {
		if strings.TrimSpace(c.PostForm(field)) != "" {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请求不得包含组织、群聊标识或钉钉凭据字段"})
			return
		}
	}

	targetID, err := strconv.ParseUint(strings.TrimSpace(c.PostForm("group_target_id")), 10, 64)
	if err != nil || targetID == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请选择已绑定群聊"})
		return
	}
	month := strings.TrimSpace(c.PostForm("month"))
	if parsed, parseErr := time.Parse("2006-01", month); parseErr != nil || parsed.Format("2006-01") != month {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "月份格式必须为 YYYY-MM"})
		return
	}

	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请上传作息表图片（image）"})
		return
	}
	if file.Size <= 0 || file.Size > weekSchedulePersonalPushMaxBytes {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "图片不能为空且不能超过 8MB"})
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "仅支持 PNG/JPEG 图片"})
		return
	}
	if err := validateUploadContent(file, ext); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "图片校验失败: " + err.Error()})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取图片失败"})
		return
	}
	defer func() { _ = src.Close() }()
	image, err := io.ReadAll(io.LimitReader(src, weekSchedulePersonalPushMaxBytes+1))
	if err != nil || len(image) == 0 || len(image) > weekSchedulePersonalPushMaxBytes {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取图片失败或图片超过 8MB"})
		return
	}
	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	svc := service.NewWeekScheduleGroupServiceWithOrgID(middleware.RequestDB(c), orgID)
	result, err := svc.Push(service.WeekScheduleGroupPushInput{
		GroupTargetID:  uint(targetID),
		OperatorUserID: c.GetString("userID"),
		OperatorName:   c.GetString("userName"),
		Title:          c.PostForm("title"),
		Content:        c.PostForm("content"),
		Month:          month,
		Image:          image,
		ContentType:    contentType,
	})
	if err != nil {
		respondWeekScheduleGroupPushError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "submitted", Data: result})
}

func ServeWeekScheduleGroupImage(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	content, contentType, expiresAt, ok := service.LoadTemporaryWeekScheduleImage(token, time.Now())
	if !ok {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if contentType != "image/png" && contentType != "image/jpeg" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", expiresAt.UTC().Format(http.TimeFormat))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, content)
}

func respondWeekScheduleGroupPushError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWeekScheduleGroupDuplicate):
		c.JSON(http.StatusConflict, Response{Code: http.StatusConflict, Message: "该群本月作息表刚刚已提交，请勿重复推送"})
	case errors.Is(err, service.ErrWeekScheduleGroupNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "群聊不存在、已解绑或不属于当前组织"})
	case errors.Is(err, service.ErrWeekScheduleGroupMissingOrg):
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "当前会话缺少组织信息"})
	case errors.Is(err, service.ErrWeekScheduleGroupImageURL):
		c.JSON(http.StatusServiceUnavailable, Response{Code: http.StatusServiceUnavailable, Message: "当前组织未配置可供钉钉访问的 HTTPS 应用地址"})
	default:
		code := dingtalk.SyncErrorCode(err)
		message := dingtalk.SyncErrorSafeMessage(err)
		if message == "" {
			message = "群消息提交失败，请稍后重试"
		}
		status := http.StatusBadGateway
		if code == dingtalk.ErrorCodeConfigMissing || code == dingtalk.ErrorCodeTokenFailed {
			status = http.StatusServiceUnavailable
		} else if code == dingtalk.ErrorCodeNetworkFailed && strings.Contains(strings.ToLower(dingtalk.SafeErrorSummary(err)), "timed out") {
			status = http.StatusGatewayTimeout
		} else if code == dingtalk.ErrorCodeGroupUnavailable {
			status = http.StatusUnprocessableEntity
		}
		c.JSON(status, Response{Code: status, Message: message, Data: gin.H{"error_code": code}})
	}
}
