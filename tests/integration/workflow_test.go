//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func workflowFixture(t *testing.T) (*storage.Store, *pgxpool.Pool, contracts.Request, contracts.Delivery) {
	t.Helper()
	s, p := resourceFixture(t)
	ctx := context.Background()
	r, err := s.Create(ctx, "workflow-request-000001", body())
	if err != nil {
		t.Fatal(err)
	}
	var d contracts.Delivery
	var data []byte
	if err = p.QueryRow(ctx, `SELECT payload,delivery_id,retry_number,destination FROM outbox_events WHERE request_id=$1`, r.ID).Scan(&data, &d.DeliveryID, &d.RetryNumber, &d.Destination); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &d.Event); err != nil {
		t.Fatal(err)
	}
	return s, p, r, d
}

func TestWorkflowFinalizationAndCompletionCannotReleaseResource(t *testing.T) {
	s, _, r, d := workflowFixture(t)
	ctx := context.Background()
	state, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"})
	if err != nil || state.Ack || state.State != "RUNNING" {
		t.Fatalf("inspect %+v %v", state, err)
	}
	resource, err := s.RegisterResource(ctx, "workflow-resource-000001", resourceBody(0, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	intent := contracts.IntentCommand{AssignmentID: "11111111111111111111111111111111", RequestID: r.ID, ResourceID: resource.ID, Generation: 1, DistanceMeters: 123}
	if _, err = s.BeginIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	state, err = s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"})
	if err != nil || state.Intent == nil || state.Intent.AssignmentID != intent.AssignmentID {
		t.Fatalf("resume %+v %v", state, err)
	}
	result, err := s.Reserve(ctx, contracts.ReserveCommand{AssignmentID: intent.AssignmentID, RequestID: r.ID, ResourceID: resource.ID})
	if err != nil || result.Status != "SUCCEEDED" {
		t.Fatal(result, err)
	}
	final := contracts.WorkflowCommand{Delivery: d, Action: "finalize", AssignmentID: intent.AssignmentID}
	state, err = s.Workflow(ctx, final)
	if err != nil || !state.Ack || state.Request.Status != "ASSIGNED" || state.Request.AssignedResourceID != resource.ID {
		t.Fatalf("final %+v %v", state, err)
	}
	again, err := s.Workflow(ctx, final)
	if err != nil || !again.Ack || again.Request.AssignedResourceID != resource.ID {
		t.Fatal(again, err)
	}
	if _, err = s.SetStatus(ctx, r.ID, "workflow-complete-000001", "COMPLETED"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetResource(ctx, resource.ID)
	if err != nil || got.Availability || got.Status != "ALLOCATED" {
		t.Fatalf("completion released resource %+v %v", got, err)
	}
}

func TestWorkflowRetryBudgetPauseAndRedrive(t *testing.T) {
	s, p, _, d := workflowFixture(t)
	ctx := context.Background()
	if _, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 4; attempt++ {
		d.RetryNumber = attempt
		d.DeliveryID = contracts.DeliveryID(d.Event.EventID, attempt, "request.match")
		if attempt > 0 {
			if _, err := p.Exec(ctx, `UPDATE processed_events SET due_at=clock_timestamp()-interval '1 second'`); err != nil {
				t.Fatal(err)
			}
		}
		state, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "retry", Reason: "DEPENDENCY_UNAVAILABLE"})
		if err != nil || !state.Ack {
			t.Fatalf("retry %+v %v", state, err)
		}
		replay, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "retry", Reason: "DEPENDENCY_UNAVAILABLE"})
		if err != nil || !replay.Ack || replay.ExpectedRetry != state.ExpectedRetry {
			t.Fatalf("retry spent twice %+v %v", replay, err)
		}
		if attempt < 3 {
			var remaining float64
			if err = p.QueryRow(ctx, `SELECT extract(epoch from due_at-clock_timestamp()) FROM processed_events WHERE event_id=$1`, d.Event.EventID).Scan(&remaining); err != nil {
				t.Fatal(err)
			}
			if state.ExpectedRetry != attempt+1 || state.State != "RUNNING" || remaining <= 0 {
				t.Fatalf("missing delayed retry %+v", state)
			}
		} else if state.State != "PAUSED" {
			t.Fatalf("unbounded retries %+v", state)
		}
	}
	work, err := s.PendingWork(ctx)
	if err != nil || len(work) != 0 {
		t.Fatalf("paused work reconciled %+v %v", work, err)
	}
	var dead int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE destination='request.dead'`).Scan(&dead); err != nil || dead != 1 {
		t.Fatalf("dead=%d %v", dead, err)
	}
	state, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "redrive"})
	if err != nil || state.State != "RUNNING" || state.ExpectedRetry != 4 {
		t.Fatalf("redrive %+v %v", state, err)
	}
	replay, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "redrive"})
	if err != nil || replay.ExpectedRetry != 4 {
		t.Fatalf("redrive duplicate %+v %v", replay, err)
	}
}

func TestStaleGenerationNoOpAndUnscheduledFutureRejected(t *testing.T) {
	s, p, r, d := workflowFixture(t)
	ctx := context.Background()
	future := d
	future.RetryNumber = 1
	future.DeliveryID = contracts.DeliveryID(d.Event.EventID, 1, "request.match")
	if _, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: future, Action: "inspect"}); err == nil {
		t.Fatal("unscheduled future attempt accepted")
	}
	if _, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "unmatched"}); err != nil {
		t.Fatal(err)
	}
	next, err := s.ScheduleMatch(ctx, r.ID, "new-generation-000001")
	if err != nil || next.Generation != 2 {
		t.Fatal(next, err)
	}
	state, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "retry"})
	if err != nil || !state.Ack || state.Request.Generation != 2 || state.Request.Status != "PENDING" {
		t.Fatalf("stale delivery regressed new generation %+v %v", state, err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("stale event scheduled retry %d %v", count, err)
	}
}
