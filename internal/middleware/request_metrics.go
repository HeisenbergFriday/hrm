package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"peopleops/internal/database"
	"peopleops/internal/requestmeta"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const requestDBKey = "requestDB"

func RequestMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		info := &requestmeta.RequestInfo{
			RequestID: requestID(c),
			Route:     route,
		}
		ctx := requestmeta.WithRequestInfo(c.Request.Context(), info)
		c.Request = c.Request.WithContext(ctx)
		c.Set("requestID", info.RequestID)
		c.Set(requestDBKey, database.DB.WithContext(ctx))

		start := time.Now()
		c.Next()

		if fullPath := c.FullPath(); fullPath != "" {
			info.Route = fullPath
		}
		log.Printf("[request] status=%d sql_count=%d duration=%s route=%s request_id=%s",
			c.Writer.Status(), info.SQLCount.Load(), time.Since(start), info.Route, info.RequestID)
	}
}

func RequestDB(c *gin.Context) *gorm.DB {
	if c != nil {
		if db, ok := c.Get(requestDBKey); ok {
			if typed, ok := db.(*gorm.DB); ok && typed != nil {
				return typed
			}
		}
	}
	if database.DB == nil {
		return nil
	}
	if c == nil || c.Request == nil {
		return database.DB
	}
	return database.DB.WithContext(c.Request.Context())
}

// RebindRequestContext replaces the request context and refreshes the cached
// request-scoped DB. Long-running server operations can use a context detached
// from the client connection while preserving request metadata and tenant values.
func RebindRequestContext(c *gin.Context, ctx context.Context) {
	if c == nil || c.Request == nil || ctx == nil {
		return
	}
	c.Request = c.Request.WithContext(ctx)
	if database.DB != nil {
		c.Set(requestDBKey, database.DB.WithContext(ctx))
	}
}

func requestID(c *gin.Context) string {
	if existing := c.GetHeader("X-Request-ID"); existing != "" {
		return existing
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b)
}
