package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The Flutter dashboard services give every request a 15 second budget.
func TestClientTimeoutMatchesDashboard(t *testing.T) {
	assert.Equal(t, 15*time.Second, NewClient("http://localhost:8080", "tok").http.Timeout)
}

// EventsApi.listNext calls GET /current-events without a limit, so the
// server default applies.
func TestCurrentEventsOmitsLimitWhenZero(t *testing.T) {
	var gotLimit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotLimit = r.URL.Query()["limit"]
		out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{{Id: 1}}})
		require.NoError(t, err)
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "tok").CurrentEvents(context.Background(), 0)
	require.NoError(t, err)
	assert.False(t, gotLimit, "limit must not be sent when zero")
}

func TestCurrentEventsSendsLimitWhenPositive(t *testing.T) {
	var gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{{Id: 1}}})
		require.NoError(t, err)
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "tok").CurrentEvents(context.Background(), 4)
	require.NoError(t, err)
	assert.Equal(t, "4", gotLimit)
}

// ProtoApi._decodeProto returns null for an empty body instead of an
// empty message.
func TestEmptyBodiesAreReportedAsNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")

	_, err := client.CurrentEvents(context.Background(), 0)
	require.True(t, errors.Is(err, ErrNoData), "events: %v", err)

	_, err = client.CheckUpdate(context.Background(), "1.0.0", "mx.qchotv.tvapp")
	require.True(t, errors.Is(err, ErrNoData), "update: %v", err)

	_, err = client.DeviceURL(context.Background(), "mx.qchotv.tvapp")
	require.True(t, errors.Is(err, ErrNoData), "url: %v", err)
}
