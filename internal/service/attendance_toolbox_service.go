package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	attendanceToolboxMaxUploadBytes = 500 * 1024 * 1024
)

type AttendanceToolboxService struct {
	engineDir string
	pythonBin string
	timeout   time.Duration
}

type AttendanceToolboxResult struct {
	FileName    string
	ContentType string
	Data        []byte
	Kind        string
	FlowKey     string
	RowCount    int
}

type attendanceToolboxRunnerResult struct {
	OK      bool `json:"ok"`
	Outputs []struct {
		Path     string `json:"path"`
		FileName string `json:"file_name"`
	} `json:"outputs"`
	Log       string `json:"log"`
	Error     string `json:"error"`
	Traceback string `json:"traceback"`
}

type attendanceToolboxDefaultsResult struct {
	OK       bool                `json:"ok"`
	Defaults map[string][]string `json:"defaults"`
	Error    string              `json:"error"`
}

type attendanceToolboxModuleSpec struct {
	SingleFiles map[string]string
	MultiFiles  map[string]string
	TextFields  []string
}

func NewAttendanceToolboxService() *AttendanceToolboxService {
	timeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("ATTENDANCE_TOOLBOX_TIMEOUT_SECONDS")))
	if timeoutSeconds <= 0 {
		timeoutSeconds = 600
	}
	return &AttendanceToolboxService{
		engineDir: resolveAttendanceToolboxEngineDir(),
		pythonBin: resolveAttendanceToolboxPython(),
		timeout:   time.Duration(timeoutSeconds) * time.Second,
	}
}

func (s *AttendanceToolboxService) Defaults(ctx context.Context) (map[string][]string, error) {
	if s.engineDir == "" {
		return nil, errors.New("未找到考勤工具箱 Python 引擎目录")
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if _, err := os.Stat(runnerPath); err != nil {
		return nil, fmt.Errorf("未找到考勤工具箱 runner：%w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := s.newRunnerCommand(runCtx, runnerPath, "--defaults")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, errors.New("读取考勤工具箱默认名单超时")
	}

	var runner attendanceToolboxDefaultsResult
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); jsonErr != nil {
		if err != nil {
			return nil, fmt.Errorf("读取考勤工具箱默认名单失败：%v\n%s", err, stderr.String())
		}
		return nil, fmt.Errorf("考勤工具箱默认名单返回格式异常：%w\n%s", jsonErr, stdout.String())
	}
	if err != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		return nil, fmt.Errorf("读取考勤工具箱默认名单失败：%s", message)
	}
	return runner.Defaults, nil
}

