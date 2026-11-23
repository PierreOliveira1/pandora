package guard

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultMaxLogSize = 10 * 1024 * 1024
	maxErrorRunes     = 500
)

type Entry struct {
	Time       time.Time `json:"time"`
	Ref        string    `json:"ref"`
	Tool       string    `json:"tool"`
	Client     string    `json:"client"`
	DurationMs int64     `json:"duration_ms"`
	OK         bool      `json:"ok"`
	Blocked    bool      `json:"blocked,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Filter struct {
	Ref        string
	OnlyErrors bool
}

type Log struct {
	path    string
	maxSize int64
}

func NewLog(path string) *Log {
	return NewLogWithLimit(path, defaultMaxLogSize)
}

func NewLogWithLimit(path string, maxBytes int64) *Log {
	if maxBytes <= 0 {
		maxBytes = defaultMaxLogSize
	}
	return &Log{path: path, maxSize: maxBytes}
}

func (l *Log) Append(entry Entry) error {
	if len([]rune(entry.Error)) > maxErrorRunes {
		entry.Error = string([]rune(entry.Error)[:maxErrorRunes])
	}
	directory := filepath.Dir(l.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("creating call log directory %s: %w", directory, err)
	}
	info, err := os.Stat(l.path)
	if err == nil && info.Size() > l.maxSize {
		backup := l.path + ".1"
		if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing old call log %s: %w", backup, err)
		}
		if err := os.Rename(l.path, backup); err != nil {
			return fmt.Errorf("rotating call log %s: %w", l.path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking call log %s: %w", l.path, err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encoding call log entry: %w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening call log %s: %w", l.path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("setting call log permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("writing call log %s: %w", l.path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing call log %s: %w", l.path, err)
	}
	return nil
}

func (l *Log) Tail(n int, filter Filter) ([]Entry, error) {
	entries := make([]Entry, 0)
	if n <= 0 {
		return entries, nil
	}
	add := func(entry Entry) {
		if !filter.matches(entry) {
			return
		}
		entries = append(entries, entry)
		if len(entries) > n {
			entries = entries[1:]
		}
	}
	if err := readCallLog(l.path+".1", add); err != nil {
		return nil, err
	}
	if err := readCallLog(l.path, add); err != nil {
		return nil, err
	}
	return entries, nil
}

func DetectClient(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if value := getenv("PANDORA_CLIENT"); value != "" {
		return value
	}
	switch {
	case getenv("CLAUDECODE") != "":
		return "claude-code"
	case getenv("OPENCODE") != "":
		return "opencode"
	case getenv("CODEX_SANDBOX") != "" || getenv("CODEX_HOME") != "":
		return "codex"
	case getenv("CURSOR_TRACE_ID") != "" || getenv("CURSOR_AGENT") != "":
		return "cursor"
	default:
		return "terminal"
	}
}

func (f Filter) matches(entry Entry) bool {
	if f.OnlyErrors && entry.OK {
		return false
	}
	if f.Ref == "" {
		return true
	}
	if strings.Contains(f.Ref, "/") {
		return entry.Ref == f.Ref
	}
	service, _, _ := strings.Cut(entry.Ref, "/")
	return service == f.Ref
}

func readCallLog(path string, add func(Entry)) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading call log %s: %w", path, err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			var entry *Entry
			if json.Unmarshal([]byte(line), &entry) == nil && entry != nil {
				add(*entry)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading call log %s: %w", path, err)
		}
	}
}
