package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const minJWTSecretLength = 32

type Claims struct {
	UserID   string `json:"user_id"`
	UserDBID string `json:"user_db_id,omitempty"`
	UserName string `json:"user_name"`
	OrgID    string `json:"org_id"` // 组织ID（多租户）
	jwt.RegisteredClaims
}

func JWTSecret() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if len(secret) < minJWTSecretLength {
		return nil, fmt.Errorf("JWT_SECRET must be at least %d characters", minJWTSecretLength)
	}
	return []byte(secret), nil
}

func ValidateJWTSecret() error {
	_, err := JWTSecret()
	return err
}

func JWTAuth() gin.HandlerFunc {
	return jwtAuth(false)
}

func JWTAuthWithQuery() gin.HandlerFunc {
	return jwtAuth(true)
}

func jwtAuth(allowQueryToken bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := bearerToken(c.GetHeader("Authorization"))
		if tokenString == "" && allowQueryToken {
			tokenString = strings.TrimSpace(c.Query("access_token"))
		}
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing auth token"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected JWT signing method: %s", token.Method.Alg())
			}
			return JWTSecret()
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth token"})
			c.Abort()
			return
		}

		// 多租户：JWT 必须携带 org_id。老 token（迁移前颁发）会被拒绝，前端应据此 code 引导重新登录。
		orgID := strings.TrimSpace(claims.OrgID)
		if orgID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "token missing org_id, please re-login",
				"code":  "token_missing_org_id",
			})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("userDBID", claims.UserDBID)
		c.Set("userName", claims.UserName)
		c.Set("orgID", orgID)
		c.Next()
	}
}

func bearerToken(authHeader string) string {
	parts := strings.SplitN(strings.TrimSpace(authHeader), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
