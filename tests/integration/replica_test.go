//go:build integration && replica

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
)

func TestOutboxClaimRequiresReplayBarrier(t *testing.T) {
	store, pool := resourceFixture(t)
	ctx := context.Background()
	request, err := store.Create(ctx, "outbox-barrier-request-0001", body())
	if err != nil {
		t.Fatal(err)
	}
	standby, err := storage.Open(ctx, os.Getenv("ERM_TEST_STANDBY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer standby.Close()
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_pause()"); err != nil {
		t.Fatal(err)
	}
	defer standby.Exec(context.Background(), "SELECT pg_wal_replay_resume()")
	attempt := func(token string) {
		t.Helper()
		limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		rows, err := store.ClaimOutbox(limited, token)
		if err == nil || len(rows) != 0 {
			t.Fatalf("unconfirmed claim escaped: %+v %v", rows, err)
		}
	}
	attempt("paused-claim-token-00001")
	var visible int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE claim_token IS NOT NULL").Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("ambiguous claim not locally visible %d %v", visible, err)
	}
	// Advance only the lease fixture without claiming synchronous durability.
	if _, err = pool.Exec(ctx, "BEGIN; SET LOCAL synchronous_commit=local; UPDATE outbox_events SET claim_until=clock_timestamp()-interval '1 second'; COMMIT"); err != nil {
		t.Fatal(err)
	}
	attempt("paused-claim-token-00002")
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_resume()"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE outbox_events SET claim_until=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ClaimOutbox(ctx, "confirmed-claim-token-0003")
	if err != nil || len(rows) != 1 || rows[0].Event.RequestID != request.ID {
		t.Fatalf("confirmed claim %+v %v", rows, err)
	}
}

func TestRetryAckRequiresReplayBarrier(t *testing.T) {
	s, _, _, d := workflowFixture(t)
	ctx := context.Background()
	if _, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"}); err != nil {
		t.Fatal(err)
	}
	standby, err := storage.Open(ctx, os.Getenv("ERM_TEST_STANDBY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer standby.Close()
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_pause()"); err != nil {
		t.Fatal(err)
	}
	defer standby.Exec(context.Background(), "SELECT pg_wal_replay_resume()")
	for range 2 {
		limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		state, err := s.Workflow(limited, contracts.WorkflowCommand{Delivery: d, Action: "retry", Reason: "fixture"})
		cancel()
		if err == nil || state.Ack {
			t.Fatalf("unconfirmed retry acknowledged %+v %v", state, err)
		}
	}
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_resume()"); err != nil {
		t.Fatal(err)
	}
	state, err := s.Workflow(ctx, contracts.WorkflowCommand{Delivery: d, Action: "inspect"})
	if err != nil || !state.Ack || state.ExpectedRetry != 1 {
		t.Fatal(state, err)
	}
}

