package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/testutil/fakemcp"
)

func main() {
	if path := os.Getenv("FAKE_START_LOG"); path != "" {
		if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			_, _ = file.WriteString("started\n")
			_ = file.Close()
		}
	}

	if raw := os.Getenv("FAKE_START_DELAY_MS"); raw != "" {
		if milliseconds, err := strconv.Atoi(raw); err == nil && milliseconds > 0 {
			time.Sleep(time.Duration(milliseconds) * time.Millisecond)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := fakemcp.NewServer(os.Getenv("FAKE_ACCOUNT")).Run(ctx, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
