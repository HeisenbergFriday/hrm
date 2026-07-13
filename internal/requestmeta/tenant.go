package requestmeta

import (
	"context"
	"errors"
	"strings"
)

const tenantOrgIDKey contextKey = "peopleops_tenant_org_id"

// ErrMissingTenant 表示上下文缺少租户组织标识。
var ErrMissingTenant = errors.New("requestmeta: tenant orgID missing in context")

// WithTenant 将租户 orgID 写入 context；空字符串会被视作未设置。
func WithTenant(ctx context.Context, orgID string) context.Context {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantOrgIDKey, orgID)
}

// TenantID 返回当前 context 中的租户 orgID；缺失时返回 ErrMissingTenant，
// 供业务代码在必须租户上下文的位置做 fail-closed 判断。
func TenantID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrMissingTenant
	}
	value := ctx.Value(tenantOrgIDKey)
	if value == nil {
		return "", ErrMissingTenant
	}
	orgID, ok := value.(string)
	if !ok || strings.TrimSpace(orgID) == "" {
		return "", ErrMissingTenant
	}
	return orgID, nil
}
