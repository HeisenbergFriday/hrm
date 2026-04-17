package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 测试任务中心服务
func TestJobService(t *testing.T) {
	// 创建内存数据库连接
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	assert.NoError(t, err)

	// 创建任务中心服务
	jobService := NewJobService(db)

	// 测试获取任务列表
	t.Run("GetJobs", func(t *testing.T) {
		jobs, err := jobService.GetJobs()
		assert.NoError(t, err)
		assert.NotNil(t, jobs)
	})

	// 测试运行任务
	t.Run("RunJob", func(t *testing.T) {
		job, err := jobService.RunJob("1")
		assert.NoError(t, err)
		assert.NotNil(t, job)
		assert.Equal(t, "running", job["status"])
	})

	// 测试获取任务运行日志
	t.Run("GetJobRunLogs", func(t *testing.T) {
		logs, total, err := jobService.GetJobRunLogs(1, 10, "1")
		assert.NoError(t, err)
		assert.NotNil(t, logs)
		assert.GreaterOrEqual(t, total, int64(0))
	})
}
