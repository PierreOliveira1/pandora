package events_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PierreOliveira1/pandora/internal/events"
)

type clock struct {
	current time.Time
}

func (c *clock) now() time.Time {
	return c.current
}

func newQueue(t *testing.T, current time.Time) (*events.Queue, *clock) {
	t.Helper()
	c := &clock{current: current}
	return events.NewQueue(t.TempDir(), c.now), c
}

func appendEvent(t *testing.T, q *events.Queue, service, account, topic string) events.Event {
	t.Helper()
	ev, duplicate, err := q.Append(context.Background(), events.Event{
		Service: service,
		Account: account,
		Topic:   topic,
		Payload: json.RawMessage(`{"ok":true}`),
	}, "")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if duplicate {
		t.Fatal("Append() duplicate = true, want false")
	}
	return ev
}

func pending(t *testing.T, q *events.Queue, reader string, f events.Filter) []events.Event {
	t.Helper()
	got, err := q.Pending(context.Background(), reader, f)
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	return got
}

func seqs(eventsList []events.Event) []int64 {
	out := make([]int64, len(eventsList))
	for i, ev := range eventsList {
		out[i] = ev.Seq
	}
	return out
}

func equalSeqs(got []events.Event, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Seq != want[i] {
			return false
		}
	}
	return true
}

func TestAReaderSeesOnlyTheEventsItHasNotAcknowledged(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	for i := 0; i < 3; i++ {
		appendEvent(t, q, "one", "local", "")
	}

	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 1, 2, 3) {
		t.Fatalf("Pending() = %v, want seqs 1, 2 and 3", seqs(got))
	}
	if err := q.Ack(context.Background(), "ai", 1); err != nil {
		t.Fatalf("Ack(1) error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 2, 3) {
		t.Fatalf("Pending() after acking 1 = %v, want seqs 2 and 3", seqs(got))
	}
	if err := q.Ack(context.Background(), "ai", 3); err != nil {
		t.Fatalf("Ack(3) error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 2) {
		t.Fatalf("Pending() after acking 3 = %v, want seq 2", seqs(got))
	}
	if err := q.Ack(context.Background(), "ai", 2); err != nil {
		t.Fatalf("Ack(2) error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); len(got) != 0 {
		t.Fatalf("Pending() after acking everything = %v, want none", seqs(got))
	}
}

func TestTwoReadersKeepIndependentPositions(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	appendEvent(t, q, "one", "local", "")
	appendEvent(t, q, "one", "local", "")

	if err := q.Ack(context.Background(), "one", 1); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if got := pending(t, q, "one", events.Filter{}); !equalSeqs(got, 2) {
		t.Fatalf("Pending(one) = %v, want seq 2", seqs(got))
	}
	if got := pending(t, q, "two", events.Filter{}); !equalSeqs(got, 1, 2) {
		t.Fatalf("Pending(two) = %v, want seqs 1 and 2", seqs(got))
	}

	if err := q.Ack(context.Background(), "two", 2); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if got := pending(t, q, "two", events.Filter{}); !equalSeqs(got, 1) {
		t.Fatalf("Pending(two) after acking 2 = %v, want seq 1", seqs(got))
	}
}

func TestAcknowledgingOneServiceKeepsOtherServicesPendingForTheSameReader(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	appendEvent(t, q, "alpha", "local", "")
	appendEvent(t, q, "beta", "local", "")
	appendEvent(t, q, "alpha", "local", "")

	if got := pending(t, q, "ai", events.Filter{Service: "alpha"}); !equalSeqs(got, 1, 3) {
		t.Fatalf("Pending(alpha) = %v, want seqs 1 and 3", seqs(got))
	}
	if err := q.Ack(context.Background(), "ai", 1, 3); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 2) {
		t.Fatalf("Pending() after acking alpha = %v, want seq 2", seqs(got))
	}
	if got := pending(t, q, "ai", events.Filter{Service: "alpha"}); len(got) != 0 {
		t.Fatalf("Pending(alpha) after acking = %v, want none", seqs(got))
	}
}

func TestPendingFiltersByAccountTopicAndLimit(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	appendEvent(t, q, "one", "a", "x")
	appendEvent(t, q, "one", "b", "x")
	appendEvent(t, q, "one", "a", "y")

	if got := pending(t, q, "ai", events.Filter{Account: "a"}); !equalSeqs(got, 1, 3) {
		t.Fatalf("Pending(account a) = %v, want seqs 1 and 3", seqs(got))
	}
	if got := pending(t, q, "ai", events.Filter{Topic: "x"}); !equalSeqs(got, 1, 2) {
		t.Fatalf("Pending(topic x) = %v, want seqs 1 and 2", seqs(got))
	}
	if got := pending(t, q, "ai", events.Filter{Limit: 1}); !equalSeqs(got, 1) {
		t.Fatalf("Pending(limit 1) = %v, want seq 1", seqs(got))
	}
}

func TestRecentReturnsNewestFirstWithoutChangingReaderProgress(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	appendEvent(t, q, "one", "local", "a")
	appendEvent(t, q, "one", "local", "b")
	appendEvent(t, q, "one", "local", "c")
	if err := q.Ack(context.Background(), "ai", 1); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	recent, err := q.Recent(context.Background(), events.Filter{})
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if !equalSeqs(recent, 3, 2, 1) {
		t.Fatalf("Recent() = %v, want seqs 3, 2 and 1", seqs(recent))
	}
	limited, err := q.Recent(context.Background(), events.Filter{Service: "one", Limit: 2})
	if err != nil {
		t.Fatalf("Recent(limit) error = %v", err)
	}
	if !equalSeqs(limited, 3, 2) {
		t.Fatalf("Recent(limit) = %v, want seqs 3 and 2", seqs(limited))
	}
	pending := pending(t, q, "ai", events.Filter{})
	if !equalSeqs(pending, 2, 3) {
		t.Fatalf("Pending() after Recent = %v, want seqs 2 and 3", seqs(pending))
	}
}

func TestARepeatedDeliveryWithin24HoursIsStoredOnce(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	q, c := newQueue(t, start)

	first, duplicate, err := q.Append(context.Background(), events.Event{Service: "one", Account: "local", Payload: json.RawMessage(`{}`)}, "same")
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if duplicate {
		t.Fatal("first Append() duplicate = true, want false")
	}
	if first.Seq != 1 {
		t.Fatalf("first Seq = %d, want 1", first.Seq)
	}

	c.current = start.Add(23 * time.Hour)
	second, duplicate, err := q.Append(context.Background(), events.Event{Service: "one", Account: "local", Payload: json.RawMessage(`{}`)}, "same")
	if err != nil {
		t.Fatalf("repeated Append() error = %v", err)
	}
	if !duplicate {
		t.Fatal("repeated Append() duplicate = false, want true")
	}
	if second.Seq != 0 || second.ID != "" {
		t.Fatalf("repeated Append() event = %+v, want an empty event", second)
	}

	c.current = start.Add(24*time.Hour + time.Second)
	third, duplicate, err := q.Append(context.Background(), events.Event{Service: "one", Account: "local", Payload: json.RawMessage(`{}`)}, "same")
	if err != nil {
		t.Fatalf("Append() after 24h error = %v", err)
	}
	if duplicate {
		t.Fatal("Append() after 24h duplicate = true, want false")
	}
	if third.Seq != 2 {
		t.Fatalf("Append() after 24h Seq = %d, want 2", third.Seq)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 1, 2) {
		t.Fatalf("Pending() = %v, want seqs 1 and 2", seqs(got))
	}
}

func TestRetentionKeepsTheNewestEventsAndDropsTheOldOnes(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	q, c := newQueue(t, start)

	times := []time.Duration{0, time.Hour, 2 * time.Hour, 3 * time.Hour}
	for _, offset := range times {
		c.current = start.Add(offset)
		appendEvent(t, q, "one", "local", "")
	}
	if err := q.Ack(context.Background(), "ai", 1); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	c.current = start.Add(3 * time.Hour)
	if err := q.Prune(context.Background(), 2*time.Hour, 2); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 3, 4) {
		t.Fatalf("Pending() after Prune = %v, want seqs 3 and 4", seqs(got))
	}

	if err := q.Prune(context.Background(), 24*time.Hour, 1); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 4) {
		t.Fatalf("Pending() after count Prune = %v, want seq 4", seqs(got))
	}
}

