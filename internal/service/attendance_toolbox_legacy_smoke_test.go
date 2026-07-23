package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRunLegacyProcessing_LeaveMapsAndInvokesRunner is a smoke that the legacy
// field mapping reaches the unified toolbox runner (not attendance-processing/cli.py).
// Requires Python + runner deps; skips when engine is unavailable.
func TestRunLegacyProcessing_LeaveMapsAndInvokesRunner(t *testing.T) {
	svc := NewAttendanceToolboxService()
	if svc.engineDir == "" {
		t.Skip("attendance toolbox engine dir not found")
	}
	runner := filepath.Join(svc.engineDir, "runner.py")
	if _, err := os.Stat(runner); err != nil {
		t.Skip("runner.py missing")
	}

	fixtureDir := t.TempDir()
	exportData, scheduleData := createLegacyLeaveFixtures(t, svc.pythonBin, fixtureDir)

	// Build a multipart form using OLD processing field names.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	writeFilePart(t, w, "input", "请假系统导出.xlsx", exportData)
	writeFilePart(t, w, "schedule", "作息表.xlsx", scheduleData)
	_ = w.Close()

	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(32 << 20)
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := form.RemoveAll(); cleanupErr != nil {
			t.Errorf("cleanup multipart form: %v", cleanupErr)
		}
	})

	// Ensure mapping targets toolbox keys.
	mapped := mapLegacyProcessingForm("leave", form)
	if len(mapped.File["leave_src"]) == 0 || len(mapped.File["leave_schedule"]) == 0 {
		t.Fatalf("legacy map failed: %+v", mapped.File)
	}

	result, runErr := svc.RunLegacyProcessing(context.Background(), "leave", form)
	if runErr != nil {
		t.Fatalf("legacy processing must complete through unified runner: %v", runErr)
	}
	if result == nil {
		t.Fatal("legacy processing returned nil result")
	}
	if len(result.Data) == 0 {
		t.Fatal("legacy processing returned empty xlsx")
	}
	if filepath.Ext(result.FileName) != ".xlsx" {
		t.Fatalf("unexpected result filename: %q", result.FileName)
	}
	validateXLSXWorkbook(t, result.Data, "请假明细", "实习生请假明细")
}

func writeFilePart(t *testing.T, w *multipart.Writer, field, filename string, data []byte) {
	t.Helper()
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	h.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
}

// createLegacyLeaveFixtures builds valid, desensitized Excel inputs with openpyxl.
// The fixture exercises the real leave workflow through the legacy upload contract.
func createLegacyLeaveFixtures(t *testing.T, pythonBin, dir string) ([]byte, []byte) {
	t.Helper()
	exportPath := filepath.Join(dir, "leave.xlsx")
	schedulePath := filepath.Join(dir, "schedule.xlsx")
	script := `
import sys
from datetime import datetime

import openpyxl
from openpyxl.styles import PatternFill

export_path, schedule_path = sys.argv[1], sys.argv[2]

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "请假导出"
ws.append([
    "发起人工号", "发起人姓名", "发起人部门", "请假类型", "开始时间", "结束时间",
    "时长", "发起时间", "完成时间", "审批编号", "审批状态", "审批结果",
])
ws.append([
    "MT001", "测试甲", "研发中心-平台组", "事假",
    datetime(2026, 3, 2, 9, 0), datetime(2026, 3, 2, 18, 30), 8,
    datetime(2026, 3, 1, 10, 0), datetime(2026, 3, 1, 11, 0),
    "LEAVE-001", "完成", "同意",
])
wb.save(export_path)
wb.close()

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "作息时间表"
ws["A1"] = "2026年3月作息时间表"
ws["A2"] = "周数"
yellow = PatternFill("solid", fgColor="FFFF00")
for column, day in enumerate(range(2, 9), start=2):
    ws.cell(row=2, column=column, value=f"星期{column - 1}")
    cell = ws.cell(row=3, column=column, value=day)
    cell.fill = yellow
ws["A3"] = 1
wb.save(schedule_path)
wb.close()
`
	cmd := exec.Command(pythonBin, "-c", script, exportPath, schedulePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create legacy xlsx fixtures: %v: %s", err, output)
	}
	exportData, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read leave fixture: %v", err)
	}
	scheduleData, err := os.ReadFile(schedulePath)
	if err != nil {
		t.Fatalf("read schedule fixture: %v", err)
	}
	return exportData, scheduleData
}

func validateXLSXWorkbook(t *testing.T, data []byte, expectedSheets ...string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("result is not a valid xlsx zip: %v", err)
	}
	var workbookXML []byte
	for _, file := range zr.File {
		if file.Name != "xl/workbook.xml" {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open workbook.xml: %v", openErr)
		}
		workbookXML, err = io.ReadAll(reader)
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close workbook.xml: %v", closeErr)
		}
		if err != nil {
			t.Fatalf("read workbook.xml: %v", err)
		}
	}
	if len(workbookXML) == 0 {
		t.Fatal("xlsx is missing xl/workbook.xml")
	}
	for _, sheet := range expectedSheets {
		if !bytes.Contains(workbookXML, []byte(`name="`+sheet+`"`)) {
			t.Fatalf("xlsx is missing expected sheet %q", sheet)
		}
	}
}
