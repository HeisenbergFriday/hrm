package database

import (
	"context"
	"log"
	"peopleops/internal/requestmeta"
	"strings"
	"time"

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
	info := requestmeta.FromContext(ctx)
	if info == nil {
		l.base.Trace(ctx, begin, fc, err)
		return
	}

	sql, rows := fc()
	count := info.SQLCount.Add(1)
	elapsed := time.Since(begin)
	log.Printf("[sql] request_id=%s table=%s sql_count=%d route=%s elapsed=%s rows=%d err=%v sql=%s",
		info.RequestID, sqlTableName(sql), count, info.Route, elapsed, rows, err, sql)
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
