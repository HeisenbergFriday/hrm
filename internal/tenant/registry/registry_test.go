package registry

import "testing"

func TestAllTablesUniqueAndClassified(t *testing.T) {
	seen := make(map[string]struct{}, len(entries))
	for _, tbl := range entries {
		if tbl.Name == "" {
			t.Fatal("registry contains empty table name")
		}
		if _, dup := seen[tbl.Name]; dup {
			t.Fatalf("duplicate table entry: %s", tbl.Name)
		}
		seen[tbl.Name] = struct{}{}
		switch tbl.Kind {
		case KindTenant, KindMembership, KindPlatform:
			// ok
		default:
			t.Fatalf("table %s has unknown Kind %d", tbl.Name, tbl.Kind)
		}
	}
}

func TestPlatformTablesExcludeTenantColumns(t *testing.T) {
	// Platform 表不能带 HasOrgID=true 或 Priority 非零；它们不做租户归属。
	for _, tbl := range PlatformTables() {
		if tbl.HasOrgID {
			t.Fatalf("platform table %s should not carry HasOrgID", tbl.Name)
		}
		if tbl.Priority != 0 {
			t.Fatalf("platform table %s should not carry Priority", tbl.Name)
		}
	}
}

func TestTenantAuditKnownTables(t *testing.T) {
	// 冒烟守护：几张关键表必须存在于清单中，并且分类正确。
	cases := []struct {
		name string
		kind Kind
	}{
		{"users", KindTenant},
		{"organizations", KindPlatform},
		{"organization_users", KindMembership},
		{"approvals", KindTenant},
		{"performance_activities", KindTenant},
		{"roles", KindPlatform},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Find(tc.name)
			if !ok {
				t.Fatalf("registry missing table %q", tc.name)
			}
			if got.Kind != tc.kind {
				t.Fatalf("%s kind = %d, want %d", tc.name, got.Kind, tc.kind)
			}
		})
	}
}

func TestTablesMissingOrgIDEmptyAfterExpand(t *testing.T) {
	// Phase 3B 后，所有 tenant/membership 模型都应该已有 nullable org_id 字段。
	if missing := TablesMissingOrgID(); len(missing) != 0 {
		names := make([]string, 0, len(missing))
		for _, tbl := range missing {
			names = append(names, tbl.Name)
		}
		t.Fatalf("all tenant tables should have OrgID model field after schema expand, missing: %v", names)
	}
}

func TestParentTableRefersToKnownEntry(t *testing.T) {
	// ParentTable 必须指向 registry 中另一张 tenant/membership 表，避免打错名。
	for _, tbl := range entries {
		if tbl.ParentTable == "" {
			continue
		}
		parent, ok := Find(tbl.ParentTable)
		if !ok {
			t.Fatalf("table %s parent %q not in registry", tbl.Name, tbl.ParentTable)
		}
		if parent.Kind == KindPlatform {
			t.Fatalf("table %s parent %q is platform-global (invalid tenant parent)", tbl.Name, tbl.ParentTable)
		}
	}
}
