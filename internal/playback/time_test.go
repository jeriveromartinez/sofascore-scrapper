package playback

import (
	"testing"
	"time"
)

func TestNormalizeUnixMillis(t *testing.T) {
	now := func() time.Time { return time.UnixMilli(1_750_000_000_123) }
	cases := []struct {
		in      int64
		want    int64
		wantErr bool
	}{
		{0, 1_750_000_000_123, false},
		{1_750_000_000, 1_750_000_000_000, false},
		{1_750_000_000_123, 1_750_000_000_123, false},
		{-1, 0, true},
	}
	for _, tc := range cases {
		got, err := NormalizeUnixMillis(tc.in, now)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("in=%d got=%d err=%v", tc.in, got, err)
		}
	}
}

func TestNormalizeUnixMillisPreservesAlreadyMS(t *testing.T) {
	now := func() time.Time { return time.UnixMilli(0) }
	ms := int64(1_700_000_000_999)
	got, err := NormalizeUnixMillis(ms, now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != ms {
		t.Fatalf("got=%d want=%d", got, ms)
	}
}
