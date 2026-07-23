package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrMissingOrgContext indicates the authenticated request has no org_id.
// Handlers must map it to a single 401 response — helpers must not write HTTP.
var ErrMissingOrgContext = errors.New("missing org context")

// currentOrgID returns the organization bound to the authenticated request.
// It never writes to the response; callers decide the HTTP status once.
func currentOrgID(c *gin.Context) (string, error) {
	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID == "" {
		return "", ErrMissingOrgContext
	}
	return orgID, nil
}

// currentOrgIDOrAbort returns the organization bound to the authenticated request.
// Missing organization context is rejected with a single 401 response.
// Prefer currentOrgID + explicit handler error mapping when the caller may write other errors.
func currentOrgIDOrAbort(c *gin.Context) (string, bool) {
	orgID, err := currentOrgID(c)
	if err != nil {
		respondMissingOrgContext(c)
		return "", false
	}
	return orgID, true
}

func respondMissingOrgContext(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    http.StatusUnauthorized,
		Message: "缺少组织上下文，请重新登录",
	})
}

// respondScopeError maps scope/org resolution failures to exactly one HTTP response.
func respondScopeError(c *gin.Context, err error, fallbackMessage string) {
	if err == nil {
		return
	}
	if errors.Is(err, ErrMissingOrgContext) {
		respondMissingOrgContext(c)
		return
	}
	if fallbackMessage == "" {
		fallbackMessage = "获取数据范围失败"
	}
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: fallbackMessage,
		Data:    gin.H{"error": err.Error()},
	})
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

// rejectClientOrganizationID rejects any org_id supplied by an untrusted request
// body. Tenant selection is exclusively derived from authenticated context.
func rejectClientOrganizationID(c *gin.Context, supplied *string) bool {
	if supplied == nil {
		return true
	}
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "不允许在请求体中指定 org_id",
	})
	return false
}
