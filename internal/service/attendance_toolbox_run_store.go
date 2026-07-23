package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AttendanceToolboxRunStore keeps short-lived calculation results bound to user+org.
// Files live under rootDir/<runID>/ only — never under raw user/org path segments.
type AttendanceToolboxRunStore struct {
	mu        sync.RWMutex
	ttl       time.Duration
	maxBytes  int64
	rootDir   string
	runs      map[string]*AttendanceToolboxRun
	stopCh    chan struct{}
	stopped   bool
	startOnce sync.Once
}

type AttendanceToolboxRunFile struct {
	FileKey     string `json:"file_key"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Kind        string `json:"kind,omitempty"`
	FlowKey     string `json:"flow_key,omitempty"`
	RowCount    int    `json:"row_count,omitempty"`
	// Absolute path never exposed to clients.
	path string
}

type AttendanceToolboxRun struct {
	RunID     string                     `json:"run_id"`
	UserID    string                     `json:"-"`
	OrgID     string                     `json:"-"`
	Module    string                     `json:"module"`
	CreatedAt time.Time                  `json:"created_at"`
	ExpiresAt time.Time                  `json:"expires_at"`
	Log       string                     `json:"log,omitempty"`
	Stats     map[string]interface{}     `json:"stats,omitempty"`
	Meta      map[string]interface{}     `json:"meta,omitempty"`
	Files     []AttendanceToolboxRunFile `json:"files"`
	dir       string
}

var (
	ErrToolboxRunNotFound = errors.New("run not found")
	ErrToolboxRunExpired  = errors.New("run expired")
	ErrToolboxRunDenied   = errors.New("run access denied")
	ErrToolboxFileMissing = errors.New("result file not found")
	ErrToolboxRunTooLarge = errors.New("run total size exceeds limit")
	ErrToolboxStoreClosed = errors.New("run store closed")
)

const (
	defaultToolboxRunTTL      = 2 * time.Hour
	minToolboxRunTTL          = 5 * time.Minute
	maxToolboxRunTTL          = 24 * time.Hour
	defaultToolboxRunMaxBytes = 512 * 1024 * 1024 // 512MB per run
)

var defaultToolboxRunStore = newDefaultAttendanceToolboxRunStore()

func newDefaultAttendanceToolboxRunStore() *AttendanceToolboxRunStore {
	ttl := defaultToolboxRunTTL
	if raw := strings.TrimSpace(os.Getenv("ATTENDANCE_TOOLBOX_RUN_TTL_SECONDS")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			ttl = time.Duration(sec) * time.Second
			// Env-provided TTL is clamped to sane bounds.
			if ttl < minToolboxRunTTL {
				ttl = minToolboxRunTTL
			}
			if ttl > maxToolboxRunTTL {
				ttl = maxToolboxRunTTL
			}
		}
	}
	maxBytes := int64(defaultToolboxRunMaxBytes)
	if raw := strings.TrimSpace(os.Getenv("ATTENDANCE_TOOLBOX_RUN_MAX_BYTES")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			maxBytes = n
		}
	}
	store := NewAttendanceToolboxRunStore(ttl, "", maxBytes)
	// Restart cleanup: metadata is in-memory only, so orphaned disk runs are unreachable.
	store.cleanupOrphanDirs()
	store.startCleanupLoop()
	return store
}

func DefaultAttendanceToolboxRunStore() *AttendanceToolboxRunStore {
	return defaultToolboxRunStore
}

// NewAttendanceToolboxRunStore creates a store. Tests should call Close() when done.
// TTL bounds are enforced when reading from env (default store); direct callers
// (tests) may pass any positive TTL. maxBytes <= 0 uses defaultToolboxRunMaxBytes.
func NewAttendanceToolboxRunStore(ttl time.Duration, rootDir string, maxBytes int64) *AttendanceToolboxRunStore {
	if ttl <= 0 {
		ttl = defaultToolboxRunTTL
	}
	if maxBytes <= 0 {
		maxBytes = defaultToolboxRunMaxBytes
	}
	if rootDir == "" {
		rootDir = filepath.Join(os.TempDir(), "peopleops-attendance-toolbox-runs")
	}
	_ = os.MkdirAll(rootDir, 0o700)
	return &AttendanceToolboxRunStore{
		ttl:      ttl,
		maxBytes: maxBytes,
		rootDir:  rootDir,
		runs:     make(map[string]*AttendanceToolboxRun),
		stopCh:   make(chan struct{}),
	}
}

func (s *AttendanceToolboxRunStore) startCleanupLoop() {
	s.startOnce.Do(func() {
		go s.cleanupLoop()
	})
}

func (s *AttendanceToolboxRunStore) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.CleanupExpired()
		case <-s.stopCh:
			return
		}
	}
}

// Close stops the cleanup loop. Safe for tests; the process-global store is not closed.
func (s *AttendanceToolboxRunStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
}

func (s *AttendanceToolboxRunStore) CleanupExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, run := range s.runs {
		if now.After(run.ExpiresAt) {
			_ = os.RemoveAll(run.dir)
			delete(s.runs, id)
		}
	}
}

// cleanupOrphanDirs removes all run directories under rootDir (used on process start).
func (s *AttendanceToolboxRunStore) cleanupOrphanDirs() {
	entries, err := os.ReadDir(s.rootDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only remove hex run-id shaped dirs (32 hex chars from 16-byte id).
		name := e.Name()
		if len(name) != 32 {
			continue
		}
		if _, err := hex.DecodeString(name); err != nil {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.rootDir, name))
	}
}

func newRunID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Put materializes result files under rootDir/<runID>/ and indexes them in memory.
func (s *AttendanceToolboxRunStore) Put(userID, orgID, module, logText string, stats, meta map[string]interface{}, files []AttendanceToolboxResult) (*AttendanceToolboxRun, error) {
	s.mu.RLock()
	closed := s.stopped
	s.mu.RUnlock()
	if closed {
		return nil, ErrToolboxStoreClosed
	}

	userID = strings.TrimSpace(userID)
	orgID = strings.TrimSpace(orgID)
	if userID == "" || orgID == "" {
		return nil, errors.New("user_id and org_id are required")
	}
	runID, err := newRunID()
	if err != nil {
		return nil, err
	}
	// Path uses only cryptographically random runID — never raw user/org segments.
	dir := filepath.Join(s.rootDir, runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	var total int64
	storedFiles := make([]AttendanceToolboxRunFile, 0, len(files))
	for i, f := range files {
		total += int64(len(f.Data))
		if total > s.maxBytes {
			_ = os.RemoveAll(dir)
			return nil, ErrToolboxRunTooLarge
		}
		name := strings.TrimSpace(f.FileName)
		if name == "" {
			name = fmt.Sprintf("result_%d", i+1)
		}
		name = filepath.Base(name)
		fileKey := fmt.Sprintf("%d_%s", i+1, sanitizeFileKey(name))
		path := filepath.Join(dir, fileKey)
		// Ensure final path stays under dir.
		if !isPathWithin(dir, path) {
			_ = os.RemoveAll(dir)
			return nil, errors.New("invalid result path")
		}
		if err := os.WriteFile(path, f.Data, 0o600); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		ct := f.ContentType
		if ct == "" {
			ct = contentTypeForName(name)
		}
		kind := strings.TrimSpace(f.Kind)
		if kind == "" {
			kind = guessFileKind(name)
		}
		storedFiles = append(storedFiles, AttendanceToolboxRunFile{
			FileKey:     fileKey,
			FileName:    name,
			ContentType: ct,
			Size:        int64(len(f.Data)),
			Kind:        kind,
			FlowKey:     f.FlowKey,
			RowCount:    f.RowCount,
			path:        path,
		})
	}

	now := time.Now()
	run := &AttendanceToolboxRun{
		RunID:     runID,
		UserID:    userID,
		OrgID:     orgID,
		Module:    module,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
		Log:       logText,
		Stats:     stats,
		Meta:      meta,
		Files:     storedFiles,
		dir:       dir,
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		_ = os.RemoveAll(dir)
		return nil, ErrToolboxStoreClosed
	}
	s.runs[runID] = run
	s.mu.Unlock()
	return cloneRunPublic(run), nil
}

func (s *AttendanceToolboxRunStore) Get(runID, userID, orgID string) (*AttendanceToolboxRun, error) {
	runID = filepath.Base(strings.TrimSpace(runID))
	s.mu.RLock()
	run, ok := s.runs[runID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrToolboxRunNotFound
	}
	if time.Now().After(run.ExpiresAt) {
		s.delete(runID)
		return nil, ErrToolboxRunExpired
	}
	if run.UserID != userID || run.OrgID != orgID {
		return nil, ErrToolboxRunDenied
	}
	return cloneRunPublic(run), nil
}

func (s *AttendanceToolboxRunStore) ReadFile(runID, fileKey, userID, orgID string) (fileName, contentType string, data []byte, err error) {
	runID = filepath.Base(strings.TrimSpace(runID))
	s.mu.RLock()
	run, ok := s.runs[runID]
	s.mu.RUnlock()
	if !ok {
		return "", "", nil, ErrToolboxRunNotFound
	}
	if time.Now().After(run.ExpiresAt) {
		s.delete(runID)
		return "", "", nil, ErrToolboxRunExpired
	}
	if run.UserID != userID || run.OrgID != orgID {
		return "", "", nil, ErrToolboxRunDenied
	}
	fileKey = filepath.Base(fileKey)
	for _, f := range run.Files {
		if f.FileKey == fileKey {
			if !isPathWithin(run.dir, f.path) {
				return "", "", nil, ErrToolboxFileMissing
			}
			raw, readErr := os.ReadFile(f.path)
			if readErr != nil {
				return "", "", nil, ErrToolboxFileMissing
			}
			return f.FileName, f.ContentType, raw, nil
		}
	}
	return "", "", nil, ErrToolboxFileMissing
}

// ReadAllDownloadable reads every non-meta file; fails if any expected file is missing.
func (s *AttendanceToolboxRunStore) ReadAllDownloadable(runID, userID, orgID string) ([]AttendanceToolboxResult, error) {
	run, err := s.Get(runID, userID, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]AttendanceToolboxResult, 0, len(run.Files))
	for _, f := range run.Files {
		if strings.EqualFold(f.Kind, "meta") {
			continue
		}
		_, ct, data, readErr := s.ReadFile(runID, f.FileKey, userID, orgID)
		if readErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrToolboxFileMissing, f.FileName)
		}
		out = append(out, AttendanceToolboxResult{
			FileName:    f.FileName,
			ContentType: ct,
			Data:        data,
			Kind:        f.Kind,
			FlowKey:     f.FlowKey,
			RowCount:    f.RowCount,
		})
	}
	if len(out) == 0 {
		return nil, ErrToolboxFileMissing
	}
	return out, nil
}

func (s *AttendanceToolboxRunStore) delete(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run, ok := s.runs[runID]; ok {
		_ = os.RemoveAll(run.dir)
		delete(s.runs, runID)
	}
}

func cloneRunPublic(run *AttendanceToolboxRun) *AttendanceToolboxRun {
	if run == nil {
		return nil
	}
	out := *run
	out.Files = append([]AttendanceToolboxRunFile(nil), run.Files...)
	for i := range out.Files {
		out.Files[i].path = ""
	}
	return &out
}

func sanitizeFileKey(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	return replacer.Replace(name)
}

func contentTypeForName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip":
		return "application/zip"
	case ".json":
		return "application/json"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".xls":
		return "application/vnd.ms-excel"
	default:
		return "application/octet-stream"
	}
}

func guessFileKind(name string) string {
	if strings.EqualFold(filepath.Ext(name), ".json") {
		return "meta"
	}
	if strings.Contains(name, "审计") {
		return "audit"
	}
	return "export"
}

// HashIdentity is available if callers need a stable opaque label (not used in path).
func HashIdentity(userID, orgID string) string {
	sum := sha256.Sum256([]byte(orgID + "\x00" + userID))
	return hex.EncodeToString(sum[:8])
}
