package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/events"
)

const maxBodyBytes = 1 << 20

type Verification struct {
	Style           string
	SignatureHeader string
	SignaturePrefix string
	Encoding        string
	TokenHeader     string
	TokenQuery      string
	Challenge       string
}

type Hook struct {
	Service     string
	Account     string
	ID          string
	Verify      Verification
	TopicHeader string
	TopicFields []string
	IDHeader    string
}

type Secrets struct {
	Signing     string
	VerifyToken string
}

type Lookup func(ctx context.Context, service, account, id string) (Hook, Secrets, bool, error)

type handler struct {
	lookup Lookup
	queue  *events.Queue
}

func NewHandler(lookup Lookup, queue *events.Queue) http.Handler {
	h := &handler{lookup: lookup, queue: queue}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hooks/{service}/{account}/{id}", h.serveGET)
	mux.HandleFunc("POST /hooks/{service}/{account}/{id}", h.servePOST)
	mux.HandleFunc("GET /pandora/hooks/{service}/{account}/{id}", h.serveGET)
	mux.HandleFunc("POST /pandora/hooks/{service}/{account}/{id}", h.servePOST)
	return mux
}

func (h *handler) serveGET(w http.ResponseWriter, r *http.Request) {
	hook, secrets, ok, err := h.resolve(r.Context(), r)
	if err != nil {
		h.lookupError(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if hook.Verify.Challenge != "hub" {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	if query.Get("hub.mode") != "subscribe" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if secrets.VerifyToken == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if subtleCompare(query.Get("hub.verify_token"), secrets.VerifyToken) != 1 {
		slog.Warn("webhook verification refused: the verify token does not match", "service", hook.Service, "account", hook.Account)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	slog.Info("webhook verified", "service", hook.Service, "account", hook.Account)
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(query.Get("hub.challenge")))
}

func (h *handler) servePOST(w http.ResponseWriter, r *http.Request) {
	hook, secrets, ok, err := h.resolve(r.Context(), r)
	if err != nil {
		h.lookupError(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if !verifyRequest(hook.Verify, secrets, r, body) {
		slog.Warn("webhook delivery refused: the signature or token does not match", "service", hook.Service, "account", hook.Account)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ev := events.Event{
		Service: hook.Service,
		Account: hook.Account,
		Topic:   eventTopic(hook, r.Header, body),
		Headers: eventHeaders(hook, r),
		Payload: eventPayload(body),
	}
	stored, duplicate, err := h.queue.Append(r.Context(), ev, eventDedupKey(hook, r, body))
	if err != nil {
		slog.Error("storing webhook event", "service", hook.Service, "account", hook.Account, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if duplicate {
		slog.Info("webhook event already received", "service", hook.Service, "account", hook.Account)
	} else {
		slog.Info("webhook event received", "service", hook.Service, "account", hook.Account, "topic", stored.Topic, "seq", stored.Seq)
	}
	_, _ = w.Write([]byte("ok"))
}

func (h *handler) resolve(ctx context.Context, r *http.Request) (Hook, Secrets, bool, error) {
	if h.lookup == nil {
		return Hook{}, Secrets{}, false, errors.New("webhook lookup is not configured")
	}
	return h.lookup(ctx, r.PathValue("service"), r.PathValue("account"), r.PathValue("id"))
}

func (h *handler) lookupError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("resolving webhook", "path", r.URL.Path, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return nil, false
		}
		http.Error(w, "could not read request body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

func eventHeaders(hook Hook, r *http.Request) map[string]string {
	verify := withDefaults(hook.Verify)
	hidden := map[string]bool{}
	if verify.SignatureHeader != "" {
		hidden[strings.ToLower(verify.SignatureHeader)] = true
	}
	if verify.TokenHeader != "" {
		hidden[strings.ToLower(verify.TokenHeader)] = true
	}

	headers := map[string]string{}
	add := func(name string) {
		if name == "" || hidden[strings.ToLower(name)] {
			return
		}
		if value := r.Header.Get(name); value != "" {
			headers[name] = value
		}
	}
	add("Content-Type")
	add("User-Agent")
	add(hook.TopicHeader)
	add(hook.IDHeader)
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func eventDedupKey(hook Hook, r *http.Request, body []byte) string {
	prefix := hook.Service + "/" + hook.Account + ":"
	if hook.IDHeader != "" {
		if value := r.Header.Get(hook.IDHeader); value != "" {
			return prefix + value
		}
	}
	sum := sha256.Sum256(body)
	return prefix + "sha256:" + hex.EncodeToString(sum[:])
}

func eventPayload(body []byte) json.RawMessage {
	if json.Valid(body) {
		return json.RawMessage(append([]byte(nil), body...))
	}
	data, err := json.Marshal(string(body))
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(data)
}
