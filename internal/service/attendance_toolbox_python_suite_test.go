package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPythonToolboxUnittestSuite runs the full Python toolbox suite when python is available.
func TestPythonToolboxUnittestSuite(t *testing.T) {
	root := findRepoRoot(t)
	py := "python"
	if runtime.GOOS != "windows" {
		py = "python3"
	}
	if _, err := exec.LookPath(py); err != nil {
		t.Skip("python not on PATH")
	}
	cmd := exec.Command(py, "-m", "unittest", "discover", "-s", filepath.Join("tools", "attendance_toolbox", "python"), "-p", "*_test.py", "-v")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	text := string(out)
	t.Log(text)
	if err != nil {
		// Allow skips but not failures.
		if strings.Contains(text, "FAILED") || strings.Contains(text, "ERROR") {
			t.Fatalf("python suite failed: %v", err)
		}
		t.Fatalf("python suite error: %v\n%s", err, text)
	}
}
