package app

import (
	"context"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scheduler"
)

func TestGracefulShutdown(t *testing.T) {
	baseline := runtime.NumGoroutine()
	t.Logf("goroutine baseline: %d", baseline)

	srv := &http.Server{
		Addr: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	sched := scheduler.New()
	sched.Init(nil, nil, nil, nil)

	app := &App{
		HTTP:      srv,
		Scheduler: sched,
	}
	app.ready.Store(true)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	if !app.IsReady() {
		t.Error("app should be ready after start")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}

	if app.IsReady() {
		t.Error("app should not be ready after shutdown")
	}

	time.Sleep(200 * time.Millisecond)

	got := runtime.NumGoroutine()
	if got > baseline+3 {
		t.Errorf("goroutine leak: baseline=%d, got=%d", baseline, got)
	}
}

func TestReadinessToggle(t *testing.T) {
	srv := &http.Server{
		Addr: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	sched := scheduler.New()
	sched.Init(nil, nil, nil, nil)

	app := &App{
		HTTP:      srv,
		Scheduler: sched,
	}
	app.ready.Store(true)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	if !app.IsReady() {
		t.Error("app should be ready")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	if app.IsReady() {
		t.Error("app should not be ready after cancellation")
	}
}
