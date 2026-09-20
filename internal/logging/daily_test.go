package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyWriterKeepsSevenCalendarDays(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().In(businessLocation)
	old := now.AddDate(0, 0, -7).Format("2006-01-02")
	kept := now.AddDate(0, 0, -6).Format("2006-01-02")
	for _, day := range []string{old, kept} {
		if err := os.WriteFile(filepath.Join(dir, "peopleops-"+day+".log"), []byte("old"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewDailyWriter(dir, "peopleops", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	}()
	if _, err := w.Write([]byte("current")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "peopleops-"+old+".log")); !os.IsNotExist(err) {
		t.Fatalf("expected expired log removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "peopleops-"+kept+".log")); err != nil {
		t.Fatalf("expected six-day log kept, err=%v", err)
	}
}
