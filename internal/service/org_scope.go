package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/requestmeta"
	"strings"

	"gorm.io/gorm"
)

// orgIDFromDB is a deprecated compatibility helper that falls back to the
// "default" organization when the DB session has no tenant context.
//
// Deprecated: business request paths, background jobs, sync tasks and report
// jobs MUST call requireOrgIDFromDB / database.RequireOrganizationIDFromDB
// instead. New business code must not call this helper.
func orgIDFromDB(db *gorm.DB) string {
	return database.CurrentOrganizationIDFromDB(db)
}

// requireOrgIDFromDB returns the request organization id and fails closed when
// the DB session has no explicit org context. Unlike orgIDFromDB, it never
// falls back to "default" for an empty context.
func requireOrgIDFromDB(db *gorm.DB) (string, error) {
	if db == nil || db.Statement == nil {
		return "", fmt.Errorf("missing organization context")
	}
	ctx := db.Statement.Context
	if info := requestmeta.FromContext(ctx); info != nil {
		if orgID := strings.TrimSpace(info.OrgID); orgID != "" {
			return database.NormalizeOrganizationID(orgID), nil
		}
		return "", fmt.Errorf("missing organization context")
	}
	if tenantID, err := requestmeta.TenantID(ctx); err == nil {
		if orgID := strings.TrimSpace(tenantID); orgID != "" {
			return database.NormalizeOrganizationID(orgID), nil
		}
	}
	return "", fmt.Errorf("missing organization context")
}

// resolveServiceOrgID prefers an explicit service-bound org, then the DB
// session tenant. Empty context returns an error — never invents "default".
func resolveServiceOrgID(boundOrgID string, db *gorm.DB) (string, error) {
	if orgID := strings.TrimSpace(boundOrgID); orgID != "" {
		return database.NormalizeOrganizationID(orgID), nil
	}
	return requireOrgIDFromDB(db)
}
