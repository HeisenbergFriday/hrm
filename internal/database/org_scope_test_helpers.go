package database

import "gorm.io/gorm"

// RegisterOrganizationCallbacksForTest exposes org-scope GORM callbacks for
// integration-style tests that open a raw *gorm.DB without going through Init().
func RegisterOrganizationCallbacksForTest(db *gorm.DB) {
	registerOrganizationCallbacks(db)
}
