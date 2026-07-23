package service

import (
	"mime/multipart"
	"testing"
)

func TestMapLegacyProcessingForm_LeaveAndOvertime(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{},
		File: map[string][]*multipart.FileHeader{
			"input":    {{Filename: "请假系统导出.xlsx", Size: 10}},
			"schedule": {{Filename: "作息表.xlsx", Size: 10}},
		},
	}
	mapped := mapLegacyProcessingForm("leave", form)
	if len(mapped.File["leave_src"]) != 1 {
		t.Fatalf("leave_src missing: %+v", mapped.File)
	}
	if len(mapped.File["leave_schedule"]) != 1 {
		t.Fatalf("leave_schedule missing")
	}

	ot := &multipart.Form{
		Value: map[string][]string{},
		File: map[string][]*multipart.FileHeader{
			"input":      {{Filename: "加班.xlsx", Size: 10}},
			"attendance": {{Filename: "打卡.xlsx", Size: 10}},
			"roster":     {{Filename: "花名册.xlsx", Size: 10}},
			"schedule":   {{Filename: "排班1.xlsx", Size: 10}, {Filename: "排班2.xlsx", Size: 10}},
		},
	}
	mappedOT := mapLegacyProcessingForm("overtime", ot)
	if len(mappedOT.File["overtime_src"]) != 1 {
		t.Fatalf("overtime_src missing")
	}
	if len(mappedOT.File["overtime_schedules"]) != 2 {
		t.Fatalf("overtime_schedules want 2 got %d", len(mappedOT.File["overtime_schedules"]))
	}
}

func TestMapLegacyProcessingForm_FinalSubsidyParttime(t *testing.T) {
	final := mapLegacyProcessingForm("final", &multipart.Form{
		File: map[string][]*multipart.FileHeader{
			"roster":   {{Filename: "在职.xlsx", Size: 1}},
			"schedule": {{Filename: "作息.xlsx", Size: 1}},
			"leave":    {{Filename: "假.xlsx", Size: 1}},
			"overtime": {{Filename: "加.xlsx", Size: 1}},
			"subsidy":  {{Filename: "补.xlsx", Size: 1}},
			"resigned": {{Filename: "离.xlsx", Size: 1}},
			"transfer": {{Filename: "异.xlsx", Size: 1}},
		},
	})
	for _, key := range []string{"final_active", "final_schedule", "final_leave", "final_overtime", "final_subsidy", "final_resign", "final_transfer"} {
		if len(final.File[key]) != 1 {
			t.Fatalf("missing %s", key)
		}
	}

	sub := mapLegacyProcessingForm("subsidy", &multipart.Form{
		File: map[string][]*multipart.FileHeader{
			"source":     {{Filename: "s.xlsx", Size: 1}},
			"attendance": {{Filename: "a.xlsx", Size: 1}},
			"schedule":   {{Filename: "c.xlsx", Size: 1}},
			"signin":     {{Filename: "i.xlsx", Size: 1}},
			"result":     {{Filename: "r.xlsx", Size: 1}},
		},
	})
	for _, key := range []string{"subsidy_src", "subsidy_attendance", "subsidy_schedule", "subsidy_checkin", "subsidy_attendance_result"} {
		if len(sub.File[key]) != 1 {
			t.Fatalf("missing %s", key)
		}
	}

	pt := mapLegacyProcessingForm("parttime", &multipart.Form{
		File: map[string][]*multipart.FileHeader{
			"default_schedule":  {{Filename: "d.xlsx", Size: 1}},
			"attendance_detail": {{Filename: "ad.xlsx", Size: 1}},
			"monthly_summary":   {{Filename: "m1.xlsx", Size: 1}, {Filename: "m2.xlsx", Size: 1}},
			"schedule":          {{Filename: "s1.xlsx", Size: 1}},
		},
	})
	if len(pt.File["parttime_monthly"]) != 2 {
		t.Fatalf("parttime_monthly want 2")
	}
	if len(pt.File["parttime_schedules"]) != 1 {
		t.Fatalf("parttime_schedules want 1")
	}
}

func TestMapHasCustomRules(t *testing.T) {
	if !MapHasCustomRules(map[string][]string{"rules_json": {`{"a":1}`}}, nil) {
		t.Fatal("rules_json should count")
	}
	if !MapHasCustomRules(nil, map[string]int{"rules_file": 1}) {
		t.Fatal("rules_file should count")
	}
	if MapHasCustomRules(map[string][]string{"rules_json": {""}}, map[string]int{"rules_file": 0}) {
		t.Fatal("empty should not count")
	}
}
