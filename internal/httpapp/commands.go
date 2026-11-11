package httpapp

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"net/http"
)

type Commands interface {
	ScheduleMatch(context.Context, string, string) (contracts.Request, error)
	BeginIntent(context.Context, contracts.IntentCommand) (contracts.Intent, error)
	Reserve(context.Context, contracts.ReserveCommand) (contracts.ReservationResult, error)
	ClaimOutbox(context.Context, string) ([]contracts.Delivery, error)
	MarkSent(context.Context, string, string, string) error
}
type claimCommand struct {
	Token string `json:"token"`
}
type sentCommand struct {
	RequestID  string `json:"requestId"`
	DeliveryID string `json:"deliveryId"`
	Token      string `json:"token"`
}

func serveCommands(w http.ResponseWriter, r *http.Request, s Requests, parts []string, internal bool) {
	if internal && len(parts) == 2 && parts[0] == "commands" && (parts[1] == "workflow" || parts[1] == "handle" || parts[1] == "pending" || parts[1] == "candidate" || parts[1] == "reservation-intent") {
		serveWorkflow(w, r, s, parts[1])
		return
	}
	if r.Method != "POST" {
		write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
		return
	}
	if len(parts) != 2 {
		fail(w, contracts.ErrNotFound, "")
		return
	}
	c, ok := s.(Commands)
	if !ok {
		fail(w, contracts.ErrUnavailable, "")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var out any
	var err error
	if parts[0] == "match" {
		if _, err = routing.ShardFor(parts[1]); err != nil || contracts.ValidateKey(key) != nil {
			fail(w, contracts.ErrInvalid, key)
			return
		}
		out, err = c.ScheduleMatch(r.Context(), parts[1], key)
	} else if internal {
		switch parts[1] {
		case "intent":
			var v contracts.IntentCommand
			if !decode(w, r, &v) {
				return
			}
			if v.Validate() != nil {
				fail(w, contracts.ErrInvalid, key)
				return
			}
			out, err = c.BeginIntent(r.Context(), v)
		case "reserve":
			var v contracts.ReserveCommand
			if !decode(w, r, &v) {
				return
			}
			if v.Validate() != nil {
				fail(w, contracts.ErrInvalid, key)
				return
			}
			out, err = c.Reserve(r.Context(), v)
		case "claim":
			var v claimCommand
			if !decode(w, r, &v) {
				return
			}
			out, err = c.ClaimOutbox(r.Context(), v.Token)
		case "sent":
			var v sentCommand
			if !decode(w, r, &v) {
				return
			}
			err = c.MarkSent(r.Context(), v.RequestID, v.DeliveryID, v.Token)
			out = map[string]bool{"sent": err == nil}
		default:
			err = contracts.ErrNotFound
		}
	} else {
		err = contracts.ErrNotFound
	}
	if err != nil {
		fail(w, err, key)
		return
	}
	write(w, 200, out)
}
func (c *Client) ScheduleMatch(ctx context.Context, id, key string) (out contracts.Request, err error) {
	err = c.exchange(ctx, "POST", "/match/"+id, key, nil, &out)
	return
}
func (c *Client) BeginIntent(ctx context.Context, v contracts.IntentCommand) (out contracts.Intent, err error) {
	err = c.exchange(ctx, "POST", "/commands/intent", "", v, &out)
	return
}
func (c *Client) Reserve(ctx context.Context, v contracts.ReserveCommand) (out contracts.ReservationResult, err error) {
	err = c.exchange(ctx, "POST", "/commands/reserve", "", v, &out)
	return
}
func (c *Client) ClaimOutbox(ctx context.Context, token string) (out []contracts.Delivery, err error) {
	err = c.exchange(ctx, "POST", "/commands/claim", "", claimCommand{token}, &out)
	return
}
func (c *Client) MarkSent(ctx context.Context, request, id, token string) error {
	var out map[string]bool
	return c.exchange(ctx, "POST", "/commands/sent", "", sentCommand{request, id, token}, &out)
}
