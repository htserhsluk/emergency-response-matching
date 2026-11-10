//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReservationSingleWinner(t *testing.T) {
	s, _ := resourceFixture(t)
	ctx := context.Background()
	r, err := s.RegisterResource(ctx, "reservation-resource-0001", resourceBody(0, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var winners atomic.Int32
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := contracts.ReserveCommand{AssignmentID: fmt.Sprintf("%032x", i+1), RequestID: fmt.Sprintf("%032x", i+100), ResourceID: r.ID}
			out, err := s.Reserve(ctx, cmd)
			if err != nil {
				t.Error(err)
				return
			}
			if out.Status == "SUCCEEDED" {
				winners.Add(1)
			} else if out.Status != "REJECTED" {
				t.Errorf("invalid outcome %+v", out)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners=%d", winners.Load())
	}
	got, err := s.GetResource(ctx, r.ID)
	if err != nil || got.Status != "ALLOCATED" || got.Availability || got.AssignedRequestID == "" {
		t.Fatalf("ownership %+v %v", got, err)
	}
}
func TestReservationReplayAndDefiniteRejection(t *testing.T) {
	s, _ := resourceFixture(t)
	ctx := context.Background()
	r, err := s.RegisterResource(ctx, "reservation-resource-0002", resourceBody(0, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	cmd := contracts.ReserveCommand{AssignmentID: "00000000000000000000000000000001", RequestID: "00000000000000000000000000000002", ResourceID: r.ID}
	first, err := s.Reserve(ctx, cmd)
	if err != nil || first.Status != "REJECTED" {
		t.Fatalf("rejection %+v %v", first, err)
	}
	if _, err = s.SetResourceStatus(ctx, r.ID, "reservation-enable-0001", "AVAILABLE"); err != nil {
		t.Fatal(err)
	}
	again, err := s.Reserve(ctx, cmd)
	if err != nil || again != first {
		t.Fatalf("definite rejection changed %+v %v", again, err)
	}
	cmd.RequestID = "00000000000000000000000000000003"
	if _, err = s.Reserve(ctx, cmd); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("payload conflict %v", err)
	}
	cmd.AssignmentID = "00000000000000000000000000000004"
	won, err := s.Reserve(ctx, cmd)
	if err != nil || won.Status != "SUCCEEDED" {
		t.Fatalf("new reservation %+v %v", won, err)
	}
	replay, err := s.Reserve(ctx, cmd)
	if err != nil || replay != won {
		t.Fatalf("success replay %+v %v", replay, err)
	}
}
func TestIntentProtectsUnfinishedAssignment(t *testing.T) {
	s, _ := resourceFixture(t)
	ctx := context.Background()
	r, err := s.Create(ctx, "intent-request-000001", body())
	if err != nil {
		t.Fatal(err)
	}
	c := contracts.IntentCommand{RequestID: r.ID, AssignmentID: "00000000000000000000000000000001", ResourceID: "00000000000000000000000000000002", Generation: 1, DistanceMeters: 123}
	a, err := s.BeginIntent(ctx, c)
	if err != nil || a.Phase != "INTENT" {
		t.Fatalf("intent %+v %v", a, err)
	}
	replay, err := s.BeginIntent(ctx, c)
	if err != nil || replay != a {
		t.Fatalf("intent replay %+v %v", replay, err)
	}
	c.AssignmentID = "00000000000000000000000000000003"
	c.ResourceID = "00000000000000000000000000000004"
	if _, err = s.BeginIntent(ctx, c); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("second intent %v", err)
	}
	if _, err = s.SetStatus(ctx, r.ID, "intent-status-0000001", "COMPLETED"); !errors.Is(err, contracts.ErrConflict) {
		t.Fatalf("unfinished bypass %v", err)
	}
	got, err := s.Get(ctx, r.ID)
	if err != nil || got.Status != "ASSIGNING" {
		t.Fatalf("intent state %+v %v", got, err)
	}
}
