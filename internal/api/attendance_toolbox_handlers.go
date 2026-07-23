package api

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"peopleops/internal/middleware"
	"peopleops/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

const attendanceToolboxMultipartMemory = 64 << 20

func GetAttendanceToolboxDefaults(c *gin.Context) {
	defaults, err := service.NewAttendanceToolboxService().Defaults(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    defaults,
	})
}

func RunAttendanceToolbox(c *gin.Context) {
	module := strings.TrimSpace(c.Param("module"))
	if !service.IsAttendanceToolboxStandardModule(module) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "不支持的考勤工具箱模块",
		})
		return
	}
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "上传文件解析失败",
		})
		return
	}
	if formHasCustomRules(c) && !toolboxHasPermission(c, "attendance_toolbox_rules_edit") {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "你缺少考勤工具箱规则编辑权限，需要联系管理员添加"})
		return
	}

	result, err := service.NewAttendanceToolboxService().Run(c.Request.Context(), module, c.Request.MultipartForm)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	c.Header("Content-Disposition", service.ContentDispositionAttachment(result.FileName))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, result.ContentType, result.Data)
}

func RunDingtalkSync(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req struct {
		StartDate    string   `json:"start_date"`
		EndDate      string   `json:"end_date"`
		FlowKeys     []string `json:"flow_keys"`
		MaxInstances *int     `json:"max_instances"`
		PaddingDays  *int     `json:"padding_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请求参数解析失败",
		})
		return
	}

	result, err := service.NewAttendanceToolboxService().RunDingtalkSyncForOrg(c.Request.Context(), orgID, &service.DingtalkSyncRequest{
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		FlowKeys:     req.FlowKeys,
		MaxInstances: req.MaxInstances,
		PaddingDays:  req.PaddingDays,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	if len(result.Outputs) == 1 {
		output := result.Outputs[0]
		c.Header("Content-Disposition", service.ContentDispositionAttachment(output.FileName))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", output.Data)
		return
	}

	c.Header("Content-Disposition", service.ContentDispositionAttachment("钉钉同步结果.zip"))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/zip", result.ZipData)
}

// RunAttendanceToolboxWorkflow handles standard modules only (leave/overtime/...).
// quick and dingtalk_sync have dedicated handlers with dedicated permissions.
func RunAttendanceToolboxWorkflow(c *gin.Context) {
	module := strings.TrimSpace(c.Param("module"))
	if !service.IsAttendanceToolboxStandardModule(module) {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "不支持的考勤工具箱模块；quick/dingtalk_sync 请使用专用路由"})
		return
	}
	if formHasCustomRules(c) && !toolboxHasPermission(c, "attendance_toolbox_rules_edit") {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "你缺少考勤工具箱规则编辑权限，需要联系管理员添加"})
		return
	}
	runToolboxWorkflow(c, module)
}

// RunAttendanceToolboxQuickWorkflow: operate + dingtalk_sync; rules_edit if custom rules present.
func RunAttendanceToolboxQuickWorkflow(c *gin.Context) {
	if formHasCustomRules(c) && !toolboxHasPermission(c, "attendance_toolbox_rules_edit") {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "你缺少考勤工具箱规则编辑权限，需要联系管理员添加"})
		return
	}
	// Auto-merge leave/overtime into flow_keys when corresponding run flags are set.
	if c.Request.MultipartForm != nil {
		ensureQuickFlowKeys(c.Request.MultipartForm)
	}
	runToolboxWorkflow(c, "quick")
}

// RunAttendanceToolboxDingtalkSyncWorkflow is the structured dingtalk sync endpoint.
func RunAttendanceToolboxDingtalkSyncWorkflow(c *gin.Context) {
	runToolboxWorkflow(c, "dingtalk_sync")
}

func runToolboxWorkflow(c *gin.Context, module string) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, Response{Code: http.StatusUnauthorized, Message: "not logged in"})
		return
	}
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		if module != "dingtalk_sync" {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "上传文件解析失败"})
			return
		}
	}

	extra := map[string]interface{}{}
	if module == "dingtalk_sync" && c.Request.MultipartForm == nil {
		var req struct {
			StartDate    string   `json:"start_date"`
			EndDate      string   `json:"end_date"`
			FlowKeys     []string `json:"flow_keys"`
			MaxInstances *int     `json:"max_instances"`
			PaddingDays  *int     `json:"padding_days"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请求参数解析失败"})
			return
		}
		extra["dingtalk_sync_start_date"] = req.StartDate
		extra["dingtalk_sync_end_date"] = req.EndDate
		if len(req.FlowKeys) > 0 {
			extra["dingtalk_sync_flow_keys"] = req.FlowKeys
		}
		if req.MaxInstances != nil {
			extra["dingtalk_sync_max_instances"] = *req.MaxInstances
		}
		if req.PaddingDays != nil {
			extra["dingtalk_sync_padding_days"] = *req.PaddingDays
		}
	}
	if module == "quick" && c.Request.MultipartForm != nil {
		ensureQuickFlowKeys(c.Request.MultipartForm)
	}

	result, err := service.NewAttendanceToolboxService().RunStructured(c.Request.Context(), userID, orgID, module, c.Request.MultipartForm, extra)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: result})
}

