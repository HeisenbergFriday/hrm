package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttendanceToolboxLegacyResult(t *testing.T) {
	const passed = `{"status":"passed","totals_verified":true,"issues":[]}`
	for _, tc := range []struct {
		name       string
		audit      string
		businesses int
		duplicate  bool
		wantError  string
	}{
		{name: "one Excel with internal metadata", audit: passed, businesses: 1},
		{name: "multiple business files", audit: passed, businesses: 2},
		{name: "missing audit", businesses: 1, wantError: "缺少完整性检查"},
		{name: "malformed audit", audit: `{`, businesses: 1, wantError: "格式异常"},
		{name: "null audit", audit: `null`, businesses: 1, wantError: "检查未通过"},
		{name: "review report only", audit: `{"status":"needs_review","issues":[{"detail":"月份不一致"}]}`, wantError: "月份不一致"},
		{name: "review with business file", audit: `{"status":"needs_review","totals_verified":true}`, businesses: 1, wantError: "检查未通过"},
		{name: "unknown status", audit: `{"status":"unknown","totals_verified":true}`, businesses: 1, wantError: "检查未通过"},
		{name: "unverified totals", audit: `{"status":"passed","totals_verified":false}`, businesses: 1, wantError: "回读验证"},
		{name: "missing totals verification", audit: `{"status":"passed"}`, businesses: 1, wantError: "回读验证"},
		{name: "unresolved issues", audit: `{"status":"passed","totals_verified":true,"issues":[{"detail":"仍有待处理问题"}]}`, businesses: 1, wantError: "仍有待处理问题"},
		{name: "no business result", audit: passed, wantError: "未生成业务结果"},
		{name: "duplicate audit", audit: passed, businesses: 1, duplicate: true, wantError: "检查结果重复"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var outputs []attendanceToolboxRunnerOutput
			add := func(name, kind, data string) {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				outputs = append(outputs, attendanceToolboxRunnerOutput{Path: path, FileName: name, Kind: kind})
			}
			if tc.audit != "" {
				add("integrity_audit.json", "meta", tc.audit)
				if tc.duplicate {
					outputs = append(outputs, outputs[0])
				}
			}
			add("module_audit.json", "meta", `{}`)
			add("检查报告.xlsx", "audit", "audit workbook")
			if tc.businesses >= 1 {
				add("请假明细.xlsx", "", "first workbook")
			}
			if tc.businesses >= 2 {
				add("补充结果.xlsx", "export", "second workbook")
			}
			result, err := readAttendanceToolboxLegacyResult(dir, "leave", outputs)
			if tc.wantError != "" {
				if result != nil || err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected no download and error containing %q; result=%v err=%v", tc.wantError, result, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.businesses == 1 {
				if result.FileName != "请假明细.xlsx" || string(result.Data) != "first workbook" || result.ContentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
					t.Fatalf("expected the original business Excel, got %+v", result)
				}
				return
			}
			if filepath.Ext(result.FileName) != ".zip" || result.ContentType != "application/zip" {
				t.Fatalf("expected a ZIP for multiple business files: %+v", result)
			}
			zr, err := zip.NewReader(bytes.NewReader(result.Data), int64(len(result.Data)))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"请假明细.xlsx": "first workbook", "补充结果.xlsx": "second workbook"}
			if len(zr.File) != len(want) {
				t.Fatalf("archive must contain only business files; got %d entries", len(zr.File))
			}
			for _, file := range zr.File {
				r, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				closeErr := r.Close()
				if err != nil || closeErr != nil || string(data) != want[file.Name] {
					t.Fatalf("unexpected archive entry %q: data=%q err=%v closeErr=%v", file.Name, data, err, closeErr)
				}
				delete(want, file.Name)
			}
		})
	}
}

func TestAttendanceToolboxLegacyResultRejectsUnreadableAudit(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{filepath.Join(dir, "missing.json"), filepath.Join(t.TempDir(), "outside.json")} {
		result, err := readAttendanceToolboxLegacyResult(dir, "leave", []attendanceToolboxRunnerOutput{{
			Path: path, FileName: "integrity_audit.json", Kind: "meta",
		}})
		if result != nil || err == nil {
			t.Fatalf("unreadable audit must fail: result=%v err=%v", result, err)
		}
	}
}

func TestAttendanceToolboxLegacyRejectsInvalidMonth(t *testing.T) {
	for _, module := range []string{"leave", "overtime", "subsidy", "final", "parttime"} {
		for _, month := range []string{"", "2026-00", "2026-13", "2026-3", "26-03", "1999-03", "2100-03"} {
			t.Run(module+"/"+month, func(t *testing.T) {
				form := &multipart.Form{Value: map[string][]string{module + "_target_month": {month}}}
				result, err := (&AttendanceToolboxService{}).RunLegacyProcessing(context.Background(), module, form)
				if result != nil || err == nil || !strings.Contains(err.Error(), "请先选择处理月份") {
					t.Fatalf("invalid month must fail before running Python: result=%v err=%v", result, err)
				}
			})
		}
	}
}
