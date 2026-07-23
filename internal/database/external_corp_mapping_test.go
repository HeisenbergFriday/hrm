package database

import (
	"testing"
)

func TestMapOrgIDByCorpIDAndName(t *testing.T) {
	if got := MapOrgIDByCorpID("ding9314d42ff309fbda24f2f5cc6abecb85"); got != OrgIDXiaotie {
		t.Fatalf("xiaotie corp_id map = %q", got)
	}
	if got := MapOrgIDByCorpID("dingd723fb3b8d1f3b9f35c2f4657eb6378f"); got != OrgIDMuteng {
		t.Fatalf("muteng corp_id map = %q", got)
	}
	if got := MapOrgIDByCorpID("unknown"); got != "" {
		t.Fatalf("unknown corp_id should be empty, got %q", got)
	}
	if got := MapOrgIDByCorpName("深圳小铁文娱科技有限公司"); got != OrgIDXiaotie {
		t.Fatalf("xiaotie corp_name map = %q", got)
	}
	if got := MapOrgIDByCorpName("深圳市沐腾科技有限公司"); got != OrgIDMuteng {
		t.Fatalf("muteng corp_name map = %q", got)
	}
	if got := MapOrgIDByCorpName("不存在的公司"); got != "" {
		t.Fatalf("unknown corp_name should be empty, got %q", got)
	}
}

func TestScopedExternalIDOnce(t *testing.T) {
	id := ScopedExternalID(OrgIDXiaotie, "1001")
	if id != "xiaotie:1001" {
		t.Fatalf("unexpected scoped id: %s", id)
	}
	// must not double-scope
	if again := ScopedExternalID(OrgIDXiaotie, id); again != id {
		t.Fatalf("double scope changed id: %s -> %s", id, again)
	}
}

func TestIsKnownExternalOrg(t *testing.T) {
	if !IsKnownExternalOrg("xiaotie") || !IsKnownExternalOrg("muteng") {
		t.Fatal("expected known orgs")
	}
	if IsKnownExternalOrg("default") {
		t.Fatal("default must not be treated as external attendance org")
	}
}