func (s *AttendanceToolboxService) Run(ctx context.Context, module string, form *multipart.Form) (*AttendanceToolboxResult, error) {
	module = strings.TrimSpace(module)
	spec, ok := attendanceToolboxSpecs()[module]
	if !ok {
		return nil, fmt.Errorf("不支持的考勤工具箱模块：%s", module)
	}
	if form == nil {
		return nil, errors.New("请上传 Excel 文件")
	}
	if s.engineDir == "" {
		return nil, errors.New("未找到考勤工具箱 Python 引擎目录")
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if _, err := os.Stat(runnerPath); err != nil {
		return nil, fmt.Errorf("未找到考勤工具箱 runner：%w", err)
	}

	workdir, err := os.MkdirTemp("", "peopleops-attendance-toolbox-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	config := map[string]interface{}{}
	if err := saveAttendanceToolboxFiles(workdir, form, spec, config); err != nil {
		return nil, err
	}
	for _, field := range spec.TextFields {
		if values, ok := form.Value[field]; ok && len(values) > 0 {
			config[field] = values
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := s.newRunnerCommand(runCtx, runnerPath, "--module", module, "--workdir", workdir, "--config-json", string(configJSON))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, errors.New("考勤工具箱计算超时，请缩小文件范围后重试")
	}

	var runner attendanceToolboxRunnerResult
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); jsonErr != nil {
		if err != nil {
			return nil, fmt.Errorf("考勤工具箱运行失败：%v\n%s", err, stderr.String())
		}
		return nil, fmt.Errorf("考勤工具箱返回格式异常：%w\n%s", jsonErr, stdout.String())
	}
	if err != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		return nil, fmt.Errorf("考勤工具箱计算失败：%s", message)
	}
	if len(runner.Outputs) == 0 {
		return nil, errors.New("考勤工具箱未生成结果文件")
	}

	if len(runner.Outputs) == 1 {
		output := runner.Outputs[0]
		data, err := readAttendanceToolboxOutput(workdir, output.Path)
		if err != nil {
			return nil, err
		}
		fileName := strings.TrimSpace(output.FileName)
		if fileName == "" {
			fileName = "考勤工具箱结果.xlsx"
		}
		return &AttendanceToolboxResult{
			FileName:    fileName,
			ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Data:        data,
		}, nil
	}

	archive, err := zipAttendanceToolboxOutputs(workdir, runner.Outputs)
	if err != nil {
		return nil, err
	}
	return &AttendanceToolboxResult{
		FileName:    attendanceToolboxZipName(module),
		ContentType: "application/zip",
		Data:        archive,
	}, nil
}

type DingtalkSyncRequest struct {
	StartDate         string   `json:"start_date"`
	EndDate           string   `json:"end_date"`
	FlowKeys          []string `json:"flow_keys"`
	MaxInstances      *int     `json:"max_instances"`
	PaddingDays       *int     `json:"padding_days"`
	ProcessLeave      string   `json:"process_leave"`
	ProcessOvertime   string   `json:"process_overtime"`
	ProcessCorrection string   `json:"process_attendance_correction"`
	ProcessTransfer   string   `json:"process_position_transfer"`
}

type DingtalkSyncResult struct {
	Outputs []AttendanceToolboxResult
	ZipData []byte
}

func firstNonEmptyTrim(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *AttendanceToolboxService) RunDingtalkSync(ctx context.Context, req *DingtalkSyncRequest) (*DingtalkSyncResult, error) {
	if req.StartDate == "" || req.EndDate == "" {
		return nil, errors.New("请提供同步开始日期和结束日期")
	}
	if s.engineDir == "" {
		return nil, errors.New("未找到考勤工具箱 Python 引擎目录")
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if _, err := os.Stat(runnerPath); err != nil {
		return nil, fmt.Errorf("未找到考勤工具箱 runner：%w", err)
	}

	workdir, err := os.MkdirTemp("", "peopleops-dingtalk-sync-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	config := map[string]interface{}{
		"dingtalk_sync_start_date": req.StartDate,
		"dingtalk_sync_end_date":   req.EndDate,
	}
	if len(req.FlowKeys) > 0 {
		config["dingtalk_sync_flow_keys"] = req.FlowKeys
	}
	if req.MaxInstances != nil {
		config["dingtalk_sync_max_instances"] = *req.MaxInstances
	}
	if req.PaddingDays != nil {
		config["dingtalk_sync_padding_days"] = *req.PaddingDays
	}
	// DingTalk credentials
	dingtalkConfig := map[string]string{}
	if clientID := strings.TrimSpace(os.Getenv("DINGTALK_APP_KEY")); clientID != "" {
		dingtalkConfig["client_id"] = clientID
	}
	if clientSecret := strings.TrimSpace(os.Getenv("DINGTALK_APP_SECRET")); clientSecret != "" {
		dingtalkConfig["client_secret"] = clientSecret
	}
	if processLeave := firstNonEmptyTrim(req.ProcessLeave, os.Getenv("DINGTALK_PROCESS_LEAVE")); processLeave != "" {
		dingtalkConfig["process_leave"] = processLeave
	}
	if processOvertime := firstNonEmptyTrim(req.ProcessOvertime, os.Getenv("DINGTALK_PROCESS_OVERTIME")); processOvertime != "" {
		dingtalkConfig["process_overtime"] = processOvertime
	}
	if processCorrection := firstNonEmptyTrim(req.ProcessCorrection, os.Getenv("DINGTALK_PROCESS_ATTENDANCE_CORRECTION")); processCorrection != "" {
		dingtalkConfig["process_attendance_correction"] = processCorrection
	}
	if processTransfer := firstNonEmptyTrim(req.ProcessTransfer, os.Getenv("DINGTALK_PROCESS_POSITION_TRANSFER")); processTransfer != "" {
		dingtalkConfig["process_position_transfer"] = processTransfer
	}
	if len(dingtalkConfig) > 0 {
		config["dingtalk"] = dingtalkConfig
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	outputDir := filepath.Join(workdir, "outputs")
	_ = os.MkdirAll(outputDir, 0o755)

	cmd := s.newRunnerCommand(runCtx, runnerPath, "--module", "dingtalk_sync", "--workdir", workdir, "--config-json", string(configJSON))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, errors.New("钉钉同步超时，请缩小日期范围后重试")
	}

	var runner attendanceToolboxRunnerResult
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); jsonErr != nil {
		if err != nil {
			return nil, fmt.Errorf("钉钉同步失败：%v\n%s", err, stderr.String())
		}
		return nil, fmt.Errorf("钉钉同步返回格式异常：%w\n%s", jsonErr, stdout.String())
	}
	if err != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		return nil, fmt.Errorf("钉钉同步失败：%s", message)
	}
	if len(runner.Outputs) == 0 {
		return nil, errors.New("钉钉同步未生成结果文件")
	}

	result := &DingtalkSyncResult{}
	for _, output := range runner.Outputs {
		data, readErr := readAttendanceToolboxOutput(workdir, output.Path)
		if readErr != nil {
			return nil, readErr
		}
		fileName := strings.TrimSpace(output.FileName)
		if fileName == "" {
			fileName = "钉钉同步结果.xlsx"
		}
		result.Outputs = append(result.Outputs, AttendanceToolboxResult{
			FileName:    fileName,
			ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Data:        data,
		})
	}

	if len(result.Outputs) > 1 {
		archive, zipErr := zipAttendanceToolboxOutputs(workdir, runner.Outputs)
		if zipErr != nil {
			return nil, zipErr
		}
		result.ZipData = archive
	}

	return result, nil
}

