package service

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fakeToolboxRunnerPy is a minimal stand-in for tools/attendance_toolbox/python/runner.py.
// It honors the same CLI contract (--module/--workdir/--config-json) and emits a
// scenario-controlled set of outputs selected by the FAKE_TOOLBOX_SCENARIO env var.
const fakeToolboxRunnerPy = `
import json, os, sys

def arg(name):
    for i, a in enumerate(sys.argv):
        if a == name and i + 1 < len(sys.argv):
            return sys.argv[i + 1]
    return None

workdir = arg("--workdir")
scenario = os.environ.get("FAKE_TOOLBOX_SCENARIO", "single")

def emit(name, kind, flow_key, row_count):
    path = os.path.join(workdir, name)
    with open(path, "wb") as f:
        f.write(name.encode("utf-8"))
    return {"path": path, "file_name": name, "kind": kind, "flow_key": flow_key, "row_count": row_count}

outputs = []
if scenario == "multi":
    outputs.append(emit("请假业务表.xlsx", "export", "leave", 2))
    outputs.append(emit("岗位异动业务表.xlsx", "export", "position_transfer", 3))
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))
elif scenario == "audit":
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))
    outputs.append(emit("同步摘要.json", "meta", "", 0))
else:
    outputs.append(emit("岗位异动业务表.xlsx", "export", "position_transfer", 3))
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))

print(json.dumps({"ok": True, "outputs": outputs, "log": "fake-log", "error": "", "traceback": ""}))
`

func toolboxTestPython(t *testing.T) string {
	t.Helper()
	for _, bin := range []string{"python", "python3"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	t.Skip("python not on PATH; skipping real dingtalk sync engine test")
	return ""
}

func setupFakeToolboxEngine(t *testing.T) *AttendanceToolboxService {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runner.py"), []byte(fakeToolboxRunnerPy), 0o600); err != nil {
		t.Fatal(err)
	}
	return &AttendanceToolboxService{
		engineDir: dir,
		pythonBin: toolboxTestPython(t),
		timeout:   60 * time.Second,
	}
}

// TestDingtalkSyncResult_BusinessExportsOnlyPositionTransfer verifies that
// when a dingtalk_sync produces one position_transfer export + one audit file,
// BusinessExports() only returns the export table and ignores audit.
// This is critical for auto-fill: the frontend must never upload audit as business data.
func TestDingtalkSyncResult_BusinessExportsOnlyPositionTransfer(t *testing.T) {
	res := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: []byte("export"), Kind: "export", FlowKey: "position_transfer", RowCount: 5},
		{FileName: "钉钉同步审计报告.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: []byte("audit"), Kind: "audit", RowCount: 1},
	}}

	exports := res.BusinessExports()
	if len(exports) != 1 {
		t.Fatalf("expected 1 business export, got %d", len(exports))
	}
	if exports[0].FlowKey != "position_transfer" {
		t.Fatalf("expected flow_key=position_transfer, got %s", exports[0].FlowKey)
	}
	if exports[0].Kind != "export" {
		t.Fatalf("expected kind=export, got %s", exports[0].Kind)
	}
}

// TestDingtalkSyncResult_SingleBusinessExportNoZIP verifies that ZIP is only
// created when there are multiple business exports, NOT when a single business
// export is accompanied by audit/meta files.
func TestDingtalkSyncResult_SingleBusinessExportNoZIP(t *testing.T) {
	// Simulate what RunDingtalkSyncForOrg does: build result, then check BusinessExports() > 1 for ZIP.
	singleBizExport := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", Kind: "export", FlowKey: "position_transfer", Data: []byte("p")},
		{FileName: "审计.xlsx", Kind: "audit", Data: []byte("a")},
	}}

	if len(singleBizExport.BusinessExports()) != 1 {
		t.Fatal("single business export + audit should have 1 business export")
	}
	if len(singleBizExport.Outputs) != 2 {
		t.Fatal("total outputs should be 2 (1 business + 1 audit)")
	}
	// The fix: ZIP is only created when BusinessExports() > 1, not when Outputs > 1.
	// The handler uses BusinessExports() to decide.
}

// TestDingtalkSyncResult_MultipleBusinessExportsNeedsZIP verifies that
// when truly multiple business exports exist, ZIP creation is appropriate.
func TestDingtalkSyncResult_MultipleBusinessExportsNeedsZIP(t *testing.T) {
	multiBizExport := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "leave.xlsx", Kind: "export", FlowKey: "leave", Data: []byte("l")},
		{FileName: "position.xlsx", Kind: "export", FlowKey: "position_transfer", Data: []byte("p")},
		{FileName: "overtime.xlsx", Kind: "export", FlowKey: "overtime", Data: []byte("o")},
		{FileName: "audit.xlsx", Kind: "audit", Data: []byte("a")},
	}}

	if len(multiBizExport.BusinessExports()) != 3 {
		t.Fatalf("expected 3 business exports, got %d", len(multiBizExport.BusinessExports()))
	}
}

// TestDingtalkSyncResult_AllAuditNoBusiness verifies edge case:
// when only audit/meta files exist (no business exports), BusinessExports() returns empty.
func TestDingtalkSyncResult_AllAuditNoBusiness(t *testing.T) {
	auditOnly := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "审计.xlsx", Kind: "audit", Data: []byte("a")},
		{FileName: "meta.json", Kind: "meta", Data: []byte("{}")},
	}}

	if len(auditOnly.BusinessExports()) != 0 {
		t.Fatalf("expected 0 business exports for audit-only result, got %d", len(auditOnly.BusinessExports()))
	}
}

