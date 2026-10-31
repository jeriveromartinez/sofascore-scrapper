package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderEventsMatchesDashboardSlider(t *testing.T) {
	list := &pb.EventsList{Data: []*pb.ExternalEvent{
		{
			Id:              11,
			ExternalMatchId: "fotmob-4711",
			Sport:           "football",
			HomeScore:       2,
			AwayScore:       1,
			StartTimestamp:  time.Date(2026, 10, 20, 19, 30, 0, 0, time.UTC).UnixMilli(),
			StatusType:      "finished",
			TeamHome:        &pb.Team{TeamId: 100, Name: "Local", PrimaryColor: "#123456", LogoUrl: "/api/app/v1/teams/logo/100"},
			TeamAway:        &pb.Team{TeamId: 200, Name: "Visitante", PrimaryColor: "", LogoUrl: "/api/app/v1/teams/logo/200"},
			League:          &pb.Tournament{Id: 900, Name: "Liga MX"},
		},
	}}

	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, list, time.UTC, "https://panel.test"))

	text := out.String()
	assert.Contains(t, text, "[20/10 19:30] Local vs Visitante")
	assert.Contains(t, text, "Liga MX")
	assert.Contains(t, text, "color: 0xFF123456")
	assert.Contains(t, text, "color: 0xFF888888")
	assert.Contains(t, text, "https://panel.test/api/app/v1/teams/logo/100")
	assert.Contains(t, text, "https://panel.test/api/app/v1/teams/logo/200")
	assert.Contains(t, text, "2 - 1")
	assert.Contains(t, text, "football")
	assert.Contains(t, text, "finished")
}

func TestRenderEventsLabelsToday(t *testing.T) {
	now := time.Now().In(time.UTC)
	start := time.Date(now.Year(), now.Month(), now.Day(), 21, 5, 0, 0, time.UTC)
	list := &pb.EventsList{Data: []*pb.ExternalEvent{{
		StartTimestamp: start.UnixMilli(),
		StatusType:     "inprogress",
		TeamHome:       &pb.Team{Name: "Local"},
		TeamAway:       &pb.Team{Name: "Visitante"},
		League:         &pb.Tournament{Name: "Liga MX"},
	}}}

	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, list, time.UTC, "http://localhost:8080"))
	assert.Contains(t, out.String(), "[hoy 21:05] Local vs Visitante")
}

func TestRenderEventsUsesTheDeviceTimezone(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)

	list := &pb.EventsList{Data: []*pb.ExternalEvent{{
		StartTimestamp: time.Date(2026, 10, 30, 20, 15, 0, 0, time.UTC).UnixMilli(),
		TeamHome:       &pb.Team{Name: "Local"},
		TeamAway:       &pb.Team{Name: "Visitante"},
	}}}

	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, list, mx, "http://localhost:8080"))
	assert.Contains(t, out.String(), "[30/10 14:15]")
}

func TestDashboardColorFallsBackToGray(t *testing.T) {
	assert.Equal(t, "0xFF123456", dashboardColor("#123456"))
	assert.Equal(t, "0xFF888888", dashboardColor(""))
	assert.Equal(t, "0xFF888888", dashboardColor("#zzzzzz"))
	assert.Equal(t, "0x00FFFFFF", dashboardColor("FFFFFF"))
}

func TestRenderEventsWithoutMatchesSaysSo(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, &pb.EventsList{}, time.UTC, "http://localhost:8080"))
	assert.Contains(t, out.String(), "sin eventos")
}

func TestRenderPushShowsTitleBodyAndAckStatus(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, RenderPush(&out, PushEvent{
		PushID:    99,
		MessageID: "msg-1",
		Category:  pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
		Title:     "Gol",
		Body:      "Local 2 - 1 Visitante",
		SentAt:    1750000000123,
	}, true))

	text := out.String()
	assert.Contains(t, text, "push #99")
	assert.Contains(t, text, "Gol")
	assert.Contains(t, text, "Local 2 - 1 Visitante")
	assert.Contains(t, text, "EVENT_ALERT")
	assert.Contains(t, text, "ack")
}

func TestMarshalJSONIsStableAndReadable(t *testing.T) {
	list := &pb.EventsList{Data: []*pb.ExternalEvent{{Id: 11, Sport: "football", TeamHome: &pb.Team{Name: "Local"}}}}

	first, err := MarshalJSON(list)
	require.NoError(t, err)
	second, err := MarshalJSON(list)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(first, &decoded))
	data, ok := decoded["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	entry := data[0].(map[string]any)
	assert.Equal(t, "football", entry["sport"])
}
