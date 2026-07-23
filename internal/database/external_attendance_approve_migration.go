package database

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// migrateExternalAttendanceApproveLinksSchema safely upgrades external_attendance_approve_links
// from the legacy unique key (org_id, source_row_key, begin_time) to
// (org_id, source_row_key, item_key) without silent data loss.
//
// Steps:
//  1. Table missing → no-op (AutoMigrate creates the full schema).
//  2. item_key missing → add as nullable.
//  3. Backfill empty item_key using BuildExternalApproveItemKey.
//  4. Audit conflicts on (org_id, source_row_key, item_key); abort with redacted samples.
//  5. Drop legacy uk_ext_appr_link when present.
//  6. Enforce item_key NOT NULL.
//  7. Create uk_ext_appr_item when missing.
func migrateExternalAttendanceApproveLinksSchema() error {
	if DB == nil {
		return nil
	}
	table := ExternalAttendanceApproveLink{}.TableName()
	if !DB.Migrator().HasTable(&ExternalAttendanceApproveLink{}) {
		return nil
	}

	// 2) Ensure column exists (nullable first for safe backfill).
	if !DB.Migrator().HasColumn(&ExternalAttendanceApproveLink{}, "ItemKey") {
		// Prefer raw SQL so we can add as NULL regardless of GORM tag.
		if err := DB.Exec(fmt.Sprintf(
			"ALTER TABLE `%s` ADD COLUMN `item_key` varchar(64) NULL", table,
		)).Error; err != nil {
			// Column may have been added concurrently; re-check.
			if !DB.Migrator().HasColumn(&ExternalAttendanceApproveLink{}, "ItemKey") {
				return fmt.Errorf("add item_key column: %w", err)
			}
		}
	}

	// 3) Backfill empty item_key.
	type legacyRow struct {
		ID           uint       `gorm:"column:id"`
		OrgID        string     `gorm:"column:org_id"`
		SourceRowKey string     `gorm:"column:source_row_key"`
		ItemKey      string     `gorm:"column:item_key"`
		ProcInstID   string     `gorm:"column:proc_inst_id"`
		TagName      string     `gorm:"column:tag_name"`
		BizType      string     `gorm:"column:biz_type"`
		SubType      string     `gorm:"column:sub_type"`
		BeginTime    *time.Time `gorm:"column:begin_time"`
		EndTime      *time.Time `gorm:"column:end_time"`
		Duration     string     `gorm:"column:duration"`
		DurationUnit string     `gorm:"column:duration_unit"`
	}
	var rows []legacyRow
	if err := DB.Table(table).
		Select("id, org_id, source_row_key, item_key, proc_inst_id, tag_name, biz_type, sub_type, begin_time, end_time, duration, duration_unit").
		Where("item_key IS NULL OR item_key = ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("load approve links for item_key backfill: %w", err)
	}
	for _, row := range rows {
		begin := ""
		end := ""
		if row.BeginTime != nil {
			begin = row.BeginTime.UTC().Format(time.RFC3339Nano)
		}
		if row.EndTime != nil {
			end = row.EndTime.UTC().Format(time.RFC3339Nano)
		}
		key := BuildExternalApproveItemKey(
			row.ProcInstID, row.TagName, row.BizType, row.SubType,
			begin, end, row.Duration, row.DurationUnit,
		)
		if key == "" {
			return fmt.Errorf("item_key backfill produced empty key for id=%d org=%s", row.ID, redactOrgID(row.OrgID))
		}
		if err := DB.Table(table).Where("id = ?", row.ID).Update("item_key", key).Error; err != nil {
			return fmt.Errorf("backfill item_key id=%d: %w", row.ID, err)
		}
	}

	// 4) Audit conflicts — never silent-delete business data.
	type conflictGroup struct {
		OrgID        string `gorm:"column:org_id"`
		SourceRowKey string `gorm:"column:source_row_key"`
		ItemKey      string `gorm:"column:item_key"`
		Cnt          int64  `gorm:"column:cnt"`
	}
	var conflicts []conflictGroup
	if err := DB.Table(table).
		Select("org_id, source_row_key, item_key, COUNT(*) AS cnt").
		Group("org_id, source_row_key, item_key").
		Having("COUNT(*) > 1").
		Limit(5).
		Scan(&conflicts).Error; err != nil {
		return fmt.Errorf("audit item_key conflicts: %w", err)
	}
	if len(conflicts) > 0 {
		samples := make([]string, 0, len(conflicts))
		for _, c := range conflicts {
			samples = append(samples, fmt.Sprintf(
				"org=%s source_row_key=%s item_key=%s count=%d",
				redactOrgID(c.OrgID),
				redactToken(c.SourceRowKey, 8),
				redactToken(c.ItemKey, 8),
				c.Cnt,
			))
		}
		return fmt.Errorf(
			"external_attendance_approve_links item_key conflicts detected; refuse silent delete. samples: %s",
			strings.Join(samples, "; "),
		)
	}

	// 6) Enforce NOT NULL (MySQL). On SQLite test dialects this may be a no-op or emulated.
	if err := DB.Exec(fmt.Sprintf(
		"ALTER TABLE `%s` MODIFY COLUMN `item_key` varchar(64) NOT NULL", table,
	)).Error; err != nil {
		// SQLite / partial dialects: tolerate if column already not-null-ish or MODIFY unsupported.
		// Still require no empty values.
		var emptyCount int64
		if countErr := DB.Table(table).Where("item_key IS NULL OR item_key = ''").Count(&emptyCount).Error; countErr != nil {
			return fmt.Errorf("enforce item_key not null: %w (and count empty: %v)", err, countErr)
		}
		if emptyCount > 0 {
			return fmt.Errorf("enforce item_key not null failed (%v) and %d empty keys remain", err, emptyCount)
		}
		log.Printf("[migrate] item_key MODIFY NOT NULL skipped/unsupported: %v", err)
	}

	// Unique-index replacement is centralized in the phase-4 atomic migration.
	return nil
}

func redactOrgID(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return "?"
	}
	return orgID // org_id is not PII; keep for ops diagnosis
}

func redactToken(v string, keep int) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "?"
	}
	if keep <= 0 || len(v) <= keep {
		return v[:minInt(len(v), 4)] + "…"
	}
	return v[:keep] + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
