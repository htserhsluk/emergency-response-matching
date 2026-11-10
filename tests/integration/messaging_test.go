//go:build integration && broker

package integration

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/delivery"
	"os"
	"testing"
)

func TestBrokerOutageAndUnroutableKeepOutboxPending(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	if address == "" {
		t.Fatal("ERM_TEST_AMQP_URL required")
	}
	s, p := resourceFixture(t)
	ctx := context.Background()
	r, err := s.Create(ctx, "broker-outbox-0000001", body())
	if err != nil {
		t.Fatal(err)
	}
	if err = delivery.PumpOnce(ctx, s, broker.NewPublisher("amqp://fixture:fixture@127.0.0.1:1/")); err == nil {
		t.Fatal("outage reported successful publication")
	}
	assertPending := func() {
		t.Helper()
		var pending bool
		if err := p.QueryRow(ctx, `SELECT sent_at IS NULL FROM outbox_events WHERE request_id=$1`, r.ID).Scan(&pending); err != nil || !pending {
			t.Fatalf("unconfirmed row marked sent %v", err)
		}
	}
	assertPending()
	if _, err = p.Exec(ctx, `UPDATE outbox_events SET claim_until=clock_timestamp()-interval '1 second',destination='unbound.test.route' WHERE request_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	pub := broker.NewPublisher(address)
	if err = delivery.PumpOnce(ctx, s, pub); err == nil {
		t.Fatal("unroutable publication reported success")
	}
	assertPending()
	if _, err = p.Exec(ctx, `UPDATE outbox_events SET claim_until=clock_timestamp()-interval '1 second',destination='request.match' WHERE request_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = delivery.PumpOnce(ctx, s, pub); err != nil {
		t.Fatal(err)
	}
	var sent bool
	if err = p.QueryRow(ctx, `SELECT sent_at IS NOT NULL FROM outbox_events WHERE request_id=$1`, r.ID).Scan(&sent); err != nil || !sent {
		t.Fatalf("confirmed delivery not marked %v", err)
	}
}
