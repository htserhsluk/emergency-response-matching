package coordinator

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"log/slog"
	"math"
	"time"
)

func (r *Router) lockRequest(ctx context.Context, id string) (func(), error) {
	if _, err := routing.ShardFor(id); err != nil {
		return nil, contracts.ErrInvalid
	}
	r.lockInit.Do(func() {
		for i := range r.locks {
			r.locks[i] = make(chan struct{}, 1)
		}
	})
	lock := r.locks[sha256.Sum256([]byte(id))[0]]
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, contracts.ErrUnavailable
	}
}
func (r *Router) Workflow(ctx context.Context, c contracts.WorkflowCommand) (contracts.WorkflowState, error) {
	// Finalization/rejection are private orchestration steps; a caller cannot
	// assert a reservation outcome or manufacture an exhaustive-search result.
	if c.Action != "inspect" && c.Action != "pause" && c.Action != "redrive" {
		return contracts.WorkflowState{}, contracts.ErrInvalid
	}
	unlock, err := r.lockRequest(ctx, c.Delivery.Event.RequestID)
	if err != nil {
		return contracts.WorkflowState{}, err
	}
	defer unlock()
	n, err := r.node(c.Delivery.Event.RequestID)
	if err != nil {
		return contracts.WorkflowState{}, err
	}
	return n.Workflow(ctx, c)
}
func (r *Router) PendingWork(ctx context.Context) ([]contracts.Delivery, error) {
	return fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Delivery, error) { return n.PendingWork(ctx) })
}

var errSearchYield = errors.New("search processing budget reached")

func (r *Router) HandleDelivery(parent context.Context, d contracts.Delivery) (contracts.WorkflowState, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if (contracts.WorkflowCommand{Delivery: d, Action: "inspect"}).Validate() != nil {
		return contracts.WorkflowState{}, contracts.ErrInvalid
	}
	unlock, err := r.lockRequest(ctx, d.Event.RequestID)
	if err != nil {
		return contracts.WorkflowState{}, err
	}
	defer unlock()
	n, err := r.node(d.Event.RequestID)
	if err != nil {
		return contracts.WorkflowState{}, err
	}
	state, err := n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"})
	if err != nil || state.Ack {
		return state, err
	}
	// Leave time in the parent operation to durably record retry/backoff after a
	// slow dependency. Reusing an exhausted assignment context would starve scans.
	step, cancelStep := context.WithTimeout(ctx, 2*time.Second)
	result, err := r.assign(step, n, d, state)
	cancelStep()
	if err == nil {
		return result, nil
	}
	action := "retry"
	if errors.Is(err, errSearchYield) {
		action = "yield"
	}
	if errors.Is(err, contracts.ErrInvalid) {
		action = "pause"
	}
	// If the request shard or deadline is unavailable, no ack is authorized.
	return n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: action, Reason: "ASSIGNMENT_STEP_UNCONFIRMED"})
}
func (r *Router) assign(ctx context.Context, n *httpapp.Client, d contracts.Delivery, state contracts.WorkflowState) (contracts.WorkflowState, error) {
	reserve := func(intent contracts.Intent) (contracts.WorkflowState, error) {
		if err := r.validateCandidate(ctx, state.Request, intent.IntentCommand); err != nil {
			return contracts.WorkflowState{}, err
		}
		result, err := r.reserveResource(ctx, contracts.ReserveCommand{AssignmentID: intent.AssignmentID, RequestID: intent.RequestID, ResourceID: intent.ResourceID}, intent.DistanceMeters > contracts.MaxMatchDistanceMeters)
		if err != nil {
			return contracts.WorkflowState{}, err
		}
		action := "reject"
		if result.Status == "SUCCEEDED" {
			action = "finalize"
		} else if result.Status != "REJECTED" {
			return contracts.WorkflowState{}, contracts.ErrUnavailable
		}
		return n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: action, AssignmentID: intent.AssignmentID})
	}
	if state.Intent != nil {
		var err error
		state, err = reserve(*state.Intent)
		if err != nil || state.Ack {
			return state, err
		}
	}
	steps := 0
	for state.Progress.RadiusIndex < len(contracts.SearchRadii) {
		if steps >= 16 {
			return contracts.WorkflowState{}, errSearchYield
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 300*time.Millisecond {
			return contracts.WorkflowState{}, errSearchYield
		}
		steps++
		progress := state.Progress
		query := contracts.SearchQuery{Type: state.Request.Type, Location: state.Request.Location, RadiusMeters: contracts.SearchRadii[progress.RadiusIndex], Limit: 20, AfterID: progress.AfterID, AfterDistance: progress.AfterDistance}
		candidates, err := r.Search(ctx, query)
		if err != nil {
			return contracts.WorkflowState{}, err
		}
		for _, candidate := range candidates {
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 400*time.Millisecond {
				return contracts.WorkflowState{}, errSearchYield
			}
			command := contracts.IntentCommand{AssignmentID: contracts.EntityID("assignment", fmt.Sprintf("%s:%s", d.Event.EventID, candidate.ResourceID)), RequestID: d.Event.RequestID, ResourceID: candidate.ResourceID, Generation: d.Event.Generation, DistanceMeters: candidate.DistanceMeters}
			intent, err := n.BeginIntent(ctx, command)
			if err != nil {
				return contracts.WorkflowState{}, err
			}
			if intent.Phase != "REJECTED" {
				state, err = reserve(intent)
				if err != nil || state.Ack {
					return state, err
				}
			}
			progress.AfterID = candidate.ResourceID
			progress.AfterDistance = candidate.DistanceMeters
			state, err = n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "advance", Progress: &progress})
			if err != nil || state.Ack {
				return state, err
			}
		}
		if len(candidates) < query.Limit {
			progress = contracts.SearchProgress{RadiusIndex: progress.RadiusIndex + 1}
		} else {
			continue
		}
		state, err = n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "advance", Progress: &progress})
		if err != nil || state.Ack {
			return state, err
		}
	}
	return n.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "unmatched"})
}

