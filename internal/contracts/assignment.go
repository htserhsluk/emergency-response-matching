package contracts

import (
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"math"
)

type ReserveCommand struct {
	AssignmentID string `json:"assignmentId"`
	RequestID    string `json:"requestId"`
	ResourceID   string `json:"resourceId"`
}

func (c ReserveCommand) Validate() error {
	for _, id := range []string{c.AssignmentID, c.RequestID, c.ResourceID} {
		if _, err := routing.ShardFor(id); err != nil {
			return ErrInvalid
		}
	}
	return nil
}

type ReservationResult struct {
	AssignmentID string `json:"assignmentId"`
	RequestID    string `json:"requestId"`
	ResourceID   string `json:"resourceId"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
}
type IntentCommand struct {
	AssignmentID   string  `json:"assignmentId"`
	RequestID      string  `json:"requestId"`
	ResourceID     string  `json:"resourceId"`
	Generation     int64   `json:"generation"`
	DistanceMeters float64 `json:"distanceMeters"`
}

func (c IntentCommand) Validate() error {
	if c.Generation < 1 || math.IsNaN(c.DistanceMeters) || math.IsInf(c.DistanceMeters, 0) || c.DistanceMeters < 0 {
		return ErrInvalid
	}
	return (ReserveCommand{AssignmentID: c.AssignmentID, RequestID: c.RequestID, ResourceID: c.ResourceID}).Validate()
}

type Intent struct {
	IntentCommand
	Phase string `json:"phase"`
}
