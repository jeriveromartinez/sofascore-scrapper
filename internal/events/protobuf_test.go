package events

import "testing"

func TestTeamToProtoAlwaysEmitsCanonicalLocalLogoURL(t *testing.T) {
	tests := []struct {
		name         string
		storedLogo   string
		wantLogoURL  string
	}{
		{
			name:        "empty stored URL still resolves to canonical local path",
			storedLogo:  "",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "external SofaScore URL is overridden by canonical local path",
			storedLogo:  "https://img.sofascore.com/api/v1/team/123/image",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "protocol-relative URL is overridden by canonical local path",
			storedLogo:  "//cdn.example/x",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "local relative URL is overridden by canonical local path",
			storedLogo:  "/teams/logo/123",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "already-prefixed local URL stays canonical (no double prefix)",
			storedLogo:  "/api/app/v1/teams/logo/123",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "api root URL is overridden by canonical local path",
			storedLogo:  "/api/app/v1",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
		{
			name:        "api prefix with trailing slash is overridden by canonical local path",
			storedLogo:  "/api/app/v1/",
			wantLogoURL: "/api/app/v1/teams/logo/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			team := &Team{TeamId: 123, LogoUrl: tt.storedLogo}

			first := TeamToProto(team)
			second := TeamToProto(team)

			if first.LogoUrl != tt.wantLogoURL {
				t.Fatalf("first.LogoUrl = %q, want %q", first.LogoUrl, tt.wantLogoURL)
			}
			if second.LogoUrl != tt.wantLogoURL {
				t.Fatalf("second.LogoUrl = %q, want %q", second.LogoUrl, tt.wantLogoURL)
			}
			if team.LogoUrl != tt.storedLogo {
				t.Fatalf("TeamToProto mutated source model: LogoUrl = %q, want %q", team.LogoUrl, tt.storedLogo)
			}
		})
	}
}

func TestTeamToProtoUsesTeamIDForCanonicalPath(t *testing.T) {
	cases := []struct {
		name   string
		teamID int64
		want   string
	}{
		{name: "normal id", teamID: 42, want: "/api/app/v1/teams/logo/42"},
		{name: "zero id", teamID: 0, want: "/api/app/v1/teams/logo/0"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := TeamToProto(&Team{TeamId: tt.teamID})
			if got.LogoUrl != tt.want {
				t.Fatalf("LogoUrl = %q, want %q", got.LogoUrl, tt.want)
			}
		})
	}
}

func TestEventToProtoHandlesMissingTeams(t *testing.T) {
	event := EventToProto(Event{})

	if event.TeamHome != nil {
		t.Fatalf("TeamHome = %#v, want nil", event.TeamHome)
	}
	if event.TeamAway != nil {
		t.Fatalf("TeamAway = %#v, want nil", event.TeamAway)
	}
}
