package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCurrentEventsSendsLimitAndDecodesList(t *testing.T) {
	var gotLimit string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/current-events", r.URL.Path)
		gotLimit = r.URL.Query().Get("limit")

		out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{
			{
				Id:              11,
				ExternalMatchId: "fotmob-4711",
				Sport:           "football",
				HomeScore:       2,
				AwayScore:       1,
				StartTimestamp:  1750000000,
				StatusType:      "finished",
				TeamHome:        &pb.Team{Id: 1, TeamId: 100, Name: "Local"},
				TeamAway:        &pb.Team{Id: 2, TeamId: 200, Name: "Visitante"},
				League:          &pb.Tournament{Id: 900, Name: "Liga MX"},
			},
		}})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	list, err := c.CurrentEvents(context.Background(), 3)

	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "3", gotLimit)
	assert.Equal(t, uint32(11), list.Data[0].Id)
	assert.Equal(t, "fotmob-4711", list.Data[0].ExternalMatchId)
	assert.Equal(t, "Local", list.Data[0].TeamHome.Name)
	assert.Equal(t, "Visitante", list.Data[0].TeamAway.Name)
	assert.Equal(t, int32(2), list.Data[0].HomeScore)
	assert.Equal(t, int32(1), list.Data[0].AwayScore)
	assert.Equal(t, "Liga MX", list.Data[0].League.Name)
}

func TestCheckUpdateSendsVersionAndPackage(t *testing.T) {
	var gotQuery map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/update", r.URL.Path)
		gotQuery = map[string]string{
			"version": r.URL.Query().Get("version"),
			"package": r.URL.Query().Get("package"),
		}

		out, err := proto.Marshal(&pb.ApkUpdateCheckResponse{
			UpdateAvailable: true,
			LatestVersion:   "1.0.30",
			PackageName:     "mx.qchotv.tvapp",
			VersionCode:     30,
			DownloadUrl:     "/api/app/v1/apk/download/token-1",
			FileSize:        1234,
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	resp, err := c.CheckUpdate(context.Background(), "1.0.22", "mx.qchotv.tvapp")

	require.NoError(t, err)
	assert.Equal(t, "1.0.22", gotQuery["version"])
	assert.Equal(t, "mx.qchotv.tvapp", gotQuery["package"])
	assert.True(t, resp.UpdateAvailable)
	assert.Equal(t, "1.0.30", resp.LatestVersion)
	assert.Equal(t, int32(30), resp.VersionCode)
	assert.Equal(t, "/api/app/v1/apk/download/token-1", resp.DownloadUrl)
}

func TestDeviceURLDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/devices/url/mx.qchotv.tvapp", r.URL.Path)

		out, err := proto.Marshal(&pb.DeviceUrl{Url: "https://panel.example.com"})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	panelURL, err := c.DeviceURL(context.Background(), "mx.qchotv.tvapp")

	require.NoError(t, err)
	assert.Equal(t, "https://panel.example.com", panelURL)
}
