package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyWriter writes one file per UTC+8 calendar day and removes files older
// than retentionDays. It is safe for concurrent use by the standard logger
// and Logrus.
type DailyWriter struct {
	dir           string
	prefix        string
	retentionDays int
	location      *time.Location
	mu            sync.Mutex
	file          *os.File
	day           string
	stop          chan struct{}
	done          chan struct{}
	closeOnce     sync.Once
	closed        bool
}

var _ io.WriteCloser = (*DailyWriter)(nil)

var businessLocation = time.FixedZone("UTC+8", 8*60*60)

func NewDailyWriter(dir, prefix string, retentionDays int) (*DailyWriter, error) {
	if retentionDays < 1 {
		return nil, fmt.Errorf("retention days must be positive")
	}
	if dir == "" {
		return nil, fmt.Errorf("log directory is required")
	}
	if prefix == "" {
		prefix = "peopleops"
	}
	w := &DailyWriter{
		dir: dir, prefix: prefix, retentionDays: retentionDays, location: businessLocation,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := w.cleanup(time.Now()); err != nil {
		return nil, err
	}
	go w.cleanupLoop()
	return w, nil
}

func (w *DailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, fmt.Errorf("daily log writer is closed")
	}
	now := time.Now().In(w.location)
	day := now.Format("2006-01-02")
	if w.day != day {
		if w.file != nil {
			if err := w.file.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "peopleops daily log close failed: %v\n", err)
			}
			w.file = nil
		}
		path := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, day))
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return 0, err
		}
		w.file = file
		w.day = day
		if err := w.cleanupLocked(now); err != nil {
			fmt.Fprintf(os.Stderr, "peopleops log cleanup failed: %v\n", err)
		}
	}
	if w.file == nil {
		return 0, fmt.Errorf("daily log file is not open")
	}
	return w.file.Write(p)
}

func (w *DailyWriter) Close() error {
	w.closeOnce.Do(func() { close(w.stop) })
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.day = ""
	return err
}

func (w *DailyWriter) cleanupLoop() {
	defer close(w.done)
	for {
		now := time.Now().In(w.location)
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, w.location)
		timer := time.NewTimer(time.Until(nextMidnight))
		select {
		case <-timer.C:
			if err := w.cleanup(time.Now()); err != nil {
				fmt.Fprintf(os.Stderr, "peopleops scheduled log cleanup failed: %v\n", err)
			}
		case <-w.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (w *DailyWriter) cleanup(now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cleanupLocked(now.In(w.location))
}

func (w *DailyWriter) cleanupLocked(now time.Time) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("list log directory: %w", err)
	}
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, w.location).AddDate(0, 0, -(w.retentionDays - 1))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		prefix := w.prefix + "-"
		if len(entry.Name()) != len(prefix)+len("2006-01-02.log") || entry.Name()[:len(prefix)] != prefix {
			continue
		}
		datePart := entry.Name()[len(prefix) : len(prefix)+10]
		day, parseErr := time.ParseInLocation("2006-01-02", datePart, w.location)
		if parseErr != nil || !day.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(w.dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove expired log %s: %w", entry.Name(), err)
		}
	}
	return nil
}
