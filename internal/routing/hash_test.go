package routing

import "testing"

func TestShardForKnownIDs(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int
	}{
		{"00000000000000000000000000000000", 2},
		{"11111111111111111111111111111111", 2},
		{"ffffffffffffffffffffffffffffffff", 1},
	} {
		got, err := ShardFor(tc.id)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %d,%v want %d", tc.id, got, err, tc.want)
		}
	}
	for _, id := range []string{"", "123", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		if _, err := ShardFor(id); err == nil {
			t.Errorf("invalid id accepted %q", id)
		}
	}
}
