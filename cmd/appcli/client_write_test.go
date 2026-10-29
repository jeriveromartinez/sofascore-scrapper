package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestReportViewingPostsProtobufPayload(t *testing.T) {
	var gotReq pb.LogPlaybackRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/devices/viewing", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, proto.Unmarshal(body, &gotReq))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	err := c.ReportViewing(context.Background(), &pb.LogPlaybackRequest{
		DeviceToken: "sim-token",
		Content:     "Y2hhbm5lbC1lc3Bu",
		StartedAt:   1750000000000,
	})

	require.NoError(t, err)
	assert.Equal(t, "sim-token", gotReq.DeviceToken)
	assert.Equal(t, "Y2hhbm5lbC1lc3Bu", gotReq.Content)
	assert.Equal(t, int64(1750000000000), gotReq.StartedAt)
}

func TestReportCrashPostsJSONPayload(t *testing.T) {
	var gotReport map[string]any
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/crash-report", r.URL.Path)
		gotContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotReport))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	err := c.ReportCrash(context.Background(), &CrashReport{
		Fatal:      true,
		Error:      "Null check operator used on a null value",
		StackTrace: "#0 main",
		Context:    "channel player",
		App:        AppReport{Name: "tvapp", Version: "1.0.22", Build: "22", Environment: "debug", Platform: "android"},
		Device:     DeviceReport{OsVersion: "Android 14", Locale: "es_MX"},
	})

	require.NoError(t, err)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, true, gotReport["fatal"])
	assert.Equal(t, "Null check operator used on a null value", gotReport["error"])
	assert.Equal(t, "#0 main", gotReport["stackTrace"])
	assert.Equal(t, "channel player", gotReport["context"])

	app, ok := gotReport["app"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tvapp", app["name"])
	assert.Equal(t, "1.0.22", app["version"])
	assert.Equal(t, "debug", app["environment"])

	device, ok := gotReport["device"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Android 14", device["osVersion"])
	assert.Equal(t, "es_MX", device["locale"])
}