func TestPruningEveryEventClearsWhatReadersHadAcknowledged(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	c := &clock{current: start}
	q := events.NewQueue(dir, c.now)

	for i := 0; i < 3; i++ {
		appendEvent(t, q, "one", "local", "")
	}
	if err := q.Ack(context.Background(), "ai", 2); err != nil {
		t.Fatalf("Ack(2) error = %v", err)
	}

	c.current = start.Add(2 * time.Hour)
	if err := q.Prune(context.Background(), time.Hour, 100); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); len(got) != 0 {
		t.Fatalf("Pending() after pruning everything = %v, want none", seqs(got))
	}

	appendEvent(t, q, "one", "local", "")
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 4) {
		t.Fatalf("Pending() after a new event = %v, want seq 4", seqs(got))
	}
	if err := q.Ack(context.Background(), "ai", 4); err != nil {
		t.Fatalf("Ack(4) error = %v", err)
	}
	if got := pending(t, q, "ai", events.Filter{}); len(got) != 0 {
		t.Fatalf("Pending() after acking the new event = %v, want none", seqs(got))
	}

	data, err := os.ReadFile(filepath.Join(dir, "readers.json"))
	if err != nil {
		t.Fatalf("reading readers.json: %v", err)
	}
	var readers map[string]struct {
		Position int64   `json:"position"`
		Acked    []int64 `json:"acked"`
	}
	if err := json.Unmarshal(data, &readers); err != nil {
		t.Fatalf("parsing readers.json: %v", err)
	}
	reader := readers["ai"]
	if reader.Position != 4 {
		t.Errorf("reader position = %d, want 4", reader.Position)
	}
	if len(reader.Acked) != 0 {
		t.Errorf("reader acked = %v, want none", reader.Acked)
	}
}

