package tournaments

import (
	"context"
	"errors"
	"testing"
)

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestRepositoryHonoursCancelledContext(t *testing.T) {
	repo := NewRepository(newAssignmentsTestDB(t, 0, 1))
	ctx := cancelledContext()

	checks := map[string]func() error{
		"GetAll":  func() error { _, err := repo.GetAll(ctx); return err },
		"GetByID": func() error { _, err := repo.GetByID(ctx, 1); return err },
		"Create":  func() error { _, err := repo.Create(ctx, "Cup", "cup"); return err },
		"Update":  func() error { _, err := repo.Update(ctx, 1, "Cup", "cup"); return err },
		"Delete":  func() error { return repo.Delete(ctx, 1) },
	}
	for name, call := range checks {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s with cancelled ctx: want context.Canceled, got %v", name, err)
		}
	}
}

func TestDeviceAssignmentsRepositoryHonoursCancelledContext(t *testing.T) {
	repo := NewDeviceAssignmentsRepository(newAssignmentsTestDB(t, 1, 1))
	ctx := cancelledContext()

	checks := map[string]func() error{
		"GetAll":      func() error { _, err := repo.GetAll(ctx); return err },
		"GetByDevice": func() error { _, err := repo.GetByDevice(ctx, 1); return err },
		"Assign":      func() error { _, err := repo.Assign(ctx, 1, 1); return err },
		"Remove":      func() error { return repo.Remove(ctx, 1, 1) },
		"Set":         func() error { return repo.Set(ctx, 1, []uint{1}) },
	}
	for name, call := range checks {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s with cancelled ctx: want context.Canceled, got %v", name, err)
		}
	}
}
