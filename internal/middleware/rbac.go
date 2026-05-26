package middleware

import (
	"net/http"
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"github.com/gin-gonic/gin"
)

// RequirePermission 返回中间件，检查当前用户是否具有指定权限码（任一即可）
func RequirePermission(permissionCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			c.Abort()
			return
		}

		uid, ok := userID.(string)
		if !ok || uid == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户ID无效"})
			c.Abort()
			return
		}

		// admin 用户拥有所有权限
		if uid == "admin" {
			c.Next()
			return
		}

		permRepo := repository.NewRolePermissionRepository(database.DB)
		permissions, err := permRepo.FindByUserRole(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询权限失败"})
			c.Abort()
			return
		}

		permSet := make(map[string]struct{}, len(permissions))
		for _, p := range permissions {
			permSet[p.Code] = struct{}{}
		}

		for _, code := range permissionCodes {
			if _, ok := permSet[code]; ok {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
		c.Abort()
	}
}
