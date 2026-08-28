package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"time"
)

// NewPprofServer returns an *http.Server serving /debug/pprof/* on the
// given addr, or nil if either addr is empty or enabled is false.
//
// The enabled flag exists so that an operator who sets PPROF_ADDR out
// of habit does not silently expose pprof on the network — pprof must
// be opted in via ENABLE_PPROF=true. The default for that flag is
// false, so a misconfigured deployment stays closed.
func NewPprofServer(addr string, enabled bool) *http.Server {
	if !enabled || addr == "" {
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
