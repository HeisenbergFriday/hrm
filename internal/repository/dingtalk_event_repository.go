package repository

import (
	"errors"
	"peopleops/internal/database"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DingTalkEventStatusProcessing = "processing"
	DingTalkEventStatusSuccess    = "success"
	DingTalkEventStatusFailed     = "failed"
	DingTalkEventStatusSkipped    = "skipped"

	// DingTalkEventProcessingStaleAfter controls when a crashed processing row may be reclaimed.
	DingTalkEventProcessingStaleAfter = 5 * time.Minute
)

// ErrDingTalkEventAlreadyProcessed indicates the event was already handled successfully.
var ErrDingTalkEventAlreadyProcessed = errors.New("dingtalk event already processed")

// ErrDingTalkEventInProgress indicates another worker is currently processing the event.
var ErrDingTalkEventInProgress = errors.New("dingtalk event is still processing")

type DingTalkEventRepository struct {
	db    *gorm.DB
	orgID string
	// nowFn is overridable in tests.
	nowFn func() time.Time
}

func NewDingTalkEventRepository(db *gorm.DB) *DingTalkEventRepository {
	return &DingTalkEventRepository{db: db, nowFn: time.Now}
}

func NewDingTalkEventRepositoryWithOrgID(db *gorm.DB, orgID string) *DingTalkEventRepository {
	return &DingTalkEventRepository{db: db, orgID: strings.TrimSpace(orgID), nowFn: time.Now}
}

func (r *DingTalkEventRepository) now() time.Time {
	if r.nowFn != nil {
		return r.nowFn()
	}
	return time.Now()
}

// TryBeginProcessing inserts a processing log for (org_id, event_id).
// Reclaim of failed / stale processing rows is atomic (transaction + compare-and-set).
func (r *DingTalkEventRepository) TryBeginProcessing(log *database.DingTalkEventLog) error {
	if log == nil {
		return gorm.ErrInvalidData
	}
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, log.OrgID)
	if err != nil {
		return err
	}
	log.OrgID = merged
	log.OrgID = database.NormalizeOrganizationID(log.OrgID)
	log.EventID = strings.TrimSpace(log.EventID)
	if log.EventID == "" {
		return errors.New("event_id is required")
	}
	if strings.TrimSpace(log.Status) == "" {
		log.Status = DingTalkEventStatusProcessing
	}

	// Fast path: first writer inserts processing row.
	err = r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(log).Error
	if err != nil {
		return err
	}
	if log.ID != 0 {
		return nil
	}

	// Conflict path: lock the existing row and reclaim atomically when allowed.
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing database.DingTalkEventLog
		if findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND event_id = ?", log.OrgID, log.EventID).
			First(&existing).Error; findErr != nil {
			// Race: row disappeared between insert conflict and select — retry insert once.
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				if createErr := tx.Create(log).Error; createErr != nil {
					return createErr
				}
				return nil
			}
			return findErr
		}

		switch existing.Status {
		case DingTalkEventStatusSuccess, DingTalkEventStatusSkipped:
			return ErrDingTalkEventAlreadyProcessed
		case DingTalkEventStatusProcessing:
			if r.now().Sub(existing.UpdatedAt) < DingTalkEventProcessingStaleAfter {
				return ErrDingTalkEventInProgress
			}
			// Stale processing: CAS reclaim only if still processing and not refreshed.
			return r.casReclaim(tx, &existing, log, []string{DingTalkEventStatusProcessing}, true)
		case DingTalkEventStatusFailed:
			return r.casReclaim(tx, &existing, log, []string{DingTalkEventStatusFailed}, false)
		default:
			// Unknown status: treat as reclaimable only when status still matches.
			return r.casReclaim(tx, &existing, log, []string{existing.Status}, false)
		}
	})
}

func (r *DingTalkEventRepository) casReclaim(
	tx *gorm.DB,
	existing *database.DingTalkEventLog,
	incoming *database.DingTalkEventLog,
	allowedStatuses []string,
	requireStale bool,
) error {
	now := r.now()
	updates := map[string]interface{}{
		"event_type":          incoming.EventType,
		"process_instance_id": incoming.ProcessInstanceID,
		"process_code":        incoming.ProcessCode,
		"change_type":         incoming.ChangeType,
		"result":              incoming.Result,
		"status":              DingTalkEventStatusProcessing,
		"error_message":       "",
		"event_born_time":     incoming.EventBornTime,
		"updated_at":          now,
	}
	// 不在 CAS Updates 中写入 payload_summary(map)，避免部分驱动/序列化器对 map 参数不兼容。

	q := tx.Model(&database.DingTalkEventLog{}).
		Where("id = ? AND status IN ?", existing.ID, allowedStatuses)
	if requireStale {
		// Compare-and-set on both status and stale updated_at to prevent double reclaim.
		staleBefore := now.Add(-DingTalkEventProcessingStaleAfter)
		q = q.Where("updated_at <= ?", staleBefore)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Lost the race to another worker.
		return ErrDingTalkEventInProgress
	}
	incoming.ID = existing.ID
	return nil
}

func (r *DingTalkEventRepository) MarkSuccess(orgID, eventID string) error {
	return r.updateStatus(orgID, eventID, DingTalkEventStatusSuccess, "")
}

func (r *DingTalkEventRepository) MarkSkipped(orgID, eventID, reason string) error {
	return r.updateStatus(orgID, eventID, DingTalkEventStatusSkipped, reason)
}

func (r *DingTalkEventRepository) MarkFailed(orgID, eventID, reason string) error {
	return r.updateStatus(orgID, eventID, DingTalkEventStatusFailed, reason)
}

func (r *DingTalkEventRepository) FindByEventID(orgID, eventID string) (*database.DingTalkEventLog, error) {
	var log database.DingTalkEventLog
	if r.orgID != "" {
		orgID = r.orgID
	}
	orgID = database.NormalizeOrganizationID(orgID)
	err := r.db.Where("org_id = ? AND event_id = ?", orgID, strings.TrimSpace(eventID)).First(&log).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *DingTalkEventRepository) updateStatus(orgID, eventID, status, errMsg string) error {
	bound := strings.TrimSpace(r.orgID)
	if bound == "" {
		return ErrMissingOrgID
	}
	if arg := strings.TrimSpace(orgID); arg != "" && database.NormalizeOrganizationID(arg) != database.NormalizeOrganizationID(bound) {
		return ErrOrgMismatch
	}
	orgID = database.NormalizeOrganizationID(bound)
	eventID = strings.TrimSpace(eventID)
	return r.db.Model(&database.DingTalkEventLog{}).
		Where("org_id = ? AND event_id = ?", orgID, eventID).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errMsg,
		}).Error
}
