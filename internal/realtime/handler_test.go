package realtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAckHandler_SingleArgument(t *testing.T) {
	var captured string
	handler := AckHandler(func(messageID string) {
		captured = messageID
	})
	handler("0190f8e4-1f5d-7c2e-9a4b-3e6f8c2d9e1a")
	assert.Equal(t, "0190f8e4-1f5d-7c2e-9a4b-3e6f8c2d9e1a", captured)
}
