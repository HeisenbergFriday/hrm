package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCompareAppSourceScript_Live runs the source compare against D:\app when present.
//
// Current intentional state is pinned per file below. The three identical files have
// byte-equivalent normalized business source. finally/calc_finally.py differs only by
// the allowlisted toolbox path/excel_compat adapter. Leave, overtime fill, and subsidy
// contain HR business behavior that does not canonicalize to the D:\app source.
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
	repoManifestPath := filepath.Join(root, "tools", "attendance_toolbox", "python", "SOURCE_MANIFEST.json")
	repoManifestBefore, err := os.ReadFile(repoManifestPath)
	if err != nil {
		t.Fatalf("read repository manifest before compare: %v", err)
	}
	gitDiffBefore := gitDiffForCompareTest(t, root)
	t.Cleanup(func() {
		repoManifestAfter, readErr := os.ReadFile(repoManifestPath)
		if readErr != nil {
			t.Errorf("read repository manifest after compare: %v", readErr)
		} else if !bytes.Equal(repoManifestBefore, repoManifestAfter) {
			t.Error("live compare modified repository SOURCE_MANIFEST.json")
		}
		if gitDiffAfter := gitDiffForCompareTest(t, root); !bytes.Equal(gitDiffBefore, gitDiffAfter) {
			t.Error("live compare changed the repository git diff")
		}
	})

	outputDir := t.TempDir()
	cmd := exec.Command(py, script, "--output-dir", outputDir)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	text := string(out)
	t.Log(text)

	// Exit code 1 is the compare tool's documented result while business divergence exists.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("compare exit code = %v, want 1 for known business divergence\n%s", err, text)
	}

	manifestPath := filepath.Join(outputDir, "SOURCE_MANIFEST.json")
	raw, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatalf("read temporary manifest: %v", readErr)
	}
	var manifest struct {
		PairCount               int `json:"pair_count"`
		EqualCount              int `json:"equal_count"`
		AdapterOnlyCount        int `json:"adapter_only_count"`
		BusinessDivergenceCount int `json:"business_divergence_count"`
		Files                   []struct {
			Path           string `json:"path"`
			DifferenceKind string `json:"difference_kind"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse temporary manifest: %v", err)
	}
	if manifest.PairCount != 7 || manifest.EqualCount != 3 || manifest.AdapterOnlyCount != 1 || manifest.BusinessDivergenceCount != 3 {
		t.Fatalf("manifest counts = total %d, equal %d, adapter_only %d, business_divergence %d; want 7/3/1/3",
			manifest.PairCount, manifest.EqualCount, manifest.AdapterOnlyCount, manifest.BusinessDivergenceCount)
	}

	actualKinds := make(map[string]string, len(manifest.Files))
	for _, file := range manifest.Files {
		actualKinds[file.Path] = file.DifferenceKind
	}
	expectedKinds := map[string]struct {
		kind   string
		reason string
	}{
		"leave/calc_leave.py":               {"business_divergence", "toolbox contains HR leave-calculation behavior absent from D:\\app"},
		"overtime/fill_overtime_fields.py":  {"business_divergence", "toolbox contains HR overtime field and operations-group business rules"},
		"overtime/rules_engine.py":          {"equal", "normalized business source is identical"},
		"subsidy/calc_subsidy_deduction.py": {"business_divergence", "toolbox contains HR subsidy eligibility and source-validation rules"},
		"finally/calc_finally.py":           {"adapter_only", "canonical source differs only by allowlisted path and excel_compat adapters"},
		"parttime/calc_parttime_summary.py": {"equal", "normalized business source is identical"},
		"dingtalk_sync.py":                  {"equal", "normalized business source is identical"},
	}
	for path, expected := range expectedKinds {
		if actualKinds[path] != expected.kind {
			t.Errorf("%s difference_kind = %q, want %q (%s)", path, actualKinds[path], expected.kind, expected.reason)
		}
		t.Logf("%s: %s (%s)", path, expected.kind, expected.reason)
	}
	if len(actualKinds) != len(expectedKinds) {
		t.Errorf("manifest contains %d file entries, want %d", len(actualKinds), len(expectedKinds))
	}

	for _, name := range []string{"_diff_report.txt", "compare_app_source.last_run.txt"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("temporary output %s missing: %v", name, err)
		}
	}
}

func gitDiffForCompareTest(t *testing.T, root string) []byte {
	t.Helper()
	cmd := exec.Command("git", "diff", "--binary")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("capture git diff: %v", err)
	}
	return out
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
