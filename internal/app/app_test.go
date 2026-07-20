package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/scheduler"
	"gorm.io/gorm"
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

	sched := scheduler.New(slog.Default())
	sched.Init(nil, nil, nil, nil)

	app := &App{
		HTTP:      srv,
		Scheduler: sched,
		logger:    slog.Default(),
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

	sched := scheduler.New(slog.Default())
	sched.Init(nil, nil, nil, nil)

	app := &App{
		HTTP:      srv,
		Scheduler: sched,
		logger:    slog.Default(),
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

func TestShutdownWaitsForLogoSchedulerBeforeClosingSQL(t *testing.T) {
	closed := make(chan struct{})
	driverName := fmt.Sprintf("shutdown-order-%d", shutdownDriverSequence.Add(1))
	sql.Register(driverName, &closeTrackingDriver{closed: closed})
	sqlDB, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open tracked SQL connection: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping tracked SQL connection: %v", err)
	}

	logos := &blockingLogoLifecycle{
		stopped:         make(chan struct{}),
		shutdownStarted: make(chan struct{}),
		release:         make(chan struct{}),
	}
	sched := scheduler.New(slog.Default())
	sched.Init(nil, nil, nil, nil)
	app := &App{
		HTTP:          &http.Server{},
		Scheduler:     sched,
		SQL:           sqlDB,
		logoScheduler: logos,
		logger:        slog.Default(),
	}

	done := make(chan error, 1)
	go func() { done <- app.shutdown() }()
	select {
	case <-logos.stopped:
	case <-time.After(time.Second):
		t.Fatal("logo scheduler did not stop accepting work")
	}
	select {
	case <-logos.shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("logo scheduler shutdown did not start")
	}
	select {
	case <-closed:
		t.Fatal("SQL closed before logo scheduler shutdown finished")
	default:
	}

	close(logos.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("application shutdown did not finish")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("SQL did not close after logo scheduler shutdown")
	}
}

var shutdownDriverSequence atomic.Uint64

type blockingLogoLifecycle struct {
	stopOnce        sync.Once
	shutdownOnce    sync.Once
	stopped         chan struct{}
	shutdownStarted chan struct{}
	release         chan struct{}
}

func (s *blockingLogoLifecycle) Stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *blockingLogoLifecycle) Schedule(*gorm.DB, int64, string) {}

func (s *blockingLogoLifecycle) Shutdown(ctx context.Context) {
	s.shutdownOnce.Do(func() { close(s.shutdownStarted) })
	select {
	case <-s.release:
	case <-ctx.Done():
	}
}

type closeTrackingDriver struct {
	closed chan struct{}
}

func (d *closeTrackingDriver) Open(string) (driver.Conn, error) {
	return &closeTrackingConn{closed: d.closed}, nil
}

type closeTrackingConn struct {
	closeOnce sync.Once
	closed    chan struct{}
}

func (c *closeTrackingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *closeTrackingConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *closeTrackingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}
