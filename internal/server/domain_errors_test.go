package server

import (
	"errors"
	"testing"
)

func TestTypedErrorsAreSentinel(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound is not a sentinel")
	}
	if !errors.Is(ErrConflict, ErrConflict) {
		t.Error("ErrConflict is not a sentinel")
	}
	if !errors.Is(ErrUnavailable, ErrUnavailable) {
		t.Error("ErrUnavailable is not a sentinel")
	}
}

func TestTypedErrorsAreDistinct(t *testing.T) {
	if errors.Is(ErrNotFound, ErrConflict) {
		t.Error("ErrNotFound should not be ErrConflict")
	}
	if errors.Is(ErrNotFound, ErrUnavailable) {
		t.Error("ErrNotFound should not be ErrUnavailable")
	}
	if errors.Is(ErrConflict, ErrUnavailable) {
		t.Error("ErrConflict should not be ErrUnavailable")
	}
}

func TestTypedErrorMessages(t *testing.T) {
	if ErrNotFound.Error() != "not found" {
		t.Errorf("ErrNotFound message: want 'not found', got '%s'", ErrNotFound.Error())
	}
	if ErrConflict.Error() != "conflict" {
		t.Errorf("ErrConflict message: want 'conflict', got '%s'", ErrConflict.Error())
	}
	if ErrUnavailable.Error() != "dependency unavailable" {
		t.Errorf("ErrUnavailable message: want 'dependency unavailable', got '%s'", ErrUnavailable.Error())
	}
}

func TestErrorsAreCloneable(t *testing.T) {
	e1 := errors.New(ErrNotFound.Error())
	e2 := errors.New(ErrNotFound.Error())
	if e1.Error() != e2.Error() {
		t.Error("cloned errors should have same message")
	}
}
