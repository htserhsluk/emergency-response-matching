package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"log/slog"
	randv2 "math/rand/v2"
	"time"
)

type Source interface {
	ClaimOutbox(context.Context, string) ([]contracts.Delivery, error)
	MarkSent(context.Context, string, string, string) error
}
type Publisher interface {
	PublishConfirmed(context.Context, contracts.Delivery) error
}

func PumpOnce(parent context.Context, source Source, publisher Publisher) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	deliveries, err := source.ClaimOutbox(ctx, hex.EncodeToString(nonce[:]))
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		if err = publisher.PublishConfirmed(ctx, d); err != nil {
			return err
		}
		if err = source.MarkSent(ctx, d.Event.RequestID, d.DeliveryID, d.ClaimToken); err != nil {
			return err
		}
	}
	return nil
}
func Run(ctx context.Context, source Source, publisher Publisher) {
	delay := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := PumpOnce(ctx, source, publisher)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("outbox publication deferred", "reason", err)
			delay = min(30*time.Second, delay*2)
		} else {
			delay = time.Second
		}
		timer := time.NewTimer(delay + time.Duration(randv2.IntN(250))*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
