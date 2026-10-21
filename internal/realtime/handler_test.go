package realtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAckHandler_TwoArguments(t *testing.T) {
	var captured string
	var capturedCtx context.Context
	handler := AckHandler(func(ctx context.Context, messageID string) {
		capturedCtx = ctx
		captured = messageID
	})
	handler(context.Background(), "0190f8e4-1f5d-7c2e-9a4b-3e6f8c2d9e1a")
	assert.Equal(t, "0190f8e4-1f5d-7c2e-9a4b-3e6f8c2d9e1a", captured)
	assert.NotNil(t, capturedCtx)
}
