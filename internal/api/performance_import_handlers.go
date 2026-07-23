package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const performanceActivityImportUploadMaxBytes = 10 * 1024 * 1024

func AnalyzePerformanceActivityImport(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请上传绩效 Excel 文件", Data: nil})
		return
	}
	if !strings.EqualFold(filepath.Ext(file.Filename), ".xlsx") {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "仅支持 .xlsx Excel 文件", Data: nil})
		return
	}
	if file.Size <= 0 || file.Size > performanceActivityImportUploadMaxBytes {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "Excel 文件不能为空且不能超过 10MB", Data: nil})
		return
	}
	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "读取上传文件失败", Data: gin.H{"error": err.Error()}})
		return
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			logrus.Warnf("close performance activity import file failed: %v", closeErr)
		}
	}()

	svc := service.NewPerformanceService(middleware.RequestDB(c))
	batch, err := svc.AnalyzePerformanceActivityImport(file.Filename, reader, currentOperatorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "识别完成，请确认预览内容", Data: batch})
}

func GetPerformanceActivityImportBatch(c *gin.Context) {
	svc := service.NewPerformanceService(middleware.RequestDB(c))
	batch, err := svc.GetPerformanceActivityImportBatch(c.Param("batch_id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "导入批次不存在", Data: nil})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "读取导入批次失败", Data: gin.H{"error": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: batch})
}

func CommitPerformanceActivityImport(c *gin.Context) {
	var req service.PerformanceActivityImportCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "提交参数格式错误", Data: gin.H{"error": err.Error()}})
		return
	}
	svc := service.NewPerformanceService(middleware.RequestDB(c))
	result, err := svc.CommitPerformanceActivityImport(c.Param("batch_id"), req, currentOperatorID(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "导入批次不存在", Data: nil})
			return
		}
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: err.Error(), Data: nil})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "绩效模板和草稿活动已创建", Data: result})
}
