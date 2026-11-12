package httpapp

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"net/http"
	"net/url"
	"strconv"
)

type Notifications interface {
	ListNotifications(context.Context, string, contracts.ListQuery) (contracts.NotificationPage, error)
	ReadNotification(context.Context, string, string, string) (contracts.Notification, error)
}

func serveNotifications(w http.ResponseWriter, r *http.Request, store Requests, parts []string) {
	s, ok := store.(Notifications)
	if !ok {
		fail(w, contracts.ErrUnavailable, "")
		return
	}
	if _, err := routing.ShardFor(parts[1]); err != nil {
		fail(w, contracts.ErrInvalid, "")
		return
	}
	var out any
	var err error
	key := r.Header.Get("Idempotency-Key")
	if len(parts) == 3 && r.Method == "GET" {
		q := contracts.ListQuery{Limit: 20, After: r.URL.Query().Get("after")}
		if v := r.URL.Query().Get("limit"); v != "" {
			q.Limit, err = strconv.Atoi(v)
		}
		if err != nil || q.Validate() != nil {
			fail(w, contracts.ErrInvalid, "")
			return
		}
		out, err = s.ListNotifications(r.Context(), parts[1], q)
	} else if len(parts) == 5 && parts[4] == "read" && r.Method == "PUT" {
		if _, err = routing.ShardFor(parts[3]); err != nil || contracts.ValidateKey(key) != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		var body struct{}
		if !decode(w, r, &body) {
			return
		}
		out, err = s.ReadNotification(r.Context(), parts[1], parts[3], key)
	} else {
		write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
		return
	}
	if err != nil {
		fail(w, err, key)
		return
	}
	write(w, 200, out)
}
func (c *Client) ListNotifications(ctx context.Context, id string, q contracts.ListQuery) (out contracts.NotificationPage, err error) {
	err = c.exchange(ctx, "GET", "/requests/"+id+"/notifications?limit="+strconv.Itoa(q.Limit)+"&after="+url.QueryEscape(q.After), "", nil, &out)
	return
}
func (c *Client) ReadNotification(ctx context.Context, requestID, id, key string) (out contracts.Notification, err error) {
	err = c.exchange(ctx, "PUT", "/requests/"+requestID+"/notifications/"+id+"/read", key, struct{}{}, &out)
	return
}
