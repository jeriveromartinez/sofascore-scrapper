package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRunEventsPrintsTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/current-events", r.URL.Path)
		assert.Equal(t, "sim-token", r.Header.Get("APP-XIPTV"))

		out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{{
			Id:         11,
			Sport:      "football",
			TeamHome:   &pb.Team{Name: "Local"},
			TeamAway:   &pb.Team{Name: "Visitante"},
			HomeScore:  3,
			AwayScore:  0,
			StatusType: "finished",
		}}})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"events", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Local")
	assert.Contains(t, stdout.String(), "3 - 0")
}

func TestRunEventsJSONModePrintsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{{Id: 11, Sport: "football"}}})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"events", "--base-url", srv.URL, "--token", "sim-token", "--json"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
	assert.Contains(t, decoded, "data")
}

func TestRunRegisterSendsTheGivenToken(t *testing.T) {
	var gotToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/app/v1/devices", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req pb.DeviceRegisterRequest
		require.NoError(t, proto.Unmarshal(body, &req))
		gotToken = req.Token

		out, err := proto.Marshal(&pb.Device{Id: 7, Token: req.Token, PackageId: req.PackageId, IptvUrl: "https://panel.example.com"})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Equal(t, "sim-token", gotToken)
	assert.Contains(t, stdout.String(), "sim-token")
	assert.Contains(t, stdout.String(), "mx.qchotv.tvapp")
}

func TestRunRegisterGeneratesATokenWhenNoneIsGiven(t *testing.T) {
	var gotToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req pb.DeviceRegisterRequest
		require.NoError(t, proto.Unmarshal(body, &req))
		gotToken = req.Token

		out, err := proto.Marshal(&pb.Device{Id: 7, Token: req.Token})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.NotEmpty(t, gotToken)
	assert.Contains(t, stdout.String(), gotToken)
}

func TestRunWSPrintsReceivedPushes(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Hello{Hello: &pb.WsHello{DeviceId: 42}}})
		writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
			PushId:    99,
			MessageId: "msg-1",
			Category:  pb.PushCategory_PUSH_CATEGORY_EVENT_ALERT,
			Title:     "Gol",
			Body:      "Local 2 - 1 Visitante",
		}}})

		// Wait for the ack, then close.
		_, _, _ = conn.ReadMessage()
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"ws", "--base-url", srv.URL, "--token", "sim-token", "--duration", "2s"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Gol")
	assert.Contains(t, stdout.String(), "push #99")
}

func TestRunUnknownCommandFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"nope"}, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "unknown command")
}

func TestRunWithoutArgumentsPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "appcli")
}

func TestRunReportsServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid device registration"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "invalid device registration")
}

func TestRunSessionRunsTheWholeAppFlow(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/app/v1/devices":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var req pb.DeviceRegisterRequest
			require.NoError(t, proto.Unmarshal(body, &req))
			out, err := proto.Marshal(&pb.Device{Id: 7, Token: req.Token, IptvUrl: "https://panel.example.com"})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(out)
		case "/api/app/v1/update":
			out, err := proto.Marshal(&pb.ApkUpdateCheckResponse{UpdateAvailable: true, LatestVersion: "1.0.30"})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(out)
		case "/api/app/v1/current-events":
			out, err := proto.Marshal(&pb.EventsList{Data: []*pb.ExternalEvent{{
				Id: 11, Sport: "football", TeamHome: &pb.Team{Name: "Local"}, TeamAway: &pb.Team{Name: "Visitante"},
			}}})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(out)
		case "/api/app/v1/ws":
			conn, err := upgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer conn.Close()
			writeFrame(t, conn, &pb.WsFrame{Payload: &pb.WsFrame_Push{Push: &pb.WsPush{
				PushId: 99, MessageId: "msg-1", Title: "Gol", Body: "Golazo",
			}}})
			_, _, _ = conn.ReadMessage()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"session", "--base-url", srv.URL, "--duration", "2s"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	text := stdout.String()
	assert.Contains(t, text, "Local")
	assert.Contains(t, text, "1.0.30")
	assert.Contains(t, text, "Gol")
}

func TestRunWSStopsOnContextCancellation(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	code := runWithContext(ctx, []string{"ws", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
}

func TestRunWSRespectsTheDurationFlag(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	start := time.Now()
	var stdout, stderr bytes.Buffer
	code := run([]string{"ws", "--base-url", srv.URL, "--token", "sim-token", "--duration", "300ms"}, &stdout, &stderr)

	elapsed := time.Since(start)
	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond)
	assert.Less(t, elapsed, 5*time.Second)
}
