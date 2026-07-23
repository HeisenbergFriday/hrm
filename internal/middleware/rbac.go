package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RequirePermission(permissionCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetString("userID"))
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		ok, err := HasAnyPermission(c, permissionCodes...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "permission lookup failed"})
			c.Abort()
			return
		}
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAllPermissions requires every listed permission code (AND).
func RequireAllPermissions(permissionCodes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetString("userID"))
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}
		authCtx, err := GetAuthContext(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "permission lookup failed"})
			c.Abort()
			return
		}
		for _, code := range permissionCodes {
			code = strings.TrimSpace(code)
			if code == "" {
				continue
			}
			if _, ok := authCtx.PermissionSet[code]; !ok {
				// Compat: attendance_manage still authorizes toolbox actions.
				if strings.HasPrefix(code, "attendance_toolbox_") {
					if _, ok2 := authCtx.PermissionSet["attendance_manage"]; ok2 {
						continue
					}
				}
				c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func RequirePermissionOrMenu(permissionCodes []string, menuKeys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetString("userID"))
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		if len(permissionCodes) > 0 {
			ok, err := HasAnyPermission(c, permissionCodes...)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "permission lookup failed"})
				c.Abort()
				return
			}
			if ok {
				c.Next()
				return
			}
		}

		for _, menuKey := range menuKeys {
			ok, err := HasMenuPermission(c, menuKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "menu permission lookup failed"})
				c.Abort()
				return
			}
			if ok {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		c.Abort()
	}
}

func RequireMenuPermission(menuKeys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetString("userID"))
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		for _, menuKey := range menuKeys {
			ok, err := HasMenuPermission(c, menuKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "menu permission lookup failed"})
				c.Abort()
				return
			}
			if ok {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "menu permission denied"})
		c.Abort()
	}
}
