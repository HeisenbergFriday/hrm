package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 测试同步服务
func TestSyncService(t *testing.T) {
	// 创建内存数据库连接
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	assert.NoError(t, err)

	// 创建同步服务
	syncService := NewSyncService(db)

	// 测试同步部门数据
	t.Run("SyncDepartments", func(t *testing.T) {
		count, err := syncService.SyncDepartments()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
	})

	// 测试同步用户数据
	t.Run("SyncUsers", func(t *testing.T) {
		count, err := syncService.SyncUsers()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
	})

	// 测试同步考勤数据
	t.Run("SyncAttendance", func(t *testing.T) {
		count, err := syncService.SyncAttendance(time.Now().AddDate(0, 0, -7), time.Now())
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
	})

	// 测试同步审批数据
	t.Run("SyncApproval", func(t *testing.T) {
		count, err := syncService.SyncApproval(time.Now().AddDate(0, 0, -7), time.Now())
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
	})

	// 测试获取同步状态
	t.Run("GetSyncStatus", func(t *testing.T) {
		status, err := syncService.GetSyncStatus()
		assert.NoError(t, err)
		assert.NotNil(t, status)
	})
}
