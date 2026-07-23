package database

import "testing"

func TestOrganizationExtensionWithDingTalkProcessCodesPreservesExistingValues(t *testing.T) {
	extension := organizationExtensionWithDingTalkProcessCodes(
		map[string]interface{}{"existing": "value"},
		map[string]string{
			"leave":    " leave-code ",
			"overtime": "overtime-code",
		},
	)

	if extension["existing"] != "value" {
		t.Fatalf("existing extension value was lost: %#v", extension)
	}
	codes := organizationDingTalkProcessCodes(extension)
	if codes["leave"] != "leave-code" || codes["overtime"] != "overtime-code" {
		t.Fatalf("unexpected process codes: %#v", codes)
	}
}

func TestOrganizationFromEnvConfigStoresProcessCodesInExtension(t *testing.T) {
	org := organizationFromEnvConfig(envOrganization{
		OrgID:        "muteng",
		Name:         "Muteng",
		CorpID:       "corp-muteng",
		ProcessCodes: map[string]string{"leave": "leave-code"},
	})

	codes := organizationDingTalkProcessCodes(org.Extension)
	if codes["leave"] != "leave-code" {
		t.Fatalf("organization process codes = %#v", codes)
	}
}
