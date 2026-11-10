package contracts

import (
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"math"
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
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Location          *Location `json:"location"`
	Availability      bool      `json:"availability"`
	Status            string    `json:"status"`
	AssignedRequestID string    `json:"assignedRequestId,omitempty"`
}
type ListQuery struct {
	Limit int    `json:"limit"`
	After string `json:"after,omitempty"`
}

func (q ListQuery) Validate() error {
	if q.Limit < 1 || q.Limit > 100 {
		return ErrInvalid
	}
	if q.After != "" {
		if _, err := routing.ShardFor(q.After); err != nil {
			return ErrInvalid
		}
	}
	return nil
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
	if (CreateRequest{Type: q.Type, Location: q.Location, Details: "search"}).Validate() != nil || math.IsNaN(q.RadiusMeters) || math.IsInf(q.RadiusMeters, 0) || q.RadiusMeters <= 0 || q.RadiusMeters > 20000000 || math.IsNaN(q.AfterDistance) || math.IsInf(q.AfterDistance, 0) || q.AfterDistance < 0 {
		return ErrInvalid
	}
	if q.AfterID == "" && q.AfterDistance != 0 {
		return ErrInvalid
	}
	return (ListQuery{Limit: q.Limit, After: q.AfterID}).Validate()
}
