//go:build integration

package realtime

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

// setupWSTest wires a real httptest.Server with the realtime handler
// and a GORM DB seeded with one device. It returns the server URL,
// the hub, the device token, and a cleanup function.
func setupWSTest(t *testing.T) (string, *Hub, string, func()) {
	t.Helper()

	id := atomic.AddInt64(&wsTestCounter, 1)
	dsn := fmt.Sprintf("file:test_ws_%s_%d?mode=memory&cache=shared", t.Name(), id)
	db, err := gorm.Open(openDriver(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &domains.Domain{}, &devices.Device{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	u := &users.User{Email: "u@x.com", Password: "x", Role: users.RoleUser}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	d := &domains.Domain{Domain: "client.iptv.example", UserID: u.ID}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	didP := d.ID
	uidP := u.ID
	dev := &devices.Device{
		UserID:   &uidP,
		DomainID: &didP,
		Token:    fmt.Sprintf("tok-%d", id),
		Platform: "android",
		Name:     "Test Box",
		Version:  "1.0",
	}
	if err := db.Create(dev).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}

	hub := NewHub()
	auth := NewAuthenticator(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/app/v1/ws", Handler(HandlerConfig{
		Authenticator: auth,
		Hub:           hub,
		Logger:        slog.Default(),
		AckHandler:    func(string) {},
	}))

	srv := httptest.NewServer(r)
	cleanup := func() {
		srv.Close()
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
	return srv.URL, hub, dev.Token, cleanup
}

var wsTestCounter int64

// TestWSHandler_AcceptsValidTokenAndSendsHello is the canonical
// end-to-end check: a real gorilla client dials the endpoint, sends
// the APP-XIPTV header, the server upgrades, and the client
// receives a WsHello with the device_id echoed back. The hub
// registers the connection in the same call.
func TestWSHandler_AcceptsValidTokenAndSendsHello(t *testing.T) {
	wsURL, hub, token, cleanup := setupWSTest(t)
	defer cleanup()

	header := http.Header{}
	header.Set(headerAppXIPTV, token)

	u, _ := url.Parse(wsURL)
	u.Scheme = "ws"
	u.Path = "/api/app/v1/ws"

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		t.Fatalf("dial: %v (status=%d)", err, statusOf(resp))
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	frame, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	hello := frame.GetHello()
	if hello == nil {
		t.Fatalf("first frame is not Hello: %T", frame.Payload)
	}
	if hello.DeviceId == 0 {
		t.Errorf("DeviceId = 0, want non-zero")
	}
	if hello.ServerTime == 0 {
		t.Errorf("ServerTime = 0, want non-zero")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.Count() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hub.Count() != 1 {
		t.Errorf("hub.Count = %d, want 1", hub.Count())
	}
}

// TestWSHandler_RejectsInvalidToken covers the 4401 contract: a
// request with a bogus APP-XIPTV must not upgrade. The server
// returns 401 and the client sees a non-101 status.
func TestWSHandler_RejectsInvalidToken(t *testing.T) {
	wsURL, hub, _, cleanup := setupWSTest(t)
	defer cleanup()

	u, _ := url.Parse(wsURL)
	u.Scheme = "ws"
	u.Path = "/api/app/v1/ws"

	header := http.Header{}
	header.Set(headerAppXIPTV, "not-a-real-token")

	_, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err == nil {
		t.Fatal("expected error dialing with invalid token")
	}
	if resp == nil {
		t.Fatalf("dial returned no response (err=%v)", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if hub.Count() != 0 {
		t.Errorf("hub.Count = %d, want 0 (rejected conn must not register)", hub.Count())
	}
}

// TestWSHandler_AcceptsTokenInQuery covers the ?token= fallback for
// clients that cannot set the APP-XIPTV header (embedded web views,
// some proxies).
func TestWSHandler_AcceptsTokenInQuery(t *testing.T) {
	wsURL, hub, token, cleanup := setupWSTest(t)
	defer cleanup()

	u, _ := url.Parse(wsURL)
	u.Scheme = "ws"
	u.Path = "/api/app/v1/ws"
	u.RawQuery = "token=" + token

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial with query token: %v (status=%d)", err, statusOf(resp))
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	// Same poll-then-assert dance as the header test: the upgrade
	// handler registers synchronously, but the goroutine scheduling
	// can race the assertion on slow CI.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.Count() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hub.Count() != 1 {
		t.Errorf("hub.Count = %d, want 1", hub.Count())
	}
}

// statusOf extracts the status code from a possibly-nil *http.Response.
func statusOf(r *http.Response) int {
	if r == nil {
		return 0
	}
	return r.StatusCode
}
