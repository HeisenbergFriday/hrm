package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type CrossOrganizationSyncService struct {
	repo *repository.CrossOrganizationSyncRepository
}

var ErrCrossOrgSyncSourceRequired = errors.New("cross-organization sync source is required")
var ErrCrossOrgSyncMultipleSources = errors.New("multiple cross-organization sync sources configured")

func NewCrossOrganizationSyncService(db *gorm.DB) *CrossOrganizationSyncService {
	return &CrossOrganizationSyncService{repo: repository.NewCrossOrganizationSyncRepository(db)}
}

func (s *CrossOrganizationSyncService) CreateOrUpdateLink(ctx context.Context, sourceOrgID, targetOrgID string, scopes []string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("cross organization sync service is unavailable")
	}
	return s.repo.CreateOrUpdateLink(ctx, &database.OrganizationSyncLink{
		SourceOrgID: sourceOrgID, TargetOrgID: targetOrgID, Status: "active",
		EmployeeSync: true, BusinessSync: true, BusinessScopes: strings.Join(normalizeScopes(scopes), ","),
	})
}

func (s *CrossOrganizationSyncService) ListLinks(ctx context.Context, sourceOrgID string) ([]database.OrganizationSyncLink, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	return s.repo.ListLinks(ctx, sourceOrgID)
}

func (s *CrossOrganizationSyncService) ListInboundLinks(ctx context.Context, targetOrgID string) ([]database.OrganizationSyncLink, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	return s.repo.ListTargetLinks(ctx, targetOrgID)
}

// PrepareInbound resolves only an already persisted source→target relation,
// allowing the target-side data center to start a read-only mirror run.
func (s *CrossOrganizationSyncService) PrepareInbound(ctx context.Context, targetOrgID, sourceOrgID, triggeredBy string) (*database.OrganizationSyncRun, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	links, err := s.repo.ListTargetLinks(ctx, targetOrgID)
	if err != nil {
		return nil, err
	}
	sourceOrgID = strings.TrimSpace(sourceOrgID)
	var selected *database.OrganizationSyncLink
	for i := range links {
		link := &links[i]
		if strings.TrimSpace(link.Status) != "active" {
			continue
		}
		if sourceOrgID != "" && link.SourceOrgID != sourceOrgID {
			continue
		}
		if selected != nil {
			return nil, ErrCrossOrgSyncMultipleSources
		}
		selected = link
	}
	if selected == nil {
		return nil, ErrCrossOrgSyncSourceRequired
	}
	requestID, err := newSyncRequestID()
	if err != nil {
		return nil, err
	}
	return s.repo.StartRun(ctx, selected.SourceOrgID, selected.TargetOrgID, requestID, triggeredBy)
}

func (s *CrossOrganizationSyncService) Prepare(ctx context.Context, sourceOrgID, targetOrgID, triggeredBy string) (*database.OrganizationSyncRun, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	link, err := s.repo.FindLink(ctx, sourceOrgID, targetOrgID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(link.Status) != "active" {
		return nil, fmt.Errorf("cross organization sync link is not active")
	}
	requestID, err := newSyncRequestID()
	if err != nil {
		return nil, err
	}
	run, err := s.repo.StartRun(ctx, sourceOrgID, targetOrgID, requestID, triggeredBy)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (s *CrossOrganizationSyncService) Sync(ctx context.Context, sourceOrgID, targetOrgID, triggeredBy string) (*database.OrganizationSyncRun, error) {
	run, err := s.Prepare(ctx, sourceOrgID, targetOrgID, triggeredBy)
	if err != nil {
		return nil, err
	}
	return s.Execute(ctx, run)
}

// Execute continues a previously persisted running batch. It is safe to call
// from a detached background context after the HTTP request has returned.
func (s *CrossOrganizationSyncService) Execute(ctx context.Context, run *database.OrganizationSyncRun) (_ *database.OrganizationSyncRun, returnErr error) {
	if s == nil || s.repo == nil || run == nil {
		return run, fmt.Errorf("cross organization sync service is unavailable")
	}
	link, err := s.repo.FindLink(ctx, run.SourceOrgID, run.TargetOrgID)
	if err != nil {
		return run, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("sync panic: %v", recovered)
			_ = s.repo.FinishRun(context.WithoutCancel(ctx), run.ID, "failed", run.EmployeeCount, run.BusinessCount, run.FailureCount+1, message)
			run.Status, run.ErrorMessage = "failed", message
			returnErr = fmt.Errorf("cross organization sync failed")
		}
	}()
	employeeCount, businessCount, failureCount := 0, 0, 0
	var firstError string
	if link.EmployeeSync {
		count, failures, syncErr := s.syncEmployees(ctx, link, run.ID)
		employeeCount += count
		failureCount += failures
		if syncErr != nil && firstError == "" {
			firstError = syncErr.Error()
		}
	}
	if link.BusinessSync {
		for _, entityType := range parseScopes(link.BusinessScopes) {
			count, failures, syncErr := s.syncBusinessEntity(ctx, link, run.ID, entityType)
			businessCount += count
			failureCount += failures
			if syncErr != nil && firstError == "" {
				firstError = syncErr.Error()
			}
		}
	}
	status := "success"
	if failureCount > 0 {
		status = "partial"
	}
	if employeeCount == 0 && businessCount == 0 && firstError != "" {
		status = "failed"
	}
	finishErr := s.repo.FinishRun(context.WithoutCancel(ctx), run.ID, status, employeeCount, businessCount, failureCount, firstError)
	if finishErr != nil {
		return run, finishErr
	}
	now := time.Now()
	run.Status, run.EmployeeCount, run.BusinessCount, run.FailureCount = status, employeeCount, businessCount, failureCount
	run.ErrorMessage, run.FinishedAt = firstError, &now
	if firstError != "" && status == "failed" {
		return run, fmt.Errorf("cross organization sync failed: %s", firstError)
	}
	return run, nil
}

func (s *CrossOrganizationSyncService) GetRun(ctx context.Context, sourceOrgID, requestID string) (*database.OrganizationSyncRun, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	return s.repo.FindRun(ctx, sourceOrgID, requestID)
}

func (s *CrossOrganizationSyncService) GetInboundRun(ctx context.Context, targetOrgID, requestID string) (*database.OrganizationSyncRun, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("cross organization sync service is unavailable")
	}
	return s.repo.FindRunByTarget(ctx, targetOrgID, requestID)
}

