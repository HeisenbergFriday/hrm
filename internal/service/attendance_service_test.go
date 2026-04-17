package service

import (
	"testing"
	"time"
	"peopleops/internal/database"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 测试考勤服务
func TestAttendanceService(t *testing.T) {
	// 创建内存数据库连接
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	assert.NoError(t, err)

	// 自动迁移表结构
	err = db.AutoMigrate(&database.Attendance{})
	assert.NoError(t, err)

	// 创建考勤服务
	attendanceService := NewAttendanceService(db)

	// 测试获取考勤记录
	t.Run("GetAttendanceRecords", func(t *testing.T) {
		records, total, err := attendanceService.GetAttendanceRecords(1, 10, "", "", time.Now().AddDate(0, 0, -7), time.Now())
		assert.NoError(t, err)
		assert.NotNil(t, records)
		assert.GreaterOrEqual(t, total, int64(0))
	})

	// 测试获取考勤统计
	t.Run("GetAttendanceStats", func(t *testing.T) {
		stats, err := attendanceService.GetAttendanceStats("", time.Now().AddDate(0, 0, -30), time.Now())
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})

	// 测试同步考勤数据
	t.Run("SyncAttendance", func(t *testing.T) {
		count, err := attendanceService.SyncAttendance(time.Now().AddDate(0, 0, -7), time.Now())
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, int64(0))
	})

	// 测试导出考勤数据
	t.Run("ExportAttendance", func(t *testing.T) {
		filePath, err := attendanceService.ExportAttendance("", "", time.Now().AddDate(0, 0, -30), time.Now(), "test_user", "测试用户")
		assert.NoError(t, err)
		assert.NotEmpty(t, filePath)
	})

	// 测试获取最近同步时间
	t.Run("GetLastSyncTime", func(t *testing.T) {
		time, err := attendanceService.GetLastSyncTime()
		assert.NoError(t, err)
		assert.NotNil(t, time)
	})
}
