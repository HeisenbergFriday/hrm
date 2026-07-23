package service

import "testing"

func TestIsAnnualLeaveApprovalConsumable(t *testing.T) {
	tests := []struct {
		name   string
		status string
		result string
		want   bool
	}{
		{name: "COMPLETED+agree", status: "COMPLETED", result: "agree", want: true},
		{name: "completed+agree", status: "completed", result: "agree", want: true},
		{name: "COMPLETED+refuse", status: "COMPLETED", result: "refuse", want: false},
		{name: "running", status: "RUNNING", result: "agree", want: false},
		{name: "completed+empty-result-compat", status: "completed", result: "", want: true},
		{name: "completed+拒绝", status: "COMPLETED", result: "拒绝", want: false},
		{name: "completed+通过", status: "COMPLETED", result: "通过", want: true},
		{name: "completed+unknown", status: "COMPLETED", result: "redirect", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnnualLeaveApprovalConsumable(tt.status, tt.result); got != tt.want {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestApprovalResultFromExtension(t *testing.T) {
	if got := approvalResultFromExtension(map[string]interface{}{"result": "agree"}); got != "agree" {
		t.Fatalf("got=%s", got)
	}
	if got := approvalResultFromExtension(nil); got != "" {
		t.Fatalf("got=%s", got)
	}
}
