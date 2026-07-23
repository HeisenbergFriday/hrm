package main

import (
	"strings"
	"testing"

	"peopleops/internal/dingtalk"
)

func TestGuessProcessBusinessType(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"员工加班申请", "加班"},
		{"请假流程", "请假"},
		{"补卡申请", "补卡"},
		{"出差申请", "出差"},
		{"外出报备", "外出"},
		{"通用审批", "待确认"},
		{"", "待确认"},
	}
	for _, tt := range tests {
		if got := guessProcessBusinessType(tt.name); got != tt.want {
			t.Fatalf("name=%q got=%s want=%s", tt.name, got, tt.want)
		}
	}
}

func TestFindExactProcessTemplate(t *testing.T) {
	items := []dingtalk.ApprovalProcessTemplate{
		{Name: "加班", ProcessCode: "a3ba921b2f3c3c63a634f82dd5b305a7"},
		{Name: "相似", ProcessCode: "a3ba921b2f3c3c63a634f82dd5b305a8"},
	}
	hit := findExactProcessTemplate(items, "a3ba921b2f3c3c63a634f82dd5b305a7")
	if hit == nil || hit.Name != "加班" {
		t.Fatalf("exact match failed: %+v", hit)
	}
	if findExactProcessTemplate(items, "a3ba921b2f3c3c63a634f82dd5b305a8").Name != "相似" {
		t.Fatal("similar should only exact-match itself")
	}
	if findExactProcessTemplate(items, "missing") != nil {
		t.Fatal("missing should be nil")
	}
	// prefix must not match
	if findExactProcessTemplate(items, "a3ba921b2f3c3c63") != nil {
		t.Fatal("prefix should not match")
	}
}

func TestDescribeCodeShape(t *testing.T) {
	if !strings.Contains(describeCodeShape("PROC-ABC"), "PROC-") {
		t.Fatal("proc prefix")
	}
	if !strings.Contains(describeCodeShape("a3ba921b2f3c3c63a634f82dd5b305a7"), "32 位") {
		t.Fatal("hex shape")
	}
}

func TestSuggestAttendanceProcessCodes(t *testing.T) {
	items := []dingtalk.ApprovalProcessTemplate{
		{Name: "加班申请", ProcessCode: "OT"},
		{Name: "通用审批", ProcessCode: "X"},
		{Name: "补卡", ProcessCode: "BK"},
	}
	got := suggestAttendanceProcessCodes(items)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
}
