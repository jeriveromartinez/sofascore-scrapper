package events

import "testing"

func TestEventToProtoNormalizesLogoURLsWithoutMutatingModels(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "external SofaScore URL",
			url:  "https://img.sofascore.com/api/v1/team/123/image",
			want: "https://img.sofascore.com/api/v1/team/123/image",
		},
		{
			name: "local relative URL",
			url:  "/teams/logo/123",
			want: "/api/app/v1/teams/logo/123",
		},
		{
			name: "already prefixed local URL",
			url:  "/api/app/v1/teams/logo/123",
			want: "/api/app/v1/teams/logo/123",
		},
		{
			name: "already prefixed root URL",
			url:  "/api/app/v1",
			want: "/api/app/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			team := &Team{TeamId: 123, LogoUrl: tt.url}
			event := Event{HomeTeamModel: team}

			first := EventToProto(event)
			second := EventToProto(event)

			if first.TeamHome.LogoUrl != tt.want {
				t.Fatalf("first logo URL = %q, want %q", first.TeamHome.LogoUrl, tt.want)
			}
			if second.TeamHome.LogoUrl != tt.want {
				t.Fatalf("second logo URL = %q, want %q", second.TeamHome.LogoUrl, tt.want)
			}
			if team.LogoUrl != tt.url {
				t.Fatalf("EventToProto mutated model URL from %q to %q", tt.url, team.LogoUrl)
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
