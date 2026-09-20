package database

import (
	"context"
	"errors"
	"log"
	"peopleops/internal/requestmeta"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type requestLogger struct {
	base logger.Interface
}

func newRequestLogger(base logger.Interface) logger.Interface {
	return &requestLogger{base: base}
}

func (l *requestLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &requestLogger{base: l.base.LogMode(level)}
}

func (l *requestLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	l.base.Info(ctx, msg, data...)
}

func (l *requestLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.base.Warn(ctx, msg, data...)
}

func (l *requestLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.base.Error(ctx, msg, data...)
}

func (l *requestLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	isSlow := elapsed >= time.Second
	info := requestmeta.FromContext(ctx)
	if info == nil {
		if (err == nil && !isSlow) || errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		sql, rows := fc()
		if err == nil {
			log.Printf("[sql-slow] table=%s elapsed=%s rows=%d", sqlTableName(sql), elapsed, rows)
			return
		}
		log.Printf("[sql-error] table=%s elapsed=%s rows=%d err=%v",
			sqlTableName(sql), elapsed, rows, err)
		return
	}

	count := info.SQLCount.Add(1)
	if (err == nil && !isSlow) || errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}

	// Keep database failures diagnosable without copying complete SQL
	// statements, which may contain large payloads or sensitive values.
	sql, rows := fc()
	if err == nil {
		log.Printf("[sql-slow] request_id=%s table=%s sql_count=%d route=%s elapsed=%s rows=%d",
			info.RequestID, sqlTableName(sql), count, info.Route, elapsed, rows)
		return
	}
	log.Printf("[sql-error] request_id=%s table=%s sql_count=%d route=%s elapsed=%s rows=%d err=%v",
		info.RequestID, sqlTableName(sql), count, info.Route, elapsed, rows, err)
}

func sqlTableName(sql string) string {
	for _, marker := range []string{"FROM `", "JOIN `", "UPDATE `", "INTO `"} {
		if table := tableNameAfter(sql, marker); table != "" {
			return table
		}
	}
	return "unknown"
}

func tableNameAfter(sql, marker string) string {
	idx := strings.Index(sql, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	end := strings.Index(sql[start:], "`")
	if end < 0 {
		return ""
	}
	return sql[start : start+end]
}
