//go:build integration && broker

package integration

import (
	"context"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/delivery"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/matching"
	amqp "github.com/rabbitmq/amqp091-go"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestBrokerConsumerLooksUpRequestAndCandidatesWithoutAcknowledging(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	if address == "" {
		t.Fatal("ERM_TEST_AMQP_URL required")
	}
	s, _ := resourceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const secret = "consumer-fixture-secret-32-characters"
	node := httptest.NewServer(httpapp.NewServer(s, secret, true))
	defer node.Close()
	router := coordinator.NewRouter([3]string{node.URL, node.URL, node.URL}, secret)
	coord := httptest.NewServer(httpapp.NewServer(router, secret, true))
	defer coord.Close()
	request, err := s.Create(ctx, "consumer-request-00001", body())
	if err != nil {
		t.Fatal(err)
	}
	resource, err := s.RegisterResource(ctx, "consumer-resource-00001", resourceBody(0, 0, true))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := amqp.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	if err = broker.Declare(ch); err != nil {
		t.Fatal(err)
	}
	q, err := ch.QueueDeclare(fmt.Sprintf("inspection-%d", time.Now().UnixNano()), true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.QueueDelete(q.Name, false, false, false)
	if err = ch.QueueBind(q.Name, "request.match", "emergency.events", false, nil); err != nil {
		t.Fatal(err)
	}
	if err = delivery.PumpOnce(ctx, s, broker.NewPublisher(address)); err != nil {
		t.Fatal(err)
	}
	preview, err := matching.InspectOne(ctx, address, q.Name, httpapp.NewClient(coord.URL, secret))
	if err != nil || preview.Request.ID != request.ID || len(preview.Candidates) == 0 || preview.Candidates[0].ResourceID != resource.ID {
		t.Fatalf("preview %+v %v", preview, err)
	}
	message, ok, err := ch.Get(q.Name, false)
	if err != nil || !ok {
		t.Fatalf("inspection lost unfinished delivery: %v %v", ok, err)
	}
	message.Ack(false)
	got, err := s.Get(ctx, request.ID)
	if err != nil || got.Status != "PENDING" {
		t.Fatalf("inspection claimed assignment %+v %v", got, err)
	}
}
