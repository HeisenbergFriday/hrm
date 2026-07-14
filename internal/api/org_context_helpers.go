package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// currentOrgIDOrAbort returns the organization bound to the authenticated request.
// Missing organization context is rejected instead of falling back to another tenant.
func currentOrgIDOrAbort(c *gin.Context) (string, bool) {
	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "缺少组织上下文，请重新登录",
		})
		return "", false
	}
	return orgID, true
}

func rejectCrossOrgParam(c *gin.Context, currentOrgID string, candidates ...string) bool {
	for _, raw := range candidates {
		target := strings.TrimSpace(raw)
		if target == "" {
			continue
		}
		if target != currentOrgID {
			c.JSON(http.StatusForbidden, Response{
				Code:    http.StatusForbidden,
				Message: "不允许通过参数切换到其它组织",
			})
			return false
		}
	}
	return true
}
