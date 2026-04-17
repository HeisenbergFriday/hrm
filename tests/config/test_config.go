package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// TestConfig 测试配置
type TestConfig struct {
	DatabaseURL string
	DingTalkAppKey string
	DingTalkAppSecret string
	JWTSecret string
	Port string
}

// LoadTestConfig 加载测试配置
func LoadTestConfig() (*TestConfig, error) {
	// 加载.env.test文件
	envFile := filepath.Join(".", ".env.test")
	if _, err := os.Stat(envFile); err == nil {
		if err := godotenv.Load(envFile); err != nil {
			return nil, err
		}
	}

	// 加载默认配置
	config := &TestConfig{
		DatabaseURL: getEnv("DATABASE_URL", "root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"),
		DingTalkAppKey: getEnv("DINGTALK_APP_KEY", "test_app_key"),
		DingTalkAppSecret: getEnv("DINGTALK_APP_SECRET", "test_app_secret"),
		JWTSecret: getEnv("JWT_SECRET", "test_jwt_secret"),
		Port: getEnv("PORT", "8080"),
	}

	return config, nil
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
