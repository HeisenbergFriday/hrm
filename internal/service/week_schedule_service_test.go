package service

import "testing"

func TestResolveEffectiveShiftAssignmentPrefersMostSpecificSource(t *testing.T) {
	assignment := resolveEffectiveShiftAssignment(
		100,
		"user-1",
		"dept-1",
		map[string]int64{"user-1": 301},
		map[string]int64{"user-1": 201},
		map[string]int64{"dept-1": 151},
		101,
	)

	if assignment.Source != "custom" {
		t.Fatalf("expected custom source, got %q", assignment.Source)
	}
	if assignment.ShiftID != 301 {
		t.Fatalf("expected custom shift id 301, got %d", assignment.ShiftID)
	}
}

func TestResolveEffectiveShiftAssignmentFallsBackToDefault(t *testing.T) {
	assignment := resolveEffectiveShiftAssignment(
		100,
		"user-1",
		"dept-1",
		map[string]int64{},
		map[string]int64{},
		map[string]int64{},
		0,
	)

	if assignment.Source != "default" {
		t.Fatalf("expected default source, got %q", assignment.Source)
	}
	if assignment.ShiftID != 100 {
		t.Fatalf("expected default shift id 100, got %d", assignment.ShiftID)
	}
}

func TestInferCompanyWeekRuleFromSignalsReturnsBigFirstBaseDate(t *testing.T) {
	signals := []saturdayScheduleSignal{
		{Date: "2026-04-11", WorkUsers: 0, RestUsers: 3, UnknownUsers: 0},
		{Date: "2026-04-18", WorkUsers: 3, RestUsers: 0, UnknownUsers: 0},
		{Date: "2026-04-25", WorkUsers: 0, RestUsers: 3, UnknownUsers: 0},
	}

	inference, err := inferCompanyWeekRuleFromSignals(signals)
	if err != nil {
		t.Fatalf("expected a valid inference, got error: %v", err)
	}

	if inference.Pattern != "big_first" {
		t.Fatalf("expected big_first pattern, got %q", inference.Pattern)
	}
	if inference.BaseDate != "2026-04-06" {
		t.Fatalf("expected base date 2026-04-06, got %s", inference.BaseDate)
	}
}

func TestInferCompanyWeekRuleFromSignalsRejectsInconsistentPattern(t *testing.T) {
	signals := []saturdayScheduleSignal{
		{Date: "2026-04-11", WorkUsers: 3, RestUsers: 0, UnknownUsers: 0},
		{Date: "2026-04-18", WorkUsers: 3, RestUsers: 0, UnknownUsers: 0},
	}

	if _, err := inferCompanyWeekRuleFromSignals(signals); err == nil {
		t.Fatal("expected inconsistent saturday pattern to be rejected")
	}
}

func TestInferCompanyWeekRuleFromSignalsSkipsUncleanSamples(t *testing.T) {
	signals := []saturdayScheduleSignal{
		{Date: "2026-04-11", WorkUsers: 0, RestUsers: 2, UnknownUsers: 1},
		{Date: "2026-04-18", WorkUsers: 3, RestUsers: 0, UnknownUsers: 0},
	}

	if _, err := inferCompanyWeekRuleFromSignals(signals); err == nil {
		t.Fatal("expected missing-user saturday sample to block inference")
	}
}
