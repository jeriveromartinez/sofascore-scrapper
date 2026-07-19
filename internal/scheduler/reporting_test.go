package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type persistingDownloadCounter struct {
	flushCalls     int
	reprocessCalls int
	persisted      bool
}

func (*persistingDownloadCounter) Increment(context.Context, uint) error { return nil }

func (c *persistingDownloadCounter) Flush(context.Context) error {
	c.flushCalls++
	c.persisted = true
	return nil
}

func (c *persistingDownloadCounter) ReprocessOrphans(context.Context) error {
	c.reprocessCalls++
	if !c.persisted {
		return errors.New("flush did not persist")
	}
	return nil
}

func TestAPKDownloadCounterJobFlushesWithoutOuterLock(t *testing.T) {
	counter := &persistingDownloadCounter{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	job := apkDownloadCounterJob(counter, logger)
	job()

	if counter.flushCalls != 1 {
		t.Fatalf("Flush calls = %d, want 1", counter.flushCalls)
	}
	if !counter.persisted {
		t.Fatal("Flush did not persist counters")
	}
	if counter.reprocessCalls != 1 {
		t.Fatalf("ReprocessOrphans calls = %d, want 1", counter.reprocessCalls)
	}
}
