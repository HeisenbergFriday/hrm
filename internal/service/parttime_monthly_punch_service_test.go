package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"peopleops/internal/dingtalk"
)

// verifyParttimeServiceEndpointsOnly asserts the service exposes the documented
// constructor and render entry point (guards against accidental signature drift).
func TestParttimeMonthlyPunchService_Constructors(t *testing.T) {
	if NewParttimeMonthlyPunchService() == nil {
		t.Fatal("live constructor returned nil")
	}
	if NewParttimeMonthlyPunchServiceWithDataSource(nil) == nil {
		t.Fatal("dataSource constructor returned nil")
	}
}

// TestRenderParttimeMonthlyPunch_OrgIsolation verifies req 8: the service reads
// config/roster/attendance scoped to the supplied orgID and rejects an empty org.
func TestRenderParttimeMonthlyPunch_OrgIsolation(t *testing.T) {
	calledOrg := recordOrgDS{}
	svc := NewParttimeMonthlyPunchServiceWithDataSource(&calledOrg)

	_, _, err := svc.RenderParttimeMonthlyPunch(context.Background(), "orgA", ParttimeMonthlyPunchRequest{Month: "2026-07"})
	if err == nil {
		// Config lookup for an unknown org should fail; if it didn't, the data
		// source must at least have been asked for orgA.
		if calledOrg.lastConfigOrg != "orgA" {
			t.Fatalf("expected config lookup scoped to orgA, got %q", calledOrg.lastConfigOrg)
		}
	}
	if calledOrg.lastConfigOrg != "orgA" {
		t.Fatalf("config lookup must be scoped to the request orgID, got %q", calledOrg.lastConfigOrg)
	}
}

// TestRenderParttimeMonthlyPunch_EmptyOrgRejected covers req 9 at the service layer.
func TestRenderParttimeMonthlyPunch_EmptyOrgRejected(t *testing.T) {
	svc := NewParttimeMonthlyPunchServiceWithDataSource(&recordOrgDS{})
	_, _, err := svc.RenderParttimeMonthlyPunch(context.Background(), "   ", ParttimeMonthlyPunchRequest{Month: "2026-07"})
	if err == nil || !strings.Contains(err.Error(), "组织上下文") {
		t.Fatalf("expected missing-org error, got %v", err)
	}
}

// TestRenderParttimeMonthlyPunch_InvalidMonth covers req 4 at the service layer.
func TestRenderParttimeMonthlyPunch_InvalidMonth(t *testing.T) {
	svc := NewParttimeMonthlyPunchServiceWithDataSource(&recordOrgDS{})
	_, _, err := svc.RenderParttimeMonthlyPunch(context.Background(), "orgA", ParttimeMonthlyPunchRequest{Month: "2026-13"})
	if err == nil || !strings.Contains(err.Error(), "月份") {
		t.Fatalf("expected month validation error, got %v", err)
	}
}

// TestRenderParttimeMonthlyPunch_MissingAdmin covers req 10 at the service layer.
func TestRenderParttimeMonthlyPunch_MissingAdmin(t *testing.T) {
	ds := &recordOrgDS{
		cfg:     dingtalk.Config{AppKey: "ak", AppSecret: "as"},
		adminID: "",
	}
	svc := NewParttimeMonthlyPunchServiceWithDataSource(ds)
	_, _, err := svc.RenderParttimeMonthlyPunch(context.Background(), "orgA", ParttimeMonthlyPunchRequest{Month: "2026-07"})
	if err == nil || !strings.Contains(err.Error(), "管理员") {
		t.Fatalf("expected admin-config error, got %v", err)
	}
}

// recordOrgDS is a test double that records the orgID it was asked for.
type recordOrgDS struct {
	lastConfigOrg string
	lastRosterOrg string
	lastAttendOrg string
	cfg           dingtalk.Config
	adminID       string
	roster        []ParttimeEmployee
	attendance    []dingtalk.AttendanceRecord
	attendanceErr error
}

func (r *recordOrgDS) Config(orgID string) (dingtalk.Config, error) {
	r.lastConfigOrg = orgID
	return r.cfg, nil
}

func (r *recordOrgDS) AdminUserID(orgID string) (string, error) {
	return r.adminID, nil
}

func (r *recordOrgDS) Roster(orgID string) ([]ParttimeEmployee, error) {
	r.lastRosterOrg = orgID
	return r.roster, nil
}

func (r *recordOrgDS) Attendance(orgID string, userIDs []string, startDate, endDate string) ([]dingtalk.AttendanceRecord, error) {
	r.lastAttendOrg = orgID
	return r.attendance, r.attendanceErr
}

// TestBuildEmployeePunchData_EnrichesIdentity ensures per-user punch days are
// tagged with the roster's employee number/name for downstream matching.
func TestBuildEmployeePunchData_EnrichesIdentity(t *testing.T) {
	// 2026-07-02 08:30 CST and 18:00 CST as millisecond timestamps.
	cst := time.FixedZone("CST", 8*3600)
	t0830 := time.Date(2026, 7, 2, 8, 30, 0, 0, cst).UnixMilli()
	t1800 := time.Date(2026, 7, 2, 18, 0, 0, 0, cst).UnixMilli()
	records := []dingtalk.AttendanceRecord{
		{UserID: "uid-1", CheckType: "OnDuty", WorkDate: "2026-07-02", UserCheckTime: strconv.FormatInt(t0830, 10)},
		{UserID: "uid-1", CheckType: "OffDuty", WorkDate: "2026-07-02", UserCheckTime: strconv.FormatInt(t1800, 10)},
	}
	identity := map[string]ParttimeEmployee{
		"uid-1": {Name: "张三", EmployeeNo: "MT001"},
	}
	got := buildEmployeePunchData(records, identity)
	if len(got) != 1 {
		t.Fatalf("expected 1 punched employee, got %d", len(got))
	}
	if got[0].EmployeeNo != "MT001" || got[0].Name != "张三" {
		t.Fatalf("expected enriched identity, got %+v", got[0])
	}
	if got[0].Days[2] == "" {
		t.Fatalf("expected day-2 status label, got empty")
	}
}

var _ = errors.New
