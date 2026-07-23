package api

import (
	"net/http"
	"peopleops/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// Legacy attendance-processing endpoints keep the old multipart field names and
// blob response contract, but now delegate to AttendanceToolboxService/runner
// instead of maintaining a second Python business copy.

const attendanceProcessingMultipartMemory = 64 << 20

func processAttendanceViaToolbox(c *gin.Context, module string) {
	if err := c.Request.ParseMultipartForm(attendanceProcessingMultipartMemory); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "上传文件解析失败"})
		return
	}
	result, err := service.NewAttendanceToolboxService().RunLegacyProcessing(c.Request.Context(), module, c.Request.MultipartForm)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error()})
		return
	}
	c.Header("Content-Disposition", service.ContentDispositionAttachment(result.FileName))
	c.Header("X-Content-Type-Options", "nosniff")
	contentType := result.ContentType
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	c.Data(http.StatusOK, contentType, result.Data)
}

// ProcessLeaveDetail 处理请假明细（兼容旧 /attendance/processing/leave）
func ProcessLeaveDetail(c *gin.Context) {
	processAttendanceViaToolbox(c, "leave")
}

// ProcessOvertimeDetail 处理加班明细（兼容旧接口）
func ProcessOvertimeDetail(c *gin.Context) {
	processAttendanceViaToolbox(c, "overtime")
}

// ProcessOvertimeDetailFull 处理加班明细（带可选文件，兼容旧接口）
func ProcessOvertimeDetailFull(c *gin.Context) {
	processAttendanceViaToolbox(c, "overtime")
}

// ProcessSubsidyCheck 处理补贴扣款核对
func ProcessSubsidyCheck(c *gin.Context) {
	processAttendanceViaToolbox(c, "subsidy")
}

// ProcessFinalTable 处理最终表生成
func ProcessFinalTable(c *gin.Context) {
	processAttendanceViaToolbox(c, "final")
}

// ProcessParttimeSummary 处理兼职汇总
func ProcessParttimeSummary(c *gin.Context) {
	processAttendanceViaToolbox(c, "parttime")
}
