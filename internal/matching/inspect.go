package matching

import (
	"context"
	"encoding/json"
	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	amqp "github.com/rabbitmq/amqp091-go"
	"net"
	"time"
)

type Inspection struct {
	Request    contracts.Request     `json:"request"`
	Candidates []contracts.Candidate `json:"candidates"`
	DeliveryID string                `json:"deliveryId"`
}

// InspectOne proves broker -> consumer -> coordinator lookup/search without
// claiming matching completion. It explicitly returns the unfinished message.
// Assignment, durable retry state and completion acknowledgements follow in G6/G7.
func InspectOne(parent context.Context, address, queue string, client *httpapp.Client) (Inspection, error) {
	out := Inspection{}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var raw net.Conn
	stop := func() bool { return false }
	defer func() {
		stop()
		if raw != nil {
			raw.Close()
		}
	}()
	conn, err := amqp.DialConfig(address, amqp.Config{Recovery: nil, Dial: func(network, address string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if e != nil {
			return nil, e
		}
		raw = c
		deadline, _ := ctx.Deadline()
		c.SetDeadline(deadline)
		stop = context.AfterFunc(ctx, func() { c.Close() })
		return c, nil
	}})
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	ch, err := conn.Channel()
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	if err = broker.Declare(ch); err != nil {
		return out, contracts.ErrUnavailable
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return out, contracts.ErrUnavailable
	}
	deliveries, err := ch.ConsumeWithContext(ctx, queue, "inspection", false, false, false, false, nil)
	if err != nil {
		return out, contracts.ErrUnavailable
	}
	select {
	case message, ok := <-deliveries:
		if !ok {
			return out, contracts.ErrUnavailable
		}
		var event contracts.MatchEvent
		if json.Unmarshal(message.Body, &event) != nil || event.Validate() != nil {
			return out, contracts.ErrInvalid
		}
		retry, ok := message.Headers["retryNumber"].(int32)
		if !ok || retry < 0 || message.MessageId != contracts.DeliveryID(event.EventID, int(retry), message.RoutingKey) {
			return out, contracts.ErrInvalid
		}
		out.Request, err = client.Get(ctx, event.RequestID)
		if err != nil {
			return out, err
		}
		out.DeliveryID = message.MessageId
		if out.Request.Generation != event.Generation {
			return out, contracts.ErrConflict
		}
		out.Candidates, err = client.Search(ctx, contracts.SearchQuery{Type: out.Request.Type, Location: out.Request.Location, RadiusMeters: 50000, Limit: 20})
		if err != nil {
			return out, err
		}
		// Stop deliveries before requeueing; QueueInspect is an ordered RPC barrier,
		// not a sleep-based assumption that RabbitMQ processed the nack.
		if err = ch.Cancel("inspection", false); err != nil {
			return out, contracts.ErrUnavailable
		}
		if err = message.Nack(false, true); err != nil {
			return out, contracts.ErrUnavailable
		}
		if _, err = ch.QueueInspect(queue); err != nil {
			return out, contracts.ErrUnavailable
		}
		return out, nil
	case <-ctx.Done():
		return out, contracts.ErrUnavailable
	}
}
