package backup

import (
	"errors"
	"strings"
	"testing"
)

func TestVerifyPassphrase_Match(t *testing.T) {
	hash, err := hashWithArgon2("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashWithArgon2: %v", err)
	}
	if err := verifyPassphrase("correct horse battery staple", hash); err != nil {
		t.Errorf("verifyPassphrase match returned %v, want nil", err)
	}
}

func TestVerifyPassphrase_Mismatch(t *testing.T) {
	hash, err := hashWithArgon2("right")
	if err != nil {
		t.Fatalf("hashWithArgon2: %v", err)
	}
	err = verifyPassphrase("wrong", hash)
	if err == nil {
		t.Fatal("verifyPassphrase mismatch returned nil, want error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error should name the failure mode, got %q", err)
	}
}

func TestVerifyPassphrase_EmptyStored(t *testing.T) {
	err := verifyPassphrase("anything", "")
	if err == nil {
		t.Fatal("verifyPassphrase with empty stored returned nil, want error")
	}
}

func TestVerifyPassphrase_MalformedStored(t *testing.T) {
	for _, bad := range []string{
		"not-an-argon2-string",
		"$argon2id$v=19$m=65536,t=3,p=4$only-three-fields",
		"$argon2id$v=99$m=65536,t=3,p=4$YWFhYWFhYWFhYWFhYWFhYQ$YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE",
		"$argon2id$v=19$m=abc,t=3,p=4$YWFhYWFhYWFhYWFhYWFhYQ$YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE",
	} {
		if err := verifyPassphrase("x", bad); err == nil {
			t.Errorf("verifyPassphrase(%q) returned nil, want error", bad)
		}
	}
}

func TestVerifyPassphrase_BadBase64(t *testing.T) {
	// $argon2id$v=19$m=65536,t=3,p=4$!!!notbase64!!!$alsoBad
	bad := "$argon2id$v=19$m=65536,t=3,p=4$!!!notbase64!!!$alsoBad"
	err := verifyPassphrase("x", bad)
	if err == nil {
		t.Fatal("expected error for non-base64 salt, got nil")
	}
	if !errors.Is(err, err) { // just confirm err is non-nil
		t.Fatalf("expected non-nil error, got %v", err)
	}
}