func (r *Router) validateCandidate(ctx context.Context, request contracts.Request, intent contracts.IntentCommand) error {
	if intent.Validate() != nil || intent.RequestID != request.ID || intent.Generation != request.Generation {
		return contracts.ErrInvalid
	}
	n, err := r.node(intent.ResourceID)
	if err != nil {
		return err
	}
	candidate, err := n.CheckCandidate(ctx, contracts.CandidateQuery{ResourceID: intent.ResourceID, Type: request.Type, Location: request.Location})
	if err != nil {
		return err
	}
	if candidate.ResourceID != intent.ResourceID || (candidate.DistanceMeters > contracts.MaxMatchDistanceMeters) != (intent.DistanceMeters > contracts.MaxMatchDistanceMeters) || math.Abs(candidate.DistanceMeters-intent.DistanceMeters) > 0.000001 {
		return contracts.ErrInvalid
	}
	return nil
}
func (r *Router) ReconcileOnce(ctx context.Context) error {
	work, err := r.PendingWork(ctx)
	if err != nil {
		return err
	}
	var first error
	for _, d := range work {
		if _, err = r.HandleDelivery(ctx, d); err != nil && first == nil {
			first = err
		}
	}
	checks, err := fanout(ctx, r, func(ctx context.Context, n *httpapp.Client) ([]contracts.Request, error) {
		return n.UnmatchedChecks(ctx)
	})
	if err != nil {
		return err
	}
	probed := make(map[string]bool)
	for _, request := range checks {
		candidates, err := r.Search(ctx, contracts.SearchQuery{Type: request.Type, Location: request.Location, RadiusMeters: contracts.MaxMatchDistanceMeters, Limit: 1})
		if err == nil && len(candidates) > 0 {
			resourceID := candidates[0].ResourceID
			if !probed[resourceID] {
				probed[resourceID] = true
				err = r.wakeNearest(ctx, resourceID)
			}
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}

// The rotation discovers available supply. Choose the closest eligible request
// across all shards, including requests beyond this pass's rotation batch.
func (r *Router) wakeNearest(ctx context.Context, resourceID string) error {
	preview, err := r.RequestCandidates(ctx, resourceID)
	if err != nil || len(preview.Items) == 0 {
		return err
	}
	n, err := r.node(preview.Items[0].ID)
	if err != nil {
		return err
	}
	request, err := n.Get(ctx, preview.Items[0].ID)
	if err != nil || request.Status != "UNMATCHED" {
		return err
	}
	_, err = n.WakeMatch(ctx, request.ID, request.Generation)
	return err
}
func (r *Router) RunReconciler(ctx context.Context) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			step, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := r.ReconcileOnce(step); err != nil && ctx.Err() == nil {
				slog.Warn("reconciliation pass incomplete", "reason", "dependency_or_workflow_unconfirmed")
			}
			cancel()
		}
	}
}
