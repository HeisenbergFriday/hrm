package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"
)

func TestSyncOrgDataUsesOperationalDatabaseInitialization(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	fullInitCalls := 0
	operationalInitCalls := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if !ok || packageName.Name != "database" {
			return true
		}
		switch selector.Sel.Name {
		case "Init":
			fullInitCalls++
		case "InitOperational":
			operationalInitCalls++
		}
		return true
	})

	if fullInitCalls != 0 {
		t.Fatalf("sync_org_data must not call database.Init; found %d call(s)", fullInitCalls)
	}
	if operationalInitCalls != 1 {
		t.Fatalf("sync_org_data must call database.InitOperational exactly once; found %d", operationalInitCalls)
	}
}

func TestValidateOptionsRequiresExplicitOrganizationForScopedModes(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		confirm        bool
		diagnoseCounts bool
	}{
		{name: "sync", confirm: true},
		{name: "diagnose", diagnoseCounts: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			orgID, err := validateOptions("  ", testCase.confirm, testCase.diagnoseCounts, false, time.Minute)
			if err == nil {
				t.Fatal("expected missing organization to fail closed")
			}
			if orgID != "" {
				t.Fatalf("missing organization must not fall back to a default: %q", orgID)
			}
		})
	}
}

func TestValidateOptionsKeepsExplicitOrganization(t *testing.T) {
	orgID, err := validateOptions("  muteng  ", true, false, false, time.Minute)
	if err != nil {
		t.Fatalf("validate explicit organization: %v", err)
	}
	if orgID != "muteng" {
		t.Fatalf("unexpected organization: %q", orgID)
	}
}
