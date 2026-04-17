package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

func Load() error {
	// 加载.env文件
	// 尝试从当前目录和上级目录加载
	if err := godotenv.Load(); err != nil {
		// 尝试从上级目录加载
		if err := godotenv.Load(filepath.Join("..", ".env")); err != nil {
			// 尝试从上级的上级目录加载
			if err := godotenv.Load(filepath.Join("..", "..", ".env")); err != nil {
				// .env文件不存在时不报错
			}
		}
	}

	// 初始化默认值
	setDefaults()

	return nil
}

func setDefaults() {
	if os.Getenv("PORT") == "" {
		os.Setenv("PORT", "8080")
	}

	if os.Getenv("DATABASE_URL") == "" {
		os.Setenv("DATABASE_URL", "root:password@tcp(localhost:3306)/peopleops?charset=utf8mb4&parseTime=True&loc=Local")
	}

	if os.Getenv("DINGTALK_APP_KEY") == "" {
		os.Setenv("DINGTALK_APP_KEY", "your_app_key")
	}

	if os.Getenv("DINGTALK_APP_SECRET") == "" {
		os.Setenv("DINGTALK_APP_SECRET", "your_app_secret")
	}

	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "your_jwt_secret")
	}
}