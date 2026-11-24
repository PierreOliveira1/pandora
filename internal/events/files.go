package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	eventsFile  = "events.jsonl"
	stateFile   = "state.json"
	dedupFile   = "dedup.json"
	readersFile = "readers.json"
	lockFile    = "events.lock"
	lockRetry   = 50 * time.Millisecond
	lockTimeout = 10 * time.Second
	maxEventLen = 16 << 20
)

type queueState struct {
	Next int64 `json:"next"`
}

type readerState struct {
	Position int64   `json:"position"`
	Acked    []int64 `json:"acked"`
}

type readersState map[string]*readerState

func (q *Queue) acquire(ctx context.Context, shared bool) (*flock.Flock, error) {
	if err := os.MkdirAll(q.dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating events directory %s: %w", q.dir, err)
	}
	if err := os.Chmod(q.dir, 0o700); err != nil {
		return nil, fmt.Errorf("setting events directory permissions: %w", err)
	}
	lock := flock.New(filepath.Join(q.dir, lockFile))
	lockCtx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()
	var (
		locked bool
		err    error
	)
	if shared {
		locked, err = lock.TryRLockContext(lockCtx, lockRetry)
	} else {
		locked, err = lock.TryLockContext(lockCtx, lockRetry)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("locking events directory %s: timed out waiting for another process", q.dir)
		}
		return nil, fmt.Errorf("locking events directory %s: %w", q.dir, err)
	}
	if !locked {
		return nil, fmt.Errorf("locking events directory %s: timed out waiting for another process", q.dir)
	}
	return lock, nil
}

func (q *Queue) loadState() (queueState, error) {
	data, err := os.ReadFile(filepath.Join(q.dir, stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return queueState{Next: 1}, nil
	}
	if err != nil {
		return queueState{}, fmt.Errorf("reading events state: %w", err)
	}
	var state queueState
	if err := json.Unmarshal(data, &state); err != nil {
		return queueState{}, fmt.Errorf("parsing events state: %w", err)
	}
	if state.Next < 1 {
		state.Next = 1
	}
	return state, nil
}

func (q *Queue) loadDedup() (map[string]time.Time, error) {
	data, err := os.ReadFile(filepath.Join(q.dir, dedupFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading dedup index: %w", err)
	}
	dedup := map[string]time.Time{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &dedup); err != nil {
			return nil, fmt.Errorf("parsing dedup index: %w", err)
		}
	}
	if dedup == nil {
		dedup = map[string]time.Time{}
	}
	return dedup, nil
}

func (q *Queue) loadReaders() (readersState, error) {
	data, err := os.ReadFile(filepath.Join(q.dir, readersFile))
	if errors.Is(err, os.ErrNotExist) {
		return readersState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading event readers: %w", err)
	}
	readers := readersState{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &readers); err != nil {
			return nil, fmt.Errorf("parsing event readers: %w", err)
		}
	}
	if readers == nil {
		readers = readersState{}
	}
	return readers, nil
}

func (q *Queue) readEvents() ([]Event, error) {
	path := filepath.Join(q.dir, eventsFile)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening events file: %w", err)
	}
	defer file.Close()

	events := make([]Event, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxEventLen)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("parsing event in %s: %w", path, err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading events file: %w", err)
	}
	return events, nil
}

func (q *Queue) appendEvent(ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}
	path := filepath.Join(q.dir, eventsFile)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("appending event: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("syncing events file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing events file: %w", err)
	}
	return nil
}

func (q *Queue) writeEvents(events []Event) (err error) {
	tmp, err := os.CreateTemp(q.dir, ".events-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temporary events file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting events file permissions: %w", err)
	}
	writer := bufio.NewWriter(tmp)
	for _, ev := range events {
		data, marshalErr := json.Marshal(ev)
		if marshalErr != nil {
			err = fmt.Errorf("encoding event: %w", marshalErr)
			return err
		}
		if _, writeErr := writer.Write(append(data, '\n')); writeErr != nil {
			err = fmt.Errorf("writing events file: %w", writeErr)
			return err
		}
	}
	if err = writer.Flush(); err != nil {
		return fmt.Errorf("writing events file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("syncing events file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing events file: %w", err)
	}
	if err = os.Rename(tmpName, filepath.Join(q.dir, eventsFile)); err != nil {
		return fmt.Errorf("replacing events file: %w", err)
	}
	return nil
}

func (q *Queue) writeJSON(name string, value any) (err error) {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", name, err)
	}
	tmp, err := os.CreateTemp(q.dir, "."+name+"-*")
	if err != nil {
		return fmt.Errorf("creating temporary %s: %w", name, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting %s permissions: %w", name, err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", name, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", name, err)
	}
	if err = os.Rename(tmpName, filepath.Join(q.dir, name)); err != nil {
		return fmt.Errorf("replacing %s: %w", name, err)
	}
	return nil
}
