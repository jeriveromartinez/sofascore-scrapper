package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRegisterDevicePostsProtobufAndDecodesResponse(t *testing.T) {
	var gotReq pb.DeviceRegisterRequest
	var gotHeader http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		assert.Equal(t, "/api/app/v1/devices", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, proto.Unmarshal(body, &gotReq))

		out, err := proto.Marshal(&pb.Device{
			Id:        7,
			Token:     "sim-token",
			Platform:  "android",
			Name:      "appcli-sim",
			Version:   "1.0.22",
			PackageId: "mx.qchotv.tvapp",
			IptvUrl:   "https://panel.example.com",
			DomainId:  3,
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	dev, err := c.Register(context.Background(), &pb.DeviceRegisterRequest{
		Token:     "sim-token",
		Platform:  "android",
		Name:      "appcli-sim",
		Version:   "1.0.22",
		PackageId: "mx.qchotv.tvapp",
		Timezone:  "America/Mexico_City",
	})

	require.NoError(t, err)
	require.NotNil(t, dev)
	assert.Equal(t, uint32(7), dev.Id)
	assert.Equal(t, "sim-token", dev.Token)
	assert.Equal(t, "https://panel.example.com", dev.IptvUrl)
	assert.Equal(t, uint32(3), dev.DomainId)

	assert.Equal(t, "sim-token", gotReq.Token)
	assert.Equal(t, "android", gotReq.Platform)
	assert.Equal(t, "appcli-sim", gotReq.Name)
	assert.Equal(t, "1.0.22", gotReq.Version)
	assert.Equal(t, "mx.qchotv.tvapp", gotReq.PackageId)
	assert.Equal(t, "America/Mexico_City", gotReq.Timezone)

	assert.Equal(t, "sim-token", gotHeader.Get("APP-XIPTV"))
	assert.Equal(t, "application/x-protobuf", gotHeader.Get("Content-Type"))
	assert.Equal(t, "application/x-protobuf", gotHeader.Get("Accept"))
	assert.Equal(t, AppUserAgent, gotHeader.Get("User-Agent"))
}

func TestRegisterDeviceSurfacesServerErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid device registration"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sim-token")
	_, err := c.Register(context.Background(), &pb.DeviceRegisterRequest{Token: "sim-token"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid device registration")
	assert.Contains(t, err.Error(), "400")
}
