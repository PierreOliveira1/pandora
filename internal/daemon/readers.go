package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/events"
)

const readerMaxOutput = 64 * 1024

type runningReader struct {
	cancel context.CancelFunc
	done   chan struct{}
	config config.EventReader
}

func (s *Server) runReader(ctx context.Context, reader config.EventReader) {
	failures := 0
	failingSeq := int64(0)
	for {
		if ctx.Err() != nil {
			return
		}
		version, err := s.core.Events().Version()
		if err != nil {
			slog.Error("checking webhook events", "reader", reader.Name, "error", err)
		}
		pending, err := s.core.PendingEvents(ctx, reader.Name, events.Filter{
			Service: reader.Service,
			Account: reader.Account,
			Topic:   reader.Topic,
			Limit:   1,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("reading webhook events", "reader", reader.Name, "error", err)
			if !waitReader(ctx, time.Second) {
				return
			}
			continue
		}
		if len(pending) == 0 {
			if !s.waitForEventVersion(ctx, reader.Name, version) {
				return
			}
			continue
		}

		event := pending[0]
		if event.Seq != failingSeq {
			failures = 0
			failingSeq = event.Seq
		}
		code, stderr, runErr := s.runReaderCommand(ctx, reader, event)
		if runErr == nil {
			if err := s.core.AckEvents(ctx, reader.Name, []int64{event.Seq}); err != nil {
				slog.Error("acknowledging webhook event", "reader", reader.Name, "seq", event.Seq, "error", err)
			} else {
				slog.Info("delivered webhook event", "reader", reader.Name, "seq", event.Seq)
			}
			failures = 0
			failingSeq = 0
			continue
		}

		failures++
		if ctx.Err() != nil {
			return
		}
		slog.Error("reader command failed",
			"reader", reader.Name,
			"seq", event.Seq,
			"code", code,
			"stderr", strings.TrimSpace(stderr),
			"error", runErr,
		)
		if failures >= 5 {
			slog.Error("skipping event after 5 failed attempts", "reader", reader.Name, "seq", event.Seq)
			if err := s.core.AckEvents(ctx, reader.Name, []int64{event.Seq}); err != nil {
				slog.Error("acknowledging skipped webhook event", "reader", reader.Name, "seq", event.Seq, "error", err)
			}
			failures = 0
			failingSeq = 0
			continue
		}
		if !s.waitReaderBackoff(ctx, failures) {
			return
		}
	}
}

func (s *Server) waitForEventVersion(ctx context.Context, reader, version string) bool {
	for {
		current, err := s.core.Events().Version()
		if err != nil {
			slog.Error("checking webhook events", "reader", reader, "error", err)
		} else if current != version {
			return true
		}
		if !waitReader(ctx, time.Second) {
			return false
		}
	}
}

func (s *Server) waitReaderBackoff(ctx context.Context, failures int) bool {
	if len(s.readerBackoff) == 0 {
		return true
	}
	index := failures - 1
	if index < 0 {
		index = 0
	}
	if index >= len(s.readerBackoff) {
		index = len(s.readerBackoff) - 1
	}
	return waitReader(ctx, s.readerBackoff[index])
}

func waitReader(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Server) runReaderCommand(ctx context.Context, reader config.EventReader, event events.Event) (int, string, error) {
	if len(reader.Command) == 0 {
		return -1, "reader has no command", errors.New("reader has no command")
	}
	timeout := reader.TimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	data, err := json.Marshal(event)
	if err != nil {
		return -1, "", fmt.Errorf("encoding event: %w", err)
	}
	command := exec.CommandContext(commandCtx, reader.Command[0], reader.Command[1:]...)
	command.WaitDelay = 2 * time.Second
	command.Stdin = bytes.NewReader(append(data, '\n'))
	command.Env = readerEnvironment(reader.Name, event)
	stdout := &limitedBuffer{limit: readerMaxOutput}
	stderr := &limitedBuffer{limit: readerMaxOutput}
	command.Stdout = stdout
	command.Stderr = stderr

	err = command.Run()
	if err == nil {
		return 0, stderr.String(), nil
	}
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return code, stderr.String(), err
}

func readerEnvironment(reader string, event events.Event) []string {
	env := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "PANDORA_EVENT_SEQ", "PANDORA_EVENT_ID", "PANDORA_EVENT_SERVICE",
			"PANDORA_EVENT_ACCOUNT", "PANDORA_EVENT_TOPIC", "PANDORA_READER":
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"PANDORA_EVENT_SEQ="+strconv.FormatInt(event.Seq, 10),
		"PANDORA_EVENT_ID="+event.ID,
		"PANDORA_EVENT_SERVICE="+event.Service,
		"PANDORA_EVENT_ACCOUNT="+event.Account,
		"PANDORA_EVENT_TOPIC="+event.Topic,
		"PANDORA_READER="+reader,
	)
	return env
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	size := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.buffer.Write(data)
	}
	return size, nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
