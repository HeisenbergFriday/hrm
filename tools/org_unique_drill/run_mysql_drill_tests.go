//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Local drill-only DSN matches tools/org_unique_drill/start_mysql.ps1.
const drillDSN = "drill:drill_only_local@tcp(127.0.0.1:13306)/peopleops_org_drill?charset=utf8mb4&parseTime=True&loc=Local"

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Println("GETWD_ERR", err)
		os.Exit(1)
	}
	// Allow invocation from tools/org_unique_drill
	if filepath.Base(root) == "org_unique_drill" {
		root = filepath.Clean(filepath.Join(root, "..", ".."))
	}

	tags := "integration,mysql_drill"
	packages := []string{
		"./internal/database",
		"./internal/api",
	}
	// Optional filter via args, e.g. TestOrgCompositeUniqueMigrationRealMySQLDrill
	runFilter := ""
	if len(os.Args) > 1 {
		runFilter = strings.Join(os.Args[1:], "|")
	}

	args := []string{"test", "-tags", tags, "-count=1", "-v"}
	if runFilter != "" {
		args = append(args, "-run", runFilter)
	}
	args = append(args, packages...)
	args = append(args, "-timeout", "15m")

	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "PEOPLEOPS_MYSQL_DRILL_DSN="+drillDSN)
	if runtime.GOOS == "windows" {
		// keep existing PATH
	}
	fmt.Printf("RUNNING: go %s\n", strings.Join(args, " "))
	fmt.Println("TARGET: 127.0.0.1:13306 / peopleops_org_drill (credentials redacted)")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Println("EXEC_ERR", err)
		os.Exit(1)
	}
}
