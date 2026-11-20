package contracts

import (
	"github.com/htserhsluk/emergency-response-matching/internal/routing"
	"strings"
)

const MapPageSize = 100

type MapLocation struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Location           *Location `json:"location"`
	AssignedRequestID  string    `json:"assignedRequestId,omitempty"`
	AssignedResourceID string    `json:"assignedResourceId,omitempty"`
}

func (p MapLocation) Cursor() string { return p.Kind + ":" + p.ID }

type MapPage struct {
	Items []MapLocation `json:"items"`
	Next  string        `json:"next,omitempty"`
}
type NearbyQuery struct {
	Type     string    `json:"type"`
	Location *Location `json:"location"`
}

func (q NearbyQuery) Validate() error {
	return (CreateRequest{Type: q.Type, Location: q.Location, Details: "candidate preview"}).Validate()
}

type NearbyRequest struct {
	ID             string    `json:"id"`
	Location       *Location `json:"location"`
	DistanceMeters float64   `json:"distanceMeters"`
}
type RequestPreview struct {
	Items           []NearbyRequest `json:"items"`
	AssignedRequest *NearbyRequest  `json:"assignedRequest,omitempty"`
}

func ValidMapQuery(kind, after string) bool {
	if !validType(kind) {
		return false
	}
	if after == "" {
		return true
	}
	entity, id, ok := strings.Cut(after, ":")
	_, err := routing.ShardFor(id)
	return ok && (entity == "request" || entity == "resource") && err == nil
}
