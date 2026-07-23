package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttendanceToolboxRunStore_PutGetAndDeny(t *testing.T) {
	store := NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	defer store.Close()
	run, err := store.Put("u1", "orgA", "leave", "log-line", map[string]interface{}{"k": 1}, map[string]interface{}{"m": "v"}, []AttendanceToolboxResult{
		{FileName: "请假明细表.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: []byte("abc"), Kind: "export", FlowKey: "leave", RowCount: 3},
		{FileName: "meta.json", ContentType: "application/json", Data: []byte(`{"ok":true}`), Kind: "meta"},
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if run.RunID == "" || len(run.Files) != 2 {
		t.Fatalf("unexpected run: %+v", run)
	}
	// path must not leak
	if run.Files[0].path != "" {
		t.Fatalf("absolute path leaked")
	}
	// metadata preserved
	if run.Files[0].Kind != "export" || run.Files[0].FlowKey != "leave" || run.Files[0].RowCount != 3 {
		t.Fatalf("metadata not preserved: %+v", run.Files[0])
	}
	// dir is rootDir/runID only
	if _, err := os.Stat(filepath.Join(store.rootDir, run.RunID)); err != nil {
		t.Fatalf("expected run dir under root: %v", err)
	}

	got, err := store.Get(run.RunID, "u1", "orgA")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Module != "leave" {
		t.Fatalf("module=%s", got.Module)
	}

	if _, err := store.Get(run.RunID, "u2", "orgA"); err != ErrToolboxRunDenied {
		t.Fatalf("expected denied, got %v", err)
	}
	if _, err := store.Get(run.RunID, "u1", "orgB"); err != ErrToolboxRunDenied {
		t.Fatalf("expected denied cross-org, got %v", err)
	}
	// path traversal in run id / file key
	if _, err := store.Get("../"+run.RunID, "u1", "orgA"); err != nil && err != ErrToolboxRunNotFound {
		t.Fatalf("unexpected traversal lookup error: %v", err)
	}
	if _, _, _, err := store.ReadFile(run.RunID, "../"+run.Files[0].FileKey, "u1", "orgA"); err != nil && err != ErrToolboxFileMissing {
		t.Fatalf("unexpected traversal file error: %v", err)
	}

	name, ct, data, err := store.ReadFile(run.RunID, run.Files[0].FileKey, "u1", "orgA")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if name != "请假明细表.xlsx" || ct == "" || string(data) != "abc" {
		t.Fatalf("unexpected file payload name=%s ct=%s data=%q", name, ct, data)
	}

	// ZIP completeness
	files, err := store.ReadAllDownloadable(run.RunID, "u1", "orgA")
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("downloadable count=%d", len(files))
	}
}

func TestAttendanceToolboxRunStore_Expire(t *testing.T) {
	store := NewAttendanceToolboxRunStore(20*time.Millisecond, t.TempDir(), 0)
	defer store.Close()
	run, err := store.Put("u1", "orgA", "leave", "", nil, nil, []AttendanceToolboxResult{
		{FileName: "a.xlsx", Data: []byte("x")},
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := store.Get(run.RunID, "u1", "orgA"); err != ErrToolboxRunExpired && err != ErrToolboxRunNotFound {
		t.Fatalf("expected expired/not found, got %v", err)
	}
}

func TestAttendanceToolboxRunStore_SizeLimit(t *testing.T) {
	store := NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 10)
	defer store.Close()
	_, err := store.Put("u1", "orgA", "leave", "", nil, nil, []AttendanceToolboxResult{
		{FileName: "big.xlsx", Data: []byte("01234567890")},
	})
	if err != ErrToolboxRunTooLarge {
		t.Fatalf("expected too large, got %v", err)
	}
}

func TestAttendanceToolboxRunStore_OrphanCleanupOnStart(t *testing.T) {
	root := t.TempDir()
	// fake orphan run dir with hex name
	orphanID := "0123456789abcdef0123456789abcdef"
	orphan := filepath.Join(root, orphanID)
	if err := os.MkdirAll(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "x.xlsx"), []byte("z"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewAttendanceToolboxRunStore(time.Hour, root, 0)
	store.cleanupOrphanDirs()
	defer store.Close()
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan dir should be removed, err=%v", err)
	}
}

func TestAttendanceToolboxRunStore_ZIPMissingFileFails(t *testing.T) {
	store := NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	defer store.Close()
	run, err := store.Put("u1", "orgA", "leave", "", nil, nil, []AttendanceToolboxResult{
		{FileName: "a.xlsx", Data: []byte("x"), Kind: "export"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// delete underlying file
	_ = os.RemoveAll(filepath.Join(store.rootDir, run.RunID))
	_, err = store.ReadAllDownloadable(run.RunID, "u1", "orgA")
	if err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestIsAttendanceToolboxStandardModule(t *testing.T) {
	if !IsAttendanceToolboxStandardModule("leave") {
		t.Fatal("leave should be standard")
	}
	if IsAttendanceToolboxStandardModule("quick") || IsAttendanceToolboxStandardModule("dingtalk_sync") {
		t.Fatal("quick/dingtalk_sync must not be standard")
	}
}

func TestAttendanceToolboxRunStore_CloseStopsLoop(t *testing.T) {
	store := NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	store.startCleanupLoop()
	store.Close()
	// second close is safe
	store.Close()
	_, err := store.Put("u1", "orgA", "leave", "", nil, nil, []AttendanceToolboxResult{
		{FileName: "a.xlsx", Data: []byte("x")},
	})
	if err != ErrToolboxStoreClosed {
		t.Fatalf("expected closed, got %v", err)
	}
}

func TestAttendanceToolboxRunStore_PathUsesRunIDOnly(t *testing.T) {
	root := t.TempDir()
	store := NewAttendanceToolboxRunStore(time.Hour, root, 0)
	defer store.Close()
	run, err := store.Put("user/../evil", "org/../x", "leave", "", nil, nil, []AttendanceToolboxResult{
		{FileName: "../../escape.xlsx", Data: []byte("z"), Kind: "export", FlowKey: "leave", RowCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Directory must be root/runID — not containing raw user/org segments.
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != run.RunID {
		t.Fatalf("unexpected root entries: %v run=%s", entries, run.RunID)
	}
	if run.Files[0].FileName != "escape.xlsx" {
		t.Fatalf("filename was not sanitized: %q", run.Files[0].FileName)
	}
	if run.Files[0].Kind != "export" || run.Files[0].RowCount != 1 {
		t.Fatalf("metadata lost: %+v", run.Files[0])
	}
}