func formHasCustomRules(c *gin.Context) bool {
	if c.Request.MultipartForm == nil {
		_ = c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory)
	}
	form := c.Request.MultipartForm
	if form == nil {
		return false
	}
	fileCounts := map[string]int{}
	for k, v := range form.File {
		fileCounts[k] = len(v)
	}
	return service.MapHasCustomRules(form.Value, fileCounts)
}

func toolboxHasPermission(c *gin.Context, code string) bool {
	ok, err := middleware.HasAnyPermission(c, code, "attendance_manage")
	return err == nil && ok
}

func ensureQuickFlowKeys(form *multipart.Form) {
	if form == nil {
		return
	}
	runLeave := true
	runOT := true
	if vals := form.Value["run_leave"]; len(vals) > 0 {
		v := strings.ToLower(strings.TrimSpace(vals[0]))
		runLeave = v != "0" && v != "false" && v != "no"
	}
	if vals := form.Value["run_overtime"]; len(vals) > 0 {
		v := strings.ToLower(strings.TrimSpace(vals[0]))
		runOT = v != "0" && v != "false" && v != "no"
	}
	keys := []string{}
	if vals := form.Value["dingtalk_sync_flow_keys"]; len(vals) > 0 {
		for _, part := range strings.Split(vals[0], ",") {
			if p := strings.TrimSpace(part); p != "" {
				keys = append(keys, p)
			}
		}
	}
	has := func(k string) bool {
		for _, x := range keys {
			if x == k {
				return true
			}
		}
		return false
	}
	if runLeave && !has("leave") {
		keys = append(keys, "leave")
	}
	if runOT && !has("overtime") {
		keys = append(keys, "overtime")
	}
	if len(keys) > 0 {
		form.Value["dingtalk_sync_flow_keys"] = []string{strings.Join(keys, ",")}
	}
}

func GetAttendanceToolboxRun(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	runID := strings.TrimSpace(c.Param("run_id"))
	run, err := service.DefaultAttendanceToolboxRunStore().Get(runID, userID, orgID)
	if err != nil {
		writeToolboxRunError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: service.AttendanceToolboxRunResponse{
			RunID:   run.RunID,
			Module:  run.Module,
			Log:     run.Log,
			Stats:   run.Stats,
			Meta:    run.Meta,
			Files:   run.Files,
			Expires: run.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	})
}

func DownloadAttendanceToolboxRunFile(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	runID := strings.TrimSpace(c.Param("run_id"))
	fileKey := strings.TrimSpace(c.Param("file_key"))
	fileName, contentType, data, err := service.DefaultAttendanceToolboxRunStore().ReadFile(runID, fileKey, userID, orgID)
	if err != nil {
		writeToolboxRunError(c, err)
		return
	}
	c.Header("Content-Disposition", service.ContentDispositionAttachment(fileName))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, data)
}

func DownloadAttendanceToolboxRunZip(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	runID := strings.TrimSpace(c.Param("run_id"))
	files, err := service.DefaultAttendanceToolboxRunStore().ReadAllDownloadable(runID, userID, orgID)
	if err != nil {
		writeToolboxRunError(c, err)
		return
	}
	archive, err := service.ZipAttendanceToolboxResultsForDownload(files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: err.Error()})
		return
	}
	c.Header("Content-Disposition", service.ContentDispositionAttachment("考勤工具箱结果.zip"))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/zip", archive)
}

// PreviewAttendanceToolboxRun returns first 200 rows of a stored result without re-running calculation.
func PreviewAttendanceToolboxRun(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	runID := strings.TrimSpace(c.Param("run_id"))
	fileKey := strings.TrimSpace(c.Query("file_key"))
	preview, err := service.NewAttendanceToolboxService().PreviewStoredRun(c.Request.Context(), runID, fileKey, userID, orgID)
	if err != nil {
		writeToolboxRunError(c, err)
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: preview})
}

