package logstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrCursorExpired = errors.New("日志分页已过期，请重新查询")

type Record struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}
type Stats struct {
	Size     int64  `json:"size"`
	MaxBytes int64  `json:"maxBytes"`
	Files    int    `json:"files"`
	Oldest   string `json:"oldest"`
	Error    string `json:"error,omitempty"`
}
type Query struct {
	Level    string
	Search   string
	From, To time.Time
	Limit    int
	Cursor   string
}
type Page struct {
	Items      []Record `json:"items"`
	NextCursor string   `json:"nextCursor"`
	HasMore    bool     `json:"hasMore"`
	Stats
}
type cursor struct {
	ID         string `json:"id"`
	Offset     int64  `json:"offset"`
	Generation uint64 `json:"generation"`
}
type entry struct {
	path, id string
	size     int64
	modified time.Time
}
type Store struct {
	mu         sync.Mutex
	File       string
	policy     Policy
	mode       os.FileMode
	activeID   string
	generation uint64
	now        func() time.Time
}

func New(file string, policy Policy, mode os.FileMode) *Store {
	return &Store{File: file, policy: policy, mode: mode, activeID: strconv.FormatInt(time.Now().UnixNano(), 10), now: time.Now}
}
func (s *Store) filesLocked() ([]entry, error) {
	dir, err := os.ReadDir(filepath.Dir(s.File))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	base := filepath.Base(s.File)
	result := []entry{}
	for _, item := range dir {
		name := item.Name()
		id := ""
		if name == base {
			id = s.activeID
		} else if strings.HasPrefix(name, base+".part-") {
			id = strings.TrimPrefix(name, base+".part-")
			if _, err := strconv.ParseInt(id, 10, 64); err != nil {
				continue
			}
		} else {
			continue
		}
		// Do not follow symlinks or touch unrelated files in the log directory.
		if !item.Type().IsRegular() {
			continue
		}
		info, err := item.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, entry{filepath.Join(filepath.Dir(s.File), name), id, info.Size(), info.ModTime()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result, nil
}
func (s *Store) segmentLocked() int64 {
	size := int64(s.policy.MaxMiB) * MiB / 4
	if size > MiB {
		size = MiB
	}
	return size
}
func (s *Store) trimActiveLocked(keep int64) error {
	f, err := os.OpenFile(s.File, os.O_RDWR, s.mode)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= keep {
		return nil
	}
	buffer := make([]byte, keep)
	if keep > 0 {
		if _, err = f.ReadAt(buffer, info.Size()-keep); err != nil && err != io.EOF {
			return err
		}
		if i := bytes.IndexByte(buffer, '\n'); i >= 0 {
			buffer = buffer[i+1:]
		}
	}
	// Truncate the same inode: legacy/adopted Core processes may still hold it open.
	if _, err = f.WriteAt(buffer, 0); err != nil {
		return err
	}
	if err = f.Truncate(int64(len(buffer))); err == nil {
		s.generation++
	}
	return err
}
func (s *Store) cleanupLocked() error {
	if info, err := os.Lstat(s.File); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("日志路径不是普通文件")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	files, err := s.filesLocked()
	if err != nil {
		return err
	}
	var total int64
	alive := []entry{}
	cutoff := s.now().Add(-time.Duration(s.policy.Days) * 24 * time.Hour)
	for _, file := range files {
		if s.policy.Days > 0 && file.modified.Before(cutoff) {
			if file.path == s.File {
				if err := s.trimActiveLocked(0); err != nil {
					return err
				}
			} else if err := os.Remove(file.path); err != nil {
				return err
			}
			continue
		}
		total += file.size
		alive = append(alive, file)
	}
	max := int64(s.policy.MaxMiB) * MiB
	for _, file := range alive {
		if total <= max {
			break
		}
		if file.path == s.File {
			continue
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= file.size
	}
	info, err := os.Stat(s.File)
	if err == nil && info.Size() > s.segmentLocked() {
		return s.trimActiveLocked(s.segmentLocked())
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func (s *Store) Configure(policy Policy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = policy
	return s.cleanupLocked()
}
func (s *Store) Cleanup() error { s.mu.Lock(); defer s.mu.Unlock(); return s.cleanupLocked() }
func (s *Store) Write(body []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.File), 0o750); err != nil {
		return 0, err
	}
	if err := s.cleanupLocked(); err != nil {
		return 0, err
	}
	written := 0
	for len(body) > 0 {
		info, err := os.Stat(s.File)
		if err != nil && !os.IsNotExist(err) {
			return written, err
		}
		var size int64
		if info != nil {
			size = info.Size()
		}
		chunk := len(body)
		if int64(chunk) > s.segmentLocked() {
			chunk = int(s.segmentLocked())
		}
		if size > 0 && (size+int64(chunk) > s.segmentLocked() || info.ModTime().UTC().Format("2006-01-02") != s.now().UTC().Format("2006-01-02")) {
			if err := os.Rename(s.File, s.File+".part-"+s.activeID); err != nil {
				return written, err
			}
			next := s.now().UnixNano()
			old, _ := strconv.ParseInt(s.activeID, 10, 64)
			if next <= old {
				next = old + 1
			}
			s.activeID = strconv.FormatInt(next, 10)
		}
		f, err := os.OpenFile(s.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, s.mode)
		if err != nil {
			return written, err
		}
		n, err := f.Write(body[:chunk])
		closeErr := f.Close()
		written += n
		body = body[n:]
		if err != nil {
			return written, err
		}
		if closeErr != nil {
			return written, closeErr
		}
		if n < chunk {
			return written, io.ErrShortWrite
		}
		if err := s.cleanupLocked(); err != nil {
			return written, err
		}
	}
	return written, nil
}
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.filesLocked()
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.path == s.File {
			err = s.trimActiveLocked(0)
		} else {
			err = os.Remove(file.path)
		}
		if err != nil {
			return err
		}
	}
	s.generation++
	return nil
}
func (s *Store) statsLocked(files []entry, records bool) Stats {
	result := Stats{MaxBytes: int64(s.policy.MaxMiB) * MiB, Files: len(files)}
	for _, file := range files {
		result.Size += file.size
		if file.size == 0 {
			continue
		}
		oldest := file.modified.UTC()
		// History uses the first retained event time; raw Core output reports
		// the oldest retained file's modification time.
		if records {
			if f, err := os.Open(file.path); err == nil {
				scanner := bufio.NewScanner(io.LimitReader(f, 256*1024))
				scanner.Buffer(make([]byte, 4096), 256*1024)
				if scanner.Scan() {
					var record Record
					if json.Unmarshal(scanner.Bytes(), &record) == nil {
						if parsed, err := time.Parse(time.RFC3339Nano, record.Time); err == nil {
							oldest = parsed.UTC()
						}
					}
				}
				_ = f.Close()
			}
		}
		if result.Oldest == "" || oldest.Format(time.RFC3339Nano) < result.Oldest {
			result.Oldest = oldest.Format(time.RFC3339Nano)
		}
	}
	return result
}
func (s *Store) Stats() (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.filesLocked()
	return s.statsLocked(files, false), err
}

func (s *Store) RecordStats() (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.filesLocked()
	return s.statsLocked(files, true), err
}

// Structured histories expire records precisely, including a quiet active file
// containing both old and recent lines. Only changed files are rewritten.
func (s *Store) PruneRecords() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policy.Days == 0 {
		return nil
	}
	files, err := s.filesLocked()
	if err != nil {
		return err
	}
	cutoff := s.now().Add(-time.Duration(s.policy.Days) * 24 * time.Hour)
	for _, file := range files {
		if file.size > MiB {
			continue
		}
		body, err := os.ReadFile(file.path)
		if err != nil {
			return err
		}
		kept := make([]byte, 0, len(body))
		for _, line := range bytes.SplitAfter(body, []byte{'\n'}) {
			var record Record
			whenErr := json.Unmarshal(line, &record)
			when, parseErr := time.Parse(time.RFC3339Nano, record.Time)
			if whenErr == nil && parseErr == nil && when.Before(cutoff) {
				continue
			}
			kept = append(kept, line...)
		}
		if len(kept) == len(body) {
			continue
		}
		f, err := os.CreateTemp(filepath.Dir(s.File), ".log-prune-*")
		if err != nil {
			return err
		}
		name := f.Name()
		if err = f.Chmod(s.mode); err == nil {
			_, err = f.Write(kept)
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, file.path)
		}
		_ = os.Remove(name)
		if err != nil {
			return err
		}
		s.generation++
	}
	return nil
}
func Rank(level string) int {
	switch level {
	case "debug":
		return 10
	case "warning", "warn":
		return 30
	case "error":
		return 40
	}
	return 20
}
func (s *Store) ReadPage(ctx context.Context, q Query) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q.Limit < 1 {
		q.Limit = 800
	}
	if q.Limit > 2000 {
		q.Limit = 2000
	}
	files, err := s.filesLocked()
	result := Page{Items: []Record{}, Stats: s.statsLocked(files, true)}
	if err != nil {
		return result, err
	}
	start := len(files) - 1
	before := int64(-1)
	if q.Cursor != "" {
		if len(q.Cursor) > 1024 {
			return result, ErrCursorExpired
		}
		body, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		var c cursor
		if err != nil || json.Unmarshal(body, &c) != nil || c.Generation != s.generation || c.Offset < 0 {
			return result, ErrCursorExpired
		}
		start = -1
		for i, file := range files {
			if file.id == c.ID {
				start = i
				before = c.Offset
				break
			}
		}
		if start < 0 || before > files[start].size {
			return result, ErrCursorExpired
		}
	}
	term := strings.ToLower(strings.TrimSpace(q.Search))
	cutoff := s.now().Add(-time.Duration(s.policy.Days) * 24 * time.Hour)
	var last cursor
	for i := start; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file := files[i]
		// A migrated legacy file is normalized by Cleanup before queries run.
		if file.size > MiB {
			return result, fmt.Errorf("日志文件超过分卷上限，请等待自动清理")
		}
		body, err := os.ReadFile(file.path)
		if err != nil {
			return result, err
		}
		if i == start && before >= 0 {
			body = body[:before]
		}
		end := len(body)
		for end > 0 {
			if body[end-1] == '\n' {
				end--
			}
			pos := bytes.LastIndexByte(body[:end], '\n') + 1
			var record Record
			valid := json.Unmarshal(body[pos:end], &record) == nil
			end = pos
			if !valid || Rank(record.Level) < Rank(q.Level) {
				continue
			}
			when, parseErr := time.Parse(time.RFC3339Nano, record.Time)
			if parseErr == nil && s.policy.Days > 0 && when.Before(cutoff) {
				continue
			}
			if !q.From.IsZero() && (parseErr != nil || when.Before(q.From)) || !q.To.IsZero() && (parseErr != nil || !when.Before(q.To)) {
				continue
			}
			if term != "" && !strings.Contains(strings.ToLower(record.Time+" "+record.Level+" "+record.Message), term) {
				continue
			}
			if len(result.Items) == q.Limit {
				result.HasMore = true
				token, _ := json.Marshal(last)
				result.NextCursor = base64.RawURLEncoding.EncodeToString(token)
				break
			}
			result.Items = append(result.Items, record)
			last = cursor{file.id, int64(pos), s.generation}
		}
		if result.HasMore {
			break
		}
	}
	for i, j := 0, len(result.Items)-1; i < j; i, j = i+1, j-1 {
		result.Items[i], result.Items[j] = result.Items[j], result.Items[i]
	}
	return result, nil
}
