package dingtalk

import "testing"

func TestIsPlaceholderLeaveName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: true},
		{name: "question marks", value: "???", want: true},
		{name: "replacement characters", value: "\ufffd\ufffd", want: true},
		{name: "real chinese name", value: "年假", want: false},
		{name: "real code-like name", value: "annual", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPlaceholderLeaveName(tt.value); got != tt.want {
				t.Fatalf("isPlaceholderLeaveName(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
