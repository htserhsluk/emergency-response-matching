package httpapp

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"net/http"
)

type Workflows interface {
	Workflow(context.Context, contracts.WorkflowCommand) (contracts.WorkflowState, error)
	PendingWork(context.Context) ([]contracts.Delivery, error)
}
type Matcher interface {
	HandleDelivery(context.Context, contracts.Delivery) (contracts.WorkflowState, error)
}
type CandidateChecker interface {
	CheckCandidate(context.Context, contracts.CandidateQuery) (contracts.Candidate, error)
}
type IntentConfirmer interface {
	IntentForReservation(context.Context, contracts.ReserveCommand) (contracts.Intent, error)
}

func serveWorkflow(w http.ResponseWriter, r *http.Request, s Requests, action string) {
	if r.Method != "POST" {
		write(w, 405, contracts.Error{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed"})
		return
	}
	var out any
	var err error
	switch action {
	case "reservation-intent":
		var c contracts.ReserveCommand
		if !decode(w, r, &c) {
			return
		}
		m, ok := s.(IntentConfirmer)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		out, err = m.IntentForReservation(r.Context(), c)
	case "candidate":
		var q contracts.CandidateQuery
		if !decode(w, r, &q) {
			return
		}
		m, ok := s.(CandidateChecker)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		out, err = m.CheckCandidate(r.Context(), q)
	case "handle":
		var d contracts.Delivery
		if !decode(w, r, &d) {
			return
		}
		m, ok := s.(Matcher)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		out, err = m.HandleDelivery(r.Context(), d)
	case "workflow":
		var c contracts.WorkflowCommand
		if !decode(w, r, &c) {
			return
		}
		m, ok := s.(Workflows)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		out, err = m.Workflow(r.Context(), c)
	case "pending":
		m, ok := s.(Workflows)
		if !ok {
			fail(w, contracts.ErrUnavailable, "")
			return
		}
		out, err = m.PendingWork(r.Context())
	}
	if err != nil {
		fail(w, err, "")
		return
	}
	write(w, 200, out)
}

func (c *Client) IntentForReservation(ctx context.Context, v contracts.ReserveCommand) (out contracts.Intent, err error) {
	err = c.exchange(ctx, "POST", "/commands/reservation-intent", "", v, &out)
	return
}

func (c *Client) CheckCandidate(ctx context.Context, q contracts.CandidateQuery) (out contracts.Candidate, err error) {
	err = c.exchange(ctx, "POST", "/commands/candidate", "", q, &out)
	return
}
func (c *Client) Workflow(ctx context.Context, v contracts.WorkflowCommand) (out contracts.WorkflowState, err error) {
	err = c.exchange(ctx, "POST", "/commands/workflow", "", v, &out)
	return
}
func (c *Client) HandleDelivery(ctx context.Context, v contracts.Delivery) (out contracts.WorkflowState, err error) {
	err = c.exchange(ctx, "POST", "/commands/handle", "", v, &out)
	return
}
func (c *Client) PendingWork(ctx context.Context) (out []contracts.Delivery, err error) {
	err = c.exchange(ctx, "POST", "/commands/pending", "", struct{}{}, &out)
	return
}
