package pagination

import (
	"errors"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	values := []string{"test@example.com", "42"}
	encoded, err := Encode(values...)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := Decode(encoded, 2)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded) != 2 || decoded[0] != values[0] || decoded[1] != values[1] {
		t.Errorf("round trip mismatch: got %v, want %v", decoded, values)
	}
}

func TestDecodeEmptyCursor(t *testing.T) {
	decoded, err := Decode("", 0)
	if err != nil {
		t.Fatalf("Decode empty cursor failed: %v", err)
	}
	if decoded != nil {
		t.Errorf("expected nil, got %v", decoded)
	}
}

func TestDecodeEmptyCursorWrongArity(t *testing.T) {
	_, err := Decode("", 2)
	if err == nil {
		t.Fatal("expected error for empty cursor with non-zero arity")
	}
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestDecodeMalformedBase64(t *testing.T) {
	_, err := Decode("!!!not-valid-base64!!!", 1)
	if err == nil {
		t.Fatal("expected error for malformed base64")
	}
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestDecodeMalformedJSON(t *testing.T) {
	_, err := Decode("bm90LWpzb24=", 1)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestDecodeWrongVersion(t *testing.T) {
	encoded, err := Encode("value")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	encoded = "eyJ2Ijo5OSwia"
	_, err = Decode(encoded, 1)
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestDecodeWrongArity(t *testing.T) {
	encoded, err := Encode("value")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	_, err = Decode(encoded, 3)
	if err == nil {
		t.Fatal("expected error for wrong arity")
	}
	if !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestEncodeEmpty(t *testing.T) {
	encoded, err := Encode()
	if err != nil {
		t.Fatalf("Encode empty failed: %v", err)
	}

	decoded, err := Decode(encoded, 0)
	if err != nil {
		t.Fatalf("Decode empty failed: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("expected zero values, got %d", len(decoded))
	}
}
