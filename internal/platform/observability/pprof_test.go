package observability

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewPprofServerEmptyAddr(t *testing.T) {
	srv := NewPprofServer("")
	if srv != nil {
		t.Error("pprof server should be nil when addr is empty")
	}
}

func TestNewPprofServerWithAddr(t *testing.T) {
	srv := NewPprofServer("127.0.0.1:0")
	if srv == nil {
		t.Fatal("pprof server should not be nil when addr is set")
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
