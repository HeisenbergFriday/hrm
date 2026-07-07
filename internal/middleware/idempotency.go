package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	HeaderIdempotencyKey    = "Idempotency-Key"
	HeaderIdempotencyStatus = "Idempotency-Status"

	idempotencyStatusProcessing = "processing"
	idempotencyStatusCompleted  = "completed"
)

var errIdempotencyBodyTooLarge = errors.New("idempotency request body too large")

var idempotencyCleanupState struct {
	sync.Mutex
	lastRun time.Time
}

func Idempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isIdempotencyMethod(c.Request.Method) {
			c.Next()
			return
		}

		key := strings.TrimSpace(c.GetHeader(HeaderIdempotencyKey))
		if key == "" {
			c.Next()
			return
		}
		if len(key) > 128 {
			c.Header(HeaderIdempotencyStatus, "invalid")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key must be 128 characters or fewer"})
			c.Abort()
			return
		}

		db := RequestDB(c)
		if db == nil {
			c.Header(HeaderIdempotencyStatus, "unavailable")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "idempotency store unavailable"})
			c.Abort()
			return
		}

		body, err := readAndRestoreRequestBody(c.Request, idempotencyMaxRequestBytes())
		if err != nil {
			c.Header(HeaderIdempotencyStatus, "invalid")
			status := http.StatusBadRequest
			message := err.Error()
			if errors.Is(err, errIdempotencyBodyTooLarge) {
				status = http.StatusRequestEntityTooLarge
				message = "request body too large for idempotency tracking"
			}
			c.JSON(status, gin.H{"error": message})
			c.Abort()
			return
		}

		now := time.Now()
		userID := idempotencyUserID(c)
		method := strings.ToUpper(c.Request.Method)
		route := idempotencyRoute(c)
		requestHash := hashRequest(method, route, c.Request.URL.RawQuery, body)
		digest := hashDigest(userID, method, route, key)

		cleanupExpiredIdempotencyRecords(db, now)

		record := &database.IdempotencyRecord{
			Digest:         digest,
			IdempotencyKey: key,
			UserID:         userID,
			Method:         method,
			Path:           route,
			RequestHash:    requestHash,
			Status:         idempotencyStatusProcessing,
			Replayable:     true,
			ExpiresAt:      now.Add(idempotencyTTL()),
		}

		existing, created, err := claimIdempotencyRecord(db, record)
		if err != nil {
			c.Header(HeaderIdempotencyStatus, "unavailable")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reserve idempotency key"})
			c.Abort()
			return
		}
		if !created {
			handleExistingIdempotencyRecord(c, existing, requestHash)
			return
		}

		writer := &idempotencyResponseWriter{
			ResponseWriter: c.Writer,
			maxBytes:       idempotencyMaxResponseBytes(),
		}
		c.Writer = writer
		c.Header(HeaderIdempotencyStatus, "created")
		c.Next()

		if err := completeIdempotencyRecord(db, record.ID, writer); err != nil {
			log.Printf("[idempotency] failed to complete record id=%d digest=%s: %v", record.ID, record.Digest, err)
		}
	}
}

type idempotencyResponseWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	maxBytes int64
	overflow bool
}

func (w *idempotencyResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *idempotencyResponseWriter) WriteString(data string) (int, error) {
	w.capture([]byte(data))
	return w.ResponseWriter.WriteString(data)
}

func (w *idempotencyResponseWriter) capture(data []byte) {
	if w.overflow || w.maxBytes <= 0 {
		w.overflow = true
		return
	}
	if int64(w.body.Len()+len(data)) > w.maxBytes {
		w.overflow = true
		w.body.Reset()
		return
	}
	_, _ = w.body.Write(data)
}

func handleExistingIdempotencyRecord(c *gin.Context, record *database.IdempotencyRecord, requestHash string) {
	if record == nil {
		c.Header(HeaderIdempotencyStatus, "unavailable")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "idempotency record unavailable"})
		c.Abort()
		return
	}
	if record.RequestHash != requestHash {
		c.Header(HeaderIdempotencyStatus, "conflict")
		c.JSON(http.StatusConflict, gin.H{"error": "Idempotency-Key has already been used for a different request"})
		c.Abort()
		return
	}
	if record.Status == idempotencyStatusProcessing {
		c.Header(HeaderIdempotencyStatus, "processing")
		c.JSON(http.StatusConflict, gin.H{"error": "request with this Idempotency-Key is still processing"})
		c.Abort()
		return
	}
	if record.Status != idempotencyStatusCompleted {
		c.Header(HeaderIdempotencyStatus, "conflict")
		c.JSON(http.StatusConflict, gin.H{"error": "request with this Idempotency-Key cannot be replayed"})
		c.Abort()
		return
	}
	if !record.Replayable {
		c.Header(HeaderIdempotencyStatus, "not-replayable")
		c.JSON(http.StatusConflict, gin.H{"error": "request was completed but its response is too large to replay"})
		c.Abort()
		return
	}

	status := record.ResponseStatus
	if status == 0 {
		status = http.StatusOK
	}
	contentType := strings.TrimSpace(record.ContentType)
	if contentType == "" {
		contentType = "application/json; charset=utf-8"
	}
	c.Header(HeaderIdempotencyStatus, "replayed")
	c.Data(status, contentType, record.ResponseBody)
	c.Abort()
}

