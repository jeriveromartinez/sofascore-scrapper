package scraper

import "testing"

func TestLogoURLForSource(t *testing.T) {
	cases := []struct {
		name     string
		sourceID int64
		logoURL  string
		want     string
	}{
		{
			name:     "explicit_url_wins",
			sourceID: 9825,
			logoURL:  "https://cdn.example.com/team/9825.png",
			want:     "https://cdn.example.com/team/9825.png",
		},
		{
			name:     "empty_url_falls_back_to_sofascore_cdn",
			sourceID: 9825,
			logoURL:  "",
			want:     "https://img.sofascore.com/api/v1/team/9825/image",
		},
		{
			name:     "zero_id_with_empty_url_stays_empty",
			sourceID: 0,
			logoURL:  "",
			want:     "",
		},
		{
			name:     "scores365_prefix_is_stripped",
			sourceID: 7_000_000_000 + 123,
			logoURL:  "",
			want:     "https://img.sofascore.com/api/v1/team/123/image",
		},
		{
			name:     "scores365_prefix_overrides_url_does_not_applies_when_url_empty",
			sourceID: 7_000_000_000 + 456,
			logoURL:  "",
			want:     "https://img.sofascore.com/api/v1/team/456/image",
		},
		{
			name:     "sportsdb_legacy_prefix_is_stripped",
			sourceID: 2_000_000_000 + 789,
			logoURL:  "",
			want:     "https://img.sofascore.com/api/v1/team/789/image",
		},
		{
			name:     "natural_fotmob_id_unaffected",
			sourceID: 9825,
			logoURL:  "",
			want:     "https://img.sofascore.com/api/v1/team/9825/image",
		},
		{
			name:     "explicit_url_wins_over_prefix_stripping",
			sourceID: 7_000_000_000 + 123,
			logoURL:  "https://cdn.example.com/team/123.png",
			want:     "https://cdn.example.com/team/123.png",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LogoURLForSource(tc.sourceID, tc.logoURL)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
