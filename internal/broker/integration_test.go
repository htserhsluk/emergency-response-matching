//go:build broker

package broker

import (
	"context"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"testing"
	"time"
)

func TestReturnedPublishNeverMarkedSent(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	if address == "" {
		t.Fatal("ERM_TEST_AMQP_URL required")
	}
	p := NewPublisher(address)
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := contracts.Delivery{Event: contracts.MatchEvent{SchemaVersion: 1, EventID: "00000000000000000000000000000001", RequestID: "00000000000000000000000000000002", Generation: 1, CreatedAt: time.Now()}, DeliveryID: "00000000000000000000000000000003", Destination: "unbound.test.route"}
	for range 30 {
		if err := p.PublishConfirmed(ctx, d); !errors.Is(err, ErrUnroutable) {
			t.Fatalf("unroutable publish reported %v", err)
		}
	}
	d.Destination = "request.match"
	if err := p.PublishConfirmed(ctx, d); err != nil {
		t.Fatal(err)
	}
	connection, err := amqp.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	delivery, ok, err := channel.Get("matching.work", false)
	if err != nil || !ok || delivery.DeliveryMode != amqp.Persistent || delivery.MessageId != d.DeliveryID {
		t.Fatalf("routed durable delivery %+v %v %v", delivery, ok, err)
	}
	// Return it to the durable queue: inspection is not matching completion.
	if err = delivery.Nack(false, true); err != nil {
		t.Fatal(err)
	}
}
