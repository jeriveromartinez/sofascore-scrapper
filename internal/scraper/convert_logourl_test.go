package scraper

import "testing"

const (
	fotmobLogo9825 = "https://images.fotmob.com/image_resources/logo/teamlogo/9825.png"
	scores365Logo  = "https://imagecache.365scores.com/image/upload/f_png,w_68,h_68,c_limit,q_auto:eco,d_Competitors:default1.png/v1/Competitors/"
)

func TestLogoURLForSource(t *testing.T) {
	cases := []struct {
		name     string
		sourceID int64
		logoURL  string
		want     string
	}{
		{
			// The 365scores source fills LogoURL in from the
			// competitor's own badge, so this is the common path.
			name:     "explicit_url_wins",
			sourceID: 9825,
			logoURL:  "https://cdn.example.com/team/9825.png",
			want:     "https://cdn.example.com/team/9825.png",
		},
		{
			// FotMob's /api/data/matches payload carries no badge
			// field, so its teams fall back to the FotMob CDN.
			name:     "empty_url_falls_back_to_fotmob_cdn",
			sourceID: 9825,
			logoURL:  "",
			want:     fotmobLogo9825,
		},
		{
			name:     "zero_id_with_empty_url_stays_empty",
			sourceID: 0,
			logoURL:  "",
			want:     "",
		},
		{
			// The shared `teams` table namespaces IDs per source, but
			// each CDN is keyed by the upstream's natural ID, so the
			// prefix has to come off before the URL is built.
			name:     "scores365_prefix_is_stripped",
			sourceID: 7_000_000_000 + 123,
			logoURL:  "",
			want:     scores365Logo + "123",
		},
		{
			name:     "scores365_second_id",
			sourceID: 7_000_000_000 + 456,
			logoURL:  "",
			want:     scores365Logo + "456",
		},
		{
			// TheSportsDB was removed; rows left in its namespace by a
			// long-lived database have no upstream left to fetch from.
			name:     "sportsdb_legacy_namespace_has_no_url",
			sourceID: 2_000_000_000 + 789,
			logoURL:  "",
			want:     "",
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
