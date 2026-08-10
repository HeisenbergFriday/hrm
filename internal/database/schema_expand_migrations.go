package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MigrateAnnualLeaveConsumeLogSchema expands annual_leave_consume_logs for multi-tenant
// request_ref without touching unique-index DDL (phase-4 matrix owns unique replacement).
// Idempotent: re-running does not re-add the column.
func MigrateAnnualLeaveConsumeLogSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	ok, err := tableExistsDB(db, "annual_leave_consume_logs")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	hasCol, err := columnExistsDB(db, "annual_leave_consume_logs", "request_ref")
	if err != nil {
		return err
	}
	if !hasCol {
		if err := db.Exec("ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `request_ref` varchar(160)").Error; err != nil {
			return err
		}
	}
	for _, column := range []struct {
		name string
		sql  string
	}{
		{"operation_no", "ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `operation_no` int NOT NULL DEFAULT 1"},
		{"entry_type", "ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `entry_type` varchar(32) NOT NULL DEFAULT 'consume'"},
		{"business_start_date", "ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `business_start_date` varchar(32)"},
		{"business_end_date", "ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `business_end_date` varchar(32)"},
		{"reversal_of_id", "ALTER TABLE `annual_leave_consume_logs` ADD COLUMN `reversal_of_id` bigint unsigned NOT NULL DEFAULT 0"},
	} {
		exists, err := columnExistsDB(db, "annual_leave_consume_logs", column.name)
		if err != nil {
			return err
		}
		if !exists {
			if err := db.Exec(column.sql).Error; err != nil {
				return err
			}
		}
	}

	// Set-based backfill only; never drop/merge legacy unique indexes here.
	return db.Exec(`
		UPDATE annual_leave_consume_logs
		SET request_ref = CASE
			WHEN approval_ref IS NULL OR approval_ref = '' THEN CONCAT('legacy:', id)
			ELSE CONCAT('approval:', approval_ref)
		END
		WHERE request_ref IS NULL OR request_ref = ''
	`).Error
}

// MigrateAnnualLeaveConsumeRequests backfills the request gate from existing
// positive consumption logs. INSERT IGNORE makes the migration repeatable and
// preserves one gate per organization and approval request.
func MigrateAnnualLeaveConsumeRequests(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	requestsExist, err := tableExistsDB(db, "annual_leave_consume_requests")
	if err != nil || !requestsExist {
		return err
	}
	logsExist, err := tableExistsDB(db, "annual_leave_consume_logs")
	if err != nil || !logsExist {
		return err
	}
	return db.Exec(`
		INSERT IGNORE INTO annual_leave_consume_requests (
			org_id, request_ref, approval_ref, user_id, status, operation_no, days,
			business_start_date, business_end_date, created_at, updated_at
		)
		SELECT
			org_id,
			CASE
				WHEN approval_ref IS NULL OR approval_ref = '' THEN request_ref
				WHEN approval_ref LIKE 'approval:%' THEN approval_ref
				ELSE CONCAT('approval:', approval_ref)
			END,
			COALESCE(NULLIF(approval_ref, ''), request_ref),
			MAX(user_id),
			'applied',
			1,
			SUM(CASE WHEN entry_type = 'reversal' THEN -days ELSE days END),
			MIN(business_start_date),
			MAX(business_end_date),
			MIN(created_at),
			MAX(updated_at)
		FROM annual_leave_consume_logs
		GROUP BY org_id, CASE
			WHEN approval_ref IS NULL OR approval_ref = '' THEN request_ref
			WHEN approval_ref LIKE 'approval:%' THEN approval_ref
			ELSE CONCAT('approval:', approval_ref)
		END, COALESCE(NULLIF(approval_ref, ''), request_ref)
		HAVING SUM(CASE WHEN entry_type = 'reversal' THEN -days ELSE days END) > 0
	`).Error
}

