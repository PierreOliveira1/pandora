package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const dedupWindow = 24 * time.Hour

type Event struct {
	Seq        int64             `json:"seq"`
	ID         string            `json:"id"`
	ReceivedAt time.Time         `json:"received_at"`
	Service    string            `json:"service"`
	Account    string            `json:"account"`
	Topic      string            `json:"topic,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Payload    json.RawMessage   `json:"payload"`
}

type Filter struct {
	Service string
	Account string
	Topic   string
	Limit   int
}

type Queue struct {
	dir string
	now func() time.Time
	mu  sync.Mutex
}

func NewQueue(dir string, now func() time.Time) *Queue {
	if now == nil {
		now = time.Now
	}
	return &Queue{dir: dir, now: now}
}

func (q *Queue) Append(ctx context.Context, ev Event, dedupKey string) (Event, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, false)
	if err != nil {
		return Event{}, false, err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	now := q.now()
	dedup, err := q.loadDedup()
	if err != nil {
		return Event{}, false, err
	}
	if dedupKey != "" {
		if seen, ok := dedup[dedupKey]; ok && now.Sub(seen) < dedupWindow {
			return Event{}, true, nil
		}
	}

	state, err := q.loadState()
	if err != nil {
		return Event{}, false, err
	}
	ev.Seq = state.Next
	if ev.ID == "" {
		id, err := newID()
		if err != nil {
			return Event{}, false, err
		}
		ev.ID = id
	}
	if ev.ReceivedAt.IsZero() {
		ev.ReceivedAt = now
	}

	if err := q.appendEvent(ev); err != nil {
		return Event{}, false, err
	}
	state.Next = ev.Seq + 1
	if err := q.writeJSON(stateFile, state); err != nil {
		return Event{}, false, err
	}

	cutoff := now.Add(-dedupWindow)
	for key, seen := range dedup {
		if !seen.After(cutoff) {
			delete(dedup, key)
		}
	}
	if dedupKey != "" {
		dedup[dedupKey] = now
	}
	if err := q.writeJSON(dedupFile, dedup); err != nil {
		return Event{}, false, err
	}
	return ev, false, nil
}

func (q *Queue) Recent(ctx context.Context, f Filter) ([]Event, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	all, err := q.readEvents()
	if err != nil {
		return nil, err
	}
	matched := make([]Event, 0, len(all))
	for _, ev := range all {
		if f.Service != "" && ev.Service != f.Service {
			continue
		}
		if f.Account != "" && ev.Account != f.Account {
			continue
		}
		if f.Topic != "" && ev.Topic != f.Topic {
			continue
		}
		matched = append(matched, ev)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Seq > matched[j].Seq
	})
	if f.Limit > 0 && len(matched) > f.Limit {
		matched = matched[:f.Limit]
	}
	return matched, nil
}

func (q *Queue) Pending(ctx context.Context, reader string, f Filter) ([]Event, error) {
	if err := validateReader(reader); err != nil {
		return nil, err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	readers, err := q.loadReaders()
	if err != nil {
		return nil, err
	}
	var position int64
	acked := map[int64]bool{}
	if state, ok := readers[reader]; ok && state != nil {
		position = state.Position
		for _, seq := range state.Acked {
			acked[seq] = true
		}
	}

	all, err := q.readEvents()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Seq < all[j].Seq
	})

	pending := make([]Event, 0)
	for _, ev := range all {
		if ev.Seq <= position || acked[ev.Seq] {
			continue
		}
		if f.Service != "" && ev.Service != f.Service {
			continue
		}
		if f.Account != "" && ev.Account != f.Account {
			continue
		}
		if f.Topic != "" && ev.Topic != f.Topic {
			continue
		}
		pending = append(pending, ev)
		if f.Limit > 0 && len(pending) >= f.Limit {
			break
		}
	}
	return pending, nil
}

func (q *Queue) Ack(ctx context.Context, reader string, seqs ...int64) error {
	if err := validateReader(reader); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	state, err := q.loadState()
	if err != nil {
		return err
	}
	last := state.Next - 1
	if last < 0 {
		last = 0
	}

	readers, err := q.loadReaders()
	if err != nil {
		return err
	}
	entry, ok := readers[reader]
	if !ok || entry == nil {
		entry = &readerState{Acked: []int64{}}
	}
	acked := make(map[int64]bool, len(entry.Acked)+len(seqs))
	for _, seq := range entry.Acked {
		acked[seq] = true
	}
	for _, seq := range seqs {
		if seq > last {
			return fmt.Errorf("event %d does not exist", seq)
		}
		if seq <= entry.Position {
			continue
		}
		acked[seq] = true
	}
	for acked[entry.Position+1] {
		delete(acked, entry.Position+1)
		entry.Position++
	}
	entry.Acked = make([]int64, 0, len(acked))
	for seq := range acked {
		if seq > entry.Position {
			entry.Acked = append(entry.Acked, seq)
		}
	}
	sort.Slice(entry.Acked, func(i, j int) bool {
		return entry.Acked[i] < entry.Acked[j]
	})
	readers[reader] = entry
	return q.writeJSON(readersFile, readers)
}

func (q *Queue) Prune(ctx context.Context, maxAge time.Duration, maxCount int) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	all, err := q.readEvents()
	if err != nil {
		return err
	}
	state, err := q.loadState()
	if err != nil {
		return err
	}
	cutoff := q.now().Add(-maxAge)
	kept := make([]Event, 0, len(all))
	for _, ev := range all {
		if ev.ReceivedAt.After(cutoff) {
			kept = append(kept, ev)
		}
	}
	if maxCount >= 0 && len(kept) > maxCount {
		kept = kept[len(kept)-maxCount:]
	}
	floor := state.Next - 1
	if len(kept) > 0 {
		floor = kept[0].Seq - 1
	}
	if floor < 0 {
		floor = 0
	}
	if err := q.writeEvents(kept); err != nil {
		return err
	}

	readers, err := q.loadReaders()
	if err != nil {
		return err
	}
	if len(readers) == 0 {
		return nil
	}
	for _, entry := range readers {
		if entry == nil {
			continue
		}
		if entry.Position < floor {
			entry.Position = floor
		}
		acked := make(map[int64]bool, len(entry.Acked))
		for _, seq := range entry.Acked {
			acked[seq] = true
		}
		for acked[entry.Position+1] {
			delete(acked, entry.Position+1)
			entry.Position++
		}
		remaining := make([]int64, 0, len(acked))
		for seq := range acked {
			if seq > entry.Position {
				remaining = append(remaining, seq)
			}
		}
		sort.Slice(remaining, func(i, j int) bool {
			return remaining[i] < remaining[j]
		})
		entry.Acked = remaining
	}
	return q.writeJSON(readersFile, readers)
}

func validateReader(name string) error {
	valid := len(name) > 0 && len(name) <= 32
	for i := 0; valid && i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' && i > 0:
		default:
			valid = false
		}
	}
	if !valid {
		return fmt.Errorf("invalid reader name %q: use lowercase letters, digits and hyphens, up to 32 characters", name)
	}
	return nil
}

func newID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generating event id: %w", err)
	}
	return "evt_" + hex.EncodeToString(random), nil
}

func (q *Queue) LastReceived(ctx context.Context) (map[string]time.Time, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	all, err := q.readEvents()
	if err != nil {
		return nil, err
	}
	last := map[string]time.Time{}
	for _, ev := range all {
		key := ev.Service + "/" + ev.Account
		if ev.ReceivedAt.After(last[key]) {
			last[key] = ev.ReceivedAt
		}
	}
	return last, nil
}

func (q *Queue) Version() (string, error) {
	info, err := os.Stat(filepath.Join(q.dir, eventsFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking events file: %w", err)
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()), nil
}

func (q *Queue) SkipPending(ctx context.Context, reader string) error {
	if err := validateReader(reader); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	lock, err := q.acquire(ctx, false)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	state, err := q.loadState()
	if err != nil {
		return err
	}
	readers, err := q.loadReaders()
	if err != nil {
		return err
	}
	readers[reader] = &readerState{Position: state.Next - 1, Acked: []int64{}}
	return q.writeJSON(readersFile, readers)
}
