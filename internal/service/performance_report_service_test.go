package service

import (
	"archive/zip"
	"bytes"
	"io"
	"peopleops/internal/database"
	"strings"
	"testing"
)

func TestBuildResultReportRedactsHiddenEmployeeResult(t *testing.T) {
	score := 9.5
	participant := database.PerformanceParticipant{
		ID:                   1,
		EmployeeID:           "emp-001",
		EmployeeName:         "张三",
		DepartmentID:         "dept-1",
		DepartmentName:       "业务部",
		Status:               "locked",
		ManagerScore:         8.8,
		AdjustedScore:        9.2,
		FinalLevel:           "A",
		DepartmentFinalScore: &score,
		DepartmentFinalLevel: "S",
		ResultHidden:         true,
		IsLocked:             true,
	}

	report := buildResultReport(database.PerformanceActivity{}, []database.PerformanceParticipant{participant}, PerformanceReportAccess{
		IdentityValues: map[string]struct{}{"emp-001": {}},
		Privileged:     false,
	})

	if len(report.Rows) != 1 {
		t.Fatalf("Rows length = %d, want 1", len(report.Rows))
	}
	row := report.Rows[0]
	if row.ResultVisible {
		t.Fatalf("hidden employee result should be invisible")
	}
	if row.ManagerScore != 0 || row.EffectiveFinalLevel != "" || row.DepartmentFinalScore != nil {
		t.Fatalf("hidden result fields were not redacted: %#v", row)
	}
	if report.Summary.HiddenCount != 1 {
		t.Fatalf("HiddenCount = %d, want 1", report.Summary.HiddenCount)
	}

	privileged := buildResultReport(database.PerformanceActivity{}, []database.PerformanceParticipant{participant}, PerformanceReportAccess{
		IdentityValues: map[string]struct{}{"emp-001": {}},
		Privileged:     true,
	})
	privilegedRow := privileged.Rows[0]
	if !privilegedRow.ResultVisible || privilegedRow.EffectiveFinalLevel != "S" || privilegedRow.DepartmentFinalScore == nil {
		t.Fatalf("privileged result should stay visible: %#v", privilegedRow)
	}
}

func TestBuildReportXLSXNewFlowContentSplitsReviewAndPlan(t *testing.T) {
	report := &PerformanceReport{
		IsNewFlow: true,
		Content: PerformanceContentReport{
			Rows: []PerformanceContentRow{
				{ID: 1, EmployeeID: "emp-001", EmployeeName: "张三", GoalPhase: "review", GoalPhaseLabel: "上季度完成情况", ItemName: "Review Goal"},
				{ID: 2, EmployeeID: "emp-001", EmployeeName: "张三", GoalPhase: "plan", GoalPhaseLabel: "下季度目标计划", ItemName: "Plan Goal"},
			},
		},
	}

	data, err := NewPerformanceReportService(nil).BuildReportXLSX(report, PerformanceReportTypeContent)
	if err != nil {
		t.Fatalf("BuildReportXLSX() error = %v", err)
	}

	files := unzipXLSXFiles(t, data)
	workbook := files["xl/workbook.xml"]
	if !strings.Contains(workbook, "上季度完成情况") || !strings.Contains(workbook, "下季度目标计划") {
		t.Fatalf("workbook did not contain split sheet names: %s", workbook)
	}
	if !strings.Contains(files["xl/worksheets/sheet1.xml"], "Review Goal") {
		t.Fatalf("review sheet missing review row: %s", files["xl/worksheets/sheet1.xml"])
	}
	if !strings.Contains(files["xl/worksheets/sheet2.xml"], "Plan Goal") {
		t.Fatalf("plan sheet missing plan row: %s", files["xl/worksheets/sheet2.xml"])
	}
}

func unzipXLSXFiles(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	files := make(map[string]string, len(zr.File))
	for _, file := range zr.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s error = %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s error = %v", file.Name, err)
		}
		files[file.Name] = string(content)
	}
	return files
}
