//go:build integration

package integration

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"net/http/httptest"
	"testing"
)

func TestCommandHTTPPathPreservesIntentAndDelivery(t *testing.T) {
	s, _ := resourceFixture(t)
	const secret = "command-test-internal-secret-32-chars"
	node := httptest.NewServer(httpapp.NewServer(s, secret, true))
	defer node.Close()
	router := coordinator.NewRouter([3]string{node.URL, node.URL, node.URL}, secret)
	coord := httptest.NewServer(httpapp.NewServer(router, secret, true))
	defer coord.Close()
	api := httptest.NewServer(httpapp.NewServer(httpapp.NewClient(coord.URL, secret), "", false))
	defer api.Close()
	client := httpapp.NewPublicClient(api.URL)
	internal := httpapp.NewClient(coord.URL, secret)
	ctx := context.Background()
	request, err := client.Create(ctx, "commands-request-0001", body())
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := client.ScheduleMatch(ctx, request.ID, "commands-schedule-0001")
	if err != nil || scheduled.Generation != 1 {
		t.Fatalf("schedule %+v %v", scheduled, err)
	}
	resource, err := client.RegisterResource(ctx, "commands-resource-0001", resourceBody(0, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := internal.BeginIntent(ctx, contracts.IntentCommand{AssignmentID: "00000000000000000000000000000011", RequestID: request.ID, ResourceID: resource.ID, Generation: 1, DistanceMeters: 0})
	if err != nil || intent.Phase != "INTENT" {
		t.Fatalf("intent %+v %v", intent, err)
	}
	reserved, err := internal.Reserve(ctx, contracts.ReserveCommand{AssignmentID: intent.AssignmentID, RequestID: request.ID, ResourceID: resource.ID})
	if err != nil || reserved.Status != "SUCCEEDED" {
		t.Fatalf("reserve %+v %v", reserved, err)
	}
	deliveries, err := internal.ClaimOutbox(ctx, "commands-claim-000001")
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("claim %+v %v", deliveries, err)
	}
	d := deliveries[0]
	if err = internal.MarkSent(ctx, d.Event.RequestID, d.DeliveryID, d.ClaimToken); err != nil {
		t.Fatal(err)
	}
}
