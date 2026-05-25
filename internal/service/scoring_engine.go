package service

import (
	"math"
	"strconv"
	"strings"
)

// AutoScoreResult 单个指标的自动评分结果
type AutoScoreResult struct {
	RecordID   uint    `json:"record_id"`
	Score      float64 `json:"score"`
	Breakdown  string  `json:"breakdown"`
	AutoScored bool    `json:"auto_scored"`
}

// AutoScoreResponse 自动评分响应
type AutoScoreResponse struct {
	Items      []AutoScoreResult `json:"items"`
	TotalScore float64           `json:"total_score"`
}

// AutoItemInput 自动评分请求项
type AutoItemInput struct {
	RecordID       uint    `json:"record_id"`
	SectionType    string  `json:"section_type"`
	Weight         float64 `json:"weight"`
	RedLineValue   string  `json:"red_line_value"`
	TargetValue    string  `json:"target_value"`
	ChallengeValue string  `json:"challenge_value"`
	ScoringRule    string  `json:"scoring_rule"`
	ActualResult   string  `json:"actual_result"`
}

// CalculateAutoScores 批量自动评分
func CalculateAutoScores(items []AutoItemInput) AutoScoreResponse {
	results := make([]AutoScoreResult, 0, len(items))
	totalScore := 0.0

	for _, item := range items {
		result := calculateSingleScore(item)
		results = append(results, result)
		if item.SectionType != "bonus_penalty" {
			totalScore += result.Score * item.Weight
		}
	}

	return AutoScoreResponse{
		Items:      results,
		TotalScore: roundScore(totalScore),
	}
}

func calculateSingleScore(item AutoItemInput) AutoScoreResult {
	result := AutoScoreResult{
		RecordID: item.RecordID,
	}

	actual := parseNumber(item.ActualResult)
	redLine := parseNumber(item.RedLineValue)
	target := parseNumber(item.TargetValue)
	challenge := parseNumber(item.ChallengeValue)
	rule := parseScoringRuleType(item.ScoringRule)

	// 关键行动类型：无法自动评分
	if item.SectionType == "key_action" {
		result.Breakdown = "关键行动指标无法自动量化评分，请手动打分"
		result.AutoScored = false
		return result
	}

	// 无法解析实际值：无法自动评分
	if actual == nil {
		result.Breakdown = "实际达成值无法解析为数字"
		result.AutoScored = false
		return result
	}

	// 至少需要目标值才能自动评分
	if target == nil {
		result.Breakdown = "目标值未设置，无法自动评分"
		result.AutoScored = false
		return result
	}

	switch rule {
	case "threshold":
		score, breakdown := calcThresholdScore(*actual, redLine, target, challenge)
		result.Score = score
		result.Breakdown = breakdown
		result.AutoScored = true
	case "ratio":
		score, breakdown := calcRatioScore(*actual, *target)
		result.Score = score
		result.Breakdown = breakdown
		result.AutoScored = true
	default:
		// 默认：区间线性插值（interval）
		score, breakdown := calcIntervalScore(*actual, redLine, target, challenge)
		result.Score = score
		result.Breakdown = breakdown
		result.AutoScored = true
	}

	return result
}

// parseNumber 尝试解析数字，支持百分号
func parseNumber(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.TrimRight(s, "%")
	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &val
}

// parseScoringRuleType 从评分规则文本中解析类型
func parseScoringRuleType(rule string) string {
	rule = strings.ToLower(strings.TrimSpace(rule))
	if rule == "" {
		return "interval"
	}
	if strings.Contains(rule, "达标") || strings.Contains(rule, "合格") ||
		strings.Contains(rule, "threshold") || strings.Contains(rule, "pass") {
		return "threshold"
	}
	if strings.Contains(rule, "比率") || strings.Contains(rule, "比例") ||
		strings.Contains(rule, "ratio") || strings.Contains(rule, "percent") {
		return "ratio"
	}
	return "interval"
}

