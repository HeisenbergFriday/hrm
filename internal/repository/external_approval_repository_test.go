package repository

import (
	"strings"
	"testing"
	"time"
)

func TestApprovalWhereUsesExistingSearchColumnsOnly(t *testing.T) {
	where, args := approvalWhere(
		[]string{"corp_name", "process_instance_id", "approval_title"},
		"深圳市沐腾科技有限公司",
		"请假",
	)
	if !strings.Contains(where, "corp_name = ?") || !strings.Contains(where, "`process_instance_id`") || !strings.Contains(where, "`approval_title`") {
		t.Fatalf("where = %q", where)
	}
	if strings.Contains(where, "originator_user_name") {
		t.Fatalf("where references a missing source column: %q", where)
	}
	if len(args) != 3 {
		t.Fatalf("args = %#v, want corp + 2 keyword values", args)
	}
}

func TestNormalizeApprovalValue(t *testing.T) {
	if got := normalizeApprovalValue([]byte("审批")); got != "审批" {
		t.Fatalf("byte value = %#v", got)
	}
	value := time.Date(2026, 7, 23, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	if got := normalizeApprovalValue(value); got != "2026-07-23T12:30:00+08:00" {
		t.Fatalf("time value = %#v", got)
	}
}
