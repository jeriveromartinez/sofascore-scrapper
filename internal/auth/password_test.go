package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidatePassword_AcceptsStrongPassword(t *testing.T) {
	cases := []string{
		"correct-horse-battery-staple",
		"sup3rL0ngP@ssw0rd!",
		"this is a long passphrase",
		"1234567890ab", // exactly 12 chars, digits only
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidatePassword(p); err != nil {
				t.Errorf("ValidatePassword(%q) = %v, want nil", p, err)
			}
		})
	}
}

func TestValidatePassword_RejectsTooShort(t *testing.T) {
	cases := []string{
		"",
		"a",
		"abcdefghijk",  // 11 chars
		"   trim me  ", // 11 chars after trim
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidatePassword(p); err == nil {
				t.Errorf("ValidatePassword(%q) = nil, want error", p)
			}
		})
	}
}

func TestValidatePassword_RejectsCommonPasswords(t *testing.T) {
	// Each entry is 12+ chars so the only reason it should be rejected
	// is membership in the common-password list.
	cases := []string{
		"password1234",
		"qwerty123456",
		"letmein12345",
		"welcome12345",
		"iloveyou12345",
		"admin1234567",
		"changeme12345",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidatePassword(p); err == nil {
				t.Errorf("ValidatePassword(%q) = nil, want error (common password)", p)
			}
		})
	}
}

func TestValidatePassword_NormalizesWhitespace(t *testing.T) {
	// Leading/trailing whitespace should be trimmed before validation.
	if err := ValidatePassword("  mysecretpassword  "); err != nil {
		t.Errorf("ValidatePassword with surrounding spaces = %v, want nil", err)
	}
}

func TestHashPassword_UsesBcryptCost12(t *testing.T) {
	// bcrypt cost is encoded as a 2-digit prefix in the $2a$XX$ hash.
	// Cost 12 is the project target.
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("hash does not look like bcrypt: %q", hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) < 4 {
		t.Fatalf("malformed bcrypt hash: %q", hash)
	}
	if parts[2] != "12" {
		t.Errorf("bcrypt cost = %q, want %q", parts[2], "12")
	}
}

func TestHashPassword_RoundTripsThroughCheck(t *testing.T) {
	const p = "correct-horse-battery-staple"
	hash, err := HashPassword(p)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, p) {
		t.Error("CheckPassword against own hash = false, want true")
	}
}

func TestVerifyAndUpgrade_KeepsCurrentCost(t *testing.T) {
	// A password hashed at the current cost must not be re-hashed.
	const p = "correct-horse-battery-staple"
	hash, err := HashPassword(p)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := VerifyAndUpgrade(hash, p)
	if err != nil {
		t.Fatalf("VerifyAndUpgrade: %v", err)
	}
	if upgraded != "" {
		t.Errorf("upgraded = %q, want empty (cost already current)", upgraded)
	}
}

func TestVerifyAndUpgrade_RehashesLegacyCost(t *testing.T) {
	// Manually build a bcrypt hash at cost 10 (legacy) and verify
	// VerifyAndUpgrade re-hashes it at the current cost on success.
	const p = "correct-horse-battery-staple"
	legacyBytes, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("legacy hash: %v", err)
	}
	legacy := string(legacyBytes)
	if !strings.HasPrefix(legacy, "$2a$04$") && !strings.HasPrefix(legacy, "$2b$04$") {
		t.Fatalf("legacy hash not at the minimum cost: %q", legacy)
	}
	upgraded, err := VerifyAndUpgrade(legacy, p)
	if err != nil {
		t.Fatalf("VerifyAndUpgrade: %v", err)
	}
	if upgraded == "" {
		t.Fatal("upgraded = empty, want re-hashed value (legacy cost should trigger upgrade)")
	}
	if !strings.HasPrefix(upgraded, "$2a$12$") && !strings.HasPrefix(upgraded, "$2b$12$") {
		t.Errorf("upgraded hash not at cost 12: %q", upgraded)
	}
}

func TestVerifyAndUpgrade_RejectsWrongPassword(t *testing.T) {
	const p = "correct-horse-battery-staple"
	hash, err := HashPassword(p)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := VerifyAndUpgrade(hash, p+"x")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if upgraded != "" {
		t.Errorf("upgraded = %q, want empty (password mismatch must not produce an upgrade)", upgraded)
	}
}
