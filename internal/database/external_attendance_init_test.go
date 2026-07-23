package database

import (
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestInitExternalAttendanceDBRetriesAfterFailure(t *testing.T) {
	_ = CloseExternalAttendanceDB()
	t.Cleanup(func() {
		externalAttendanceConnect = defaultExternalAttendanceConnect
		_ = CloseExternalAttendanceDB()
		t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "")
	})

	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "user:pass@tcp(127.0.0.1:1)/dwd?parseTime=true")

	var calls atomic.Int32
	externalAttendanceConnect = func(cfg ExternalAttendanceConfig) (*sql.DB, error) {
		n := calls.Add(1)
		if n == 1 {
			return nil, errors.New("ping external attendance db: temporary unavailable")
		}
		// Return a closed sql.DB placeholder? sql.Open without ping is enough for success path.
		db, err := sql.Open("mysql", cfg.DSN)
		if err != nil {
			return nil, err
		}
		// Skip real ping: just return open handle as "success"
		return db, nil
	}

	if err := InitExternalAttendanceDB(); err == nil {
		t.Fatal("first init should fail")
	}
	if GetExternalAttendanceDB() != nil {
		t.Fatal("failed init must not cache db")
	}
	if err := InitExternalAttendanceDB(); err != nil {
		t.Fatalf("second init should succeed: %v", err)
	}
	if GetExternalAttendanceDB() == nil {
		t.Fatal("success should cache db")
	}
	// third call uses cache
	if err := InitExternalAttendanceDB(); err != nil {
		t.Fatalf("cached init: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("connect calls=%d want 2", calls.Load())
	}
}

func TestInitExternalAttendanceDBConcurrentSafe(t *testing.T) {
	_ = CloseExternalAttendanceDB()
	t.Cleanup(func() {
		externalAttendanceConnect = defaultExternalAttendanceConnect
		_ = CloseExternalAttendanceDB()
		t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "")
	})
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "user:pass@tcp(127.0.0.1:1)/dwd?parseTime=true")

	var calls atomic.Int32
	externalAttendanceConnect = func(cfg ExternalAttendanceConfig) (*sql.DB, error) {
		calls.Add(1)
		db, err := sql.Open("mysql", cfg.DSN)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- InitExternalAttendanceDB()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent init: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("connect calls=%d want 1 (mutex serialize, cache after first)", calls.Load())
	}
}

func TestCloseExternalAttendanceDBAllowsReinit(t *testing.T) {
	_ = CloseExternalAttendanceDB()
	t.Cleanup(func() {
		externalAttendanceConnect = defaultExternalAttendanceConnect
		_ = CloseExternalAttendanceDB()
		t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "")
	})
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "user:pass@tcp(127.0.0.1:1)/dwd?parseTime=true")
	externalAttendanceConnect = func(cfg ExternalAttendanceConfig) (*sql.DB, error) {
		return sql.Open("mysql", cfg.DSN)
	}
	if err := InitExternalAttendanceDB(); err != nil {
		t.Fatal(err)
	}
	if err := CloseExternalAttendanceDB(); err != nil {
		t.Fatal(err)
	}
	if GetExternalAttendanceDB() != nil {
		t.Fatal("closed")
	}
	if err := InitExternalAttendanceDB(); err != nil {
		t.Fatal(err)
	}
	if GetExternalAttendanceDB() == nil {
		t.Fatal("reinit")
	}
}

func TestBuildExternalApproveItemKeyStable(t *testing.T) {
	a := BuildExternalApproveItemKey("p", "请假", "", "", "", "", "1", "day")
	b := BuildExternalApproveItemKey("p", "请假", "", "", "", "", "1", "day")
	if a == "" || a != b {
		t.Fatal("unstable")
	}
}
