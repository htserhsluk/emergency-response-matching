//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
)

func TestNotificationAtomicWithAssignment(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	l.resource(t, 2, "inbox-atomic-resource")
	request, d := l.request(t, 0, "inbox-atomic-request")
	// A database constraint failure proves finalization and inbox insert share a transaction.
	if _, err := l.pools[0].Exec(ctx, `ALTER TABLE notifications ADD CONSTRAINT fixture_fail CHECK (recipient <> 'demo')`); err != nil {
		t.Fatal(err)
	}
	state, err := l.router.HandleDelivery(ctx, d)
	if err != nil || state.State != "RUNNING" || state.ExpectedRetry != 1 {
		t.Fatalf("retry after failed finalization %+v %v", state, err)
	}
	got, err := l.router.Get(ctx, request.ID)
	if err != nil || got.Status != "ASSIGNING" {
		t.Fatalf("partial commit %+v %v", got, err)
	}
	page, err := l.router.ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 20})
	if err != nil || len(page.Items) != 0 {
		t.Fatal(page, err)
	}
	if _, err = l.pools[0].Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT fixture_fail; UPDATE processed_events SET due_at=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	d.RetryNumber = 1
	d.DeliveryID = contracts.DeliveryID(d.Event.EventID, 1, d.Destination)
	state, err = l.router.HandleDelivery(ctx, d)
	if err != nil || state.Request.Status != "ASSIGNED" {
		t.Fatal(state, err)
	}
	page, err = l.router.ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
}

func TestNotificationDuplicateReplayAndReadSurvivesRestart(t *testing.T) {
	l := newWorkflowLab(t)
	ctx := context.Background()
	resource := l.resource(t, 2, "inbox-resource")
	request, d := l.request(t, 0, "inbox-request")
	for range 3 {
		if _, err := l.router.HandleDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(httpapp.NewServer(l.router, "", false))
	client := httpapp.NewPublicClient(server.URL)
	page, err := client.ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ResourceID != resource.ID || page.Items[0].ReadAt != nil {
		t.Fatalf("inbox %+v %v", page, err)
	}
	n := page.Items[0]
	read, err := client.ReadNotification(ctx, request.ID, n.ID, "notification-read-key-0001")
	if err != nil || read.ReadAt == nil {
		t.Fatal(read, err)
	}
	server.Close()
	l.restart()
	server = httptest.NewServer(httpapp.NewServer(l.router, "", false))
	defer server.Close()
	client = httpapp.NewPublicClient(server.URL)
	replay, err := client.ReadNotification(ctx, request.ID, n.ID, "notification-read-key-0001")
	if err != nil || replay.ReadAt == nil || !replay.ReadAt.Equal(*read.ReadAt) {
		t.Fatal(replay, err)
	}
	persisted, err := storage.New(l.pools[0]).ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 20})
	if err != nil || len(persisted.Items) != 1 || persisted.Items[0].ReadAt == nil {
		t.Fatal(persisted, err)
	}
	other, _ := l.request(t, 0, "inbox-other-request")
	if _, err = client.ReadNotification(ctx, other.ID, n.ID, "notification-read-key-0002"); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("cross-parent mutation allowed: %v", err)
	}
	if _, err = client.ListNotifications(ctx, "00000000000000000000000000000000", contracts.ListQuery{Limit: 20}); !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("missing parent: %v", err)
	}
	if _, err = client.ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 101}); !errors.Is(err, contracts.ErrInvalid) {
		t.Fatalf("unbounded page: %v", err)
	}
	empty, err := client.ListNotifications(ctx, request.ID, contracts.ListQuery{Limit: 20, After: n.ID})
	if err != nil || len(empty.Items) != 0 {
		t.Fatal(empty, err)
	}
}
