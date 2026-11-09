package routing

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

func ShardFor(id string) (int, error) {
	if len(id) != 32 {
		return 0, errors.New("invalid entity ID")
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return 0, errors.New("invalid entity ID")
		}
	}
	sum := sha256.Sum256([]byte(id))
	return int(binary.BigEndian.Uint64(sum[:8]) % 3), nil
}
