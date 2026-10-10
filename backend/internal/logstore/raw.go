package logstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type RawLine struct {
	Key       string `json:"key"`
	Message   string `json:"message"`
	Truncated bool   `json:"truncated,omitempty"`
}
type RawPage struct {
	Lines      []RawLine `json:"lines"`
	NextCursor string    `json:"nextCursor"`
	HasMore    bool      `json:"hasMore"`
	Stats
}

// ReadRawPage returns bounded plain stdout/stderr, including a final line that
// has not yet ended in a newline. Cursors refer to byte offsets, not line counts.
func (s *Store) ReadRawPage(ctx context.Context, q Query) (RawPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q.Limit < 1 {
		q.Limit = 800
	}
	if q.Limit > 2000 {
		q.Limit = 2000
	}
	files, err := s.filesLocked()
	result := RawPage{Lines: []RawLine{}, Stats: s.statsLocked(files, false)}
	if err != nil {
		return result, err
	}
	start, before := len(files)-1, int64(-1)
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
				start, before = i, c.Offset
				break
			}
		}
		if start < 0 || before > files[start].size {
			return result, ErrCursorExpired
		}
	}
	term := strings.ToLower(strings.TrimSpace(q.Search))
	var last cursor
	used := 0
	for i := start; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		file := files[i]
		if file.size > MiB {
			return result, fmt.Errorf("日志文件超过分卷上限，请等待自动清理")
		}
		body, err := os.ReadFile(file.path)
		if err != nil {
			return result, err
		}
		if i == start && before >= 0 {
			if before > int64(len(body)) {
				return result, ErrCursorExpired
			}
			body = body[:before]
		}
		end := len(body)
		for end > 0 {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if body[end-1] == '\n' {
				end--
			}
			pos := bytes.LastIndexByte(body[:end], '\n') + 1
			line := bytes.TrimSuffix(body[pos:end], []byte{'\r'})
			end = pos
			if term != "" && !strings.Contains(strings.ToLower(string(line)), term) {
				continue
			}
			truncated := len(line) > 32*1024
			if truncated {
				line = line[:32*1024]
			}
			if len(result.Lines) == q.Limit || used+len(line) > int(MiB) {
				result.HasMore = true
				token, _ := json.Marshal(last)
				result.NextCursor = base64.RawURLEncoding.EncodeToString(token)
				break
			}
			result.Lines = append(result.Lines, RawLine{Key: fmt.Sprintf("%s:%d", file.id, pos), Message: string(line), Truncated: truncated})
			used += len(line)
			last = cursor{file.id, int64(pos), s.generation}
		}
		if result.HasMore {
			break
		}
	}
	for i, j := 0, len(result.Lines)-1; i < j; i, j = i+1, j-1 {
		result.Lines[i], result.Lines[j] = result.Lines[j], result.Lines[i]
	}
	return result, nil
}
