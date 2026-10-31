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

func TestRenderEventsShowsTeamsScoresAndLeague(t *testing.T) {
	list := &pb.EventsList{Data: []*pb.ExternalEvent{
		{
			Id:              11,
			ExternalMatchId: "fotmob-4711",
			Sport:           "football",
			HomeScore:       2,
			AwayScore:       1,
			StartTimestamp:  time.Date(2026, 10, 20, 19, 30, 0, 0, time.UTC).UnixMilli(),
			StatusType:      "finished",
			TeamHome:        &pb.Team{TeamId: 100, Name: "Local"},
			TeamAway:        &pb.Team{TeamId: 200, Name: "Visitante"},
			League:          &pb.Tournament{Id: 900, Name: "Liga MX"},
		},
	}}

	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, list, time.UTC))

	text := out.String()
	assert.Contains(t, text, "Liga MX")
	assert.Contains(t, text, "Local")
	assert.Contains(t, text, "Visitante")
	assert.Contains(t, text, "2 - 1")
	assert.Contains(t, text, "football")
	assert.Contains(t, text, "19:30")
	assert.Contains(t, text, "finished")
}

func TestRenderEventsWithoutMatchesSaysSo(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, RenderEvents(&out, &pb.EventsList{}, time.UTC))
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
