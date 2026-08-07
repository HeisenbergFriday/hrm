package repository

import "gorm.io/gorm"

// colCollate returns a case-insensitive utf8mb4 collation fragment when the
// underlying database is MySQL/MariaDB. For other dialects
// (e.g. SQLite used in unit tests) it returns an empty string so the SQL stays
// portable.  The returned fragment must be placed immediately after the column
// name in the SQL string, e.g.:
//
//	"users.name" + colCollate(db) + " LIKE ?"
//
// becomes "users.name COLLATE utf8mb4_unicode_ci LIKE ?" on MySQL and
// "users.name LIKE ?" on SQLite.
func colCollate(db *gorm.DB) string {
	if db != nil && db.Dialector != nil && db.Name() == "mysql" {
		return " COLLATE utf8mb4_unicode_ci"
	}
	return ""
}
