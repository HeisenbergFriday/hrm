package config

import (
	"github.com/joho/godotenv"
)

// LoadEnv 加载环境变量
func LoadEnv() error {
	return godotenv.Load()
}

// Config 应用配置
type Config struct {
	DefaultCheckIn  string `json:"default_check_in"`  // 默认上班时间
	DefaultCheckOut string `json:"default_check_out"` // 默认下班时间
	HolidaysFile    string `json:"holidays_file"`    // 节假日配置文件路径
}

// LoadConfig 加载配置
func LoadConfig() (*Config, error) {
	// 默认配置
	config := &Config{
		DefaultCheckIn:  "09:00",
		DefaultCheckOut: "18:30",
		HolidaysFile:    "internal/config/holidays.json",
	}

	// 尝试从环境变量或配置文件加载
	// 这里可以扩展为从配置文件加载

	return config, nil
}

// GetDefaultCheckIn 获取默认上班时间
func GetDefaultCheckIn() string {
	config, err := LoadConfig()
	if err != nil {
		return "09:00"
	}
	return config.DefaultCheckIn
}

// GetDefaultCheckOut 获取默认下班时间
func GetDefaultCheckOut() string {
	config, err := LoadConfig()
	if err != nil {
		return "18:30"
	}
	return config.DefaultCheckOut
}

// Load 加载配置（保持向后兼容）
func Load() error {
	// 加载环境变量
	if err := LoadEnv(); err != nil {
		// 环境变量加载失败不影响程序运行
	}
	_, err := LoadConfig()
	return err
}