func TestReplayBarrierBeforeCrossShardAdvance(t *testing.T) {
	l := newWorkflowLab(t)
	resource := l.resource(t, 2, "barrier-supply")
	request, d := l.request(t, 0, "barrier-request")
	if _, err := l.stores[0].BeginIntent(context.Background(), contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 0xabc99), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	standby, err := storage.Open(context.Background(), os.Getenv("ERM_TEST_STANDBY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer standby.Close()
	if _, err = standby.Exec(context.Background(), "SELECT pg_wal_replay_pause()"); err != nil {
		t.Fatal(err)
	}
	defer standby.Exec(context.Background(), "SELECT pg_wal_replay_resume()")
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	state, err := l.router.HandleDelivery(ctx, d)
	cancel()
	if err == nil || state.Ack {
		t.Fatalf("unconfirmed workflow advanced %+v %v", state, err)
	}
	var count int
	if err = l.pools[2].QueryRow(context.Background(), `SELECT count(*) FROM reservation_results`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reserved before intent barrier %d %v", count, err)
	}
	if _, err = standby.Exec(context.Background(), "SELECT pg_wal_replay_resume()"); err != nil {
		t.Fatal(err)
	}
	state, err = l.router.HandleDelivery(context.Background(), d)
	if err != nil || state.Request.Status != "ASSIGNED" {
		t.Fatal(state, err)
	}
}

func TestReservationAndIntentReplaysRequireBarrier(t *testing.T) {
	store, _ := resourceFixture(t)
	ctx := context.Background()
	resource, err := store.RegisterResource(ctx, "reserve-barrier-resource-0001", resourceBody(0, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	request, err := store.Create(ctx, "reserve-barrier-request-0001", body())
	if err != nil {
		t.Fatal(err)
	}
	intent := contracts.IntentCommand{AssignmentID: fmt.Sprintf("%032x", 0xabc01), RequestID: request.ID, ResourceID: resource.ID, Generation: 1}
	if _, err = store.BeginIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	reserve := contracts.ReserveCommand{AssignmentID: intent.AssignmentID, RequestID: request.ID, ResourceID: resource.ID}
	if _, err = store.Reserve(ctx, reserve); err != nil {
		t.Fatal(err)
	}
	rejected := reserve
	rejected.AssignmentID = fmt.Sprintf("%032x", 0xabc02)
	rejected.RequestID = fmt.Sprintf("%032x", 0xabc03)
	if out, err := store.Reserve(ctx, rejected); err != nil || out.Status != "REJECTED" {
		t.Fatalf("rejection %+v %v", out, err)
	}
	standby, err := storage.Open(ctx, os.Getenv("ERM_TEST_STANDBY_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer standby.Close()
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_pause()"); err != nil {
		t.Fatal(err)
	}
	defer standby.Exec(context.Background(), "SELECT pg_wal_replay_resume()")
	for _, cmd := range []contracts.ReserveCommand{reserve, rejected} {
		limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		_, err := store.Reserve(limited, cmd)
		cancel()
		if err == nil {
			t.Fatal("unconfirmed reservation replay escaped")
		}
	}
	limited, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	_, err = store.BeginIntent(limited, intent)
	cancel()
	if err == nil {
		t.Fatal("unconfirmed intent replay escaped")
	}
}

func TestReplayRequiresSynchronousBarrier(t *testing.T) {
	store, primary := fixture(t)
	url := os.Getenv("ERM_TEST_STANDBY_URL")
	if url == "" {
		t.Fatal("ERM_TEST_STANDBY_URL required")
	}
	standby, err := storage.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer standby.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var mode, peers string
	if err = primary.QueryRow(ctx, "SHOW synchronous_commit").Scan(&mode); err != nil || mode != "remote_apply" {
		t.Fatalf("sync mode %q %v", mode, err)
	}
	if err = primary.QueryRow(ctx, "SHOW synchronous_standby_names").Scan(&peers); err != nil || peers == "" {
		t.Fatal("synchronous standby not configured")
	}
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_pause()"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := standby.Exec(cleanup, "SELECT pg_wal_replay_resume()"); err != nil {
			t.Errorf("restore replay: %v", err)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var state string
		if err = standby.QueryRow(ctx, "SELECT pg_get_wal_replay_pause_state()").Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "paused" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("replay did not pause")
		}
		time.Sleep(20 * time.Millisecond)
	}
	key := fmt.Sprintf("replay-barrier-%d", time.Now().UnixNano())
	for range 2 {
		attempt, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		_, err = store.Create(attempt, key, body())
		cancel()
		if err == nil {
			t.Fatal("reported durable success while replay paused")
		}
	}
	var visible int
	if err = primary.QueryRow(ctx, "SELECT count(*) FROM operations WHERE entity_id=$1", contracts.EntityID("create-request", key)).Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("test did not reach an ambiguous local commit: %d %v", visible, err)
	}
	changed := body()
	changed.Details = "changed during paused replay"
	uncertain, cancelAttempt := context.WithTimeout(ctx, 300*time.Millisecond)
	_, err = store.Create(uncertain, key, changed)
	cancelAttempt()
	if !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("unconfirmed conflict must remain unavailable, got %v", err)
	}
	if _, err = standby.Exec(ctx, "SELECT pg_wal_replay_resume()"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Create(ctx, key, body())
	if err != nil {
		t.Fatal(err)
	}
	copy, err := storage.New(standby).Get(ctx, result.ID)
	if err != nil || copy.ID != result.ID {
		t.Fatalf("replica missing acknowledged record: %+v %v", copy, err)
	}
	if _, err = storage.New(standby).Create(ctx, key+"-readonly", body()); err == nil {
		t.Fatal("standby accepted a write")
	}
	var count int
	if err = primary.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE request_id=$1", result.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate outbox after ambiguity: %d %v", count, err)
	}
}
