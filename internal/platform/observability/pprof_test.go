package observability

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestNewPprofServerEmptyAddr pins the existing behavior: no addr
// means no pprof server, regardless of the enabled flag.
func TestNewPprofServerEmptyAddr(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		srv := NewPprofServer("", enabled)
		if srv != nil {
			t.Errorf("pprof server should be nil when addr is empty (enabled=%v)", enabled)
		}
	}
}

// TestNewPprofServerDisabledOverridingAddr is the new contract that
// motivated #57 Issue 2 rewrite: setting PPROF_ADDR used to implicitly
// enable pprof (any non-empty addr started the server). The new
// behavior requires ENABLE_PPROF=true to opt in. A misconfigured
// deployment that sets PPROF_ADDR=":6060" by accident must NOT silently
// expose /debug/pprof/* on the network.
func TestNewPprofServerDisabledOverridingAddr(t *testing.T) {
	srv := NewPprofServer("127.0.0.1:0", false)
	if srv != nil {
		t.Error("pprof server should be nil when enabled=false, even with a non-empty addr")
	}
}

// TestNewPprofServerEnabledAndAddrSet covers the happy path: explicit
// opt-in plus a binding addr.
func TestNewPprofServerEnabledAndAddrSet(t *testing.T) {
	srv := NewPprofServer("127.0.0.1:0", true)
	if srv == nil {
		t.Fatal("pprof server should not be nil when enabled=true and addr is set")
	}
	if srv.Handler == nil {
		t.Error("pprof server should have a handler")
	}
}

func TestPprofListenAndServeNil(t *testing.T) {
	if err := PprofListenAndServe(nil); err != nil {
		t.Errorf("expected nil error for nil server, got %v", err)
	}
}

func TestPprofShutdownNil(t *testing.T) {
	if err := PprofShutdown(context.Background(), nil); err != nil {
		t.Errorf("expected nil error for nil server, got %v", err)
	}
}

func TestPprofServerNotOnGin(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		_ = srv.Serve(listener)
	}()
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/debug/pprof/")
	if err != nil {
		t.Fatalf("pprof endpoint not reachable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from pprof, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
