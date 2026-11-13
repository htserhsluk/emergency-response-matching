package httpapp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
)

type Requests interface {
	Create(context.Context, string, contracts.CreateRequest) (contracts.Request, error)
	Get(context.Context, string) (contracts.Request, error)
	SetStatus(context.Context, string, string, string) (contracts.Request, error)
	Health(context.Context) error
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error, key string) {
	status, code := 503, "UNAVAILABLE"
	switch {
	case errors.Is(err, contracts.ErrInvalid):
		status, code = 400, "INVALID"
	case errors.Is(err, contracts.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(err, contracts.ErrConflict):
		status, code = 409, "CONFLICT"
	}
	messages := map[int]string{400: "Invalid request", 404: "Request not found", 409: "Operation key or status conflict", 503: contracts.ErrUnavailable.Error()}
	write(w, status, contracts.Error{Code: code, Message: messages[status], OperationID: key})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(v)
	if err == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			err = contracts.ErrInvalid
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			write(w, 413, contracts.Error{Code: "TOO_LARGE", Message: "Body exceeds 64 KiB"})
		} else {
			fail(w, contracts.ErrInvalid, "")
		}
		return false
	}
	return true
}

func NewServer(store Requests, secret string, internal bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/live" {
			write(w, 200, map[string]string{"status": "live"})
			return
		}
		if r.URL.Path == "/health/ready" {
			if store == nil || store.Health(r.Context()) != nil {
				fail(w, contracts.ErrUnavailable, "")
				return
			}
			write(w, 200, map[string]string{"status": "ready"})
			return
		}
		path := r.URL.Path
		if internal {
			if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Secret")), []byte(secret)) != 1 {
				write(w, 401, contracts.Error{Code: "UNAUTHORIZED", Message: "Internal credential required"})
				return
			}
			if !strings.HasPrefix(path, "/internal/") {
				fail(w, contracts.ErrNotFound, "")
				return
			}
			path = strings.TrimPrefix(path, "/internal")
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "requests" && parts[2] == "notifications" {
			serveNotifications(w, r, store, parts)
			return
		}
		if len(parts) > 0 && (parts[0] == "match" || (internal && parts[0] == "commands")) {
			serveCommands(w, r, store, parts, internal)
			return
		}
		if internal && len(parts) > 0 && parts[0] == "data" {
			serveData(w, r, store, parts)
			return
		}
		if len(parts) > 0 && (parts[0] == "resources" || (internal && parts[0] == "search")) {
			serveResources(w, r, store, parts, internal)
			return
		}
		if len(parts) == 0 || parts[0] != "requests" || len(parts) > 3 || (len(parts) == 3 && parts[2] != "status") {
			fail(w, contracts.ErrNotFound, "")
			return
		}
		expected := "POST"
		if len(parts) == 2 {
			expected = "GET"
		}
		if len(parts) == 3 {
			expected = "PUT"
		}
		if r.Method != expected {
			w.Header().Set("Allow", expected)
			write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if r.Method != "GET" && contracts.ValidateKey(key) != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		if len(parts) > 1 {
			if _, err := routing.ShardFor(parts[1]); err != nil {
				fail(w, contracts.ErrInvalid, key)
				return
			}
		}
		var result contracts.Request
		var err error
		switch {
		case len(parts) == 1:
			var c contracts.CreateRequest
			if !decode(w, r, &c) {
				return
			}
			if err = c.Validate(); err != nil {
				fail(w, err, key)
				return
			}
			if store == nil {
				fail(w, contracts.ErrUnavailable, key)
				return
			}
			result, err = store.Create(r.Context(), key, c)
		case len(parts) == 2:
			if store == nil {
				fail(w, contracts.ErrUnavailable, key)
				return
			}
			result, err = store.Get(r.Context(), parts[1])
		case len(parts) == 3:
			var c contracts.StatusCommand
			if !decode(w, r, &c) {
				return
			}
			if c.Status == "" {
				fail(w, contracts.ErrInvalid, key)
				return
			}
			if store == nil {
				fail(w, contracts.ErrUnavailable, key)
				return
			}
			result, err = store.SetStatus(r.Context(), parts[1], key, c.Status)
		}
		if err != nil {
			fail(w, err, key)
			return
		}
		status := 200
		if len(parts) == 1 {
			status = 201
		}
		write(w, status, result)
	})
}