func (s *AttendanceToolboxService) newRunnerCommand(ctx context.Context, runnerPath string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{runnerPath}, args...)
	cmd := exec.CommandContext(ctx, s.pythonBin, cmdArgs...)
	cmd.Dir = s.engineDir
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	return cmd
}

func attendanceToolboxSpecs() map[string]attendanceToolboxModuleSpec {
	return map[string]attendanceToolboxModuleSpec{
		"leave": {
			SingleFiles: map[string]string{
				"leave_src":              "leave_src",
				"leave_schedule":         "leave_schedule",
				"leave_offsite_duration": "leave_offsite_duration",
			},
			TextFields: []string{"leave_special_names", "chengdu_schedule_names"},
		},
		"overtime": {
			SingleFiles: map[string]string{
				"overtime_src":        "overtime_src",
				"overtime_attendance": "overtime_attendance",
				"overtime_calendar":   "overtime_calendar",
				"overtime_roster":     "overtime_roster",
			},
			MultiFiles: map[string]string{
				"overtime_schedules": "overtime_schedules",
			},
			TextFields: []string{"chengdu_schedule_names"},
		},
		"subsidy": {
			SingleFiles: map[string]string{
				"subsidy_src":               "subsidy_src",
				"subsidy_checkin":           "subsidy_checkin",
				"subsidy_attendance":        "subsidy_attendance",
				"subsidy_attendance_result": "subsidy_attendance_result",
				"subsidy_schedule":          "subsidy_schedule",
			},
			TextFields: []string{"sub_dept_keywords", "sub_late22_names"},
		},
		"final": {
			SingleFiles: map[string]string{
				"final_active":   "final_active",
				"final_resign":   "final_resign",
				"final_transfer": "final_transfer",
				"final_schedule": "final_schedule",
				"final_leave":    "final_leave",
				"final_overtime": "final_overtime",
				"final_subsidy":  "final_subsidy",
			},
			TextFields: []string{"chengdu_schedule_names"},
		},
		"parttime": {
			SingleFiles: map[string]string{
				"parttime_default_schedule":  "parttime_default_schedule",
				"parttime_attendance_detail": "parttime_attendance_detail",
			},
			MultiFiles: map[string]string{
				"parttime_monthly":   "parttime_monthly",
				"parttime_schedules": "parttime_schedules",
			},
			TextFields: []string{"part_special_names"},
		},
		"dingtalk_sync": {
			TextFields: []string{
				"dingtalk_sync_start_date",
				"dingtalk_sync_end_date",
				"dingtalk_sync_flow_keys",
				"dingtalk_sync_max_instances",
				"dingtalk_sync_padding_days",
				"dingtalk_client_id",
				"dingtalk_client_secret",
				"dingtalk_process_leave",
				"dingtalk_process_overtime",
				"dingtalk_process_attendance_correction",
				"dingtalk_process_position_transfer",
			},
		},
	}
}

