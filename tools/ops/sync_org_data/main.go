package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"time"

	"peopleops/internal/api"
	"peopleops/internal/config"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	orgID := flag.String("org", "", "要同步的组织 ID")
	confirm := flag.Bool("confirm-sync", false, "确认执行会写入组织数据的同步")
	diagnoseCounts := flag.Bool("diagnose-counts", false, "只读诊断钉钉多部门归属与本地人数差异")
	listOrgs := flag.Bool("list-orgs", false, "只读列出可诊断组织及本地用户数")
	timeout := flag.Duration("timeout", 15*time.Minute, "同步执行上限")
	flag.Parse()

	normalizedOrgID := database.NormalizeOrganizationID(strings.TrimSpace(*orgID))
	modeCount := 0
	for _, enabled := range []bool{*confirm, *diagnoseCounts, *listOrgs} {
		if enabled {
			modeCount++
		}
	}
	if modeCount != 1 || (!*listOrgs && normalizedOrgID == "") {
		fmt.Fprintln(os.Stderr, "必须在 -confirm-sync、-diagnose-counts、-list-orgs 中选择一个；同步或诊断人数时还需提供 -org")
		os.Exit(2)
	}
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "-timeout 必须大于 0")
		os.Exit(2)
	}

	if err := config.Load(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	database.DB = database.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	if *listOrgs {
		if err := listOrganizations(); err != nil {
			log.Fatalf("列出组织失败: %v", err)
		}
		return
	}
	if err := dingtalk.Init(); err != nil {
		log.Fatalf("初始化钉钉客户端失败: %v", err)
	}
	if *diagnoseCounts {
		if err := diagnoseDepartmentCounts(normalizedOrgID); err != nil {
			log.Fatalf("诊断部门人数失败: %v", err)
		}
		return
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.RequestMetrics())
	router.Use(func(c *gin.Context) {
		c.Set("orgID", normalizedOrgID)
		c.Set("userID", "ops-sync-org-data")
		c.Next()
	})
	router.Use(middleware.TenantContext())
	router.POST("/api/v1/org/sync", api.SyncOrgData)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/sync", bytes.NewBufferString(`{}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)
	fmt.Print(recorder.Body.String())
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusMultiStatus {
		os.Exit(1)
	}
}

type organizationDiagnostic struct {
	OrgID            string `json:"org_id"`
	Name             string `json:"name"`
	LocalUsers       int64  `json:"local_users"`
	LocalActiveUsers int64  `json:"local_active_users"`
}