// MigrateShiftCatalogSchema expands dingtalk_shift_catalogs.shift_key and backfills
// empty keys with set-based DML. Unique-index DDL stays in phase-4; existing non-empty
// shift_key values are never overwritten.
func MigrateShiftCatalogSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	ok, err := tableExistsDB(db, "dingtalk_shift_catalogs")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	hasCol, err := columnExistsDB(db, "dingtalk_shift_catalogs", "shift_key")
	if err != nil {
		return err
	}
	if !hasCol {
		if err := db.Exec("ALTER TABLE `dingtalk_shift_catalogs` ADD COLUMN `shift_key` varchar(255)").Error; err != nil {
			return err
		}
	}
	// Only fill blank keys; leave unique indexes to MigrateOrgCompositeUniqueIndexes.
	return db.Exec(`
		UPDATE dingtalk_shift_catalogs
		SET shift_key = CONCAT(LOWER(TRIM(name)), '|', TRIM(IFNULL(check_in, '')), '|', TRIM(IFNULL(check_out, '')))
		WHERE (shift_key IS NULL OR shift_key = '')
		  AND name IS NOT NULL
		  AND TRIM(name) <> ''
	`).Error
}

// MigratePerformanceParticipantOrgIDsFromActivity copies activity.org_id onto
// participants (including soft-deleted) and related tables in one transaction.
// Idempotent when already aligned.
func MigratePerformanceParticipantOrgIDsFromActivity(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	ok, err := tableExistsDB(db, "performance_participants")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	okAct, err := tableExistsDB(db, "performance_activities")
	if err != nil {
		return err
	}
	if !okAct {
		return nil
	}

	// Need work when either participants OR related tables are misaligned.
	// Early-exit only on participants would leave logs/versions unrepaired.
	var participantMisaligned int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM performance_participants p
		INNER JOIN performance_activities a
			ON CAST(a.id AS BINARY) = CAST(p.activity_id AS BINARY)
		WHERE a.org_id IS NOT NULL AND a.org_id <> ''
		  AND (p.org_id IS NULL OR p.org_id = '' OR p.org_id <> a.org_id)
	`).Scan(&participantMisaligned).Error; err != nil {
		return err
	}

	relatedMisaligned := int64(0)
	if okLogs, err := tableExistsDB(db, "performance_relationship_change_logs"); err != nil {
		return err
	} else if okLogs {
		var n int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM performance_relationship_change_logs l
			INNER JOIN performance_participants p ON p.id = l.participant_id
			WHERE p.org_id IS NOT NULL AND p.org_id <> ''
			  AND (l.org_id IS NULL OR l.org_id = '' OR l.org_id <> p.org_id)
		`).Scan(&n).Error; err != nil {
			return err
		}
		relatedMisaligned += n
	}
	if okVers, err := tableExistsDB(db, "performance_review_versions"); err != nil {
		return err
	} else if okVers {
		var n int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM performance_review_versions v
			INNER JOIN performance_participants p ON p.id = v.participant_id
			WHERE p.org_id IS NOT NULL AND p.org_id <> ''
			  AND (v.org_id IS NULL OR v.org_id = '' OR v.org_id <> p.org_id)
		`).Scan(&n).Error; err != nil {
			return err
		}
		relatedMisaligned += n
	}
	if participantMisaligned == 0 && relatedMisaligned == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Soft-deleted participants must be repaired too (no p.deleted_at filter).
		if err := tx.Exec(`
			UPDATE performance_participants p
			INNER JOIN performance_activities a
				ON CAST(a.id AS BINARY) = CAST(p.activity_id AS BINARY)
			SET p.org_id = a.org_id
			WHERE a.org_id IS NOT NULL AND a.org_id <> ''
			  AND (p.org_id IS NULL OR p.org_id = '' OR p.org_id <> a.org_id)
		`).Error; err != nil {
			return err
		}

		if okLogs, err := tableExistsDB(tx, "performance_relationship_change_logs"); err != nil {
			return err
		} else if okLogs {
			if err := tx.Exec(`
				UPDATE performance_relationship_change_logs l
				INNER JOIN performance_participants p ON p.id = l.participant_id
				SET l.org_id = p.org_id
				WHERE p.org_id IS NOT NULL AND p.org_id <> ''
				  AND (l.org_id IS NULL OR l.org_id = '' OR l.org_id <> p.org_id)
			`).Error; err != nil {
				return err
			}
		}

		if okVers, err := tableExistsDB(tx, "performance_review_versions"); err != nil {
			return err
		} else if okVers {
			if err := tx.Exec(`
				UPDATE performance_review_versions v
				INNER JOIN performance_participants p ON p.id = v.participant_id
				SET v.org_id = p.org_id
				WHERE p.org_id IS NOT NULL AND p.org_id <> ''
				  AND (v.org_id IS NULL OR v.org_id = '' OR v.org_id <> p.org_id)
			`).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// allowedDingTalkBackfillSources whitelists columns that may seed ding_talk_user_id.
var allowedDingTalkBackfillSources = map[string]struct{}{
	"user_id": {},
}

// backfillUserDingTalkID backfills users.ding_talk_user_id on the package DB.
func backfillUserDingTalkID(sourceColumn string) error {
	return backfillUserDingTalkIDDB(DB, sourceColumn)
}

// backfillUserDingTalkIDDB copies sourceColumn into ding_talk_user_id for empty rows.
// Same-org collisions are audited fail-closed (no delete/merge/survivor pick).
func backfillUserDingTalkIDDB(db *gorm.DB, sourceColumn string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	src := strings.TrimSpace(sourceColumn)
	if _, ok := allowedDingTalkBackfillSources[src]; !ok {
		return fmt.Errorf("unsupported ding_talk_user_id backfill source column: %s", src)
	}
	ok, err := tableExistsDB(db, "users")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	hasSrc, err := columnExistsDB(db, "users", src)
	if err != nil {
		return err
	}
	if !hasSrc {
		return fmt.Errorf("users.%s missing", src)
	}
	hasDst, err := columnExistsDB(db, "users", "ding_talk_user_id")
	if err != nil {
		return err
	}
	// Column creation belongs to ensureUserDingTalkColumn; backfill is set-based DML only.
	if !hasDst {
		return fmt.Errorf("users.ding_talk_user_id missing")
	}

	// Effective ding_talk_user_id after backfill: keep existing non-empty, else source.
	// Audit same-org collisions before any UPDATE; never pick MIN/MAX survivors.
	// Query must include FROM `users` + HAVING COUNT(*) > 1 for injectable test stubs.
	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(TRIM(org_id), ''), 'default') AS org_id,
		       CASE
		         WHEN ding_talk_user_id IS NOT NULL AND TRIM(ding_talk_user_id) <> '' THEN TRIM(ding_talk_user_id)
		         ELSE TRIM(%[1]s)
		       END AS ding_talk_user_id,
		       COUNT(*) AS duplicate_count,
		       SUBSTRING_INDEX(GROUP_CONCAT(id ORDER BY id SEPARATOR ','), ',', 5) AS sample_ids
		FROM `+"`users`"+`
		WHERE deleted_at IS NULL
		  AND (
		    (ding_talk_user_id IS NOT NULL AND TRIM(ding_talk_user_id) <> '')
		    OR (%[1]s IS NOT NULL AND TRIM(%[1]s) <> '')
		  )
		GROUP BY 1, 2
		HAVING COUNT(*) > 1
		LIMIT 20
	`, quoteIdentifier(src))

	rows, err := db.Raw(query).Rows()
	if err != nil {
		return fmt.Errorf("users ding_talk_user_id conflict audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	conflicts := make([]string, 0, 8)
	for rows.Next() {
		var orgID, dingID, samples string
		var dupCount any
		if err := rows.Scan(&orgID, &dingID, &dupCount, &samples); err != nil {
			return fmt.Errorf("users ding_talk_user_id conflict scan: %w", err)
		}
		conflicts = append(conflicts, fmt.Sprintf(
			"table=users org_id=%s ding_talk_user_id=%s duplicate_count=%v sample_ids=%s",
			orgID, dingID, dupCount, samples,
		))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("cannot backfill ding_talk_user_id: same-organization duplicates found; no rows were deleted or merged; %s; resolve manually and rerun",
			strings.Join(conflicts, " | "))
	}

	// Set-based backfill of every eligible empty row; no survivor selection.
	return db.Exec(fmt.Sprintf(
		"UPDATE `users` SET %s = %s WHERE (`ding_talk_user_id` IS NULL OR `ding_talk_user_id` = '') AND %s IS NOT NULL AND TRIM(%s) <> ''",
		quoteIdentifier("ding_talk_user_id"), quoteIdentifier(src), quoteIdentifier(src), quoteIdentifier(src),
	)).Error
}
