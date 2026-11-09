package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid input")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("operation or state conflict")
var ErrUnavailable = errors.New("dependency unavailable; retry with the same operation key")

type Location struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type CreateRequest struct {
	Type     string    `json:"type"`
	Location *Location `json:"location"`
	Details  string    `json:"details"`
}

func (c CreateRequest) Validate() error {
	switch c.Type {
	case "WATER", "FOOD", "MEDICINE", "TRANSPORTATION", "SHELTER":
	default:
		return ErrInvalid
	}
	if c.Location == nil || c.Location.Latitude == nil || c.Location.Longitude == nil {
		return ErrInvalid
	}
	lat, lon := *c.Location.Latitude, *c.Location.Longitude
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return ErrInvalid
	}
	if strings.TrimSpace(c.Details) == "" || utf8.RuneCountInString(c.Details) > 2000 {
		return ErrInvalid
	}
	return nil
}

func ValidateKey(key string) error {
	if len(key) < 16 || len(key) > 128 {
		return ErrInvalid
	}
	for _, c := range []byte(key) {
		if c < 32 || c > 126 {
			return ErrInvalid
		}
	}
	return nil
}

func EntityID(kind, key string) string {
	h := sha256.Sum256([]byte("erm:v1\x00" + kind + "\x00" + key))
	return hex.EncodeToString(h[:16])
}

func PayloadHash(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

type Request struct {
	ID                 string    `json:"id"`
	Type               string    `json:"type"`
	Location           *Location `json:"location"`
	Details            string    `json:"details"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	Generation         int64     `json:"generation"`
	AssignedResourceID string    `json:"assignedResourceId,omitempty"`
}

type StatusCommand struct {
	Status string `json:"status"`
}
type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	OperationID string `json:"operationId,omitempty"`
}
