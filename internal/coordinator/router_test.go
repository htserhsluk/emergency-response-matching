package coordinator

import (
	"context"
	"errors"
	"github.com/htserhsluk/emergency-response-matching/internal/contracts"
	"testing"
)

func TestMissingShardFailsClosed(t *testing.T) {
	router := NewRouter([3]string{}, "secret")
	if _, err := router.Get(context.Background(), "00000000000000000000000000000000"); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatalf("missing route returned %v", err)
	}
}
