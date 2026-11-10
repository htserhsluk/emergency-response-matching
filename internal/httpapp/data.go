package httpapp

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"net/http"
	"strings"
)

type DataStore interface {
	DeleteData(context.Context, string, string) error
}
type dataCommand struct {
	Operation string                      `json:"operation"`
	Request   *contracts.CreateRequest    `json:"request,omitempty"`
	Resource  *contracts.RegisterResource `json:"resource,omitempty"`
	Status    string                      `json:"status,omitempty"`
}

func serveData(w http.ResponseWriter, r *http.Request, s Requests, parts []string) {
	if len(parts) != 2 {
		fail(w, contracts.ErrInvalid, "")
		return
	}
	kind, id, ok := strings.Cut(parts[1], ":")
	if !ok || (kind != "request" && kind != "resource") {
		fail(w, contracts.ErrInvalid, "")
		return
	}
	if _, err := routing.ShardFor(id); err != nil {
		fail(w, contracts.ErrInvalid, "")
		return
	}
	resource, ok := s.(Resources)
	if s == nil || !ok {
		fail(w, contracts.ErrUnavailable, "")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var result any
	var err error
	switch r.Method {
	case "GET":
		if kind == "request" {
			result, err = s.Get(r.Context(), id)
		} else {
			result, err = resource.GetResource(r.Context(), id)
		}
	case "DELETE":
		d, ok := s.(DataStore)
		if !ok {
			fail(w, contracts.ErrUnavailable, key)
			return
		}
		err = d.DeleteData(r.Context(), kind, id)
		result = map[string]bool{"deleted": true}
	case "PUT":
		var c dataCommand
		if !decode(w, r, &c) {
			return
		}
		if contracts.ValidateKey(key) != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		switch {
		case kind == "request" && c.Operation == "create-request" && c.Request != nil && c.Resource == nil && c.Status == "" && id == contracts.EntityID(c.Operation, key):
			result, err = s.Create(r.Context(), key, *c.Request)
		case kind == "resource" && c.Operation == "register-resource" && c.Resource != nil && c.Request == nil && c.Status == "" && id == contracts.EntityID(c.Operation, key):
			result, err = resource.RegisterResource(r.Context(), key, *c.Resource)
		case c.Operation == "status" && c.Status != "" && c.Request == nil && c.Resource == nil:
			if kind == "request" {
				result, err = s.SetStatus(r.Context(), id, key, c.Status)
			} else {
				result, err = resource.SetResourceStatus(r.Context(), id, key, c.Status)
			}
		default:
			err = contracts.ErrInvalid
		}
	default:
		write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
		return
	}
	if err != nil {
		fail(w, err, key)
		return
	}
	write(w, 200, result)
}
func (c *Client) DeleteData(ctx context.Context, kind, id string) error {
	var out map[string]bool
	return c.exchange(ctx, "DELETE", "/data/"+kind+":"+id, "", nil, &out)
}