func writeToolboxRunError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrToolboxRunExpired):
		c.JSON(http.StatusGone, Response{Code: http.StatusGone, Message: "结果已过期，请重新计算"})
	case errors.Is(err, service.ErrToolboxRunDenied):
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "无权访问该结果"})
	case errors.Is(err, service.ErrToolboxRunNotFound), errors.Is(err, service.ErrToolboxFileMissing):
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "结果不存在"})
	default:
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error()})
	}
}

// ── New Action Handlers ──────────────────────────────────────────────────────

func ExportOvertimeRules(c *gin.Context) {
	// Optional: current session rules_json from form or JSON body.
	rulesJSON := ""
	_ = c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory)
	if c.Request.MultipartForm != nil {
		if vals := c.Request.MultipartForm.Value["rules_json"]; len(vals) > 0 {
			rulesJSON = strings.TrimSpace(vals[0])
		}
	}
	if rulesJSON == "" {
		var body struct {
			RulesJSON string `json:"rules_json"`
		}
		_ = c.ShouldBindJSON(&body)
		rulesJSON = strings.TrimSpace(body.RulesJSON)
	}
	if rulesJSON != "" && !toolboxHasPermission(c, "attendance_toolbox_rules_edit") {
		c.JSON(http.StatusForbidden, Response{Code: http.StatusForbidden, Message: "你缺少考勤工具箱规则编辑权限，需要联系管理员添加"})
		return
	}
	result, err := service.NewAttendanceToolboxService().ExportRules(c.Request.Context(), rulesJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	c.Header("Content-Disposition", service.ContentDispositionAttachment(result.FileName))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, result.ContentType, result.Data)
}

func ImportOvertimeRulesPreview(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "上传文件解析失败",
		})
		return
	}

	// Get the uploaded rules file
	files := c.Request.MultipartForm.File["rules_file"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请上传规则配置文件",
		})
		return
	}

	// Save to temp file for Python to read
	workdir, err := os.MkdirTemp("", "peopleops-rules-import-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建临时目录失败",
		})
		return
	}
	defer func() {
		_ = os.RemoveAll(workdir)
	}()

	src, err := files[0].Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "读取上传文件失败",
		})
		return
	}
	defer func() {
		_ = src.Close()
	}()

	rulesPath := filepath.Join(workdir, "rules.xlsx")
	dst, err := os.Create(rulesPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存规则文件失败",
		})
		return
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存规则文件失败",
		})
		return
	}
	if err := dst.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存规则文件失败",
		})
		return
	}

	preview, err := service.NewAttendanceToolboxService().ImportRulesPreview(c.Request.Context(), rulesPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    preview,
	})
}

func ExportAttendanceToolboxTemplates(c *gin.Context) {
	var req struct {
		TemplateID string `json:"template_id"`
	}
	_ = c.ShouldBindJSON(&req)

	result, err := service.NewAttendanceToolboxService().ExportTemplates(c.Request.Context(), req.TemplateID)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	if !result.MetaOnly {
		c.Header("Content-Disposition", service.ContentDispositionAttachment(result.FileName))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, result.ContentType, result.Data)
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result.Meta,
	})
}

func AuditAttendanceToolbox(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		// fall back to JSON body containing files metadata
		var req struct {
			Files       []map[string]interface{} `json:"files"`
			MaxWarnMB   int                      `json:"max_warn_mb"`
			MaxWarnRows int                      `json:"max_warn_rows"`
			MaxWarnCols int                      `json:"max_warn_cols"`
		}
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: "上传文件解析失败",
			})
			return
		}
		result, err := service.NewAttendanceToolboxService().AuditUploads(c.Request.Context(), service.AttendanceToolboxAuditRequest{
			Files:       req.Files,
			MaxWarnMB:   req.MaxWarnMB,
			MaxWarnRows: req.MaxWarnRows,
			MaxWarnCols: req.MaxWarnCols,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data:    result,
		})
		return
	}

	// multipart path: collect saved file paths & expose them to the Python audit action
	files := []map[string]interface{}{}
	for field, headers := range c.Request.MultipartForm.File {
		for _, hdr := range headers {
			files = append(files, map[string]interface{}{
				"name":  hdr.Filename,
				"size":  hdr.Size,
				"field": field,
			})
		}
	}
	result, err := service.NewAttendanceToolboxService().AuditUploads(c.Request.Context(), service.AttendanceToolboxAuditRequest{
		Files: files,
		Form:  c.Request.MultipartForm,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

func ValidateAttendanceToolbox(c *gin.Context) {
	module := c.Param("module")
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "上传文件解析失败",
		})
		return
	}

	result, err := service.NewAttendanceToolboxService().Validate(c.Request.Context(), module, c.Request.MultipartForm)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}
