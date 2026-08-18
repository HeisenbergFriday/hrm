package service

import (
	"sort"
	"testing"
)

func TestMatchParttimeMonthlyPunch_EmployeeNoPriority(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "MT001", Name: "张三"},
		{EmployeeNo: "TXB002", Name: "李四"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "MT001", Name: "张三", Days: map[int]string{1: "正常"}},
		{EmployeeNo: "TXB002", Name: "李四", Days: map[int]string{1: "迟到"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)

	if len(match.Matched) != 2 {
		t.Fatalf("expected 2 matched, got %d", len(match.Matched))
	}
	if match.Matched[0].MatchedBy != "employee_no" || match.Matched[1].MatchedBy != "employee_no" {
		t.Fatalf("expected employee_no matching, got %+v", match.Matched)
	}
	if len(match.Unmatched) != 0 {
		t.Fatalf("expected 0 unmatched, got %d", len(match.Unmatched))
	}
}

func TestMatchParttimeMonthlyPunch_NameFallback(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "", Name: "王五"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "", Name: "王五", Days: map[int]string{2: "正常"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 1 || match.Matched[0].MatchedBy != "name" {
		t.Fatalf("expected name fallback match, got %+v", match.Matched)
	}
}

func TestMatchParttimeMonthlyPunch_KeepsUnmatched(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "MT001", Name: "张三"},
		{EmployeeNo: "MT002", Name: "赵六"}, // no punch record
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "MT001", Name: "张三", Days: map[int]string{1: "正常"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 1 {
		t.Fatalf("expected 1 matched, got %d", len(match.Matched))
	}
	if len(match.Unmatched) != 1 || match.Unmatched[0].Name != "赵六" {
		t.Fatalf("expected 赵六 preserved as unmatched, got %+v", match.Unmatched)
	}
}

func TestMatchParttimeMonthlyPunch_DuplicateNameNoCode_Anomaly(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "", Name: "陈七"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "", Name: "陈七", Days: map[int]string{1: "正常"}},
		{EmployeeNo: "", Name: "陈七", Days: map[int]string{2: "迟到"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 0 {
		t.Fatalf("expected 0 matched (ambiguous), got %d", len(match.Matched))
	}
	if len(match.Unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(match.Unmatched))
	}
	if len(match.Anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d: %v", len(match.Anomalies), match.Anomalies)
	}
}

func TestMatchParttimeMonthlyPunch_NoPrefixFiltering(t *testing.T) {
	// Employee numbers that do NOT start with MT/TXB/WB/JZ must still be kept.
	roster := []ParttimeEmployee{
		{EmployeeNo: "999888", Name: "周八"},
		{EmployeeNo: "JZ-X1", Name: "吴九"},
		{EmployeeNo: "", Name: "郑十"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "999888", Name: "周八", Days: map[int]string{1: "正常"}},
		{EmployeeNo: "JZ-X1", Name: "吴九", Days: map[int]string{1: "正常"}},
		{EmployeeNo: "", Name: "郑十", Days: map[int]string{1: "正常"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 3 {
		t.Fatalf("expected 3 matched (no prefix filtering), got %d unmatched=%d",
			len(match.Matched), len(match.Unmatched))
	}
}

func TestMatchParttimeMonthlyPunch_NameCleaning(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "", Name: "张三（离职）"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "", Name: "张三", Days: map[int]string{1: "正常"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 1 {
		t.Fatalf("expected name cleaning to match 张三, got %+v", match)
	}
}

func TestMatchParttimeMonthlyPunch_PreservesInternPositionAndDepartment(t *testing.T) {
	roster := []ParttimeEmployee{{
		EmployeeNo: "JZ001",
		Name:       "实习生甲",
		Position:   "实习生",
		Department: "研发部",
	}}
	punched := []EmployeePunchData{{
		EmployeeNo: "JZ001",
		Name:       "实习生甲",
		Days:       map[int]string{1: "正常"},
	}}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 1 {
		t.Fatalf("expected one matched intern, got %+v", match)
	}
	got := match.Matched[0]
	if got.Position != "实习生" || got.Department != "研发部" {
		t.Fatalf("expected intern identity fields to survive matching, got %+v", got)
	}
}

func TestMatchParttimeMonthlyPunch_EmptyNoAndNameFallsBack(t *testing.T) {
	// Numeric-only and special employee numbers are treated as valid numbers
	// (not filtered) and matched by number when present in the report.
	roster := []ParttimeEmployee{
		{EmployeeNo: "00123", Name: "A"},
		{EmployeeNo: "#SPECIAL", Name: "B"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "00123", Name: "A", Days: map[int]string{1: "正常"}},
		{EmployeeNo: "#SPECIAL", Name: "B", Days: map[int]string{1: "正常"}},
	}

	match := MatchParttimeMonthlyPunch(roster, punched)
	if len(match.Matched) != 2 {
		t.Fatalf("expected 2 matched, got %+v", match)
	}
}

func TestStatusLabel(t *testing.T) {
	cases := []struct {
		on, off, result, want string
	}{
		{"", "", "Normal", ""},
		{"08:30", "18:00", "Normal", "正常 (08:30,18:00)"},
		{"08:30", "", "Normal", "正常 (08:30)"},
		{"09:05", "18:00", "Late", "迟到 (09:05,18:00)"},
		{"08:30", "17:00", "Early", "早退 (08:30,17:00)"},
		{"", "", "NotSigned", "缺卡"},
		{"", "", "not_signed", "缺卡"},
		{"", "", "Absenteeism", "旷工"},
		{"", "", "Absent", "旷工"},
		{"", "", "SeriousLate", "严重迟到"},
		{"09:30", "", "Serious_Late", "严重迟到 (09:30)"},
	}
	for _, c := range cases {
		got := statusLabel(c.on, c.off, c.result)
		if got != c.want {
			t.Errorf("statusLabel(%q,%q,%q)=%q want %q", c.on, c.off, c.result, got, c.want)
		}
	}
}

func TestNormalizeParttimeName(t *testing.T) {
	cases := map[string]string{
		"张三（离职）":  "张三",
		"李四(已离职)": "李四",
		"王  五":    "王五",
		"  赵六  ":  "赵六",
		"（离职）孙七":  "（离职）孙七",
	}
	for in, want := range cases {
		got := normalizeParttimeName(in)
		if got != want {
			t.Errorf("normalizeParttimeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMatchParttimeMonthlyPunch_SortedOutput(t *testing.T) {
	roster := []ParttimeEmployee{
		{EmployeeNo: "Z001", Name: "Zara"},
		{EmployeeNo: "A001", Name: "Anna"},
		{EmployeeNo: "M001", Name: "Mike"},
	}
	punched := []EmployeePunchData{
		{EmployeeNo: "Z001", Name: "Zara", Days: map[int]string{}},
		{EmployeeNo: "A001", Name: "Anna", Days: map[int]string{}},
		{EmployeeNo: "M001", Name: "Mike", Days: map[int]string{}},
	}
	match := MatchParttimeMonthlyPunch(roster, punched)
	names := make([]string, len(match.Matched))
	for i, m := range match.Matched {
		names[i] = m.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("matched output not sorted: %v", names)
	}
}