func completeIdempotencyRecord(db *gorm.DB, id uint, writer *idempotencyResponseWriter) error {
	statusCode := writer.Status()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	replayable := !writer.overflow
	errorMessage := ""
	var responseBody []byte
	if replayable {
		responseBody = append([]byte(nil), writer.body.Bytes()...)
	} else {
		errorMessage = "response body exceeded idempotency replay limit"
	}

	return db.Model(&database.IdempotencyRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          idempotencyStatusCompleted,
			"response_status": statusCode,
			"content_type":    writer.Header().Get("Content-Type"),
			"response_body":   responseBody,
			"replayable":      replayable,
			"error_message":   errorMessage,
		}).Error
}

func claimIdempotencyRecord(db *gorm.DB, record *database.IdempotencyRecord) (*database.IdempotencyRecord, bool, error) {
	if err := db.Create(record).Error; err == nil {
		return record, true, nil
	} else if !isDuplicateKeyError(err) {
		return nil, false, err
	}

	var existing database.IdempotencyRecord
	if err := db.Where("digest = ?", record.Digest).First(&existing).Error; err != nil {
		return nil, false, err
	}
	if !existing.ExpiresAt.IsZero() && time.Now().After(existing.ExpiresAt) {
		if err := db.Delete(&existing).Error; err != nil {
			return nil, false, err
		}
		if err := db.Create(record).Error; err == nil {
			return record, true, nil
		} else if !isDuplicateKeyError(err) {
			return nil, false, err
		}
		if err := db.Where("digest = ?", record.Digest).First(&existing).Error; err != nil {
			return nil, false, err
		}
	}
	return &existing, false, nil
}

func cleanupExpiredIdempotencyRecords(db *gorm.DB, now time.Time) {
	idempotencyCleanupState.Lock()
	defer idempotencyCleanupState.Unlock()

	if !idempotencyCleanupState.lastRun.IsZero() && now.Sub(idempotencyCleanupState.lastRun) < time.Minute {
		return
	}
	idempotencyCleanupState.lastRun = now
	if err := db.Where("expires_at < ?", now).Delete(&database.IdempotencyRecord{}).Error; err != nil {
		log.Printf("[idempotency] failed to cleanup expired records: %v", err)
	}
}

func readAndRestoreRequestBody(req *http.Request, maxBytes int64) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBytes+1))
	if closeErr := req.Body.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errIdempotencyBodyTooLarge
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return body, nil
}

func isIdempotencyMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func idempotencyUserID(c *gin.Context) string {
	if value, ok := c.Get("userID"); ok {
		if userID, ok := value.(string); ok && strings.TrimSpace(userID) != "" {
			return strings.TrimSpace(userID)
		}
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth != "" {
		return "auth:" + hashString(auth)
	}
	return "anonymous:" + c.ClientIP()
}

func idempotencyRoute(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func hashRequest(method, route, rawQuery string, body []byte) string {
	h := sha256.New()
	writeHashPart(h, method)
	writeHashPart(h, route)
	writeHashPart(h, rawQuery)
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func hashDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		writeHashPart(h, part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeHashPart(w io.Writer, value string) {
	_, _ = w.Write([]byte(value))
	_, _ = w.Write([]byte{0})
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "duplicate entry") ||
		strings.Contains(lower, "duplicated key") ||
		strings.Contains(lower, "unique constraint failed")
}

func idempotencyTTL() time.Duration {
	hours := int64FromEnv("IDEMPOTENCY_TTL_HOURS", 24)
	if hours < 1 {
		hours = 1
	}
	return time.Duration(hours) * time.Hour
}

func idempotencyMaxRequestBytes() int64 {
	return int64FromEnv("IDEMPOTENCY_MAX_REQUEST_BYTES", 8*1024*1024)
}

func idempotencyMaxResponseBytes() int64 {
	return int64FromEnv("IDEMPOTENCY_MAX_RESPONSE_BYTES", 4*1024*1024)
}

func int64FromEnv(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
