package matching

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	amqp "github.com/rabbitmq/amqp091-go"
	"math/rand/v2"
	"net"
	"time"
)

// ConsumeSession owns its connection/channel and never acknowledges uncertain
// work. Returning closes the channel, allowing RabbitMQ to redeliver it.
func ConsumeSession(ctx context.Context, address, queue string, client *httpapp.Client) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var raw net.Conn
	defer func() {
		if raw != nil {
			raw.Close()
		}
	}()
	conn, err := amqp.DialConfig(address, amqp.Config{Heartbeat: 2 * time.Second, Recovery: nil, Dial: func(network, addr string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: time.Second}).DialContext(dialCtx, network, addr)
		if e != nil {
			return nil, e
		}
		raw = c
		c.SetDeadline(time.Now().Add(5 * time.Second))
		return c, nil
	}})
	if err != nil {
		return contracts.ErrUnavailable
	}
	raw.SetDeadline(time.Time{})
	// A shutdown permits at most twenty seconds for an in-flight operation. An
	// idle consumer returns immediately; blocked transport is bounded independently.
	stop := context.AfterFunc(ctx, func() { raw.SetDeadline(time.Now().Add(20 * time.Second)) })
	defer stop()
	ch, err := conn.Channel()
	if err != nil {
		return contracts.ErrUnavailable
	}
	raw.SetDeadline(time.Now().Add(5 * time.Second))
	if err = broker.Declare(ch); err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	raw.SetDeadline(time.Time{})
	publisher := broker.NewPublisher(address)
	for {
		select {
		case <-ctx.Done():
			return nil
		case message, ok := <-deliveries:
			if !ok {
				return contracts.ErrUnavailable
			}
			if ctx.Err() != nil {
				return nil
			}
			work, cancelWork := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
			var event contracts.MatchEvent
			valid := json.Unmarshal(message.Body, &event) == nil && event.Validate() == nil
			retry, ok := message.Headers["retryNumber"].(int32)
			valid = valid && ok && retry >= 0 && message.RoutingKey == "request.match" && message.MessageId == contracts.DeliveryID(event.EventID, int(retry), "request.match")
			if valid {
				state, e := client.HandleDelivery(work, contracts.Delivery{Event: event, DeliveryID: message.MessageId, RetryNumber: int(retry), Destination: "request.match"})
				err = e
				if err == nil && !state.Ack {
					err = contracts.ErrUnavailable
				}
				// Invalid future attempts/event identities cannot advance the stored budget.
				if errors.Is(err, contracts.ErrInvalid) || errors.Is(err, contracts.ErrNotFound) {
					err = publisher.PublishDead(work, contracts.EntityID("invalid-delivery", message.MessageId+string(message.Body)), message.Body, "INVALID_EVENT_OR_ATTEMPT")
				}
			} else {
				err = publisher.PublishDead(work, contracts.EntityID("invalid-delivery", message.MessageId+string(message.Body)), message.Body, "MALFORMED_OR_UNKNOWN_SCHEMA")
			}
			cancelWork()
			if err != nil {
				return err
			}
			// Ack only on this session's channel, after a confirmed durable decision.
			raw.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err = message.Ack(false); err != nil {
				return err
			}
			raw.SetWriteDeadline(time.Time{})
		}
	}
}

func Run(ctx context.Context, address string, client *httpapp.Client) {
	delay := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		_ = ConsumeSession(ctx, address, "matching.work", client)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > 30*time.Second {
			delay = time.Second
		} else {
			delay = min(30*time.Second, delay*2)
		}
		timer := time.NewTimer(delay + time.Duration(rand.IntN(250))*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
