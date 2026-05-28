package middleware

import (
	"net/http"
	"peopleops/internal/database"
	"peopleops/internal/service"
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

		permService := service.NewPermissionService(database.DB)
		ok, err := permService.HasAnyPermission(userID, permissionCodes...)
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

func RequirePermissionOrMenu(permissionCodes []string, menuKeys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := strings.TrimSpace(c.GetString("userID"))
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			c.Abort()
			return
		}

		permService := service.NewPermissionService(database.DB)
		if len(permissionCodes) > 0 {
			ok, err := permService.HasAnyPermission(userID, permissionCodes...)
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
			ok, err := permService.HasMenuPermission(userID, menuKey)
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

		permService := service.NewPermissionService(database.DB)
		for _, menuKey := range menuKeys {
			ok, err := permService.HasMenuPermission(userID, menuKey)
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
