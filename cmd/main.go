package main

import (
	"log"
	"os"
	"peopleops/internal/api"
	"peopleops/internal/cache"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"
)

func main() {
	// 加载配置
	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 校验 JWT_SECRET
	if err := middleware.ValidateJWTSecret(); err != nil {
		log.Fatalf("JWT_SECRET 无效: %v（请设置至少 32 字符的 JWT_SECRET 环境变量）", err)
	}

	// 初始化数据库
	if err := database.Init(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	// 初始化Redis缓存
	if err := cache.Init(); err != nil {
		log.Printf("初始化Redis缓存失败: %v，将继续运行", err)
	}

	// 初始化钉钉客户端
	if err := dingtalk.Init(); err != nil {
		log.Printf("初始化钉钉客户端失败: %v，将继续运行", err)
	}

	// 初始化路由
	router := api.SetupRouter()

	// 启动年假/调休定时任务
	leaveJobs := service.NewLeaveJobScheduler(database.DB)
	if err := leaveJobs.SeedDefaultRules(); err != nil {
		log.Fatalf("初始化年假/调休默认规则失败: %v", err)
	}
	leaveJobs.Start()

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("服务器启动在端口 %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
