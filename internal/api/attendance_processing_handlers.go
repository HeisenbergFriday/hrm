package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// pythonResult 是 Python CLI 的 JSON 输出结构
type pythonResult struct {
	Status  string `json:"status"`
	Output  string `json:"output"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// workDir 创建临时工作目录，返回 (workDir, outputDir, cleanupFunc)
func workDir(prefix string) (string, string, func(), error) {
	base := filepath.Join("uploads", "attendance-processing")
	ts := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	dir := filepath.Join(base, ts)
	outDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", "", nil, fmt.Errorf("创建工作目录失败: %w", err)
	}
	cleanup := func() { os.RemoveAll(dir) }
	return dir, outDir, cleanup, nil
}

// saveUploadedFiles 保存上传的文件到指定目录，返回保存路径 map
func saveUploadedFiles(c *gin.Context, dir string, fileKeys map[string]string) (map[string]string, error) {
	saved := make(map[string]string)
	for formKey, destName := range fileKeys {
		file, err := c.FormFile(formKey)
		if err != nil {
			return nil, fmt.Errorf("缺少文件: %s", formKey)
		}
		dest := filepath.Join(dir, destName)
		if err := c.SaveUploadedFile(file, dest); err != nil {
			return nil, fmt.Errorf("保存文件失败 %s: %w", formKey, err)
		}
		saved[formKey] = dest
	}
	return saved, nil
}

// attendanceProcessingCLIPath 向上查找 tools/attendance-processing/cli.py，避免工作目录变化导致找不到脚本。
func attendanceProcessingCLIPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "tools", "attendance-processing", "cli.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("未找到 tools/attendance-processing/cli.py")
}

// runPythonCLI 调用 Python CLI 脚本并返回解析后的结果
func runPythonCLI(args []string) (*pythonResult, error) {
	// 确定 Python 可执行路径
	pythonCmd := "python"
	if runtime.GOOS != "windows" {
		pythonCmd = "python3"
	}

	cmd := exec.Command(pythonCmd, args...)
	cmd.Dir = "."

	stdout, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(stdout))
	if err != nil {
		return nil, fmt.Errorf("Python 执行失败: %s %w", output, err)
	}

	var result pythonResult
	jsonLine := output
	if idx := strings.LastIndex(output, "\n"); idx >= 0 {
		jsonLine = strings.TrimSpace(output[idx+1:])
	}
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return nil, fmt.Errorf("解析 Python 输出失败: %w (原始输出: %s)", err, output)
	}
	return &result, nil
}

// processAttendanceData 通用处理流程：上传文件 → 调用 Python → 返回结果文件
func processAttendanceData(c *gin.Context, command string, fileKeys map[string]string) {
	// 1. 创建临时工作目录
	workDirPath, outputDir, cleanup, err := workDir(command)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	defer cleanup()

	// 2. 保存上传的文件
	savedFiles, err := saveUploadedFiles(c, workDirPath, fileKeys)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: err.Error()})
		return
	}

	// 3. 构建 Python 命令参数
	cliPath, err := attendanceProcessingCLIPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	args := []string{cliPath, command}
	for key := range savedFiles {
		args = append(args, "--"+key, savedFiles[key])
	}
	args = append(args, "--output-dir", outputDir)

	// 4. 执行 Python 脚本
	log.Printf("[AttendanceProcessing] 执行: python %v", args)
	result, err := runPythonCLI(args)
	if err != nil {
		log.Printf("[AttendanceProcessing] 错误: %v", err)
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}

	if result.Status != "ok" {
		log.Printf("[AttendanceProcessing] Python 错误: %s", result.Error)
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: result.Error})
		return
	}

	log.Printf("[AttendanceProcessing] 成功: %s (%s)", result.Message, result.Output)

	// 5. 返回输出文件供下载
	c.File(result.Output)
}

// ProcessLeaveDetail 处理请假明细
func ProcessLeaveDetail(c *gin.Context) {
	processAttendanceData(c, "leave", map[string]string{
		"input":    "请假系统导出.xlsx",
		"schedule": "作息表.xlsx",
	})
}

// ProcessOvertimeDetail 处理加班明细
func ProcessOvertimeDetail(c *gin.Context) {
	processAttendanceData(c, "overtime", map[string]string{
		"input": "加班系统导出.xlsx",
	})
}

// ProcessOvertimeDetailFull 处理加班明细（带可选文件）
func ProcessOvertimeDetailFull(c *gin.Context) {
	// 先保存必选文件
	workDirPath, outputDir, cleanup, err := workDir("overtime")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	defer cleanup()

	// 必选文件
	inputFile, err := c.FormFile("input")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "缺少文件: input"})
		return
	}
	inputPath := filepath.Join(workDirPath, "加班系统导出.xlsx")
	if err := c.SaveUploadedFile(inputFile, inputPath); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: "保存文件失败"})
		return
	}

	cliPath, err := attendanceProcessingCLIPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	args := []string{cliPath, "overtime", "--input", inputPath, "--output-dir", outputDir}

	// 可选文件
	optionalFiles := map[string]string{
		"schedule":   "排班表.xlsx",
		"attendance": "考勤打卡明细.xlsx",
		"roster":     "花名册.xlsx",
	}
	for formKey, destName := range optionalFiles {
		if f, err := c.FormFile(formKey); err == nil {
			dest := filepath.Join(workDirPath, destName)
			if err := c.SaveUploadedFile(f, dest); err == nil {
				args = append(args, "--"+formKey, dest)
			}
		}
	}

	log.Printf("[AttendanceProcessing] 执行: python %v", args)
	result, err := runPythonCLI(args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	if result.Status != "ok" {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: result.Error})
		return
	}

	c.File(result.Output)
}

// ProcessSubsidyCheck 处理补贴扣款核对
func ProcessSubsidyCheck(c *gin.Context) {
	workDirPath, outputDir, cleanup, err := workDir("subsidy")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	defer cleanup()

	// 必选文件
	requiredFiles := map[string]string{
		"source":     "补贴扣款.xlsx",
		"attendance": "考勤.xlsx",
		"schedule":   "作息表.xlsx",
	}
	cliPath, err := attendanceProcessingCLIPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	args := []string{cliPath, "subsidy", "--output-dir", outputDir}

	for formKey, destName := range requiredFiles {
		f, err := c.FormFile(formKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{Code: 400, Message: fmt.Sprintf("缺少文件: %s", formKey)})
			return
		}
		dest := filepath.Join(workDirPath, destName)
		if err := c.SaveUploadedFile(f, dest); err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: "保存文件失败"})
			return
		}
		// 使用 CLI 的实际参数名
		cliKey := formKey
		if formKey == "source" {
			cliKey = "source"
		}
		args = append(args, "--"+cliKey, dest)
	}

	// 可选文件
	for _, opt := range []struct{ key, name string }{
		{"signin", "签到表.xlsx"},
		{"result", "考勤结果表.xlsx"},
	} {
		if f, err := c.FormFile(opt.key); err == nil {
			dest := filepath.Join(workDirPath, opt.name)
			if err := c.SaveUploadedFile(f, dest); err == nil {
				args = append(args, "--"+opt.key, dest)
			}
		}
	}

	log.Printf("[AttendanceProcessing] 执行: python %v", args)
	result, err := runPythonCLI(args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	if result.Status != "ok" {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: result.Error})
		return
	}

	c.File(result.Output)
}

// ProcessFinalTable 处理最终表生成
func ProcessFinalTable(c *gin.Context) {
	workDirPath, outputDir, cleanup, err := workDir("final")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	defer cleanup()

	cliPath, err := attendanceProcessingCLIPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	args := []string{cliPath, "final", "--output-dir", outputDir}

	// 必选文件
	requiredFiles := map[string]string{
		"roster":   "在职花名册.xlsx",
		"schedule": "作息表.xlsx",
		"leave":    "请假明细表.xlsx",
		"overtime": "加班明细_回填.xlsx",
		"subsidy":  "补贴扣款表_核对.xlsx",
	}
	for formKey, destName := range requiredFiles {
		f, err := c.FormFile(formKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{Code: 400, Message: fmt.Sprintf("缺少文件: %s", formKey)})
			return
		}
		dest := filepath.Join(workDirPath, destName)
		if err := c.SaveUploadedFile(f, dest); err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: "保存文件失败"})
			return
		}
		args = append(args, "--"+formKey, dest)
	}

	// 可选文件
	for _, opt := range []struct{ key, name string }{
		{"resigned", "离职花名册.xlsx"},
		{"transfer", "异动流程表.xlsx"},
	} {
		if f, err := c.FormFile(opt.key); err == nil {
			dest := filepath.Join(workDirPath, opt.name)
			if err := c.SaveUploadedFile(f, dest); err == nil {
				args = append(args, "--"+opt.key, dest)
			}
		}
	}

	log.Printf("[AttendanceProcessing] 执行: python %v", args)
	result, err := runPythonCLI(args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	if result.Status != "ok" {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: result.Error})
		return
	}

	c.File(result.Output)
}

// ProcessParttimeSummary 处理兼职汇总
func ProcessParttimeSummary(c *gin.Context) {
	workDirPath, outputDir, cleanup, err := workDir("parttime")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	defer cleanup()

	cliPath, err := attendanceProcessingCLIPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	args := []string{cliPath, "parttime", "--output-dir", outputDir}

	// 可选文件：默认作息表
	if f, err := c.FormFile("default_schedule"); err == nil {
		dest := filepath.Join(workDirPath, "默认作息表.xlsx")
		if err := c.SaveUploadedFile(f, dest); err == nil {
			args = append(args, "--default-schedule", dest)
		}
	}

	// 可选文件：考勤明细
	if f, err := c.FormFile("attendance_detail"); err == nil {
		dest := filepath.Join(workDirPath, "考勤明细.xlsx")
		if err := c.SaveUploadedFile(f, dest); err == nil {
			args = append(args, "--attendance-detail", dest)
		}
	}

	// 可选文件：月度汇总（多个）
	if form, err := c.MultipartForm(); err == nil {
		if files := form.File["monthly_summary"]; len(files) > 0 {
			for i, f := range files {
				dest := filepath.Join(workDirPath, fmt.Sprintf("月度汇总_%d.xlsx", i))
				if err := c.SaveUploadedFile(f, dest); err == nil {
					args = append(args, "--monthly-summary", dest)
				}
			}
		}
	}

	// 可选文件：排班表（多个）
	if form, err := c.MultipartForm(); err == nil {
		if files := form.File["schedule"]; len(files) > 0 {
			for i, f := range files {
				dest := filepath.Join(workDirPath, fmt.Sprintf("排班表_%d.xlsx", i))
				if err := c.SaveUploadedFile(f, dest); err == nil {
					args = append(args, "--schedule", dest)
				}
			}
		}
	}

	log.Printf("[AttendanceProcessing] 执行: python %v", args)
	result, err := runPythonCLI(args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	if result.Status != "ok" {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: result.Error})
		return
	}

	c.File(result.Output)
}
