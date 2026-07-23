package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// External attendance Doris is read-only. Never issue DML/DDL through this pool.

var (
	externalAttendanceMu  sync.Mutex
	externalAttendanceDB  *sql.DB
	externalAttendanceErr error
	// externalAttendanceConnect is injectable for tests (success-only cache / fail-retry).
	externalAttendanceConnect = defaultExternalAttendanceConnect
)

func defaultExternalAttendanceConnect(cfg ExternalAttendanceConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open external attendance db: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping external attendance db: %w", err)
	}
	return db, nil
}

// ExternalAttendanceConfig holds non-secret connection settings.
type ExternalAttendanceConfig struct {
	DSN              string
	Enabled          bool
	SyncInterval     time.Duration
	LookbackMinutes  int
	QueryTimeout     time.Duration
	MaxOpenConns     int
	MaxIdleConns     int
	InitialStartTime *time.Time // optional; nil => full history from Unix epoch
}

// LoadExternalAttendanceConfig reads EXTERNAL_ATTENDANCE_* env vars.
// Password must come from env only; never hardcode or log DSN secrets.
func LoadExternalAttendanceConfig() ExternalAttendanceConfig {
	cfg := ExternalAttendanceConfig{
		Enabled:         parseBoolEnv("EXTERNAL_ATTENDANCE_SYNC_ENABLED", false),
		SyncInterval:    parseDurationEnv("EXTERNAL_ATTENDANCE_SYNC_INTERVAL", 15*time.Minute),
		LookbackMinutes: parseIntEnv("EXTERNAL_ATTENDANCE_SYNC_LOOKBACK_MINUTES", 30),
		QueryTimeout:    parseDurationEnv("EXTERNAL_ATTENDANCE_QUERY_TIMEOUT", 30*time.Second),
		MaxOpenConns:    parseIntEnv("EXTERNAL_ATTENDANCE_MAX_OPEN_CONNS", 5),
		MaxIdleConns:    parseIntEnv("EXTERNAL_ATTENDANCE_MAX_IDLE_CONNS", 2),
	}
	if t := parseTimeEnv("EXTERNAL_ATTENDANCE_INITIAL_START_TIME"); t != nil {
		cfg.InitialStartTime = t
	}

	if dsn := strings.TrimSpace(os.Getenv("EXTERNAL_ATTENDANCE_DATABASE_URL")); dsn != "" {
		cfg.DSN = dsn
		return cfg
	}

	host := strings.TrimSpace(os.Getenv("EXTERNAL_ATTENDANCE_DB_HOST"))
	if host == "" {
		return cfg
	}
	port := strings.TrimSpace(os.Getenv("EXTERNAL_ATTENDANCE_DB_PORT"))
	if port == "" {
		port = "9030"
	}
	user := strings.TrimSpace(os.Getenv("EXTERNAL_ATTENDANCE_DB_USER"))
	if user == "" {
		user = "hr_user"
	}
	password := os.Getenv("EXTERNAL_ATTENDANCE_DB_PASSWORD")
	schema := strings.TrimSpace(os.Getenv("EXTERNAL_ATTENDANCE_DB_SCHEMA"))
	if schema == "" {
		schema = "dwd"
	}
	cfg.DSN = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local&timeout=10s&readTimeout=30s&writeTimeout=10s",
		user, password, host, port, schema)
	return cfg
}

// InitExternalAttendanceDB opens the read-only Doris pool.
// Success is cached; failures are NOT sticky so a temporary Doris outage can recover
// without process restart. Concurrent callers are serialized by mutex.
func InitExternalAttendanceDB() error {
	externalAttendanceMu.Lock()
	defer externalAttendanceMu.Unlock()

	if externalAttendanceDB != nil {
		return nil
	}

	cfg := LoadExternalAttendanceConfig()
	if strings.TrimSpace(cfg.DSN) == "" {
		externalAttendanceErr = fmt.Errorf("external attendance DSN not configured")
		return externalAttendanceErr
	}
	db, err := externalAttendanceConnect(cfg)
	if err != nil {
		// Do not cache failure — next call may succeed after Doris recovers.
		externalAttendanceErr = err
		return externalAttendanceErr
	}
	externalAttendanceDB = db
	externalAttendanceErr = nil
	return nil
}

// GetExternalAttendanceDB returns the shared pool (may be nil if not initialized).
func GetExternalAttendanceDB() *sql.DB {
	externalAttendanceMu.Lock()
	defer externalAttendanceMu.Unlock()
	return externalAttendanceDB
}

// CloseExternalAttendanceDB closes the pool if open and allows re-init.
func CloseExternalAttendanceDB() error {
	externalAttendanceMu.Lock()
	defer externalAttendanceMu.Unlock()
	if externalAttendanceDB == nil {
		return nil
	}
	err := externalAttendanceDB.Close()
	externalAttendanceDB = nil
	externalAttendanceErr = nil
	return err
}

// ExternalAttendanceHealth checks connectivity with SELECT 1.
func ExternalAttendanceHealth(ctx context.Context) (latency time.Duration, err error) {
	if err := InitExternalAttendanceDB(); err != nil {
		return 0, err
	}
	db := GetExternalAttendanceDB()
	if db == nil {
		return 0, fmt.Errorf("external attendance db unavailable")
	}
	start := time.Now()
	if err := db.PingContext(ctx); err != nil {
		return time.Since(start), err
	}
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}

// InitialSyncStartTime returns the start cursor for first-time backfill.
// Prefer EXTERNAL_ATTENDANCE_INITIAL_START_TIME; default Unix epoch (full history).
func InitialSyncStartTime() time.Time {
	cfg := LoadExternalAttendanceConfig()
	if cfg.InitialStartTime != nil {
		return *cfg.InitialStartTime
	}
	return time.Unix(0, 0).UTC()
}

func parseBoolEnv(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func parseIntEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func parseTimeEnv(key string) *time.Time {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return &t
		}
	}
	return nil
}
