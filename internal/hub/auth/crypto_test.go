package auth

import "testing"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	secret := []byte("hub-session-secret-32-bytes-1234")
	cases := []string{
		"simple-client-secret",
		"longer secret with spaces and !@#$%^&*() symbols",
		"unicode: café résumé 🔐",
	}
	for _, plain := range cases {
		t.Run(plain, func(t *testing.T) {
			enc, err := EncryptSecret(plain, secret)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if enc == plain {
				t.Fatal("encrypted output equals plaintext")
			}
			got, err := DecryptSecret(enc, secret)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if got != plain {
				t.Fatalf("got %q, want %q", got, plain)
			}
		})
	}
}

func TestEncryptEmpty(t *testing.T) {
	enc, err := EncryptSecret("", []byte("anything"))
	if err != nil || enc != "" {
		t.Fatalf("empty plaintext: enc=%q err=%v", enc, err)
	}
	dec, err := DecryptSecret("", []byte("anything"))
	if err != nil || dec != "" {
		t.Fatalf("empty ciphertext: dec=%q err=%v", dec, err)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	enc, err := EncryptSecret("payload", []byte("key-A-32-bytes-padding-padding!!"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := DecryptSecret(enc, []byte("key-B-32-bytes-padding-padding!!")); err == nil {
		t.Fatal("decrypt with wrong key should fail; got nil err")
	}
}

func TestEncryptNoSecret(t *testing.T) {
	if _, err := EncryptSecret("payload", nil); err == nil {
		t.Fatal("encrypt without hub secret should error")
	}
}