func (s *CrossOrganizationSyncService) syncEmployees(ctx context.Context, link *database.OrganizationSyncLink, runID uint) (int, int, error) {
	rows, err := s.repo.ListSourceEmployees(ctx, link.SourceOrgID)
	if err != nil {
		return 0, 0, err
	}
	count, failures := 0, 0
	var firstError error
	for _, row := range rows {
		profile := profileMirror(row.Profile)
		sourceUpdated := row.SourceUpdated
		mirror := &database.OrganizationEmployeeMirror{
			TargetOrgID: link.TargetOrgID, SourceOrgID: link.SourceOrgID,
			SourceUserID: row.User.UserID, SourceDingTalkID: row.User.DingTalkUserID,
			EmployeeID: profileString(profile, "employee_id"), Name: row.User.Name,
			Email: row.User.Email, Mobile: row.User.Mobile, DepartmentID: row.User.DepartmentID,
			DepartmentName: row.DepartmentName, Position: row.User.Position, Status: row.User.Status,
			Profile: profile, SourceUpdatedAt: &sourceUpdated, LastSyncedAt: time.Now(),
			LastSyncRunID: runID, SyncStatus: "success",
		}
		if err := s.repo.UpsertEmployeeMirror(ctx, mirror); err != nil {
			failures++
			if firstError == nil {
				firstError = err
			}
			continue
		}
		count++
	}
	return count, failures, firstError
}

func (s *CrossOrganizationSyncService) syncBusinessEntity(ctx context.Context, link *database.OrganizationSyncLink, runID uint, entityType string) (int, int, error) {
	rows, err := s.repo.ListBusinessRows(ctx, link.SourceOrgID, entityType)
	if err != nil {
		return 0, 0, err
	}
	count, failures := 0, 0
	var firstError error
	for _, row := range rows {
		mirror := &database.OrganizationBusinessMirror{
			TargetOrgID: link.TargetOrgID, SourceOrgID: link.SourceOrgID,
			EntityType: row.EntityType, SourceKey: row.SourceKey,
			SourceUserID: row.SourceUserID, Payload: row.Payload,
			SourceUpdatedAt: row.UpdatedAt, LastSyncedAt: time.Now(),
			LastSyncRunID: runID, SyncStatus: "success",
		}
		if err := s.repo.UpsertBusinessMirror(ctx, mirror); err != nil {
			failures++
			if firstError == nil {
				firstError = err
			}
			continue
		}
		count++
	}
	return count, failures, firstError
}

func (s *CrossOrganizationSyncService) ListEmployeeMirrors(ctx context.Context, targetOrgID string, limit int) ([]database.OrganizationEmployeeMirror, error) {
	return s.repo.ListEmployeeMirrors(ctx, targetOrgID, limit)
}

func (s *CrossOrganizationSyncService) ListBusinessMirrors(ctx context.Context, targetOrgID, entityType string, limit int) ([]database.OrganizationBusinessMirror, error) {
	return s.repo.ListBusinessMirrors(ctx, targetOrgID, entityType, limit)
}

