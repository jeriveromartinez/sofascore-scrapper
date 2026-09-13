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
