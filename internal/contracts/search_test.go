package contracts

import "testing"

func TestSearchRadiusBoundary(t *testing.T) {
	zero := 0.0
	q := SearchQuery{Type: "WATER", Location: &Location{Latitude: &zero, Longitude: &zero}, Limit: 20}
	for _, radius := range []float64{0, 4_000_000} {
		q.RadiusMeters = radius
		if q.Validate() != nil {
			t.Fatalf("radius %v rejected", radius)
		}
	}
	q.RadiusMeters = 4_000_000.001
	if q.Validate() == nil {
		t.Fatal("accepted search past 4,000 km")
	}
	if SearchRadii[0] != 0 || SearchRadii[len(SearchRadii)-1] != 4_000_000 {
		t.Fatal("wrong search bounds", SearchRadii)
	}
}
