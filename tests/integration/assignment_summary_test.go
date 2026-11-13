//go:build integration

package integration

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlowResourceLocationDoesNotHideAssignedRequest(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	l.resource(t, 2, "slow-summary-resource")
	request, d := l.request(t, 0, "slow-summary-request")
	if _, err := l.router.HandleDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(6 * time.Second):
		}
	}))
	defer slow.Close()
	l.urls[2] = slow.URL
	l.restart()
	coordinator := httptest.NewServer(httpapp.NewServer(l.router, "secret", true))
	defer coordinator.Close()
	api := httptest.NewServer(httpapp.NewServer(httpapp.NewClient(coordinator.URL, "secret"), "", false))
	defer api.Close()
	limited, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	got, err := httpapp.NewPublicClient(api.URL).Get(limited, request.ID)
	if err != nil || got.Status != "ASSIGNED" || got.Assignment == nil || got.Assignment.Location != nil {
		t.Fatalf("optional location hid durable status %+v %v", got, err)
	}
}

func TestAssignmentSummaryUsesSavedDistanceAndResourceLocation(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	resource := l.resource(t, 2, "summary-resource")
	request, d := l.request(t, 0, "summary-request")
	if _, err := l.router.HandleDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := l.router.Get(ctx, request.ID)
	if err != nil || got.Assignment == nil {
		t.Fatalf("missing summary %+v %v", got, err)
	}
	if got.Assignment.ResourceID != resource.ID || got.Assignment.DistanceMeters != 0 || got.Assignment.Location == nil || *got.Assignment.Location.Latitude != *resource.Location.Latitude {
		t.Fatalf("wrong stored summary %+v", got.Assignment)
	}
	l.restart()
	again, err := l.router.Get(ctx, request.ID)
	if err != nil || again.Assignment == nil || again.Assignment.ResourceID != resource.ID {
		t.Fatal("summary lost after restart", err)
	}
}