type PeopleDataCenterSummary struct {
	EmployeeTotal       int64                         `json:"employee_total"`
	ActiveEmployeeTotal int64                         `json:"active_employee_total"`
	LocalEmployeeTotal  int64                         `json:"local_employee_total"`
	MirrorEmployeeTotal int64                         `json:"mirror_employee_total"`
	BusinessTotal       int64                         `json:"business_total"`
	LocalBusinessTotal  int64                         `json:"local_business_total"`
	MirrorBusinessTotal int64                         `json:"mirror_business_total"`
	LatestRun           *database.OrganizationSyncRun `json:"latest_run,omitempty"`
	SyncFailureCount    int                           `json:"sync_failure_count"`
}

type PeopleDataCenterEmployee struct {
	ID             uint       `json:"id"`
	SourceOrgID    string     `json:"source_org_id"`
	SourceUserID   string     `json:"source_user_id"`
	EmployeeID     string     `json:"employee_id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Mobile         string     `json:"mobile"`
	DepartmentName string     `json:"department_name"`
	Position       string     `json:"position"`
	Status         string     `json:"status"`
	LastSyncedAt   time.Time  `json:"last_synced_at"`
	SourceUpdated  *time.Time `json:"source_updated_at,omitempty"`
	IsMirror       bool       `json:"is_mirror"`
}

type PeopleDataCenterBusiness struct {
	ID            uint                   `json:"id"`
	SourceOrgID   string                 `json:"source_org_id"`
	EntityType    string                 `json:"entity_type"`
	SourceKey     string                 `json:"source_key"`
	SourceUserID  string                 `json:"source_user_id"`
	Payload       map[string]interface{} `json:"payload"`
	SourceUpdated *time.Time             `json:"source_updated_at,omitempty"`
	LastSyncedAt  time.Time              `json:"last_synced_at"`
	SyncStatus    string                 `json:"sync_status"`
	IsMirror      bool                   `json:"is_mirror"`
}

func (s *CrossOrganizationSyncService) PeopleDataCenterSummary(ctx context.Context, targetOrgID string) (*PeopleDataCenterSummary, error) {
	localTotal, localActive, err := s.repo.CountLocalEmployees(ctx, targetOrgID)
	if err != nil {
		return nil, err
	}
	mirrorTotal, mirrorActive, err := s.repo.CountEmployeeMirrors(ctx, targetOrgID)
	if err != nil {
		return nil, err
	}
	localBusiness, err := s.repo.CountLocalBusinessRows(ctx, targetOrgID)
	if err != nil {
		return nil, err
	}
	mirrorBusiness, err := s.repo.CountBusinessMirrors(ctx, targetOrgID)
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.LatestRunForTarget(ctx, targetOrgID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	result := &PeopleDataCenterSummary{EmployeeTotal: localTotal + mirrorTotal, ActiveEmployeeTotal: localActive + mirrorActive, LocalEmployeeTotal: localTotal, MirrorEmployeeTotal: mirrorTotal, BusinessTotal: localBusiness + mirrorBusiness, LocalBusinessTotal: localBusiness, MirrorBusinessTotal: mirrorBusiness, LatestRun: latest}
	if latest != nil {
		result.SyncFailureCount = latest.FailureCount
	}
	return result, nil
}

func (s *CrossOrganizationSyncService) ListPeopleDataCenterEmployees(ctx context.Context, targetOrgID, keyword, status, source string, page, pageSize int) ([]PeopleDataCenterEmployee, int64, error) {
	local, err := s.repo.ListLocalEmployeeCenterRows(ctx, targetOrgID)
	if err != nil {
		return nil, 0, err
	}
	mirrors, err := s.repo.ListAllEmployeeMirrors(ctx, targetOrgID)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]PeopleDataCenterEmployee, 0, len(local)+len(mirrors))
	needle := strings.ToLower(strings.TrimSpace(keyword))
	status = strings.TrimSpace(status)
	source = strings.TrimSpace(source)
	if source != "local" && source != "mirror" {
		source = ""
	}
	for _, item := range local {
		if source == "mirror" {
			continue
		}
		row := PeopleDataCenterEmployee{ID: item.ID, SourceOrgID: item.SourceOrgID, SourceUserID: item.SourceUserID, EmployeeID: item.EmployeeID, Name: item.Name, Email: item.Email, Mobile: item.Mobile, DepartmentName: item.DepartmentName, Position: item.Position, Status: item.Status, LastSyncedAt: item.LastSyncedAt, SourceUpdated: item.SourceUpdated, IsMirror: false}
		if peopleEmployeeMatches(row, needle, status) {
			rows = append(rows, row)
		}
	}
	for _, item := range mirrors {
		if source == "local" {
			continue
		}
		row := PeopleDataCenterEmployee{ID: item.ID, SourceOrgID: item.SourceOrgID, SourceUserID: item.SourceUserID, EmployeeID: item.EmployeeID, Name: item.Name, Email: item.Email, Mobile: item.Mobile, DepartmentName: item.DepartmentName, Position: item.Position, Status: item.Status, LastSyncedAt: item.LastSyncedAt, SourceUpdated: item.SourceUpdatedAt, IsMirror: true}
		if peopleEmployeeMatches(row, needle, status) {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return paginatePeople(rows, page, pageSize)
}

func peopleEmployeeMatches(row PeopleDataCenterEmployee, needle, status string) bool {
	if status != "" && row.Status != status {
		return false
	}
	if needle == "" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{row.Name, row.EmployeeID, row.DepartmentName, row.Position, row.Email, row.Mobile}, " "))
	return strings.Contains(text, needle)
}

func (s *CrossOrganizationSyncService) ListPeopleDataCenterBusiness(ctx context.Context, targetOrgID, entityType, source string, page, pageSize int) ([]PeopleDataCenterBusiness, int64, error) {
	entityType = strings.TrimSpace(entityType)
	source = strings.TrimSpace(source)
	if source != "local" && source != "mirror" {
		source = ""
	}
	types := []string{"attendance", "approval", "annual_leave_grant", "overtime_match", "performance_activity", "performance_participant"}
	if entityType != "" {
		types = []string{entityType}
	}
	rows := make([]PeopleDataCenterBusiness, 0)
	if source != "mirror" {
		for _, typ := range types {
			local, err := s.repo.ListBusinessRows(ctx, targetOrgID, typ)
			if err != nil {
				return nil, 0, err
			}
			for _, item := range local {
				rows = append(rows, PeopleDataCenterBusiness{SourceOrgID: targetOrgID, EntityType: item.EntityType, SourceKey: item.SourceKey, SourceUserID: item.SourceUserID, Payload: item.Payload, SourceUpdated: item.UpdatedAt, LastSyncedAt: valueOrNow(item.UpdatedAt), SyncStatus: "local", IsMirror: false})
			}
		}
	}
	if source != "local" {
		mirrors, err := s.repo.ListAllBusinessMirrors(ctx, targetOrgID, entityType)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range mirrors {
			rows = append(rows, PeopleDataCenterBusiness{ID: item.ID, SourceOrgID: item.SourceOrgID, EntityType: item.EntityType, SourceKey: item.SourceKey, SourceUserID: item.SourceUserID, Payload: item.Payload, SourceUpdated: item.SourceUpdatedAt, LastSyncedAt: item.LastSyncedAt, SyncStatus: item.SyncStatus, IsMirror: true})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LastSyncedAt.After(rows[j].LastSyncedAt) })
	return paginatePeople(rows, page, pageSize)
}

func valueOrNow(value *time.Time) time.Time {
	if value != nil {
		return *value
	}
	return time.Now()
}

func paginatePeople[T any](rows []T, page, pageSize int) ([]T, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	total := int64(len(rows))
	start := (page - 1) * pageSize
	if start >= len(rows) {
		return []T{}, total, nil
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], total, nil
}

func normalizeScopes(scopes []string) []string {
	allowed := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := businessMirrorEntityAllowed(scope); ok {
			allowed[scope] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return []string{"attendance", "approval", "annual_leave_grant", "overtime_match", "performance_activity", "performance_participant"}
	}
	result := make([]string, 0, len(allowed))
	for scope := range allowed {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result
}

func parseScopes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return normalizeScopes(nil)
	}
	return normalizeScopes(strings.Split(value, ","))
}

func businessMirrorEntityAllowed(value string) (string, bool) {
	allowed := map[string]struct{}{
		"attendance": {}, "approval": {}, "annual_leave_grant": {}, "overtime_match": {},
		"performance_activity": {}, "performance_participant": {},
	}
	_, ok := allowed[strings.TrimSpace(value)]
	return strings.TrimSpace(value), ok
}

func profileMirror(profile *database.EmployeeProfile) map[string]interface{} {
	if profile == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"employee_id": profile.EmployeeID, "gender": profile.Gender, "birth_date": profile.BirthDate,
		"nationality": profile.Nationality, "employment_type": profile.EmploymentType,
		"entry_date": profile.EntryDate, "probation_end_date": profile.ProbationEndDate,
		"planned_regular_date": profile.PlannedRegularDate, "actual_regular_date": profile.ActualRegularDate,
		"job_level": profile.JobLevel, "job_family": profile.JobFamily,
		"contract_start_date": profile.ContractStartDate, "contract_end_date": profile.ContractEndDate,
		"work_email": profile.WorkEmail, "personal_email": profile.PersonalEmail,
		"profile_status": profile.ProfileStatus, "extension": profile.Extension,
	}
}

func profileString(profile map[string]interface{}, key string) string {
	if value, ok := profile[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func newSyncRequestID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "cos-" + hex.EncodeToString(buf), nil
}
