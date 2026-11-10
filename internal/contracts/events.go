package contracts

import (
	"fmt"
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"time"
)

type MatchEvent struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	RequestID     string    `json:"requestId"`
	Generation    int64     `json:"generation"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (e MatchEvent) Validate() error {
	if e.SchemaVersion != 1 || e.Generation < 1 || e.CreatedAt.IsZero() {
		return ErrInvalid
	}
	for _, id := range []string{e.EventID, e.RequestID} {
		if _, err := routing.ShardFor(id); err != nil {
			return ErrInvalid
		}
	}
	return nil
}
func DeliveryID(event string, retry int, destination string) string {
	return EntityID("delivery", fmt.Sprintf("%s:%d:%s", event, retry, destination))
}

type Delivery struct {
	Event       MatchEvent `json:"event"`
	DeliveryID  string     `json:"deliveryId"`
	RetryNumber int        `json:"retryNumber"`
	Destination string     `json:"destination"`
	ClaimToken  string     `json:"claimToken"`
}
