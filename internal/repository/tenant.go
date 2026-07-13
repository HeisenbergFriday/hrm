package repository

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ErrMissingOrgID 表示需要租户 orgID 的仓储调用没有携带非空值。
var ErrMissingOrgID = errors.New("repository: orgID required for tenant-scoped operation")

// ErrOrgMismatch 表示写入/更新的实体所属组织与仓储绑定组织不一致。
var ErrOrgMismatch = errors.New("repository: entity org_id mismatch with tenant scope")

// RequireOrgID 校验并规范化租户 orgID；用于严格 tenant 仓储构造器与写路径。
func RequireOrgID(orgID string) (string, error) {
	trimmed := strings.TrimSpace(orgID)
	if trimmed == "" {
		return "", ErrMissingOrgID
	}
	return trimmed, nil
}

// ScopeOrg 返回一个 GORM Scope，向指定列附加 org 过滤；qualifiedColumn 支持
// "org_id" 或 "users.org_id"，orgID 为空时直接返回原 tx，供仍需兼容的旧构造使用。
// 新代码请优先使用严格 tenant 仓储：不允许空 org。
func ScopeOrg(orgID, qualifiedColumn string) func(*gorm.DB) *gorm.DB {
	col := strings.TrimSpace(qualifiedColumn)
	if col == "" {
		col = "org_id"
	}
	trimmed := strings.TrimSpace(orgID)
	return func(tx *gorm.DB) *gorm.DB {
		if trimmed == "" {
			return tx
		}
		return tx.Where(fmt.Sprintf("%s = ?", col), trimmed)
	}
}

// EnsureSameOrg 断言 entityOrgID 与租户绑定 orgID 一致；entityOrgID 为空视为
// 继承租户 org（返回 tenantOrgID）；不一致时返回 ErrOrgMismatch。
func EnsureSameOrg(tenantOrgID, entityOrgID string) (string, error) {
	tenant := strings.TrimSpace(tenantOrgID)
	if tenant == "" {
		return "", ErrMissingOrgID
	}
	entity := strings.TrimSpace(entityOrgID)
	if entity == "" {
		return tenant, nil
	}
	if entity != tenant {
		return "", ErrOrgMismatch
	}
	return tenant, nil
}
