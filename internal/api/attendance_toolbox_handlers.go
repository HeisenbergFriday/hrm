package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"peopleops/internal/service"

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
	module := c.Param("module")
	if err := c.Request.ParseMultipartForm(attendanceToolboxMultipartMemory); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "上传文件解析失败",
		})
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
	var req struct {
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
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请求参数解析失败",
		})
		return
	}

	result, err := service.NewAttendanceToolboxService().RunDingtalkSync(c.Request.Context(), &service.DingtalkSyncRequest{
		StartDate:         req.StartDate,
		EndDate:           req.EndDate,
		FlowKeys:          req.FlowKeys,
		MaxInstances:      req.MaxInstances,
		PaddingDays:       req.PaddingDays,
		ProcessLeave:      req.ProcessLeave,
		ProcessOvertime:   req.ProcessOvertime,
		ProcessCorrection: req.ProcessCorrection,
		ProcessTransfer:   req.ProcessTransfer,
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

// ── New Action Handlers ──────────────────────────────────────────────────────

func ExportOvertimeRules(c *gin.Context) {
	result, err := service.NewAttendanceToolboxService().ExportRules(c.Request.Context())
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
