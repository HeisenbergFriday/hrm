package service

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParttimeEmployee is a known part-time employee in the org roster. EmployeeNo
// (工号) may be empty for some employees; Name is the display name as stored in
// the org user directory. Position/Department seed the generated grid so the
// part-time summary module's scope filter keeps the row.
type ParttimeEmployee struct {
	EmployeeNo string
	Name       string
	UserID     string
	Position   string
	Department string
}

// EmployeePunchData holds the punch records for one employee as found in the
// DingTalk "月度打卡记录" report. EmployeeNo/Name are taken from the report
// (authoritative DingTalk identity) and Days maps day-of-month to a status label
// understood by the part-time summary parser.
type EmployeePunchData struct {
	EmployeeNo string
	Name       string
	Position   string
	Department string
	Days       map[int]string
}

// ParttimeMonthlyMatch is the result of associating the org part-time roster with
// the DingTalk monthly punch report.
type ParttimeMonthlyMatch struct {
	// Matched employees (roster entry + their punch days). Identity (EmployeeNo,
	// Name) comes from the org roster; Days come from the report.
	Matched []MatchedParttimeEmployee
	// Unmatched roster employees for whom no punch record was found. They are
	// preserved (never silently dropped) and surfaced to the user.
	Unmatched []ParttimeEmployee
	// Anomalies describes non-fatal matching problems (e.g. duplicate names with
	// no employee number) in plain language.
	Anomalies []string
}

// MatchedParttimeEmployee pairs a roster entry with its matched punch data.
type MatchedParttimeEmployee struct {
	EmployeeNo string
	Name       string
	Position   string
	Department string
	Days       map[int]string
	MatchedBy  string // "employee_no" or "name"
}

// statusLabel converts a per-day punch summary into the text the part-time
// parser understands. The parser recognises leading status keywords such as
// 正常/迟到/早退/缺卡/旷工/休息 followed by optional "(HH:MM,HH:MM)" times.
func statusLabel(onDuty, offDuty, timeResult string) string {
	// DingTalk can report an exception without a check timestamp. Preserve the
	// explicit result so the monthly grid does not silently turn that day blank.
	normalizedResult := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(timeResult)))
	status := ""
	switch normalizedResult {
	case "notsigned":
		status = "缺卡"
	case "absenteeism", "absent":
		status = "旷工"
	case "seriouslate":
		status = "严重迟到"
	}
	if status != "" {
		if onDuty != "" && offDuty != "" {
			return status + " (" + onDuty + "," + offDuty + ")"
		}
		if onDuty != "" {
			return status + " (" + onDuty + ")"
		}
		if offDuty != "" {
			return status + " (" + offDuty + ")"
		}
		return status
	}

	hasCheck := onDuty != "" || offDuty != ""
	switch {
	case !hasCheck:
		return ""
	case normalizedResult == "late" && onDuty != "":
		if offDuty != "" {
			return "迟到 (" + onDuty + "," + offDuty + ")"
		}
		return "迟到 (" + onDuty + ")"
	case timeResult == "Early" && offDuty != "":
		if onDuty != "" {
			return "早退 (" + onDuty + "," + offDuty + ")"
		}
		return "早退 (" + offDuty + ")"
	case onDuty != "" && offDuty != "":
		return "正常 (" + onDuty + "," + offDuty + ")"
	case onDuty != "":
		return "正常 (" + onDuty + ")"
	case offDuty != "":
		return "正常 (" + offDuty + ")"
	default:
		return ""
	}
}

