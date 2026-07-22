package middleware

import (
	"errors"
	"net/http"
	"strings"

	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ErrMissingOrgContext 表示当前请求上下文没有绑定组织。
var ErrMissingOrgContext = errors.New("middleware: orgID missing in gin.Context")

// CurrentOrgID 返回 JWT 中间件写入的组织标识；fail-closed，缺失时返回错误。
// 业务 handler 应使用它替代 c.GetString("orgID")，保证空 org 不会静默进入业务逻辑。
func CurrentOrgID(c *gin.Context) (string, error) {
	if c == nil {
		return "", ErrMissingOrgContext
	}
	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID == "" {
		return "", ErrMissingOrgContext
	}
	return orgID, nil
}

// TenantDB 返回一个租户上下文 DB：在 RequestDB 的基础上，把当前 orgID 写入 context，
// 供 repository/service 层通过 requestmeta.TenantID 读取。TenantDB 只做上下文携带，
// 不给 SQL 自动追加 org 条件；租户过滤仍在严格 repository 内显式完成。
// 同时回写 RequestInfo.OrgID，保证 CurrentOrganizationIDFromDB 与旧路径一致。
func TenantDB(c *gin.Context) (*gorm.DB, error) {
	orgID, err := CurrentOrgID(c)
	if err != nil {
		return nil, err
	}
	db := RequestDB(c)
	if db == nil {
		return nil, errors.New("middleware: request DB not available")
	}
	ctx := db.Statement.Context
	requestmeta.SetOrgID(ctx, orgID)
	ctx = requestmeta.WithTenant(ctx, orgID)
	return db.WithContext(ctx), nil
}

// TenantContext binds the authenticated orgID to the per-request DB context.
func TenantContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		db, err := TenantDB(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing organization context",
				"code":  "missing_org_context",
			})
			c.Abort()
			return
		}
		c.Set(requestDBKey, db)
		c.Request = c.Request.WithContext(db.Statement.Context)
		c.Next()
	}
}
