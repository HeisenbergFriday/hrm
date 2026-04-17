package database

import (
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init() error {
	dsn := os.Getenv("DATABASE_URL")

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return err
	}

	DB = db

	// 自动迁移表结构
	if err := migrate(); err != nil {
		return err
	}

	// 种子数据
	seed()

	return nil
}

func migrate() error {
	return DB.AutoMigrate(
		&User{},
		&Department{},
		&Attendance{},
		&Approval{},
		&ApprovalTemplate{},
		&Role{},
		&Permission{},
		&RolePermission{},
		&UserRole{},
		&OperationLog{},
		&SyncStatus{},
		&DingTalkBinding{},
		&UserSession{},
		&LoginLog{},
		&AttendanceExport{},
		&EmployeeProfile{},
		&EmployeeTransfer{},
		&EmployeeResignation{},
		&EmployeeOnboarding{},
		&TalentAnalysis{},
	)
}

// HashPassword 生成密码哈希
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 校验密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func seed() {
	// 创建默认管理员（如果不存在）
	var count int64
	DB.Model(&User{}).Where("user_id = ?", "admin").Count(&count)
	if count == 0 {
		hash, err := HashPassword("admin123")
		if err != nil {
			log.Printf("生成密码哈希失败: %v", err)
			return
		}
		admin := User{
			UserID:       "admin",
			Name:         "管理员",
			Email:        "admin@peopleops.local",
			Mobile:       "10000000000",
			Password:     hash,
			DepartmentID: "1",
			Position:     "系统管理员",
			Status:       "active",
		}
		if err := DB.Create(&admin).Error; err != nil {
			log.Printf("创建默认管理员失败: %v", err)
		} else {
			log.Println("已创建默认管理员账号: admin / admin123")
		}
	}

	// 创建默认部门（如果不存在）
	DB.Model(&Department{}).Count(&count)
	if count == 0 {
		departments := []Department{
			{DepartmentID: "1", Name: "总公司", ParentID: "0", Order: 1},
			{DepartmentID: "2", Name: "技术部", ParentID: "1", Order: 1},
			{DepartmentID: "3", Name: "前端组", ParentID: "2", Order: 1},
			{DepartmentID: "4", Name: "后端组", ParentID: "2", Order: 2},
			{DepartmentID: "5", Name: "市场部", ParentID: "1", Order: 2},
		}
		for _, dept := range departments {
			if err := DB.Create(&dept).Error; err != nil {
				log.Printf("创建默认部门失败: %v", err)
			}
		}
		log.Println("已创建默认部门数据")
	}

	// 创建默认角色（如果不存在）
	DB.Model(&Role{}).Count(&count)
	if count == 0 {
		roles := []Role{
			{Name: "管理员", Description: "系统管理员"},
			{Name: "部门负责人", Description: "部门负责人"},
			{Name: "普通员工", Description: "普通员工"},
		}
		for _, role := range roles {
			DB.Create(&role)
		}
		log.Println("已创建默认角色数据")
	}

	// 创建默认权限（如果不存在）
	DB.Model(&Permission{}).Count(&count)
	if count == 0 {
		permissions := []Permission{
			{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
			{Name: "部门管理", Code: "department_manage", Description: "部门管理权限"},
			{Name: "考勤管理", Code: "attendance_manage", Description: "考勤管理权限"},
			{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
			{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
		}
		for _, perm := range permissions {
			DB.Create(&perm)
		}
		log.Println("已创建默认权限数据")
	}
}
