package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// registerServer answers a device registration and returns the request
// the simulator sent.
func registerServer(t *testing.T, answer *pb.Device) (*httptest.Server, *pb.DeviceRegisterRequest) {
	t.Helper()
	got := &pb.DeviceRegisterRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, proto.Unmarshal(body, got))

		out, err := proto.Marshal(answer)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestRunRegisterSendsTheDefaultDeviceProfile(t *testing.T) {
	srv, got := registerServer(t, &pb.Device{Id: 7})

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Equal(t, "sim-token", got.Token)
	assert.Equal(t, "android", got.Platform)
	assert.Equal(t, "appcli-sim", got.Name)
	assert.Equal(t, "1.0.22", got.Version)
	assert.Equal(t, "mx.qchotv.tvapp", got.PackageId)
	assert.Equal(t, "America/Mexico_City", got.Timezone)
}

func TestRunRegisterSendsTheDomainIDWhenItIsGiven(t *testing.T) {
	srv, got := registerServer(t, &pb.Device{Id: 7})

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token", "--domain-id", "9"}, &stdout, &stderr)

	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Equal(t, uint32(9), got.DomainId)
}

func TestRunRegisterOmitsTheDomainIDByDefault(t *testing.T) {
	srv, got := registerServer(t, &pb.Device{Id: 7})

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Equal(t, uint32(0), got.DomainId)
}

func TestRunRegisterKeepsTheTimezoneGivenByTheUser(t *testing.T) {
	srv, got := registerServer(t, &pb.Device{Id: 7})

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token", "--timezone", "Europe/Madrid"}, &stdout, &stderr)

	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.Equal(t, "Europe/Madrid", got.Timezone)
}

func TestRunRegisterPrintsTheWholeDevice(t *testing.T) {
	srv, _ := registerServer(t, &pb.Device{
		Id:        7,
		Token:     "sim-token",
		Platform:  "android",
		Name:      "appcli-sim",
		Version:   "1.0.22",
		PackageId: "mx.qchotv.tvapp",
		Timezone:  "America/Mexico_City",
		LastSeen:  1700000000,
		CreatedAt: "2026-10-29 10:00:00",
		UpdatedAt: "2026-10-29 10:00:00",
		DomainId:  3,
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"register", "--base-url", srv.URL, "--token", "sim-token"}, &stdout, &stderr)

	require.Equal(t, 0, code, "stderr: %s", stderr.String())
	text := stdout.String()
	assert.Contains(t, text, "America/Mexico_City")
	assert.Contains(t, text, "1700000000")
	assert.Contains(t, text, "2026-10-29 10:00:00")
}