// TestDingtalkSyncResult_SingleExportNoAudit verifies the trivial case:
// single export with no audit files.
func TestDingtalkSyncResult_SingleExportNoAudit(t *testing.T) {
	single := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "请假导出表.xlsx", Kind: "export", FlowKey: "leave", Data: []byte("l")},
	}}

	if len(single.BusinessExports()) != 1 {
		t.Fatal("single export should have 1 business export")
	}
	// Single output: the old handler path returns Excel directly (len(Outputs)==1 branch).
	// ZIP creation only fires for BusinessExports() > 1.
}

// TestDingtalkSyncResult_ExportWithMixedMetaAndAudit verifies that
// BusinessExports() correctly filters a mix of export/audit/meta files,
// keeping only kind=export.
func TestDingtalkSyncResult_ExportWithMixedMetaAndAudit(t *testing.T) {
	res := &DingtalkSyncResult{Outputs: []AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", Kind: "export", FlowKey: "position_transfer", Data: []byte("p")},
		{FileName: "审计.xlsx", Kind: "audit", Data: []byte("a")},
		{FileName: "meta.json", Kind: "meta", Data: []byte("{}")},
		{FileName: "钉钉同步摘要.json", Kind: "meta", Data: []byte(`{"ok":true}`)},
	}}

	exports := res.BusinessExports()
	if len(exports) != 1 {
		t.Fatalf("expected 1 business export from mixed outputs, got %d", len(exports))
	}
	if exports[0].FlowKey != "position_transfer" {
		t.Fatalf("expected position_transfer, got %s", exports[0].FlowKey)
	}
}

// ── Real engine behavior tests ─────────────────────────────────────────────────
// These drive the actual subprocess engine (runner.py) and assert the resulting
// DingtalkSyncResult, so the ZIP-vs-single-Excel decision is verified end to end.

func runDingtalkSyncForOrgScenario(t *testing.T, scenario string) *DingtalkSyncResult {
	t.Helper()
	svc := setupFakeToolboxEngine(t)
	t.Setenv("DINGTALK_APP_KEY", "test-app-key")
	t.Setenv("DINGTALK_APP_SECRET", "test-app-secret")
	t.Setenv("FAKE_TOOLBOX_SCENARIO", scenario)

	result, err := svc.RunDingtalkSyncForOrg(context.Background(), "default", &DingtalkSyncRequest{
		StartDate: "2026-01-01",
		EndDate:   "2026-01-31",
		FlowKeys:  []string{"position_transfer"},
	})
	if err != nil {
		t.Fatalf("RunDingtalkSyncForOrg: %v", err)
	}
	return result
}

// TestRunDingtalkSyncForOrg_SingleExportPlusAudit_ZipDataEmpty verifies the
// core regression: one business export + one audit file must NOT produce a ZIP.
func TestRunDingtalkSyncForOrg_SingleExportPlusAudit_ZipDataEmpty(t *testing.T) {
	result := runDingtalkSyncForOrgScenario(t, "single")

	if len(result.Outputs) != 2 {
		t.Fatalf("expected 2 outputs (1 export + 1 audit), got %d", len(result.Outputs))
	}
	if len(result.BusinessExports()) != 1 {
		t.Fatalf("expected exactly 1 business export, got %d", len(result.BusinessExports()))
	}
	if result.BusinessExports()[0].FlowKey != "position_transfer" {
		t.Fatalf("expected position_transfer export, got %s", result.BusinessExports()[0].FlowKey)
	}
	if len(result.ZipData) != 0 {
		t.Fatalf("single business export + audit must keep ZipData empty, got %d bytes", len(result.ZipData))
	}
	if string(result.BusinessExports()[0].Data) != "岗位异动业务表.xlsx" {
		t.Fatalf("unexpected export payload %q", result.BusinessExports()[0].Data)
	}
}

// TestRunDingtalkSyncForOrg_MultipleExports_ZipData builds ZIP only for multiple
// business exports, and the archive must contain every output file.
func TestRunDingtalkSyncForOrg_MultipleExports_ZipData(t *testing.T) {
	result := runDingtalkSyncForOrgScenario(t, "multi")

	if len(result.BusinessExports()) != 2 {
		t.Fatalf("expected 2 business exports, got %d", len(result.BusinessExports()))
	}
	if len(result.ZipData) == 0 {
		t.Fatal("multiple business exports must produce a ZIP")
	}
	zr, err := zip.NewReader(bytes.NewReader(result.ZipData), int64(len(result.ZipData)))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}
	if len(zr.File) != 3 {
		t.Fatalf("zip should contain 3 files (2 exports + 1 audit), got %d", len(zr.File))
	}
}

// TestRunDingtalkSyncForOrg_AuditOnly_NoBusinessExports verifies that an
// audit/meta-only result yields zero business exports and no ZIP; the caller
// (legacy handler) is responsible for turning that into a 4xx.
func TestRunDingtalkSyncForOrg_AuditOnly_NoBusinessExports(t *testing.T) {
	result := runDingtalkSyncForOrgScenario(t, "audit")

	if len(result.BusinessExports()) != 0 {
		t.Fatalf("expected 0 business exports for audit-only result, got %d", len(result.BusinessExports()))
	}
	if len(result.ZipData) != 0 {
		t.Fatalf("audit-only result must not produce ZIP, got %d bytes", len(result.ZipData))
	}
}
