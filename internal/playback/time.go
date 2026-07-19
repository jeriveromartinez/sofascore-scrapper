package playback

import (
	"errors"
	"math"
	"time"
)

var ErrInvalidTimestamp = errors.New("invalid timestamp")

func NormalizeUnixMillis(value int64, now func() time.Time) (int64, error) {
	if value < 0 {
		return 0, ErrInvalidTimestamp
	}
	if value == 0 {
		return now().UnixMilli(), nil
	}
	if value < 1_000_000_000_000 {
		if value > math.MaxInt64/1000 {
			return 0, ErrInvalidTimestamp
		}
		return value * 1000, nil
	}
	return value, nil
}
