//go:build ignore

package main

import (
	"fmt"
	"os"
	"peopleops/internal/dingtalk"
)

func main() {
	// 从环境变量获取管理员用户ID
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		fmt.Println("DINGTALK_ADMIN_USER_ID 未配置")
		return
	}

	// 创建一个名为"手动发放年假"的假期类型，设置为手动发放
	leaveName := "手动发放年假"
	hoursPerDay := 8.0

	fmt.Printf("正在创建假期类型: %s (手动发放)\n", leaveName)
	leaveCode, err := dingtalk.CreateVacationTypeWithManualGrant(opUserID, leaveName, hoursPerDay)
	if err != nil {
		fmt.Printf("创建失败: %v\n", err)
		return
	}

	fmt.Printf("创建成功！假期类型编码: %s\n", leaveCode)
	fmt.Println("请在钉钉后台检查新创建的假期类型")
}
