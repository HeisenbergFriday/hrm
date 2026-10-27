package dingtalk

import "testing"

func TestMatchesCompensatoryLeaveName(t *testing.T) {
	for _, tt := range []struct {
		actual, configured string
		want               bool
	}{
		{actual: "调休", configured: "调休", want: true},
		{actual: "补休", configured: "调休", want: true},
		{actual: "手动发放", configured: "调休", want: true},
		{actual: "调休假", configured: "调休", want: true},
		{actual: "年假", configured: "调休", want: false},
		{actual: "手动发放", configured: "手动发放", want: true},
	} {
		if got := matchesCompensatoryLeaveName(tt.actual, tt.configured); got != tt.want {
			t.Errorf("matchesCompensatoryLeaveName(%q, %q) = %v, want %v", tt.actual, tt.configured, got, tt.want)
		}
	}
}
