package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 测试审计日志服务
func TestAuditService(t *testing.T) {
	// 创建内存数据库连接
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	assert.NoError(t, err)

	// 创建审计日志服务
	auditService := NewAuditService(db)

	// 测试记录审计日志
	t.Run("RecordAuditLog", func(t *testing.T) {
		err := auditService.RecordAuditLog("user123", "张三", "登录", "系统", "127.0.0.1", map[string]interface{}{"login_type": "dingtalk_qr", "status": "success"})
		assert.NoError(t, err)
	})

	// 测试获取审计日志
	t.Run("GetAuditLogs", func(t *testing.T) {
		logs, total, err := auditService.GetAuditLogs(1, 10, "", "", "")
		assert.NoError(t, err)
		assert.NotNil(t, logs)
		assert.GreaterOrEqual(t, total, int64(0))
	})
}
