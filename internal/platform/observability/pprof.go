package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"time"
)

func NewPprofServer(addr string) *http.Server {
	if addr == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func PprofListenAndServe(srv *http.Server) error {
	if srv == nil {
		return nil
	}
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func PprofShutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
