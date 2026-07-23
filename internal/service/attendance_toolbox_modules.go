package service

import "strings"

// Standard calculation modules allowed on generic run/workflow routes.
// dingtalk_sync and quick must use dedicated routes with dedicated permissions.
var AttendanceToolboxStandardModules = map[string]struct{}{
	"leave":    {},
	"overtime": {},
	"subsidy":  {},
	"final":    {},
	"parttime": {},
}

// IsAttendanceToolboxStandardModule reports whether module may be used on
// /toolbox/:module/run and /toolbox/workflows/:module.
func IsAttendanceToolboxStandardModule(module string) bool {
	_, ok := AttendanceToolboxStandardModules[strings.TrimSpace(module)]
	return ok
}

// MapHasCustomRules checks config map / form values for rules payload.
func MapHasCustomRules(values map[string][]string, files map[string]int) bool {
	if values != nil {
		if v := strings.TrimSpace(firstValue(values["rules_json"])); v != "" {
			return true
		}
	}
	if files != nil && files["rules_file"] > 0 {
		return true
	}
	return false
}

func firstValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// ConfigMapHasCustomRules checks runner config after form save.
func ConfigMapHasCustomRules(config map[string]interface{}) bool {
	if config == nil {
		return false
	}
	if v, ok := config["rules_json"]; ok {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return true
			}
		case map[string]interface{}:
			if len(t) > 0 {
				return true
			}
		}
	}
	if v, ok := config["rules_file"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}
