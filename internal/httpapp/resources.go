package httpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type Resources interface {
	RegisterResource(context.Context, string, contracts.RegisterResource) (contracts.Resource, error)
	GetResource(context.Context, string) (contracts.Resource, error)
	SetResourceStatus(context.Context, string, string, string) (contracts.Resource, error)
	ListResources(context.Context, contracts.ListQuery) (contracts.ResourcePage, error)
	Search(context.Context, contracts.SearchQuery) ([]contracts.Candidate, error)
}

func serveResources(w http.ResponseWriter, r *http.Request, store Requests, parts []string, internal bool) {
	s, ok := store.(Resources)
	if !ok {
		fail(w, contracts.ErrUnavailable, "")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var result any
	var err error
	status := 200
	if len(parts) == 1 && parts[0] == "search" && internal && r.Method == "POST" {
		var q contracts.SearchQuery
		if !decode(w, r, &q) {
			return
		}
		if q.Validate() != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		result, err = s.Search(r.Context(), q)
	} else if len(parts) == 1 && parts[0] == "resources" && r.Method == "GET" {
		q := contracts.ListQuery{Limit: 20, After: r.URL.Query().Get("after")}
		if v := r.URL.Query().Get("limit"); v != "" {
			q.Limit, err = strconv.Atoi(v)
		}
		if err != nil || q.Validate() != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		result, err = s.ListResources(r.Context(), q)
	} else if len(parts) == 2 && internal && r.Method == "GET" {
		if _, err := routing.ShardFor(parts[1]); err != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		result, err = s.GetResource(r.Context(), parts[1])
	} else if len(parts) == 1 && parts[0] == "resources" && r.Method == "POST" {
		var c contracts.RegisterResource
		if !decode(w, r, &c) {
			return
		}
		if c.Validate() != nil || contracts.ValidateKey(key) != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		result, err = s.RegisterResource(r.Context(), key, c)
		status = 201
	} else if len(parts) == 3 && parts[2] == "status" && r.Method == "PUT" {
		var c contracts.StatusCommand
		if !decode(w, r, &c) {
			return
		}
		if _, err := routing.ShardFor(parts[1]); err != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		if contracts.ValidateKey(key) != nil || c.Status == "" {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		result, err = s.SetResourceStatus(r.Context(), parts[1], key, c.Status)
	} else {
		write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
		return
	}
	if err != nil {
		fail(w, err, key)
		return
	}
	write(w, status, result)
}
func (c *Client) exchange(ctx context.Context, method, path, key string, body, out any) error {
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			return contracts.ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+c.prefix+path, &b)
	if err != nil {
		return contracts.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", c.secret)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Route-Version", strconv.FormatUint(c.version, 10))
	resp, err := c.http.Do(req)
	if err != nil {
		return contracts.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 400:
			return contracts.ErrInvalid
		case 404:
			return contracts.ErrNotFound
		case 409:
			return contracts.ErrConflict
		default:
			return contracts.ErrUnavailable
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return contracts.ErrUnavailable
	}
	return nil
}
func (c *Client) RegisterResource(ctx context.Context, key string, v contracts.RegisterResource) (out contracts.Resource, err error) {
	err = c.exchange(ctx, "POST", "/resources", key, v, &out)
	return
}
func (c *Client) GetResource(ctx context.Context, id string) (out contracts.Resource, err error) {
	err = c.exchange(ctx, "GET", "/resources/"+id, "", nil, &out)
	return
}
func (c *Client) SetResourceStatus(ctx context.Context, id, key, status string) (out contracts.Resource, err error) {
	err = c.exchange(ctx, "PUT", "/resources/"+id+"/status", key, contracts.StatusCommand{Status: status}, &out)
	return
}
func (c *Client) ListResources(ctx context.Context, q contracts.ListQuery) (out contracts.ResourcePage, err error) {
	err = c.exchange(ctx, "GET", "/resources?limit="+strconv.Itoa(q.Limit)+"&after="+url.QueryEscape(q.After), "", nil, &out)
	return
}
func (c *Client) Search(ctx context.Context, q contracts.SearchQuery) (out []contracts.Candidate, err error) {
	err = c.exchange(ctx, "POST", "/search", "", q, &out)
	return
}