func saveAttendanceToolboxFiles(workdir string, form *multipart.Form, spec attendanceToolboxModuleSpec, config map[string]interface{}) error {
	for formField, configKey := range spec.SingleFiles {
		files := form.File[formField]
		if len(files) == 0 {
			continue
		}
		path, err := saveAttendanceToolboxFile(workdir, formField, 0, files[0])
		if err != nil {
			return err
		}
		config[configKey] = path
	}
	for formField, configKey := range spec.MultiFiles {
		files := form.File[formField]
		saved := make([]string, 0, len(files))
		for idx, file := range files {
			path, err := saveAttendanceToolboxFile(workdir, formField, idx, file)
			if err != nil {
				return err
			}
			saved = append(saved, path)
		}
		config[configKey] = saved
	}
	return nil
}

func saveAttendanceToolboxFile(workdir, field string, idx int, file *multipart.FileHeader) (string, error) {
	if file == nil {
		return "", errors.New("上传文件为空")
	}
	if file.Size <= 0 {
		return "", fmt.Errorf("%s 文件为空", file.Filename)
	}
	if file.Size > attendanceToolboxMaxUploadBytes {
		return "", fmt.Errorf("%s 超过 500MB", file.Filename)
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".xlsx" && ext != ".xls" {
		return "", fmt.Errorf("%s 不是支持的 Excel 文件", file.Filename)
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = src.Close()
	}()

	name := fmt.Sprintf("%s_%d%s", field, idx+1, ext)
	dstPath := filepath.Join(workdir, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return dstPath, nil
}

func readAttendanceToolboxOutput(workdir, outputPath string) ([]byte, error) {
	if !isPathWithin(workdir, outputPath) {
		return nil, errors.New("考勤工具箱输出路径不安全")
	}
	return os.ReadFile(outputPath)
}

func zipAttendanceToolboxOutputs(workdir string, outputs []struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
}) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for idx, output := range outputs {
		data, err := readAttendanceToolboxOutput(workdir, output.Path)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		name := strings.TrimSpace(output.FileName)
		if name == "" {
			name = fmt.Sprintf("结果_%d.xlsx", idx+1)
		}
		writer, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isPathWithin(parent, child string) bool {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func resolveAttendanceToolboxEngineDir() string {
	if configured := strings.TrimSpace(os.Getenv("ATTENDANCE_TOOLBOX_DIR")); configured != "" {
		return configured
	}
	candidates := []string{
		filepath.Join("tools", "attendance_toolbox", "python"),
		filepath.Join("..", "tools", "attendance_toolbox", "python"),
		filepath.Join("..", "..", "tools", "attendance_toolbox", "python"),
		filepath.Join("attendance_toolbox", "python"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "runner.py")); err == nil {
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

func resolveAttendanceToolboxPython() string {
	if configured := strings.TrimSpace(os.Getenv("ATTENDANCE_TOOLBOX_PYTHON")); configured != "" {
		return configured
	}
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

func attendanceToolboxZipName(module string) string {
	switch module {
	case "subsidy":
		return "补贴扣款结果.zip"
	default:
		return "考勤工具箱结果.zip"
	}
}

func ContentDispositionAttachment(fileName string) string {
	escaped := url.QueryEscape(fileName)
	return fmt.Sprintf("attachment; filename=\"download\"; filename*=UTF-8''%s", escaped)
}

// ── Action: runAction (generic) ──────────────────────────────────────────────

type AttendanceToolboxActionResult struct {
	Outputs []AttendanceToolboxResult
	Log     string
}

func (s *AttendanceToolboxService) runAction(ctx context.Context, action string, config map[string]interface{}) (*AttendanceToolboxActionResult, error) {
	if s.engineDir == "" {
		return nil, errors.New("未找到考勤工具箱 Python 引擎目录")
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if _, err := os.Stat(runnerPath); err != nil {
		return nil, fmt.Errorf("未找到考勤工具箱 runner：%w", err)
	}

	workdir, err := os.MkdirTemp("", "peopleops-attendance-action-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := s.newRunnerCommand(runCtx, runnerPath, "--action", action, "--workdir", workdir, "--config-json", string(configJSON))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("考勤工具箱 %s 操作超时", action)
	}

	var runner attendanceToolboxRunnerResult
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); jsonErr != nil {
		if err != nil {
			return nil, fmt.Errorf("考勤工具箱 %s 失败：%v\n%s", action, err, stderr.String())
		}
		return nil, fmt.Errorf("考勤工具箱 %s 返回格式异常：%w\n%s", action, jsonErr, stdout.String())
	}
	if err != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		return nil, fmt.Errorf("考勤工具箱 %s 失败：%s", action, message)
	}

	result := &AttendanceToolboxActionResult{Log: runner.Log}
	for _, output := range runner.Outputs {
		data, readErr := readAttendanceToolboxOutput(workdir, output.Path)
		if readErr != nil {
			return nil, readErr
		}
		fileName := strings.TrimSpace(output.FileName)
		if fileName == "" {
			fileName = "result"
		}
		result.Outputs = append(result.Outputs, AttendanceToolboxResult{
			FileName:    fileName,
			ContentType: "application/json",
			Data:        data,
		})
	}
	return result, nil
}

// ── Action: ExportRules ──────────────────────────────────────────────────────

func (s *AttendanceToolboxService) ExportRules(ctx context.Context, rulesJSON string) (*AttendanceToolboxResult, error) {
	config := map[string]interface{}{}
	if strings.TrimSpace(rulesJSON) != "" {
		config["rules_json"] = rulesJSON
	}
	result, err := s.runAction(ctx, "export-rules", config)
	if err != nil {
		return nil, err
	}
	if len(result.Outputs) == 0 {
		return nil, errors.New("未生成规则配置文件")
	}
	output := result.Outputs[0]
	return &AttendanceToolboxResult{
		FileName:    output.FileName,
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Data:        output.Data,
	}, nil
}

// ── Action: ExportTemplates ───────────────────────────────────────────────────

type AttendanceToolboxExportTemplateResult struct {
	FileName    string
	ContentType string
	Data        []byte
	Meta        map[string]interface{}
	MetaOnly    bool
}

func attendanceToolboxTemplatesZipName() string {
	return "考勤工具箱模板.zip"
}

// ExportTemplates builds a zip of blank Excel templates used by each module
// in the attendance toolbox UI.  When templateID is non-empty, only that
// single template is returned; otherwise the full 16-template zip is returned.
// When metaOnly is requested, the returned structure contains “Meta“ (the
// template catalogue) and no binary blob, so the Go handler can serve JSON.
func (s *AttendanceToolboxService) ExportTemplates(ctx context.Context, templateID string) (*AttendanceToolboxExportTemplateResult, error) {
	cfg := map[string]interface{}{
		"templates_meta": "true",
	}
	if strings.TrimSpace(templateID) != "" {
		cfg["template_id"] = strings.TrimSpace(templateID)
	}

	result, err := s.runAction(ctx, "export-templates", cfg)
	if err != nil {
		return nil, err
	}
	if len(result.Outputs) == 0 {
		return nil, errors.New("未生成模板文件")
	}

	var meta map[string]interface{}
	var blobs []AttendanceToolboxResult
	for _, out := range result.Outputs {
		if strings.HasSuffix(out.FileName, ".json") {
			_ = json.Unmarshal(out.Data, &meta)
			continue
		}
		blobs = append(blobs, out)
	}

	// Single template requested → return it directly.
	if strings.TrimSpace(templateID) != "" && len(blobs) == 1 {
		return &AttendanceToolboxExportTemplateResult{
			FileName:    blobs[0].FileName,
			ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Data:        blobs[0].Data,
			Meta:        meta,
		}, nil
	}

	if meta != nil {
		metaBytes, _ := json.Marshal(meta)
		blobs = append(blobs, AttendanceToolboxResult{
			FileName: "templates_meta.json",
			Data:     metaBytes,
		})
	}

	zipBytes, err := s.zipInMemory(blobs)
	if err != nil {
		return nil, err
	}
	return &AttendanceToolboxExportTemplateResult{
		FileName:    attendanceToolboxTemplatesZipName(),
		ContentType: "application/zip",
		Data:        zipBytes,
		Meta:        meta,
	}, nil
}

func (s *AttendanceToolboxService) zipInMemory(entries []AttendanceToolboxResult) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for idx, ent := range entries {
		name := strings.TrimSpace(ent.FileName)
		if name == "" {
			name = fmt.Sprintf("模板_%d.xlsx", idx+1)
		}
		writer, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := writer.Write(ent.Data); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Action: AuditUploads ─────────────────────────────────────────────────────

type AttendanceToolboxAuditRequest struct {
	Files       []map[string]interface{}
	Form        *multipart.Form
	MaxWarnMB   int
	MaxWarnRows int
	MaxWarnCols int
}

type AttendanceToolboxAuditResult struct {
	Warnings []string                 `json:"warnings"`
	Audit    []map[string]interface{} `json:"audit"`
	Files    []map[string]interface{} `json:"files"`
	Meta     map[string]interface{}   `json:"meta"`
}

// AuditUploads pre-scans the uploaded Excel files with the Python
// “excel_compat.audit_upload“ helper, returning structured warnings and an
// audit log that the React UI can surface inline.
func (s *AttendanceToolboxService) AuditUploads(ctx context.Context, req AttendanceToolboxAuditRequest) (*AttendanceToolboxAuditResult, error) {
	if s.engineDir == "" {
		return nil, errors.New("未找到考勤工具箱 Python 引擎目录")
	}
	runnerPath := filepath.Join(s.engineDir, "runner.py")
	if _, err := os.Stat(runnerPath); err != nil {
		return nil, fmt.Errorf("未找到考勤工具箱 runner：%w", err)
	}

	workdir, err := os.MkdirTemp("", "peopleops-attendance-audit-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	cfg := map[string]interface{}{
		"files":         req.Files,
		"max_warn_mb":   req.MaxWarnMB,
		"max_warn_rows": req.MaxWarnRows,
		"max_warn_cols": req.MaxWarnCols,
		"scan_content":  true,
	}

	// When the multipart form is supplied, persist the uploaded files to the
	// workdir and add per-file ``path`` entries so the Python audit action can
	// inspect the real bytes on disk.
	if req.Form != nil {
		// Track a field-specific index so multiple files sharing the same
		// form field name can be matched back to their saved location.
		counter := map[string]int{}
		augmentedFiles := make([]map[string]interface{}, 0, len(req.Files))
		for _, meta := range req.Files {
			field, _ := meta["field"].(string)
			name, _ := meta["name"].(string)
			aug := map[string]interface{}{
				"name":  name,
				"size":  meta["size"],
				"field": field,
			}
			if field != "" && name != "" {
				idx := counter[field]
				counter[field] = idx + 1
				savedPath, err := s.saveAuditFile(workdir, field, idx, name, req.Form)
				if err == nil && savedPath != "" {
					aug["path"] = savedPath
				}
			}
			augmentedFiles = append(augmentedFiles, aug)
		}
		cfg["files"] = augmentedFiles
	}

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := s.newRunnerCommand(runCtx, runnerPath, "--action", "audit", "--workdir", workdir, "--config-json", string(cfgJSON))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && runCtx.Err() == context.DeadlineExceeded {
		return nil, errors.New("考勤工具箱审计超时")
	}

	var runner attendanceToolboxRunnerResult
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &runner); jsonErr != nil {
		if err != nil {
			return nil, fmt.Errorf("考勤工具箱审计失败：%v\n%s", err, stderr.String())
		}
		return nil, fmt.Errorf("考勤工具箱审计返回格式异常：%w\n%s", jsonErr, stdout.String())
	}
	if err != nil || !runner.OK {
		message := strings.TrimSpace(runner.Error)
		if message == "" {
			message = strings.TrimSpace(stderr.String())
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		return nil, fmt.Errorf("考勤工具箱审计失败：%s", message)
	}

	if len(runner.Outputs) == 0 {
		return &AttendanceToolboxAuditResult{}, nil
	}

	var auditPayload struct {
		OK       bool                     `json:"ok"`
		Warnings []string                 `json:"warnings"`
		Audit    []map[string]interface{} `json:"audit"`
		Files    []map[string]interface{} `json:"files"`
		Meta     map[string]interface{}   `json:"meta"`
	}
	for _, out := range runner.Outputs {
		if !strings.HasSuffix(out.FileName, ".json") {
			continue
		}
		raw, err := readAttendanceToolboxOutput(workdir, out.Path)
		if err != nil {
			continue
		}
		var parsed struct {
			OK       bool                     `json:"ok"`
			Warnings []string                 `json:"warnings"`
			Audit    []map[string]interface{} `json:"audit"`
			Files    []map[string]interface{} `json:"files"`
			Meta     map[string]interface{}   `json:"meta"`
		}
		if jsonErr := json.Unmarshal(raw, &parsed); jsonErr == nil {
			auditPayload = parsed
		}
	}

	return &AttendanceToolboxAuditResult{
		Warnings: auditPayload.Warnings,
		Audit:    auditPayload.Audit,
		Files:    auditPayload.Files,
		Meta:     auditPayload.Meta,
	}, nil
}

func (s *AttendanceToolboxService) saveAuditFile(workdir string, field string, idx int, name string, form *multipart.Form) (string, error) {
	files := form.File[field]
	if idx < 0 || idx >= len(files) {
		return "", errors.New("file index out of range")
	}
	file := files[idx]
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = src.Close()
	}()

	ext := strings.ToLower(filepath.Ext(name))
	dstPath := filepath.Join(workdir, fmt.Sprintf("audit_%s_%d%s", field, idx+1, ext))
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return dstPath, nil
}

// ── Action: ImportRulesPreview ───────────────────────────────────────────────

func (s *AttendanceToolboxService) ImportRulesPreview(ctx context.Context, rulesFilePath string) (map[string]interface{}, error) {
	result, err := s.runAction(ctx, "import-rules-preview", map[string]interface{}{
		"rules_file": rulesFilePath,
	})
	if err != nil {
		return nil, err
	}
	if len(result.Outputs) == 0 {
		return nil, errors.New("未生成预览数据")
	}
	var preview map[string]interface{}
	if jsonErr := json.Unmarshal(result.Outputs[0].Data, &preview); jsonErr != nil {
		return nil, fmt.Errorf("预览数据格式异常：%w", jsonErr)
	}
	return preview, nil
}

// ── Action: Validate ─────────────────────────────────────────────────────────

func (s *AttendanceToolboxService) Validate(ctx context.Context, module string, form *multipart.Form) (map[string]interface{}, error) {
	spec, ok := attendanceToolboxSpecs()[module]
	if !ok {
		return nil, fmt.Errorf("不支持的模块：%s", module)
	}
	if form == nil {
		return nil, errors.New("请上传文件")
	}

	workdir, err := os.MkdirTemp("", "peopleops-attendance-validate-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	config := map[string]interface{}{
		"validate_module": module,
	}
	if err := saveAttendanceToolboxFiles(workdir, form, spec, config); err != nil {
		return nil, err
	}

	result, err := s.runAction(ctx, "validate", config)
	if err != nil {
		return nil, err
	}
	if len(result.Outputs) == 0 {
		return map[string]interface{}{"ok": true, "results": map[string]interface{}{}}, nil
	}
	var validation map[string]interface{}
	if jsonErr := json.Unmarshal(result.Outputs[0].Data, &validation); jsonErr != nil {
		return nil, fmt.Errorf("校验结果格式异常：%w", jsonErr)
	}
	return validation, nil
}

// ── Action: Preview ──────────────────────────────────────────────────────────

func (s *AttendanceToolboxService) Preview(ctx context.Context, module string, form *multipart.Form) (*AttendanceToolboxActionResult, error) {
	spec, ok := attendanceToolboxSpecs()[module]
	if !ok {
		return nil, fmt.Errorf("不支持的模块：%s", module)
	}
	if form == nil {
		return nil, errors.New("请上传文件")
	}

	workdir, err := os.MkdirTemp("", "peopleops-attendance-preview-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	config := map[string]interface{}{
		"preview_module": module,
	}
	if err := saveAttendanceToolboxFiles(workdir, form, spec, config); err != nil {
		return nil, err
	}
	for _, field := range spec.TextFields {
		if values, ok := form.Value[field]; ok && len(values) > 0 {
			config[field] = values
		}
	}

	return s.runAction(ctx, "preview", config)
}
