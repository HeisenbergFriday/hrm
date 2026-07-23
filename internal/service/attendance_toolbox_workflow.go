package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"peopleops/internal/dingtalk"
)

// AttendanceToolboxRunResponse is the stable API view of a stored toolbox run.
type AttendanceToolboxRunResponse struct {
	RunID   string                     `json:"run_id"`
	Module  string                     `json:"module"`
	Log     string                     `json:"log,omitempty"`
	Stats   map[string]interface{}     `json:"stats,omitempty"`
	Meta    map[string]interface{}     `json:"meta,omitempty"`
	Files   []AttendanceToolboxRunFile `json:"files"`
	Expires string                     `json:"expires_at"`
}

type attendanceToolboxWorkflowOutput struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
	Kind     string `json:"kind"`
	FlowKey  string `json:"flow_key"`
	RowCount int    `json:"row_count"`
	Data     []byte `json:"-"`
}

type attendanceToolboxWorkflowRunnerResult struct {
	OK      bool                              `json:"ok"`
	Outputs []attendanceToolboxWorkflowOutput `json:"outputs"`
	Log     string                            `json:"log"`
	Error   string                            `json:"error"`
}

// RunLegacyProcessing preserves the old /attendance/processing/* multipart contract.
func (s *AttendanceToolboxService) RunLegacyProcessing(ctx context.Context, module string, form *multipart.Form) (*AttendanceToolboxResult, error) {
	return s.Run(ctx, module, mapLegacyProcessingForm(module, form))
}

