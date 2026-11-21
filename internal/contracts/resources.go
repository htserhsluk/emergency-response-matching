package contracts

import (
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"math"
	"strings"
	"time"
)

type RegisterResource struct {
	Type         string    `json:"type"`
	Location     *Location `json:"location"`
	Availability *bool     `json:"availability"`
}

func (c RegisterResource) Validate() error {
	if c.Availability == nil {
		return ErrInvalid
	}
	return (CreateRequest{Type: c.Type, Location: c.Location, Details: "resource"}).Validate()
}

type Resource struct {
	AllocatedAt           time.Time `json:"allocatedAt,omitzero"`
	AllocationConfirmedAt time.Time `json:"allocationConfirmedAt,omitzero"`
	RegisteredAt          time.Time `json:"registeredAt,omitzero"`
	ID                    string    `json:"id"`
	Type                  string    `json:"type"`
	Location              *Location `json:"location"`
	Availability          bool      `json:"availability"`
	Status                string    `json:"status"`
	AssignedRequestID     string    `json:"assignedRequestId,omitempty"`
}
type ListQuery struct {
	Sort   string `json:"sort,omitempty"`
	Type   string `json:"type,omitempty"`
	Limit  int    `json:"limit"`
	After  string `json:"after,omitempty"`
	Status string `json:"status,omitempty"`
}

func (q ListQuery) Validate() error {
	if (q.Sort != "" && q.Sort != "newest" && q.Sort != "allocated") || (q.Sort == "allocated" && q.Status != "ALLOCATED") || q.Type != "" && !validType(q.Type) {
		return ErrInvalid
	}
	switch q.Status {
	case "", "UNALLOCATED", "AVAILABLE", "UNAVAILABLE", "ALLOCATED":
	default:
		return ErrInvalid
	}
	if q.Limit < 1 || q.Limit > 100 {
		return ErrInvalid
	}
	if q.After != "" {
		if q.Sort == "newest" || q.Sort == "allocated" {
			_, _, err := q.NewestCursor()
			return err
		}
		if _, err := routing.ShardFor(q.After); err != nil {
			return ErrInvalid
		}
	}
	return nil
}

func (q ListQuery) NewestCursor() (time.Time, string, error) {
	stamp, id, ok := strings.Cut(q.After, "/")
	registered, err := time.Parse(time.RFC3339Nano, stamp)
	if _, idErr := routing.ShardFor(id); !ok || err != nil || idErr != nil || registered.Nanosecond()%1000 != 0 {
		return time.Time{}, "", ErrInvalid
	}
	return registered, id, nil
}

func (q ListQuery) CursorFor(r Resource) string {
	if q.Sort == "allocated" {
		return r.AllocationOrderTime().UTC().Format(time.RFC3339Nano) + "/" + r.ID
	}
	if q.Sort == "newest" {
		return r.RegisteredAt.UTC().Format(time.RFC3339Nano) + "/" + r.ID
	}
	return r.ID
}

type ResourcePage struct {
	Items []Resource `json:"items"`
	Next  string     `json:"next,omitempty"`
}
type Candidate struct {
	ResourceID     string  `json:"resourceId"`
	DistanceMeters float64 `json:"distanceMeters"`
}
type SearchQuery struct {
	Type          string    `json:"type"`
	Location      *Location `json:"location"`
	RadiusMeters  float64   `json:"radiusMeters"`
	Limit         int       `json:"limit"`
	AfterDistance float64   `json:"afterDistance"`
	AfterID       string    `json:"afterId,omitempty"`
}

func (q SearchQuery) Validate() error {
	if (CreateRequest{Type: q.Type, Location: q.Location, Details: "search"}).Validate() != nil || math.IsNaN(q.RadiusMeters) || math.IsInf(q.RadiusMeters, 0) || q.RadiusMeters < 0 || q.RadiusMeters > MaxMatchDistanceMeters || math.IsNaN(q.AfterDistance) || math.IsInf(q.AfterDistance, 0) || q.AfterDistance < 0 {
		return ErrInvalid
	}
	if q.AfterID == "" && q.AfterDistance != 0 {
		return ErrInvalid
	}
	return (ListQuery{Limit: q.Limit, After: q.AfterID}).Validate()
}
