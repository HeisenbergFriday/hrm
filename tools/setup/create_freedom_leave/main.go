//go:build ignore

package main

import (
	"bufio"
	"log"
	"os"
	"strings"
	"peopleops/internal/dingtalk"
)

func loadEnvFile() {
	file, err := os.Open(".env")
	if err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading .env file: %v", err)
	}
}

func main() {
	// 加载.env文件
	loadEnvFile()

	// 检查环境变量
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		log.Fatal("DINGTALK_ADMIN_USER_ID environment variable is not set")
	}

	appKey := os.Getenv("DINGTALK_APP_KEY")
	if appKey == "" {
		log.Fatal("DINGTALK_APP_KEY environment variable is not set")
	}

	appSecret := os.Getenv("DINGTALK_APP_SECRET")
	if appSecret == "" {
		log.Fatal("DINGTALK_APP_SECRET environment variable is not set")
	}

	// 创建自由假期类型
	leaveName := "自由调休"
	leaveCode, err := dingtalk.CreateCustomLeaveType(opUserID, leaveName, true)
	if err != nil {
		log.Fatalf("Failed to create freedom leave type: %v", err)
	}

	log.Printf("Successfully created freedom leave type: %s (leaveCode: %s)", leaveName, leaveCode)
}
