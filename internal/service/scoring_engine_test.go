package service

import (
	"math"
	"testing"
)

func TestCalculateAutoScoresCoversRuleTypes(t *testing.T) {
	response := CalculateAutoScores([]AutoItemInput{
		{
			RecordID:       1,
			SectionType:    "quantitative",
			Weight:         0.5,
			RedLineValue:   "60",
			TargetValue:    "100",
			ChallengeValue: "120",
			ScoringRule:    "interval",
			ActualResult:   "110",
		},
		{
			RecordID:     2,
			SectionType:  "quantitative",
			Weight:       0.5,
			TargetValue:  "200",
			ScoringRule:  "ratio",
			ActualResult: "100",
		},
		{
			RecordID:     3,
			SectionType:  "bonus_penalty",
			Weight:       1,
			TargetValue:  "10",
			ScoringRule:  "threshold",
			ActualResult: "10",
		},
	})

	if len(response.Items) != 3 {
		t.Fatalf("Items length = %d, want 3", len(response.Items))
	}
	if response.Items[0].Score != 100 {
		t.Fatalf("interval score = %v, want 100", response.Items[0].Score)
	}
	if response.Items[1].Score != 50 {
		t.Fatalf("ratio score = %v, want 50", response.Items[1].Score)
	}
	if response.Items[2].Score != 100 {
		t.Fatalf("threshold bonus score = %v, want 100", response.Items[2].Score)
	}
	if response.TotalScore != 75 {
		t.Fatalf("TotalScore = %v, want 75", response.TotalScore)
	}
}

func TestCalculateSingleScoreBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		item       AutoItemInput
		wantScore  float64
		wantScored bool
	}{
		{
			name: "key action is manual",
			item: AutoItemInput{
				RecordID:     1,
				SectionType:  "key_action",
				TargetValue:  "100",
				ActualResult: "100",
			},
			wantScored: false,
		},
		{
			name: "invalid actual is not scored",
			item: AutoItemInput{
				RecordID:     2,
				SectionType:  "quantitative",
				TargetValue:  "100",
				ActualResult: "n/a",
			},
			wantScored: false,
		},
		{
			name: "missing target is not scored",
			item: AutoItemInput{
				RecordID:     3,
				SectionType:  "quantitative",
				ActualResult: "80",
			},
			wantScored: false,
		},
		{
			name: "interval below red line",
			item: AutoItemInput{
				RecordID:       4,
				SectionType:    "quantitative",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "30",
			},
			wantScore:  40,
			wantScored: true,
		},
		{
			name: "interval at red line",
			item: AutoItemInput{
				RecordID:       5,
				SectionType:    "quantitative",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "60",
			},
			wantScore:  50,
			wantScored: true,
		},
		{
			name: "interval at target",
			item: AutoItemInput{
				RecordID:       6,
				SectionType:    "quantitative",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "100",
			},
			wantScore:  80,
			wantScored: true,
		},
		{
			name: "interval at challenge",
			item: AutoItemInput{
				RecordID:       7,
				SectionType:    "quantitative",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "120",
			},
			wantScore:  120,
			wantScored: true,
		},
		{
			name: "interval above challenge is capped",
			item: AutoItemInput{
				RecordID:       8,
				SectionType:    "quantitative",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "140",
			},
			wantScore:  120,
			wantScored: true,
		},
		{
			name: "target only below target",
			item: AutoItemInput{
				RecordID:     9,
				SectionType:  "quantitative",
				TargetValue:  "100",
				ActualResult: "50",
			},
			wantScore:  65,
			wantScored: true,
		},
		{
			name: "target only above target",
			item: AutoItemInput{
				RecordID:     10,
				SectionType:  "quantitative",
				TargetValue:  "100",
				ActualResult: "150",
			},
			wantScore:  100,
			wantScored: true,
		},
		{
			name: "ratio target zero with positive actual",
			item: AutoItemInput{
				RecordID:     11,
				SectionType:  "quantitative",
				ScoringRule:  "ratio",
				TargetValue:  "0",
				ActualResult: "1",
			},
			wantScore:  120,
			wantScored: true,
		},
		{
			name: "ratio target zero with zero actual",
			item: AutoItemInput{
				RecordID:     12,
				SectionType:  "quantitative",
				ScoringRule:  "ratio",
				TargetValue:  "0",
				ActualResult: "0",
			},
			wantScore:  50,
			wantScored: true,
		},
		{
			name: "threshold below red line",
			item: AutoItemInput{
				RecordID:     13,
				SectionType:  "quantitative",
				ScoringRule:  "threshold",
				RedLineValue: "60",
				TargetValue:  "100",
				ActualResult: "50",
			},
			wantScore:  50,
			wantScored: true,
		},
		{
			name: "threshold at red line",
			item: AutoItemInput{
				RecordID:     14,
				SectionType:  "quantitative",
				ScoringRule:  "threshold",
				RedLineValue: "60",
				TargetValue:  "100",
				ActualResult: "60",
			},
			wantScore:  80,
			wantScored: true,
		},
		{
			name: "threshold at target",
			item: AutoItemInput{
				RecordID:     15,
				SectionType:  "quantitative",
				ScoringRule:  "threshold",
				RedLineValue: "60",
				TargetValue:  "100",
				ActualResult: "100",
			},
			wantScore:  100,
			wantScored: true,
		},
		{
			name: "threshold at challenge",
			item: AutoItemInput{
				RecordID:       16,
				SectionType:    "quantitative",
				ScoringRule:    "threshold",
				RedLineValue:   "60",
				TargetValue:    "100",
				ChallengeValue: "120",
				ActualResult:   "120",
			},
			wantScore:  120,
			wantScored: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSingleScore(tt.item)
			if got.AutoScored != tt.wantScored {
				t.Fatalf("AutoScored = %v, want %v", got.AutoScored, tt.wantScored)
			}
			if got.Score != tt.wantScore {
				t.Fatalf("Score = %v, want %v", got.Score, tt.wantScore)
			}
		})
	}
}

func TestCalculateSingleScoreAdditionalEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		item       AutoItemInput
		wantScore  float64
		wantScored bool
	}{
		{
			name: "reverse wording currently keeps interval semantics",
			item: AutoItemInput{
				RecordID:       21,
				SectionType:    "quantitative",
				RedLineValue:   "120",
				TargetValue:    "100",
				ChallengeValue: "50",
				ScoringRule:    "reverse lower is better",
				ActualResult:   "80",
			},
			wantScore:  43.33,
			wantScored: true,
		},
		{
			name: "ratio extreme actual is capped",
			item: AutoItemInput{
				RecordID:     22,
				SectionType:  "quantitative",
				ScoringRule:  "ratio",
				TargetValue:  "1",
				ActualResult: "999999999999",
			},
			wantScore:  120,
			wantScored: true,
		},
		{
			name: "target only small decimal keeps rounded precision",
			item: AutoItemInput{
				RecordID:     23,
				SectionType:  "quantitative",
				TargetValue:  "0.07",
				ActualResult: "0.02",
			},
			wantScore:  58.57,
			wantScored: true,
		},
		{
			name: "ratio negative actual clamps to zero",
			item: AutoItemInput{
				RecordID:     24,
				SectionType:  "quantitative",
				ScoringRule:  "ratio",
				TargetValue:  "100",
				ActualResult: "-20",
			},
			wantScore:  0,
			wantScored: true,
		},
		{
			name: "threshold accepts negative red line range",
			item: AutoItemInput{
				RecordID:     25,
				SectionType:  "quantitative",
				ScoringRule:  "threshold",
				RedLineValue: "-10",
				TargetValue:  "0",
				ActualResult: "-5",
			},
			wantScore:  80,
			wantScored: true,
		},
		{
			name: "non finite actual is not scored",
			item: AutoItemInput{
				RecordID:     26,
				SectionType:  "quantitative",
				ScoringRule:  "ratio",
				TargetValue:  "100",
				ActualResult: "NaN",
			},
			wantScored: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSingleScore(tt.item)
			if got.AutoScored != tt.wantScored {
				t.Fatalf("AutoScored = %v, want %v", got.AutoScored, tt.wantScored)
			}
			if !almostEqual(got.Score, tt.wantScore) {
				t.Fatalf("Score = %v, want %v", got.Score, tt.wantScore)
			}
		})
	}
}

func TestCalculateAutoScoresWeightEdgeCases(t *testing.T) {
	response := CalculateAutoScores([]AutoItemInput{
		{
			RecordID:     31,
			SectionType:  "quantitative",
			Weight:       1.25,
			ScoringRule:  "threshold",
			TargetValue:  "100",
			ActualResult: "100",
		},
		{
			RecordID:     32,
			SectionType:  "quantitative",
			Weight:       -0.5,
			ScoringRule:  "ratio",
			TargetValue:  "100",
			ActualResult: "50",
		},
		{
			RecordID:     33,
			SectionType:  "quantitative",
			Weight:       0,
			ScoringRule:  "threshold",
			TargetValue:  "100",
			ActualResult: "100",
		},
		{
			RecordID:     34,
			SectionType:  "bonus_penalty",
			Weight:       999,
			ScoringRule:  "threshold",
			TargetValue:  "10",
			ActualResult: "10",
		},
	})

	if len(response.Items) != 4 {
		t.Fatalf("Items length = %d, want 4", len(response.Items))
	}
	if !almostEqual(response.TotalScore, 100) {
		t.Fatalf("TotalScore = %v, want 100", response.TotalScore)
	}
	if !almostEqual(response.Items[3].Score, 100) {
		t.Fatalf("bonus score = %v, want 100", response.Items[3].Score)
	}
}

func TestParseNumberAndScoringRuleType(t *testing.T) {
	if got := parseNumber(" 85% "); got == nil || *got != 85 {
		t.Fatalf("parseNumber percent = %v, want 85", got)
	}
	if got := parseNumber("-12.5%"); got == nil || *got != -12.5 {
		t.Fatalf("parseNumber negative percent = %v, want -12.5", got)
	}
	if got := parseNumber("not-a-number"); got != nil {
		t.Fatalf("parseNumber invalid = %v, want nil", *got)
	}
	if got := parseNumber("NaN"); got != nil {
		t.Fatalf("parseNumber NaN = %v, want nil", *got)
	}
	if got := parseNumber("+Inf"); got != nil {
		t.Fatalf("parseNumber +Inf = %v, want nil", *got)
	}

	if got := parseScoringRuleType(""); got != "interval" {
		t.Fatalf("empty rule = %q, want interval", got)
	}
	if got := parseScoringRuleType("pass threshold"); got != "threshold" {
		t.Fatalf("threshold rule = %q, want threshold", got)
	}
	if got := parseScoringRuleType("ratio percent"); got != "ratio" {
		t.Fatalf("ratio rule = %q, want ratio", got)
	}
	if got := parseScoringRuleType("custom manual"); got != "interval" {
		t.Fatalf("custom rule = %q, want interval", got)
	}
	if got := parseScoringRuleType("reverse lower is better"); got != "interval" {
		t.Fatalf("reverse rule = %q, want interval", got)
	}
}

func almostEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}
