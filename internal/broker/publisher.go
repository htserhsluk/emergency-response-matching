package broker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	amqp "github.com/rabbitmq/amqp091-go"
)

var ErrUnroutable = errors.New("broker returned unroutable publication")

// One publisher owns one outstanding publication. Each attempt opens a fresh
// channel/connection, redeclaring durable topology after a broker restart.
// The caller owns capped retry/backoff; library automatic recovery is disabled.
type Publisher struct {
	address string
	serial  chan struct{}
}

func NewPublisher(address string) *Publisher {
	return &Publisher{address: address, serial: make(chan struct{}, 1)}
}
func (p *Publisher) Close() error { return nil } // transports belong to bounded attempts

func Declare(ch *amqp.Channel) error {
	for _, exchange := range []string{"emergency.events", "emergency.dead"} {
		if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
			return err
		}
	}
	for _, q := range []struct{ name, exchange, key string }{{"matching.work", "emergency.events", "request.match"}, {"matching.dlq", "emergency.dead", "request.dead"}} {
		if _, err := ch.QueueDeclare(q.name, true, false, false, false, nil); err != nil {
			return err
		}
		if err := ch.QueueBind(q.name, q.key, q.exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}
func (p *Publisher) PublishConfirmed(parent context.Context, d contracts.Delivery) error {
	if d.Event.Validate() != nil || d.DeliveryID == "" || d.Destination == "" || d.RetryNumber < 0 {
		return contracts.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	select {
	case p.serial <- struct{}{}:
		defer func() { <-p.serial }()
	case <-ctx.Done():
		return contracts.ErrUnavailable
	}
	var raw net.Conn
	stop := func() bool { return false }
	defer func() {
		stop()
		if raw != nil {
			raw.Close()
		}
	}()
	connection, err := amqp.DialConfig(p.address, amqp.Config{Heartbeat: 2 * time.Second, Recovery: nil, Dial: func(network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		raw = conn
		deadline, _ := ctx.Deadline()
		conn.SetDeadline(deadline)
		// PublishWithContext alone does not interrupt blocked AMQP socket writes.
		stop = context.AfterFunc(ctx, func() { conn.Close() })
		return conn, nil
	}})
	if err != nil {
		return contracts.ErrUnavailable
	}
	channel, err := connection.Channel()
	if err != nil {
		return contracts.ErrUnavailable
	}
	if err = Declare(channel); err != nil {
		return contracts.ErrUnavailable
	}
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	closed := channel.NotifyClose(make(chan *amqp.Error, 1))
	if err = channel.Confirm(false); err != nil {
		return contracts.ErrUnavailable
	}
	body, err := json.Marshal(d.Event)
	if err != nil {
		return contracts.ErrInvalid
	}
	exchange := "emergency.events"
	if d.Destination == "request.dead" {
		exchange = "emergency.dead"
	}
	if err = channel.PublishWithContext(ctx, exchange, d.Destination, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: d.DeliveryID, Headers: amqp.Table{"retryNumber": int32(d.RetryNumber)}, Body: body}); err != nil {
		return contracts.ErrUnavailable
	}
	select {
	case _, ok := <-returns:
		if !ok {
			return contracts.ErrUnavailable
		}
		return ErrUnroutable
	case confirmation, ok := <-confirms:
		if !ok || !confirmation.Ack || confirmation.DeliveryTag != 1 || ctx.Err() != nil {
			return contracts.ErrUnavailable
		}
		// amqp091-go v1.16 dispatches a basic.return into this buffered channel
		// before dispatching its basic.ack. With one outstanding publish it cannot
		// overflow. Drain here because Go select may choose the confirm first.
		select {
		case _, ok := <-returns:
			if !ok {
				return contracts.ErrUnavailable
			}
			return ErrUnroutable
		default:
			return nil
		}
	case <-closed:
		return contracts.ErrUnavailable
	case <-ctx.Done():
		return contracts.ErrUnavailable
	}
}