// MatchParttimeMonthlyPunch associates the org part-time roster with the
// DingTalk monthly punch report.
//
// Matching rules (per product spec):
//  1. Prefer employee number (工号); fall back to name when the number is empty
//     or cannot be matched.
//  2. Names are cleaned with normalizeParttimeName before comparison.
//  3. Employees are NEVER filtered out because their number does not start with
//     a particular prefix — every roster employee appears in the result.
//  4. Roster employees with no punch record are kept in Unmatched (never dropped).
//  5. When several report rows share the same cleaned name and none carries an
//     employee number, the match is ambiguous and recorded as an anomaly instead
//     of being guessed.
func MatchParttimeMonthlyPunch(roster []ParttimeEmployee, punched []EmployeePunchData) ParttimeMonthlyMatch {
	match := ParttimeMonthlyMatch{}

	// Defensive copies so callers' slices are not mutated.
	roster = append([]ParttimeEmployee(nil), roster...)
	punched = append([]EmployeePunchData(nil), punched...)

	cleanedRoster := make([]ParttimeEmployee, len(roster))
	for i, e := range roster {
		cleanedRoster[i] = ParttimeEmployee{
			EmployeeNo: normalizeEmployeeNo(e.EmployeeNo),
			Name:       normalizeParttimeName(e.Name),
			UserID:     strings.TrimSpace(e.UserID),
			Position:   strings.TrimSpace(e.Position),
			Department: strings.TrimSpace(e.Department),
		}
	}

	cleanedPunched := make([]EmployeePunchData, len(punched))
	for i, p := range punched {
		cleanedPunched[i] = EmployeePunchData{
			EmployeeNo: normalizeEmployeeNo(p.EmployeeNo),
			Name:       normalizeParttimeName(p.Name),
			Days:       p.Days,
		}
	}

	// Index report rows by employee number and by cleaned name.
	byCode := map[string][]int{}
	for i, p := range cleanedPunched {
		if p.EmployeeNo == "" {
			continue
		}
		byCode[p.EmployeeNo] = append(byCode[p.EmployeeNo], i)
	}
	byName := map[string][]int{}
	for i, p := range cleanedPunched {
		if p.Name == "" {
			continue
		}
		byName[p.Name] = append(byName[p.Name], i)
	}

	used := make([]bool, len(cleanedPunched))

	for _, emp := range cleanedRoster {
		// Rule 1: prefer employee number.
		if emp.EmployeeNo != "" {
			if idxs := byCode[emp.EmployeeNo]; len(idxs) > 0 {
				idx := pickByID(idxs, used, cleanedPunched, emp)
				if idx >= 0 {
					used[idx] = true
					match.Matched = append(match.Matched, MatchedParttimeEmployee{
						EmployeeNo: emp.EmployeeNo,
						Name:       emp.Name,
						Position:   emp.Position,
						Department: emp.Department,
						Days:       cleanedPunched[idx].Days,
						MatchedBy:  "employee_no",
					})
					continue
				}
			}
		}

		// Fallback: match by cleaned name.
		if emp.Name != "" {
			nameHits := byName[emp.Name]
			switch {
			case len(nameHits) == 1:
				idx := nameHits[0]
				if !used[idx] {
					used[idx] = true
				}
				match.Matched = append(match.Matched, MatchedParttimeEmployee{
					EmployeeNo: emp.EmployeeNo,
					Name:       emp.Name,
					Position:   emp.Position,
					Department: emp.Department,
					Days:       cleanedPunched[idx].Days,
					MatchedBy:  "name",
				})
				continue
			case len(nameHits) > 1:
				// Rule 5: same name, no way to disambiguate by number on the
				// roster side. Record as anomaly; do not guess.
				match.Anomalies = append(match.Anomalies,
					"姓名「"+emp.Name+"」在打卡记录中出现多次且工号缺失，无法唯一匹配，已保留为未匹配")
				match.Unmatched = append(match.Unmatched, emp)
				continue
			}
		}

		// Rule 4: keep roster employees even when no punch record exists.
		match.Unmatched = append(match.Unmatched, emp)
	}

	sort.Slice(match.Matched, func(i, j int) bool {
		return match.Matched[i].Name < match.Matched[j].Name
	})
	sort.Slice(match.Unmatched, func(i, j int) bool {
		return match.Unmatched[i].Name < match.Unmatched[j].Name
	})
	sort.Strings(match.Anomalies)

	return match
}

// pickByID chooses the best report row for an employee-number match. If one of
// the candidates also matches the roster name, prefer it; otherwise take the
// first unused candidate.
func pickByID(candidates []int, used []bool, punched []EmployeePunchData, emp ParttimeEmployee) int {
	if len(candidates) == 0 {
		return -1
	}
	if emp.Name == "" {
		for _, idx := range candidates {
			if !used[idx] {
				return idx
			}
		}
		return candidates[0]
	}
	withName := -1
	for _, idx := range candidates {
		if punched[idx].Name == emp.Name {
			withName = idx
			if !used[idx] {
				return idx
			}
		}
	}
	if withName >= 0 {
		return withName
	}
	for _, idx := range candidates {
		if !used[idx] {
			return idx
		}
	}
	return candidates[0]
}

// normalizeEmployeeNo standardises an employee number for comparison: trimmed,
// upper-cased, surrounding full-width parentheses stripped. An empty or
// non-numeric-looking value is kept as-is (callers treat "" as "no number").
func normalizeEmployeeNo(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "（）()")
	return strings.ToUpper(s)
}

// normalizeParttimeName mirrors the Python part-time parser's name cleaning:
// strip common resignation suffixes (e.g. "（离职）") and collapse whitespace.
func normalizeParttimeName(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := s
		next = strings.TrimRight(next, "（离职）")
		next = strings.TrimRight(next, "(已离职)")
		next = strings.TrimRight(next, "（离职")
		next = strings.TrimRight(next, "离职）")
		if next == s {
			break
		}
		s = next
		s = strings.TrimSpace(s)
	}
	// Collapse all whitespace runs, matching the Python \s+ collapse.
	fields := strings.Fields(s)
	return strings.Join(fields, "")
}

// dayOf extracts the day-of-month from a "YYYY-MM-DD" date string.
func dayOf(date string) int {
	date = strings.TrimSpace(date)
	if len(date) >= 10 {
		day, err := strconv.Atoi(date[8:10])
		if err == nil {
			return day
		}
	}
	return 0
}

// hhmm renders a millisecond Unix timestamp as "HH:MM" in the given location,
// or "" when the value cannot be parsed.
func hhmm(value string, loc *time.Location) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return ""
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond)).In(loc)
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}
