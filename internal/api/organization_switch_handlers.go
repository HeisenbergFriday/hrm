package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"peopleops/internal/database"
)

const organizationSwitchPermission = "organization_switch"

type switchableOrganizationItem struct {
	OrgID     string `json:"org_id"`
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
}

// ListSwitchableOrganizations lists only organizations that the current
// identity can actually enter. The route is permission protected; the public
// /auth/orgs endpoint remains login-page-only and must not be used here.
func ListSwitchableOrganizations(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "未登录"})
		return
	}
	user, err := loadUserByAuthIDInOrg(orgID, userID)
	if err != nil {
		if errors.Is(err, ErrMissingOrgContext) {
			respondMissingOrgContext(c)
			return
		}
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "当前用户不存在或已停用"})
		return
	}

	orgs, err := database.ListSwitchableOrganizationsForUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取可切换组织失败"})
		return
	}
	items := make([]switchableOrganizationItem, 0, len(orgs)+1)
	hasCurrent := false
	for _, organization := range orgs {
		id := database.NormalizeOrganizationID(organization.OrgID)
		if id == orgID {
			hasCurrent = true
		}
		items = append(items, switchableOrganizationItem{OrgID: id, Name: organization.Name, IsCurrent: id == orgID})
	}
	if !hasCurrent {
		if current, lookupErr := database.GetOrganizationByOrgID(orgID); lookupErr == nil && current != nil {
			items = append([]switchableOrganizationItem{{OrgID: orgID, Name: current.Name, IsCurrent: true}}, items...)
		}
	}

	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"organizations": items, "current_org_id": orgID}})
}

// SwitchOrganization rotates the authenticated session into a target
// organization without logging the user out. The target account is resolved
// from the current DingTalk identity and must pass both local configuration and
// active DingTalk-membership checks in database.FindSwitchableUser.
func SwitchOrganization(c *gin.Context) {
	currentOrgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input struct {
		OrgID string `json:"org_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.OrgID) == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标组织不能为空"})
		return
	}
	targetOrgID := database.NormalizeOrganizationID(input.OrgID)
	if targetOrgID == currentOrgID {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "目标组织与当前组织相同"})
		return
	}

	userID := strings.TrimSpace(c.GetString("userID"))
	currentUser, err := loadUserByAuthIDInOrg(currentOrgID, userID)
	if err != nil || currentUser == nil {
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "当前用户不存在或已停用"})
		return
	}
	targetUser, err := database.FindSwitchableUser(currentUser, targetOrgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "你未被配置到该组织，或钉钉账号未加入该组织"})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "校验目标组织失败"})
		return
	}

	newToken, expiresAt, err := generateSessionToken(c, targetUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "切换组织会话失败"})
		return
	}
	// Revoke only the old session inside its original tenant. The newly issued
	// target session is independent and remains valid after the cookie rotates.
	if sessionID := strings.TrimSpace(c.GetString("sessionID")); sessionID != "" && database.DB != nil {
		now := time.Now()
		if revokeErr := database.DB.Model(&database.UserSession{}).
			Where("org_id = ? AND user_id = ? AND session_id = ? AND revoked_at IS NULL", currentOrgID, userID, sessionID).
			Update("revoked_at", &now).Error; revokeErr != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "切换组织会话收口失败"})
			return
		}
	}

	respondAuthSuccess(c, targetUser, newToken, expiresAt)
}
