package contracts

const MaxMatchDistanceMeters = 4_000_000

var SearchRadii = [...]float64{0, 1000, 5000, 10000, 25000, 50000, 100000, 250000, 500000, 1000000, 2500000, MaxMatchDistanceMeters}

type SearchProgress struct {
	RadiusIndex   int     `json:"radiusIndex"`
	AfterID       string  `json:"afterId,omitempty"`
	AfterDistance float64 `json:"afterDistance"`
}

func (p SearchProgress) Validate() error {
	if p.RadiusIndex < 0 || p.RadiusIndex > len(SearchRadii) {
		return ErrInvalid
	}
	q := SearchQuery{Type: "WATER", Location: &Location{Latitude: new(float64), Longitude: new(float64)}, Limit: 20, AfterID: p.AfterID, AfterDistance: p.AfterDistance}
	if p.RadiusIndex < len(SearchRadii) {
		q.RadiusMeters = SearchRadii[p.RadiusIndex]
	}
	if p.RadiusIndex == len(SearchRadii) && (p.AfterID != "" || p.AfterDistance != 0) {
		return ErrInvalid
	}
	return q.Validate()
}
