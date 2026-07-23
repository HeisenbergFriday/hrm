package database

// DingTalk corp mapping for external attendance import.
// Unknown corp_id / corp_name must be rejected — never fall back to default.

// Known external attendance organizations.
const (
	OrgIDXiaotie = "xiaotie"
	OrgIDMuteng  = "muteng"
)

// ExternalCorpMapping is a fixed org mapping entry.
type ExternalCorpMapping struct {
	OrgID    string
	CorpID   string
	CorpName string
}

// ExternalCorpMappings is the authoritative phase-1 mapping table.
var ExternalCorpMappings = []ExternalCorpMapping{
	{
		OrgID:    OrgIDXiaotie,
		CorpID:   "ding9314d42ff309fbda24f2f5cc6abecb85",
		CorpName: "深圳小铁文娱科技有限公司",
	},
	{
		OrgID:    OrgIDMuteng,
		CorpID:   "dingd723fb3b8d1f3b9f35c2f4657eb6378f",
		CorpName: "深圳市沐腾科技有限公司",
	},
}

// MapOrgIDByCorpID maps Doris attendance corp_id → org_id.
// Returns empty string when unknown.
func MapOrgIDByCorpID(corpID string) string {
	for _, m := range ExternalCorpMappings {
		if m.CorpID == corpID {
			return m.OrgID
		}
	}
	return ""
}

// MapOrgIDByCorpName maps department-relation corp_name → org_id (exact match).
// Returns empty string when unknown.
func MapOrgIDByCorpName(corpName string) string {
	for _, m := range ExternalCorpMappings {
		if m.CorpName == corpName {
			return m.OrgID
		}
	}
	return ""
}

// CorpIDForOrg returns configured corp_id for org_id.
func CorpIDForOrg(orgID string) string {
	orgID = NormalizeOrganizationID(orgID)
	for _, m := range ExternalCorpMappings {
		if m.OrgID == orgID {
			return m.CorpID
		}
	}
	return ""
}

// CorpNameForOrg returns configured corp_name for org_id.
func CorpNameForOrg(orgID string) string {
	orgID = NormalizeOrganizationID(orgID)
	for _, m := range ExternalCorpMappings {
		if m.OrgID == orgID {
			return m.CorpName
		}
	}
	return ""
}

// IsKnownExternalOrg reports whether org is in the external mapping set.
func IsKnownExternalOrg(orgID string) bool {
	orgID = NormalizeOrganizationID(orgID)
	for _, m := range ExternalCorpMappings {
		if m.OrgID == orgID {
			return true
		}
	}
	return false
}
