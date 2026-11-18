package contracts

import (
	"encoding/json"
	"testing"
)

func TestSearchAcceptsExactLocationAndGlobalCoverage(t *testing.T) {
	for _, radius := range []float64{0, 1000, 20100000} {
		var q SearchQuery
		if err := json.Unmarshal([]byte(`{"type":"WATER","location":{"latitude":0,"longitude":0},"limit":20}`), &q); err != nil {
			t.Fatal(err)
		}
		q.RadiusMeters = radius
		if err := q.Validate(); err != nil {
			t.Fatalf("radius %v: %v", radius, err)
		}
	}
}