func listOrganizations() error {
	var organizations []database.Organization
	if err := database.DB.Order("id ASC").Find(&organizations).Error; err != nil {
		return err
	}
	items := make([]organizationDiagnostic, 0, len(organizations))
	for _, organization := range organizations {
		var total int64
		var active int64
		if err := database.DB.Model(&database.User{}).Where("org_id = ? AND deleted_at IS NULL", organization.OrgID).Count(&total).Error; err != nil {
			return err
		}
		if err := database.DB.Model(&database.User{}).Where("org_id = ? AND deleted_at IS NULL AND status = ?", organization.OrgID, "active").Count(&active).Error; err != nil {
			return err
		}
		items = append(items, organizationDiagnostic{
			OrgID:            organization.OrgID,
			Name:             organization.Name,
			LocalUsers:       total,
			LocalActiveUsers: active,
		})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}

type departmentCountDifference struct {
	DepartmentName     string `json:"department_name"`
	DingTalkMembers    int    `json:"dingtalk_members"`
	DingTalkPrimary    int    `json:"dingtalk_primary_members"`
	LocalActivePrimary int    `json:"local_active_primary_members"`
}

type departmentCountDiagnostic struct {
	SourceUniqueUsers          int                         `json:"source_unique_users"`
	SourceActiveUsers          int                         `json:"source_active_users"`
	SourceMemberships          int                         `json:"source_department_memberships"`
	SourceMultiDepartmentUsers int                         `json:"source_multi_department_users"`
	LocalUsers                 int                         `json:"local_users"`
	LocalActiveUsers           int                         `json:"local_active_users"`
	LocalActiveNotInSource     int                         `json:"local_active_not_in_source"`
	LargestDifferences         []departmentCountDifference `json:"largest_department_differences"`
}

func diagnoseDepartmentCounts(orgID string) error {
	departments, err := dingtalk.SyncDepartmentsForOrg(orgID)
	if err != nil {
		return err
	}
	users, err := dingtalk.SyncUsersWithDeptsForOrg(orgID, departments)
	if err != nil {
		return err
	}

	departmentNames := make(map[string]string, len(departments))
	for _, department := range departments {
		departmentNames[fmt.Sprintf("%d", department.DeptID)] = department.Name
	}
	sourceUsers := make(map[string]struct{}, len(users))
	sourceMembers := make(map[string]int)
	sourcePrimary := make(map[string]int)
	diagnostic := departmentCountDiagnostic{SourceUniqueUsers: len(users)}
	for _, user := range users {
		sourceUsers[strings.TrimSpace(user.UserID)] = struct{}{}
		if user.Active {
			diagnostic.SourceActiveUsers++
		}
		seenDepartments := make(map[int64]struct{}, len(user.DeptIDList))
		for index, departmentID := range user.DeptIDList {
			if _, seen := seenDepartments[departmentID]; seen {
				continue
			}
			seenDepartments[departmentID] = struct{}{}
			key := fmt.Sprintf("%d", departmentID)
			sourceMembers[key]++
			diagnostic.SourceMemberships++
			if index == 0 {
				sourcePrimary[key]++
			}
		}
		if len(seenDepartments) > 1 {
			diagnostic.SourceMultiDepartmentUsers++
		}
	}

	var localDepartments []database.Department
	if err := database.DB.Where("org_id = ? AND deleted_at IS NULL", orgID).Find(&localDepartments).Error; err != nil {
		return err
	}
	localByExternalID := make(map[string]string, len(localDepartments))
	localNameByID := make(map[string]string, len(localDepartments))
	for _, department := range localDepartments {
		externalID := strings.TrimSpace(department.DingTalkDepartmentID)
		if externalID == "" {
			externalID = strings.TrimSpace(department.DepartmentID)
		}
		localByExternalID[externalID] = department.DepartmentID
		localNameByID[department.DepartmentID] = department.Name
	}

	var localUsers []database.User
	if err := database.DB.Where("org_id = ? AND deleted_at IS NULL", orgID).Find(&localUsers).Error; err != nil {
		return err
	}
	diagnostic.LocalUsers = len(localUsers)
	localActivePrimary := make(map[string]int)
	for _, user := range localUsers {
		if !strings.EqualFold(strings.TrimSpace(user.Status), "active") {
			continue
		}
		diagnostic.LocalActiveUsers++
		localActivePrimary[user.DepartmentID]++
		externalUserID := strings.TrimSpace(user.DingTalkUserID)
		if externalUserID == "" {
			externalUserID = strings.TrimSpace(user.UserID)
		}
		if _, ok := sourceUsers[externalUserID]; !ok {
			diagnostic.LocalActiveNotInSource++
		}
	}

	for externalID, sourceCount := range sourceMembers {
		localDepartmentID := localByExternalID[externalID]
		name := departmentNames[externalID]
		if name == "" {
			name = localNameByID[localDepartmentID]
		}
		localCount := localActivePrimary[localDepartmentID]
		if sourceCount == localCount && sourceCount == sourcePrimary[externalID] {
			continue
		}
		diagnostic.LargestDifferences = append(diagnostic.LargestDifferences, departmentCountDifference{
			DepartmentName:     name,
			DingTalkMembers:    sourceCount,
			DingTalkPrimary:    sourcePrimary[externalID],
			LocalActivePrimary: localCount,
		})
	}
	sort.SliceStable(diagnostic.LargestDifferences, func(i, j int) bool {
		left := diagnostic.LargestDifferences[i]
		right := diagnostic.LargestDifferences[j]
		leftDifference := left.DingTalkMembers - left.LocalActivePrimary
		if leftDifference < 0 {
			leftDifference = -leftDifference
		}
		rightDifference := right.DingTalkMembers - right.LocalActivePrimary
		if rightDifference < 0 {
			rightDifference = -rightDifference
		}
		return leftDifference > rightDifference
	})
	if len(diagnostic.LargestDifferences) > 20 {
		diagnostic.LargestDifferences = diagnostic.LargestDifferences[:20]
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(diagnostic)
}
