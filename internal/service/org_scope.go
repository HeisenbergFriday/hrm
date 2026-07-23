package service

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

func orgIDFromDB(db *gorm.DB) string {
	return database.CurrentOrganizationIDFromDB(db)
}

func requireOrgIDFromDB(db *gorm.DB) (string, error) {
	return database.RequireOrganizationIDFromDB(db)
}
