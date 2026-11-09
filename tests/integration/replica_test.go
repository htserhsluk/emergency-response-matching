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