func TestTheSequenceKeepsGrowingAfterPruning(t *testing.T) {
	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	q, c := newQueue(t, start)
	for i := 0; i < 3; i++ {
		appendEvent(t, q, "one", "local", "")
	}

	c.current = start.Add(time.Minute)
	if err := q.Prune(context.Background(), 24*time.Hour, 1); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	c.current = start.Add(2 * time.Minute)
	last := appendEvent(t, q, "one", "local", "")

	if last.Seq != 4 {
		t.Fatalf("Append() after Prune Seq = %d, want 4", last.Seq)
	}
	if got := pending(t, q, "ai", events.Filter{}); !equalSeqs(got, 3, 4) {
		t.Fatalf("Pending() = %v, want seqs 3 and 4", seqs(got))
	}
}

func TestQueueFilesAreReadableOnlyByTheOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix file permissions")
	}
	dir := t.TempDir()
	c := &clock{current: time.Now()}
	q := events.NewQueue(dir, c.now)
	appendEvent(t, q, "one", "local", "")
	if err := q.Ack(context.Background(), "ai", 1); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if err := q.Prune(context.Background(), 24*time.Hour, 10); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(queue dir) error = %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("queue directory mode = %o, want 700", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Info(%q) error = %v", entry.Name(), err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 600", entry.Name(), info.Mode().Perm())
		}
	}
}

func TestAnInvalidReaderNameIsRefused(t *testing.T) {
	q, _ := newQueue(t, time.Now())
	appendEvent(t, q, "one", "local", "")

	for _, name := range []string{"", "AI", "-reader", "reader_", strings.Repeat("a", 33)} {
		if _, err := q.Pending(context.Background(), name, events.Filter{}); err == nil || !strings.Contains(err.Error(), "invalid reader name") {
			t.Errorf("Pending(%q) error = %v, want an invalid reader name error", name, err)
		}
		if err := q.Ack(context.Background(), name, 1); err == nil || !strings.Contains(err.Error(), "invalid reader name") {
			t.Errorf("Ack(%q) error = %v, want an invalid reader name error", name, err)
		}
	}
}

func TestLastReceivedTellsWhenEachAccountGotItsLatestEvent(t *testing.T) {
	start := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	q, c := newQueue(t, start)
	appendEvent(t, q, "shop", "main", "orders")
	c.current = start.Add(time.Hour)
	appendEvent(t, q, "shop", "main", "orders")
	c.current = start.Add(2 * time.Hour)
	appendEvent(t, q, "chat", "team", "messages")

	last, err := q.LastReceived(context.Background())
	if err != nil {
		t.Fatalf("LastReceived() error = %v", err)
	}
	if got := last["shop/main"]; !got.Equal(start.Add(time.Hour)) {
		t.Fatalf("shop/main last event = %v, want %v", got, start.Add(time.Hour))
	}
	if got := last["chat/team"]; !got.Equal(start.Add(2 * time.Hour)) {
		t.Fatalf("chat/team last event = %v, want %v", got, start.Add(2*time.Hour))
	}
	if _, ok := last["shop/other"]; ok {
		t.Fatal("an account without events has a last event")
	}
}

func TestTheQueueVersionChangesWhenAnEventArrives(t *testing.T) {
	q, _ := newQueue(t, time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	empty, err := q.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	appendEvent(t, q, "shop", "main", "orders")
	first, err := q.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if first == empty {
		t.Fatal("version did not change after the first event")
	}
	appendEvent(t, q, "shop", "main", "orders")
	second, err := q.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if second == first {
		t.Fatal("version did not change after the second event")
	}
}

func TestSkippingPendingEventsMakesAReaderStartAtTheNextEvent(t *testing.T) {
	q, _ := newQueue(t, time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	appendEvent(t, q, "shop", "main", "orders")
	appendEvent(t, q, "shop", "main", "orders")

	if err := q.SkipPending(context.Background(), "replier"); err != nil {
		t.Fatalf("SkipPending() error = %v", err)
	}
	if got := pending(t, q, "replier", events.Filter{}); len(got) != 0 {
		t.Fatalf("pending after skip = %v, want none", seqs(got))
	}
	next := appendEvent(t, q, "shop", "main", "orders")
	if got := pending(t, q, "replier", events.Filter{}); !equalSeqs(got, next.Seq) {
		t.Fatalf("pending = %v, want only %d", seqs(got), next.Seq)
	}
	if got := pending(t, q, "other", events.Filter{}); len(got) != 3 {
		t.Fatalf("another reader sees %d events, want 3", len(got))
	}
}
