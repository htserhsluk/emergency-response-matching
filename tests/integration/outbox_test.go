//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"testing"
)

func TestRequestAndOutboxAtomicClaimAndReplay(t *testing.T) {
	s, p := resourceFixture(t)
	ctx := context.Background()
	r, err := s.Create(ctx, "outbox-request-000001", body())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimOutbox(ctx, "outbox-claim-token-0001")
	if err != nil || len(first) != 1 || first[0].Event.RequestID != r.ID {
		t.Fatalf("claim %+v %v", first, err)
	}
	second, err := s.ClaimOutbox(ctx, "outbox-claim-token-0002")
	if err != nil || len(second) != 0 {
		t.Fatalf("double claim %+v %v", second, err)
	}
	d := first[0]
	if err = s.MarkSent(ctx, d.Event.RequestID, d.DeliveryID, "outbox-claim-token-wrong"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("stale claimant marked sent: %v", err)
	}
	// Simulate a publish-before-mark crash. Reclaim preserves transport/logical identities.
	if _, err = p.Exec(ctx, `UPDATE outbox_events SET claim_until=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	replay, err := s.ClaimOutbox(ctx, "outbox-claim-token-0003")
	if err != nil || len(replay) != 1 || replay[0].DeliveryID != d.DeliveryID || replay[0].Event.EventID != d.Event.EventID {
		t.Fatalf("reclaim %+v %v", replay, err)
	}
	if err = s.MarkSent(ctx, d.Event.RequestID, d.DeliveryID, replay[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	done, err := s.ClaimOutbox(ctx, "outbox-claim-token-0004")
	if err != nil || len(done) != 0 {
		t.Fatalf("sent redelivered %+v %v", done, err)
	}
}
func TestManualMatchGenerationIsIdempotent(t *testing.T) {
	s, p := resourceFixture(t)
	ctx := context.Background()
	r, err := s.Create(ctx, "manual-request-000001", body())
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.ScheduleMatch(ctx, r.ID, "manual-pending-000001")
	if err != nil || pending.Generation != 1 {
		t.Fatalf("pending schedule %+v %v", pending, err)
	}
	if _, err = p.Exec(ctx, `UPDATE requests SET status='UNMATCHED' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.ScheduleMatch(ctx, r.ID, "manual-rematch-000001")
	if err != nil || next.Status != "PENDING" || next.Generation != 2 {
		t.Fatalf("rematch %+v %v", next, err)
	}
	replay, err := s.ScheduleMatch(ctx, r.ID, "manual-rematch-000001")
	if err != nil || replay.Generation != 2 {
		t.Fatalf("rematch replay %+v %v", replay, err)
	}
	var count int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE request_id=$1`, r.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}
