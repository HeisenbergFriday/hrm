package service

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

func orgIDFromDB(db *gorm.DB) string {
	return database.CurrentOrganizationIDFromDB(db)
}
