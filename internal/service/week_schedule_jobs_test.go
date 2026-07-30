package service

import "testing"

func TestBuildFridaySaturdayReminderContent(t *testing.T) {
	tests := []struct {
		name         string
		weekLabel    string
		dateLabel    string
		saturdayWork bool
		want         string
	}{
		{
			name:         "work",
			weekLabel:    "小周",
			dateLabel:    "2026年8月1日",
			saturdayWork: true,
			want:         "【明天需上班】\n明天（2026年8月1日，周六）需上班，请提前安排。\n本周为小周。",
		},
		{
			name:         "rest",
			weekLabel:    "大周",
			dateLabel:    "2026年8月8日",
			saturdayWork: false,
			want:         "【明天休息】\n明天（2026年8月8日，周六）休息，无需上班。\n本周为大周。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFridaySaturdayReminderContent(tt.weekLabel, tt.dateLabel, tt.saturdayWork); got != tt.want {
				t.Fatalf("content = %q, want %q", got, tt.want)
			}
		})
	}
}
