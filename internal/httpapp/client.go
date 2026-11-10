package httpapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"io"
	"net/http"
	"time"
)

type Client struct {
	base, secret, prefix string
	http                 *http.Client
	version              uint64
}

func (c *Client) WithVersion(v uint64) *Client { clone := *c; clone.version = v; return &clone }

func NewClient(base, secret string) *Client {
	return &Client{base: base, secret: secret, prefix: "/internal", http: &http.Client{Timeout: 5 * time.Second}}
}
func NewPublicClient(base string) *Client { c := NewClient(base, ""); c.prefix = ""; return c }
func (c *Client) call(ctx context.Context, method, path, key string, body any) (contracts.Request, error) {
	var result contracts.Request
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return result, contracts.ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+c.prefix+path, &buf)
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", c.secret)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Route-Version", fmt.Sprint(c.version))
	resp, err := c.http.Do(req)
	if err != nil {
		return result, fmt.Errorf("%w: request failed", contracts.ErrUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 400:
			return result, contracts.ErrInvalid
		case 404:
			return result, contracts.ErrNotFound
		case 409:
			return result, contracts.ErrConflict
		default:
			return result, contracts.ErrUnavailable
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return result, contracts.ErrUnavailable
	}
	return result, nil
}
func (c *Client) Create(ctx context.Context, key string, v contracts.CreateRequest) (contracts.Request, error) {
	return c.call(ctx, "POST", "/requests", key, v)
}
func (c *Client) Get(ctx context.Context, id string) (contracts.Request, error) {
	return c.call(ctx, "GET", "/requests/"+id, "", nil)
}
func (c *Client) SetStatus(ctx context.Context, id, key, status string) (contracts.Request, error) {
	return c.call(ctx, "PUT", "/requests/"+id+"/status", key, contracts.StatusCommand{Status: status})
}
func (c *Client) Health(ctx context.Context) error {
	r, err := http.NewRequestWithContext(ctx, "GET", c.base+"/health/ready", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return contracts.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return contracts.ErrUnavailable
	}
	return nil
}
