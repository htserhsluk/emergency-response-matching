//go:build integration && broker

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/delivery"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/matching"
	amqp "github.com/rabbitmq/amqp091-go"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestReadinessDropsOnBrokerLossDuringInFlightWork(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		down, e := listener.Accept()
		if e != nil {
			return
		}
		defer down.Close()
		up, e := net.Dial("tcp", parsed.Host)
		if e != nil {
			return
		}
		defer up.Close()
		accepted <- down
		go io.Copy(up, down)
		io.Copy(down, up)
	}()
	proxy := *parsed
	proxy.Host = listener.Addr().String()
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
	q, err := ch.QueueDeclare(fmt.Sprintf("readiness-%d", time.Now().UnixNano()), true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.QueueDelete(q.Name, false, false, false)
	if err = ch.QueueBind(q.Name, "request.match", "emergency.events", false, nil); err != nil {
		t.Fatal(err)
	}
	_, _, _, d := workflowFixture(t)
	payload, _ := json.Marshal(d.Event)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		http.Error(w, "fixture released", 503)
	}))
	defer server.Close()
	defer close(release)
	state := &matching.SessionState{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- matching.ConsumeSession(ctx, proxy.String(), q.Name, httpapp.NewClient(server.URL, "fixture-secret"), state)
	}()
	if err = ch.PublishWithContext(ctx, "emergency.events", "request.match", false, false, amqp.Publishing{Body: payload, MessageId: d.DeliveryID, Headers: amqp.Table{"retryNumber": int32(0)}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("no in-flight operation")
	}
	if !state.Ready() {
		t.Fatal("connected consumer not ready")
	}
	select {
	case connection := <-accepted:
		connection.Close()
	case <-time.After(time.Second):
		t.Fatal("no owned connection")
	}
	deadline := time.Now().Add(time.Second)
	for state.Ready() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if state.Ready() {
		t.Fatal("disconnected worker remained ready while HTTP operation blocked")
	}
}

func TestWorkerAssignsAcrossShardsAndUnknownSchemaToDLQ(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	if address == "" {
		t.Fatal("ERM_TEST_AMQP_URL required")
	}
	l := newWorkflowLab(t)
	resource := l.resource(t, 2, "worker-resource")
	request, _ := l.request(t, 0, "worker-request")
	server := httptest.NewServer(httpapp.NewServer(l.router, "workflow-secret-at-least-32-characters", true))
	defer server.Close()
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
	q, err := ch.QueueDeclare(fmt.Sprintf("worker-%d", time.Now().UnixNano()), true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.QueueDelete(q.Name, false, false, false)
	if err = ch.QueueBind(q.Name, "request.match", "emergency.events", false, nil); err != nil {
		t.Fatal(err)
	}
	dead, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = ch.QueueBind(dead.Name, "request.dead", "emergency.dead", false, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	readiness := &matching.SessionState{}
	if readiness.Ready() {
		t.Fatal("disconnected worker marked ready")
	}
	go func() {
		done <- matching.ConsumeSession(ctx, address, q.Name, httpapp.NewClient(server.URL, "workflow-secret-at-least-32-characters"), readiness)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("worker failed to stop")
		}
		if readiness.Ready() {
			t.Error("stopped consumer remained ready")
		}
	}()
	if err = delivery.PumpOnce(ctx, l.stores[0], broker.NewPublisher(address)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, e := l.router.Get(ctx, request.ID)
		if e == nil && got.Status == "ASSIGNED" {
			if !readiness.Ready() {
				t.Fatal("active consumer not ready")
			}
			if got.AssignedResourceID != resource.ID {
				t.Fatal(got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("assignment timeout %+v %v", got, e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err = ch.PublishWithContext(ctx, "", q.Name, false, false, amqp.Publishing{Body: []byte(`{"schemaVersion":999}`), MessageId: "unknown-schema-test"}); err != nil {
		t.Fatal(err)
	}
	for {
		msg, ok, e := ch.Get(dead.Name, true)
		if e != nil {
			t.Fatal(e)
		}
		if ok {
			if msg.RoutingKey != "request.dead" || len(msg.Body) == 0 {
				t.Fatal("missing diagnostic")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("unknown schema not dead-lettered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for {
		status, e := ch.QueueInspect(q.Name)
		if e != nil {
			t.Fatal(e)
		}
		if status.Messages == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("work not acknowledged")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestShutdownLeavesUnfinishedDelivery(t *testing.T) {
	address := os.Getenv("ERM_TEST_AMQP_URL")
	if address == "" {
		t.Fatal("ERM_TEST_AMQP_URL required")
	}
	_, _, _, d := workflowFixture(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
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
	q, err := ch.QueueDeclare(fmt.Sprintf("shutdown-%d", time.Now().UnixNano()), true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.QueueDelete(q.Name, false, false, false)
	if err = ch.QueueBind(q.Name, "request.match", "emergency.events", false, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(d.Event)
	if err = ch.PublishWithContext(context.Background(), "emergency.events", "request.match", false, false, amqp.Publishing{Body: data, MessageId: d.DeliveryID, Headers: amqp.Table{"retryNumber": int32(0)}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- matching.ConsumeSession(ctx, address, q.Name, httpapp.NewClient(server.URL, "shutdown-secret-at-least-32-characters"))
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never began request")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unfinished operation reported success")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("worker shutdown blocked")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		msg, ok, e := ch.Get(q.Name, false)
		if e != nil {
			t.Fatal(e)
		}
		if ok {
			if msg.MessageId != d.DeliveryID || !msg.Redelivered {
				t.Fatal("delivery identity lost")
			}
			msg.Ack(false)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("unfinished delivery lost")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
