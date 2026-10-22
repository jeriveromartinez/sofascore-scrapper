package realtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// silentRedis completes the connection handshake (HELLO is refused so
// go-redis falls back to RESP2, everything else gets +OK) but never
// confirms SUBSCRIBE, like a wedged Redis or a proxy whose backend is
// gone. closed receives one value per connection once the client
// closes it.
func silentRedis(t *testing.T) (addr string, closed <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	closedCh := make(chan struct{}, 16)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				serveSilentRedis(conn)
				closedCh <- struct{}{}
			}()
		}
	}()
	return ln.Addr().String(), closedCh
}

func serveSilentRedis(conn net.Conn) {
	rd := bufio.NewReader(conn)
	for {
		args, err := readRESPCommand(rd)
		if err != nil {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "SUBSCRIBE":
		case "HELLO":
			_, _ = conn.Write([]byte("-ERR unknown command 'HELLO'\r\n"))
		default:
			_, _ = conn.Write([]byte("+OK\r\n"))
		}
	}
}

func readRESPCommand(rd *bufio.Reader) ([]string, error) {
	header, err := rd.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
	if err != nil || n < 1 {
		return nil, fmt.Errorf("bad RESP array header %q", header)
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lenLine, err := rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(lenLine, "$")))
		if err != nil {
			return nil, fmt.Errorf("bad RESP bulk header %q", lenLine)
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(rd, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}

// TestSubscriberStartGivesUpOnSilentRedis guards the shutdown hang: the
// app ctx has no deadline, so waiting for the SUBSCRIBE confirmation
// on it blocked Start forever against a Redis that never answers.
func TestSubscriberStartGivesUpOnSilentRedis(t *testing.T) {
	addr, closed := silentRedis(t)
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	sub := NewSubscriber(client, NewHub(), SubscriberConfig{
		Channel: "push:fanout:silent",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	done := make(chan error, 1)
	go func() { done <- sub.Start(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Start succeeded against a Redis that never answered")
		}
	case <-time.After(subscribeConfirmTimeout + 5*time.Second):
		t.Fatal("Start blocked waiting for a SUBSCRIBE confirmation that never comes")
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the failed subscription's connection was never closed")
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSubscriberLogsExitOnContextCancel(t *testing.T) {
	client, _ := newTestRedis(t)
	logs := &lockedBuffer{}
	sub := NewSubscriber(client, NewHub(), SubscriberConfig{
		Channel: "push:fanout:cancel-log",
		Logger:  slog.New(slog.NewTextHandler(logs, nil)),
	})
	exited := make(chan struct{})
	sub.OnExit(func() { close(exited) })

	ctx, cancel := context.WithCancel(context.Background())
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	cancel()

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not exit after ctx cancel")
	}
	if !strings.Contains(logs.String(), "context canceled") {
		t.Fatalf("subscriber exit on ctx cancel was not logged, got: %q", logs.String())
	}
}
