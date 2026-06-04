package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptSecret protects a small bytestring (e.g. an OIDC client_secret)
// at rest in the settings table using AES-256-GCM. The cipher key is a
// SHA-256 derivation of the hub's session secret tagged with a fixed
// label so the same hub secret reused as a JWT signing key produces a
// different AEAD key — standard practice to avoid cross-protocol attacks.
//
// Output is base64-encoded (`12-byte-nonce || ciphertext || 16-byte-tag`).
// Returns the marker `"plain:<base64>"` for an empty key so unit tests
// can still round-trip without a hub secret configured.
func EncryptSecret(plaintext string, hubSecret []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(hubSecret) == 0 {
		return "", errors.New("hub secret unset: cannot encrypt OIDC client secret")
	}
	aead, err := newOIDCAEAD(hubSecret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce read: %w", err)
	}
	ct := aead.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// DecryptSecret reverses EncryptSecret. An empty input returns "" so the
// "OIDC disabled" / unset state round-trips cleanly without an error.
func DecryptSecret(ciphertext string, hubSecret []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if len(hubSecret) == 0 {
		return "", errors.New("hub secret unset: cannot decrypt OIDC client secret")
	}
	blob, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	aead, err := newOIDCAEAD(hubSecret)
	if err != nil {
		return "", err
	}
	if len(blob) < aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := blob[:aead.NonceSize()], blob[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}

func newOIDCAEAD(hubSecret []byte) (cipher.AEAD, error) {
	h := sha256.New()
	h.Write([]byte("lumen/oidc/v1"))
	h.Write(hubSecret)
	key := h.Sum(nil)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