// RunDingtalkSyncForOrg runs the sync with credentials and process codes bound to orgID.
func (s *AttendanceToolboxService) RunDingtalkSyncForOrg(ctx context.Context, orgID string, req *DingtalkSyncRequest) (*DingtalkSyncResult, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, errors.New("org_id is required")
	}
	if req == nil {
		return nil, errors.New("sync request is required")
	}
	config, err := dingtalk.ConfigForOrgID(orgID)
	if err != nil {
		return nil, fmt.Errorf("resolve dingtalk organization config: %w", err)
	}
	config = config.NormalizedForAPI()
	if config.AppKey == "" || config.AppSecret == "" {
		return nil, fmt.Errorf("dingtalk credentials are not configured for org %s", orgID)
	}

	extra := map[string]interface{}{
		"dingtalk_sync_start_date": req.StartDate,
		"dingtalk_sync_end_date":   req.EndDate,
	}
	if len(req.FlowKeys) > 0 {
		extra["dingtalk_sync_flow_keys"] = req.FlowKeys
	}
	if req.MaxInstances != nil {
		extra["dingtalk_sync_max_instances"] = *req.MaxInstances
	}
	if req.PaddingDays != nil {
		extra["dingtalk_sync_padding_days"] = *req.PaddingDays
	}
	extra["dingtalk"] = map[string]interface{}{
		"client_id":     config.AppKey,
		"client_secret": config.AppSecret,
		"process_codes": config.ProcessCodes,
	}
	outputs, _, err := s.runToolboxWorkflowEngine(ctx, "dingtalk_sync", nil, extra)
	if err != nil {
		return nil, err
	}
	result := &DingtalkSyncResult{}
	for _, output := range outputs {
		result.Outputs = append(result.Outputs, AttendanceToolboxResult{
			FileName:    output.FileName,
			ContentType: contentTypeForName(output.FileName),
			Data:        output.Data,
			Kind:        output.Kind,
			FlowKey:     output.FlowKey,
			RowCount:    output.RowCount,
		})
	}
	if len(result.Outputs) > 1 {
		result.ZipData, err = zipAttendanceToolboxResults(result.Outputs)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// RunStructured executes a workflow and persists its outputs in the scoped run store.
func (s *AttendanceToolboxService) RunStructured(ctx context.Context, userID, orgID, module string, form *multipart.Form, extra map[string]interface{}) (*AttendanceToolboxRun, error) {
	userID = strings.TrimSpace(userID)
	orgID = strings.TrimSpace(orgID)
	module = strings.TrimSpace(module)
	if userID == "" || orgID == "" {
		return nil, errors.New("user_id and org_id are required")
	}
	if module != "quick" && module != "dingtalk_sync" && !IsAttendanceToolboxStandardModule(module) {
		return nil, fmt.Errorf("unsupported toolbox module: %s", module)
	}
	if module != "dingtalk_sync" && form == nil {
		return nil, errors.New("upload form is required")
	}

	config := cloneToolboxConfig(extra)
	if form != nil {
		spec := structuredToolboxSpec(module)
		if err := saveAttendanceToolboxFilesForSpec(tempDirPlaceholder{}, form, spec, config); err != nil {
			return nil, err
		}
		for key, values := range form.Value {
			if len(values) == 1 {
				config[key] = values[0]
			} else if len(values) > 1 {
				config[key] = append([]string(nil), values...)
			}
		}
	}
	if module == "dingtalk_sync" || module == "quick" {
		if err := attachOrgDingtalkConfig(config, orgID); err != nil {
			return nil, err
		}
	}

	outputs, logText, err := s.runToolboxWorkflowEngine(ctx, module, form, config)
	if err != nil {
		return nil, err
	}
	files := make([]AttendanceToolboxResult, 0, len(outputs))
	stats := map[string]interface{}{"output_count": len(outputs)}
	meta := map[string]interface{}{}
	for _, output := range outputs {
		file := AttendanceToolboxResult{
			FileName:    output.FileName,
			ContentType: contentTypeForName(output.FileName),
			Data:        output.Data,
			Kind:        output.Kind,
			FlowKey:     output.FlowKey,
			RowCount:    output.RowCount,
		}
		files = append(files, file)
		if strings.EqualFold(output.Kind, "meta") || strings.EqualFold(filepath.Ext(output.FileName), ".json") {
			var value map[string]interface{}
			if json.Unmarshal(output.Data, &value) == nil {
				mergeWorkflowMeta(meta, output.FileName, value)
			}
		}
	}
	return DefaultAttendanceToolboxRunStore().Put(userID, orgID, module, logText, stats, meta, files)
}

// PreviewStoredRun previews a stored xlsx without executing its business module again.
func (s *AttendanceToolboxService) PreviewStoredRun(ctx context.Context, runID, fileKey, userID, orgID string) (map[string]interface{}, error) {
	run, err := DefaultAttendanceToolboxRunStore().Get(runID, userID, orgID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(fileKey) == "" {
		for _, file := range run.Files {
			if !strings.EqualFold(file.Kind, "meta") && strings.EqualFold(filepath.Ext(file.FileName), ".xlsx") {
				fileKey = file.FileKey
				break
			}
		}
	}
	if strings.TrimSpace(fileKey) == "" {
		return nil, ErrToolboxFileMissing
	}
	fileName, _, data, err := DefaultAttendanceToolboxRunStore().ReadFile(runID, fileKey, userID, orgID)
	if err != nil {
		return nil, err
	}
	workdir, err := os.MkdirTemp("", "peopleops-attendance-preview-stored-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(workdir) }()
	path := filepath.Join(workdir, filepath.Base(fileName))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	action, err := s.runAction(ctx, "preview-existing", map[string]interface{}{"preview_file": path, "max_rows": 200})
	if err != nil {
		return nil, err
	}
	if len(action.Outputs) == 0 {
		return nil, errors.New("preview output is empty")
	}
	var preview map[string]interface{}
	if err := json.Unmarshal(action.Outputs[0].Data, &preview); err != nil {
		return nil, fmt.Errorf("preview output is invalid: %w", err)
	}
	return preview, nil
}

// ZipAttendanceToolboxResultsForDownload creates a download archive from scoped result data.
func ZipAttendanceToolboxResultsForDownload(files []AttendanceToolboxResult) ([]byte, error) {
	return zipAttendanceToolboxResults(files)
}

func zipAttendanceToolboxResults(files []AttendanceToolboxResult) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := map[string]int{}
	for index, file := range files {
		name := filepath.Base(strings.TrimSpace(file.FileName))
		if name == "." || name == "" {
			name = fmt.Sprintf("result_%d", index+1)
		}
		seen[name]++
		if seen[name] > 1 {
			name = fmt.Sprintf("%d_%s", seen[name], name)
		}
		entry, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := entry.Write(file.Data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cloneToolboxConfig(extra map[string]interface{}) map[string]interface{} {
	config := make(map[string]interface{}, len(extra)+4)
	for key, value := range extra {
		config[key] = value
	}
	return config
}

func attachOrgDingtalkConfig(config map[string]interface{}, orgID string) error {
	resolved, err := dingtalk.ConfigForOrgID(orgID)
	if err != nil {
		return fmt.Errorf("resolve dingtalk organization config: %w", err)
	}
	resolved = resolved.NormalizedForAPI()
	if resolved.AppKey == "" || resolved.AppSecret == "" {
		return fmt.Errorf("dingtalk credentials are not configured for org %s", orgID)
	}
	config["dingtalk"] = map[string]interface{}{
		"client_id":     resolved.AppKey,
		"client_secret": resolved.AppSecret,
		"process_codes": resolved.ProcessCodes,
	}
	return nil
}

func (s *AttendanceToolboxService) runToolboxWorkflowEngine(ctx context.Context, module string, form *multipart.Form, config map[string]interface{}) ([]attendanceToolboxWorkflowOutput, string, error) {
	workdir, err := os.MkdirTemp("", "peopleops-attendance-workflow-*")
	if err != nil {
		return nil, "", err
	}
	cleanup := func() { _ = os.RemoveAll(workdir) }
	// Rebuild file paths in the real workdir after the initial config validation.
	if form != nil {
		for key := range config {
			if strings.HasSuffix(key, "_src") || strings.HasSuffix(key, "_schedule") || strings.HasSuffix(key, "_calendar") || strings.HasSuffix(key, "_attendance") || strings.HasSuffix(key, "_roster") || strings.HasSuffix(key, "_checkin") || strings.HasSuffix(key, "_result") || strings.HasSuffix(key, "_active") || strings.HasSuffix(key, "_leave") || strings.HasSuffix(key, "_overtime") || strings.HasSuffix(key, "_subsidy") || strings.HasSuffix(key, "_resign") || strings.HasSuffix(key, "_transfer") || strings.HasSuffix(key, "_detail") || strings.HasSuffix(key, "_default_schedule") || strings.HasSuffix(key, "_monthly") || strings.HasSuffix(key, "_schedules") || strings.HasSuffix(key, "_offsite_duration") {
				delete(config, key)
			}
		}
		spec := structuredToolboxSpec(module)
		if err := saveAttendanceToolboxFiles(workdir, form, spec, config); err != nil {
			cleanup()
			return nil, "", err
		}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, "", err
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if s.engineDir == "" {
		cleanup()
		return nil, "", errors.New("attendance toolbox engine directory not found")
	}
	if _, err := os.Stat(runnerPath); err != nil {
		cleanup()
		return nil, "", fmt.Errorf("attendance toolbox runner not found: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	args := []string{"--module", module, "--workdir", workdir, "--config-json", string(configJSON)}
	var stdout, stderr bytes.Buffer
	cmd := s.newRunnerCommand(runCtx, runnerPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		cleanup()
		return nil, "", errors.New("attendance toolbox workflow timed out")
	}
	var runner attendanceToolboxWorkflowRunnerResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); err != nil {
		if runErr != nil {
			cleanup()
			return nil, "", fmt.Errorf("attendance toolbox workflow failed: %v\n%s", runErr, stderr.String())
		}
		cleanup()
		return nil, "", fmt.Errorf("attendance toolbox runner returned invalid JSON: %w", err)
	}
	if runErr != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && runErr != nil {
			message = runErr.Error()
		}
		cleanup()
		return nil, "", fmt.Errorf("attendance toolbox workflow failed: %s", message)
	}
	for _, output := range runner.Outputs {
		if !isPathWithin(workdir, output.Path) {
			cleanup()
			return nil, "", errors.New("attendance toolbox output path is unsafe")
		}
		if output.FileName == "" {
			output.FileName = filepath.Base(output.Path)
		}
	}
	outputs := make([]attendanceToolboxWorkflowOutput, 0, len(runner.Outputs))
	for _, output := range runner.Outputs {
		data, err := os.ReadFile(output.Path)
		if err != nil {
			cleanup()
			return nil, "", err
		}
		output.Data = data
		outputs = append(outputs, output)
	}
	cleanup()
	return outputs, runner.Log, nil
}

// tempDirPlaceholder exists only to make the file-copy validation explicit; actual copying
// happens in runToolboxWorkflowEngine once its work directory is available.
type tempDirPlaceholder struct{}

func saveAttendanceToolboxFilesForSpec(_ tempDirPlaceholder, form *multipart.Form, spec attendanceToolboxModuleSpec, config map[string]interface{}) error {
	if form == nil {
		return nil
	}
	for key := range spec.SingleFiles {
		if len(form.File[key]) == 0 {
			continue
		}
		if form.File[key][0] == nil || form.File[key][0].Size <= 0 {
			return fmt.Errorf("uploaded file %s is empty", key)
		}
	}
	return nil
}

func structuredToolboxSpec(module string) attendanceToolboxModuleSpec {
	if module != "quick" {
		spec, _ := attendanceToolboxSpecs()[module]
		return spec
	}
	return attendanceToolboxModuleSpec{
		SingleFiles: map[string]string{
			"leave_schedule":         "leave_schedule",
			"leave_offsite_duration": "leave_offsite_duration",
			"overtime_calendar":      "overtime_calendar",
			"overtime_attendance":    "overtime_attendance",
			"overtime_roster":        "overtime_roster",
		},
		MultiFiles: map[string]string{"overtime_schedules": "overtime_schedules"},
		TextFields: []string{"dingtalk_sync_start_date", "dingtalk_sync_end_date", "dingtalk_sync_flow_keys", "dingtalk_sync_max_instances", "dingtalk_sync_padding_days", "run_leave", "run_overtime", "leave_special_names", "chengdu_schedule_names", "maternity_leave_overrides", "overtime_target_month", "rules_json"},
	}
}

func mergeWorkflowMeta(meta map[string]interface{}, fileName string, value map[string]interface{}) {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	switch {
	case strings.Contains(base, "subsidy_audit"):
		meta["subsidy_audit"] = value
	case strings.Contains(base, "overtime_rules"):
		meta["overtime_rules"] = value
	case strings.Contains(base, "dingtalk_sync"):
		meta["dingtalk_sync"] = value
	case strings.Contains(base, "quick_workflow"):
		meta["quick_workflow"] = value
	default:
		meta[base] = value
	}
}

func mapLegacyProcessingForm(module string, form *multipart.Form) *multipart.Form {
	if form == nil {
		return nil
	}
	out := &multipart.Form{Value: map[string][]string{}, File: map[string][]*multipart.FileHeader{}}
	for key, values := range form.Value {
		out.Value[key] = append([]string(nil), values...)
	}
	copyFiles := func(target string, aliases ...string) {
		for _, alias := range aliases {
			if files := form.File[alias]; len(files) > 0 {
				out.File[target] = append(out.File[target], files...)
				return
			}
		}
	}
	switch strings.TrimSpace(module) {
	case "leave":
		copyFiles("leave_src", "input", "leave_src")
		copyFiles("leave_schedule", "schedule", "leave_schedule")
		copyFiles("leave_offsite_duration", "offsite_duration", "leave_offsite_duration")
	case "overtime":
		copyFiles("overtime_src", "input", "overtime_src")
		copyFiles("overtime_attendance", "attendance", "overtime_attendance")
		copyFiles("overtime_roster", "roster", "overtime_roster")
		copyFiles("overtime_calendar", "calendar", "overtime_calendar")
		copyFiles("overtime_schedules", "schedule", "overtime_schedules")
	case "subsidy":
		copyFiles("subsidy_src", "source", "subsidy_src")
		copyFiles("subsidy_attendance", "attendance", "subsidy_attendance")
		copyFiles("subsidy_schedule", "schedule", "subsidy_schedule")
		copyFiles("subsidy_checkin", "signin", "subsidy_checkin")
		copyFiles("subsidy_attendance_result", "result", "subsidy_attendance_result")
	case "final":
		copyFiles("final_active", "roster", "final_active")
		copyFiles("final_schedule", "schedule", "final_schedule")
		copyFiles("final_leave", "leave", "final_leave")
		copyFiles("final_overtime", "overtime", "final_overtime")
		copyFiles("final_subsidy", "subsidy", "final_subsidy")
		copyFiles("final_resign", "resigned", "final_resign")
		copyFiles("final_transfer", "transfer", "final_transfer")
	case "parttime":
		copyFiles("parttime_default_schedule", "default_schedule", "parttime_default_schedule")
		copyFiles("parttime_attendance_detail", "attendance_detail", "parttime_attendance_detail")
		copyFiles("parttime_monthly", "monthly_summary", "parttime_monthly")
		copyFiles("parttime_schedules", "schedule", "parttime_schedules")
	}
	return out
}
