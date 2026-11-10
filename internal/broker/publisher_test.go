package broker

import (
	"context"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"io"
	"net"
	"testing"
	"time"
)

func TestPublishDeadlineClosesTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closed := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
		close(closed)
	}()
	p := NewPublisher("amqp://fixture:fixture@" + listener.Addr().String() + "/")
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	d := contracts.Delivery{Event: contracts.MatchEvent{SchemaVersion: 1, EventID: "00000000000000000000000000000001", RequestID: "00000000000000000000000000000002", Generation: 1, CreatedAt: time.Now()}, DeliveryID: "00000000000000000000000000000003", Destination: "request.match"}
	start := time.Now()
	if err = p.PublishConfirmed(ctx, d); err == nil {
		t.Fatal("stalled peer confirmed publication")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("deadline did not bound blocked transport")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("owned transport remained open")
	}
}
