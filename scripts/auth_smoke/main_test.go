package main

import (
	"strings"
	"testing"
)

func TestPreviewNeverEchoesTheCredential(t *testing.T) {
	const secret = "eyJhbGciOiJIUzI1NiJ9.payload.signature"

	got := preview(secret)

	if strings.Contains(got, secret) {
		t.Fatalf("preview leaked the credential: %q", got)
	}
	for _, part := range strings.Split(secret, ".") {
		if part != "" && strings.Contains(got, part) {
			t.Fatalf("preview leaked part of the credential %q: %q", part, got)
		}
	}
	if !strings.Contains(got, "<redacted") {
		t.Fatalf("preview = %q, want a redaction marker", got)
	}
}

func TestPreviewIsStableAndDistinguishesTokens(t *testing.T) {
	a := preview("token-a")
	b := preview("token-b")

	if a != preview("token-a") {
		t.Fatalf("preview is not stable: %q vs %q", a, preview("token-a"))
	}
	if a == b {
		t.Fatalf("preview does not distinguish two tokens: %q", a)
	}
}

func TestPreviewOfEmptyToken(t *testing.T) {
	if got := preview(""); got != "<empty>" {
		t.Fatalf("preview(\"\") = %q, want <empty>", got)
	}
}
