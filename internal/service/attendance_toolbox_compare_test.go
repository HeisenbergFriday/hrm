package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCompareAppSourceScript_Live runs the source compare against D:\app when present.
//
// Current intentional state (HR toolbox upgrades over D:\app):
//
//	equal=3 — overtime/rules_engine.py, parttime/calc_parttime_summary.py, dingtalk_sync.py
//	business_divergence=4 — leave / overtime fill / subsidy / finally carry intentional
//	  payroll fixes (not adapter-only import shims). See session analysis for details.
//
// compare_app_source.py exits 1 when any business_divergence remains; that is expected
// until D:\app is back-ported or divergences are deliberately reclassified.
func TestCompareAppSourceScript_Live(t *testing.T) {
	if _, err := os.Stat(`D:\app`); err != nil {
		t.Skip("D:\\app not available")
	}
	root := findRepoRoot(t)
	script := filepath.Join(root, "tools", "attendance_toolbox", "python", "scripts", "compare_app_source.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("script missing: %v", err)
	}
	py := "python"
	if runtime.GOOS != "windows" {
		py = "python3"
	}
	cmd := exec.Command(py, script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	text := string(out)
	t.Log(text)

	// Stdout only prints aggregate counters; per-file kinds live in SOURCE_MANIFEST.json.
	// Exit code is non-zero while business_divergence>0 — that is expected.
	if !strings.Contains(text, "equal=3") ||
		!strings.Contains(text, "business_divergence=4") {
		t.Fatalf("unexpected summary (want equal=3 business_divergence=4): %s\nerr=%v", text, err)
	}
	manifestPath := filepath.Join(root, "tools", "attendance_toolbox", "python", "SOURCE_MANIFEST.json")
	raw, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatalf("read manifest: %v", readErr)
	}
	body := string(raw)
	for _, path := range []string{
		"leave/calc_leave.py",
		"overtime/fill_overtime_fields.py",
		"subsidy/calc_subsidy_deduction.py",
		"finally/calc_finally.py",
	} {
		// Cheap structural pin: path entry exists and marked business_divergence nearby.
		if !strings.Contains(body, `"path": "`+path+`"`) ||
			!strings.Contains(body, `"difference_kind": "business_divergence"`) {
			t.Fatalf("manifest missing divergence for %s\n%s", path, body)
		}
	}
	for _, path := range []string{
		"overtime/rules_engine.py",
		"parttime/calc_parttime_summary.py",
		"dingtalk_sync.py",
	} {
		if !strings.Contains(body, `"path": "`+path+`"`) {
			t.Fatalf("manifest missing equal core %s", path)
		}
	}
	// Explicit counts from generated manifest (authoritative over stdout).
	if !strings.Contains(body, `"equal_count": 3`) ||
		!strings.Contains(body, `"business_divergence_count": 4`) {
		t.Fatalf("manifest counts unexpected:\n%s", body)
	}
}

// TestCompareAppSourceUnitTests runs compare_app_source_test.py via unittest.
func TestCompareAppSourceUnitTests(t *testing.T) {
	root := findRepoRoot(t)
	py := "python"
	if runtime.GOOS != "windows" {
		py = "python3"
	}
	cmd := exec.Command(py, "-m", "unittest", "discover", "-s", filepath.Join("tools", "attendance_toolbox", "python"), "-p", "compare_app_source_test.py", "-v")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("unit tests failed: %v\n%s", err, out)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "tools", "attendance_toolbox", "python", "runner.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
