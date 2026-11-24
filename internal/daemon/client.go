package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/source"
)

type Client struct {
	httpClient *http.Client
}

func Dial(ctx context.Context, socketPath string) (*Client, error) {
	client := &Client{httpClient: unixHTTPClient(socketPath)}
	probeCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, err := client.Health(probeCtx); err != nil {
		return nil, err
	}
	return client, nil
}

func unixHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport}
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var health Health
	if err := c.request(ctx, http.MethodGet, "/v1/health", nil, &health); err != nil {
		return Health{}, err
	}
	return health, nil
}

func (c *Client) Tools(ctx context.Context, service string, refresh bool) (*core.Catalog, error) {
	var catalog core.Catalog
	body := toolsRequest{Service: service, Refresh: refresh}
	if err := c.request(ctx, http.MethodPost, "/v1/tools", body, &catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (c *Client) Describe(ctx context.Context, service, toolName string) (*mcp.Tool, error) {
	var tool mcp.Tool
	body := describeRequest{Service: service, Tool: toolName}
	if err := c.request(ctx, http.MethodPost, "/v1/describe", body, &tool); err != nil {
		return nil, err
	}
	return &tool, nil
}

func (c *Client) Search(ctx context.Context, query, service string, limit int) ([]core.SearchHit, int, error) {
	var response searchResponse
	body := searchRequest{Query: query, Service: service, Limit: limit}
	if err := c.request(ctx, http.MethodPost, "/v1/search", body, &response); err != nil {
		return nil, 0, err
	}
	if response.Results == nil {
		response.Results = []core.SearchHit{}
	}
	return response.Results, response.Total, nil
}

func (c *Client) Call(ctx context.Context, ref, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	var response struct {
		Result *mcp.CallToolResult `json:"result"`
	}
	body := callRequest{
		Ref:    ref,
		Tool:   tool,
		Args:   args,
		Client: core.ClientFrom(ctx),
	}
	if err := c.request(ctx, http.MethodPost, "/v1/call", body, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (c *Client) Status(ctx context.Context, ref string) ([]core.StatusInfo, error) {
	var wire []statusWire
	if err := c.request(ctx, http.MethodPost, "/v1/status", statusRequest{Ref: ref}, &wire); err != nil {
		return nil, err
	}
	infos := make([]core.StatusInfo, 0, len(wire))
	for _, value := range wire {
		infos = append(infos, core.StatusInfo{
			Ref:        value.Ref,
			OK:         value.OK,
			NeedsLogin: value.NeedsLogin,
			Tools:      value.Tools,
			Latency:    time.Duration(value.LatencyMS) * time.Millisecond,
			Error:      value.Error,
		})
	}
	return infos, nil
}

func (c *Client) ReloadWebhooks(ctx context.Context) (WebhooksState, error) {
	var state WebhooksState
	if err := c.request(ctx, http.MethodPost, "/v1/webhooks/reload", struct{}{}, &state); err != nil {
		return WebhooksState{}, err
	}
	return state, nil
}

func (c *Client) Shutdown(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "/v1/shutdown", struct{}{}, nil)
}

type statusWire struct {
	Ref        string `json:"ref"`
	OK         bool   `json:"ok"`
	NeedsLogin bool   `json:"needs_login"`
	Tools      int    `json:"tools"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding daemon request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://pandora"+path, reader)
	if err != nil {
		return fmt.Errorf("creating daemon request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("contacting the pandora daemon: %w", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the daemon response: %w", err)
	}
	if len(bytes.TrimSpace(data)) > 0 {
		var envelope struct {
			Error *errorBody `json:"error"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Error != nil {
			return daemonError(envelope.Error)
		}
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding the daemon response: %w", err)
	}
	return nil
}

func daemonError(body *errorBody) error {
	switch {
	case body.NeedsLogin:
		return &source.AuthRequiredError{Ref: body.Ref}
	case body.Blocked:
		return &core.BlockedError{Reason: body.Message}
	default:
		return errors.New(body.Message)
	}
}
