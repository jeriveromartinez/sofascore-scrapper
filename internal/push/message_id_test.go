package push

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNextMessageID_UUIDv4(t *testing.T) {
	id := nextMessageID()
	parsed, err := uuid.Parse(id)
	assert.NoError(t, err)
	assert.Equal(t, uuid.Version(4), parsed.Version())
}

func TestNextMessageID_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := nextMessageID()
		if seen[id] {
			t.Fatalf("duplicate id after %d iterations: %s", i, id)
		}
		seen[id] = true
	}
}
