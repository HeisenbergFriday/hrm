package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const minJWTSecretLength = 32

const (
	AuthCookieName        = "peopleops_auth"
	CSRFCookieName        = "peopleops_csrf"
	HeaderCSRFToken       = "X-CSRF-Token"
	defaultSessionVersion = "cookie-v1"
)

type Claims struct {
	OrgID          string `json:"org_id"` // 组织ID（多租户）
	UserID         string `json:"user_id"`
	UserDBID       string `json:"user_db_id,omitempty"`
	UserName       string `json:"user_name"`
	SessionID      string `json:"session_id,omitempty"`
	SessionVersion string `json:"session_version,omitempty"`
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

func SessionVersion() string {
	version := strings.TrimSpace(os.Getenv("AUTH_SESSION_VERSION"))
	if version == "" {
		return defaultSessionVersion
	}
	return version
}

func JWTAuth() gin.HandlerFunc {
	return jwtAuth()
}

func jwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, fromCookie := requestToken(c)
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
		if strings.TrimSpace(claims.SessionID) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}
		if strings.TrimSpace(claims.SessionVersion) != SessionVersion() {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}
		if fromCookie && requiresCSRFCheck(c.Request.Method) && !validCSRFToken(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
			c.Abort()
			return
		}
		// 多租户：JWT 必须携带 org_id。老 token（迁移前颁发）会被拒绝，前端应据此 code 引导重新登录。
		if strings.TrimSpace(claims.OrgID) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid session org: token missing org_id, please re-login",
				"code":  "token_missing_org_id",
			})
			c.Abort()
			return
		}

		orgID := database.NormalizeOrganizationID(claims.OrgID)
		c.Set("orgID", orgID)
		requestmeta.SetOrgID(c.Request.Context(), orgID)

		user, err := activeUserForClaims(c, claims)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "inactive or invalid user"})
			c.Abort()
			return
		}
		if err := validateActiveSession(c, user.UserID, claims.SessionID); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}

		orgID = database.NormalizeOrganizationID(user.OrgID)
		c.Set("orgID", orgID)
		requestmeta.SetOrgID(c.Request.Context(), orgID)
		c.Set("userID", user.UserID)
		c.Set("userDBID", fmt.Sprintf("%d", user.ID))
		c.Set("userName", user.Name)
		c.Set("sessionID", claims.SessionID)
		c.Set("orgID", orgID)
		c.Next()
	}
}

func requestToken(c *gin.Context) (string, bool) {
	if token := bearerToken(c.GetHeader("Authorization")); token != "" {
		return token, false
	}
	token, err := c.Cookie(AuthCookieName)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(token), true
}

func requiresCSRFCheck(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func validCSRFToken(c *gin.Context) bool {
	headerToken := strings.TrimSpace(c.GetHeader(HeaderCSRFToken))
	cookieToken, err := c.Cookie(CSRFCookieName)
	cookieToken = strings.TrimSpace(cookieToken)
	if err != nil || headerToken == "" || cookieToken == "" {
		return false
	}
	if len(headerToken) != len(cookieToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) == 1
}

func bearerToken(authHeader string) string {
	parts := strings.SplitN(strings.TrimSpace(authHeader), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func activeUserForClaims(c *gin.Context, claims *Claims) (*database.User, error) {
	if claims == nil {
		return nil, gorm.ErrRecordNotFound
	}

	db := RequestDB(c)
	userID := strings.TrimSpace(claims.UserID)
	userDBID := strings.TrimSpace(claims.UserDBID)
	orgID := database.NormalizeOrganizationID(claims.OrgID)

	if userID != "" {
		var user database.User
		err := db.Where("org_id = ? AND user_id = ? AND status = ? AND deleted_at IS NULL", orgID, userID, "active").First(&user).Error
		if err == nil {
			return &user, nil
		}
		if !errorsIsRecordNotFound(err) {
			return nil, err
		}
	}

	if userDBID != "" {
		var user database.User
		err := db.Where("org_id = ? AND id = ? AND status = ? AND deleted_at IS NULL", orgID, userDBID, "active").First(&user).Error
		if err == nil {
			return &user, nil
		}
		if !errorsIsRecordNotFound(err) {
			return nil, err
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func validateActiveSession(c *gin.Context, userID, sessionID string) error {
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	orgID := database.NormalizeOrganizationID(c.GetString("orgID"))
	if userID == "" || sessionID == "" {
		return gorm.ErrRecordNotFound
	}

	var session database.UserSession
	return RequestDB(c).
		Where("org_id = ? AND user_id = ? AND session_id = ? AND revoked_at IS NULL AND expires_at > ?", orgID, userID, sessionID, time.Now()).
		First(&session).Error
}

func errorsIsRecordNotFound(err error) bool {
	return err == nil || err == gorm.ErrRecordNotFound
}
