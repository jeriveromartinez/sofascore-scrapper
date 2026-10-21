package server

import "testing"

func TestParseID(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    uint
		wantErr bool
	}{
		{name: "plain auto-increment id", raw: "67", want: 67},
		{name: "zero", raw: "0", want: 0},
		{name: "last value that fits in uint32", raw: "4294967295", want: 4294967295},
		// Regression: the tournaments cursor carries the row id, and
		// 365scores competitions live at scores365.LeagueIDPrefix
		// (6e9) + comp id. A 32-bit parse rejected these with
		// "value out of range", surfacing as
		// "invalid cursor: bad id" when paging.
		{name: "365scores league id above uint32", raw: "6000007271", want: 6000007271},
		{name: "team id above uint32", raw: "7000097718", want: 7000097718},
		{name: "empty", raw: "", wantErr: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "not a number", raw: "abc", wantErr: true},
		{name: "overflows uint64", raw: "18446744073709551616", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseID(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseID(%q) = %d, want error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseID(%q) returned error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseID(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