// calcIntervalScore 区间线性插值评分
// 红线以下=50分，红线到目标=50-80分，目标到挑战=80-120分，超挑战=120分封顶
func calcIntervalScore(actual float64, redLine, target, challenge *float64) (float64, string) {
	if target == nil {
		return 50.0, "目标值未设置，无法评分"
	}
	if redLine == nil || challenge == nil {
		t := *target
		if t == 0 {
			if actual >= 0 {
				return 100.0, "目标为0，实际有值，计100分"
			}
			return 50.0, "目标和实际均为0，计50分"
		}
		ratio := actual / t
		if ratio >= 1.0 {
			score := 80.0 + (ratio-1.0)*40.0
			score = math.Min(score, 120.0)
			return roundScore(score), buildBreakdown("ratio_to_target", actual, t, score)
		}
		score := 50.0 + ratio*30.0
		score = math.Max(score, 0)
		return roundScore(score), buildBreakdown("ratio_to_target", actual, t, score)
	}

	rl, t, ch := *redLine, *target, *challenge

	if actual <= rl {
		if rl == 0 {
			return 50.0, "红线=0，实际≤红线，计50分"
		}
		ratio := actual / rl
		score := 30.0 + ratio*20.0
		score = math.Max(score, 0)
		return roundScore(score), buildBreakdown("below_redline", actual, rl, score)
	}

	if actual <= t {
		if t == rl {
			return 80.0, "红线=目标，实际≤目标，计80分"
		}
		ratio := (actual - rl) / (t - rl)
		score := 50.0 + ratio*30.0
		return roundScore(score), buildBreakdown("redline_to_target", actual, rl, t, score)
	}

	if actual <= ch {
		if ch == t {
			return 120.0, "目标=挑战，实际≥目标，计120分"
		}
		ratio := (actual - t) / (ch - t)
		score := 80.0 + ratio*40.0
		return roundScore(score), buildBreakdown("target_to_challenge", actual, t, ch, score)
	}

	return 120.0, buildBreakdown("above_challenge", actual, ch, 120.0)
}

// calcThresholdScore 达标制评分
func calcThresholdScore(actual float64, redLine *float64, target, challenge *float64) (float64, string) {
	if challenge != nil && actual >= *challenge {
		return 120.0, "超越挑战值，计120分"
	}
	if target != nil && actual >= *target {
		return 100.0, "达到目标，计100分"
	}
	if redLine != nil && actual >= *redLine {
		return 80.0, "达到红线（最低要求），计80分"
	}
	if redLine != nil && actual < *redLine {
		return 50.0, "未达红线，计50分"
	}
	if target != nil && actual < *target {
		return 60.0, "未达目标，计60分"
	}
	return 70.0, "无法精确判断，计默认70分"
}

// calcRatioScore 比率制评分：实际/目标 * 100，封顶120
func calcRatioScore(actual, target float64) (float64, string) {
	if target == 0 {
		if actual > 0 {
			return 120.0, "目标为0，实际有值，计120分"
		}
		return 50.0, "目标和实际均为0，计50分"
	}
	ratio := actual / target
	score := ratio * 100.0
	score = math.Min(score, 120.0)
	score = math.Max(score, 0)
	return roundScore(score), "达成率=" + strconv.FormatFloat(ratio*100, 'f', 1, 64) + "%，得分=" + strconv.FormatFloat(score, 'f', 1, 64)
}

func buildBreakdown(phase string, values ...float64) string {
	switch phase {
	case "below_redline":
		return "实际(" + fmtFloat(values[0]) + ")≤红线(" + fmtFloat(values[1]) + ")，得分=" + fmtFloat(values[2])
	case "redline_to_target":
		return "实际(" + fmtFloat(values[0]) + ")介于红线(" + fmtFloat(values[1]) + ")~目标(" + fmtFloat(values[2]) + ")，得分=" + fmtFloat(values[3])
	case "target_to_challenge":
		return "实际(" + fmtFloat(values[0]) + ")介于目标(" + fmtFloat(values[1]) + ")~挑战(" + fmtFloat(values[2]) + ")，得分=" + fmtFloat(values[3])
	case "above_challenge":
		return "实际(" + fmtFloat(values[0]) + ")≥挑战值(" + fmtFloat(values[1]) + ")，得分=" + fmtFloat(values[2])
	case "ratio_to_target":
		return "实际(" + fmtFloat(values[0]) + ")/目标(" + fmtFloat(values[1]) + ")，得分=" + fmtFloat(values[2])
	default:
		return ""
	}
}

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
