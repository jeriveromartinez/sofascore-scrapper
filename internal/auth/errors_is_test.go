//go:build integration

package auth

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorsIs_AuditContract documents the contract that motivates
// this audit: a future PR that wraps one of our sentinel errors with
// fmt.Errorf("...: %w", sentinel) must not silently break the
// handler's error-comparison branches. The audit (see PR #61) replaces
// every `err == sentinel` with `errors.Is(err, sentinel)` precisely so
// the wrap is detected.
//
// This test does not exercise the production code path — the
// repository currently returns the sentinel unwrapped, so a handler
// test would pass even with the old `==` comparison. The point of
// the test is to pin the language-level contract we are depending
// on, so a Go toolchain change that broke errors.Is wrapping would
// fail this test loudly.
func TestErrorsIs_AuditContract(t *testing.T) {
	wrapped := fmt.Errorf("rotate: %w", ErrInvalidRefreshToken)

	// Sanity: == does not detect the wrapped sentinel. If this
	// assertion ever flips, the audit rationale is wrong.
	if wrapped == ErrInvalidRefreshToken {
		t.Fatal("== must not detect a wrapped sentinel — that is exactly the bug the audit fixes")
	}

	// errors.Is unwraps the chain and detects the sentinel.
	if !errors.Is(wrapped, ErrInvalidRefreshToken) {
		t.Fatal("errors.Is must detect the wrapped sentinel")
	}

	// Same check for the other sentinels touched by the audit.
	for _, sentinel := range []error{
		ErrInvalidRefreshToken,
		ErrInvalidInvitation,
		ErrInvitationExpired,
	} {
		w := fmt.Errorf("ctx: %w", sentinel)
		if !errors.Is(w, sentinel) {
			t.Errorf("errors.Is must detect wrapped %T", sentinel)
		}
	}
}
