package main

import (
	"log"
	"os"
	"peopleops/internal/api"
	"peopleops/internal/cache"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

func main() {
	// 加载配置
	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
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