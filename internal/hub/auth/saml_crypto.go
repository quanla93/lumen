// saml_crypto.go — AES-GCM encrypt/decrypt for SAML SP private key.
//
// The hub's session secret is shared by:
//   - OIDC client_secret (auth/crypto.go, label "lumen/oidc/v1")
//   - Web Push VAPID private key (webpush package, label "lumen/webpush/v1")
//   - Backup S3 secret_key (backup package, label "lumen/backup/v1")
//   - SAML SP private key (here, label "lumen/saml/v1")
//
// Distinct labels keep the same hub secret from yielding the same AEAD
// key across protocols — standard practice to avoid cross-protocol
// attacks. Operators who rotate LUMEN_HUB_SECRET have to re-enter
// these four values; the docs cross-link this constraint.
//
// Output is base64-encoded ("nonce || ciphertext || tag") matching
// the OIDC client_secret scheme, so the on-disk format is uniform
// across secrets.

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

// EncryptSPKey encrypts the SP private key (PEM) for storage. Returns
// the base64-encoded "nonce || ciphertext || tag" form, or "" for an
// empty input so the "no key yet" state round-trips cleanly.
func EncryptSPKey(plaintext string, hubSecret []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(hubSecret) == 0 {
		return "", errors.New("hub secret unset: cannot encrypt SAML SP key")
	}
	aead, err := newSAMLAEAD(hubSecret)
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

// DecryptSPKey reverses EncryptSPKey. Empty input returns "" so a
// not-yet-generated key round-trips without an error.
func DecryptSPKey(ciphertext string, hubSecret []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if len(hubSecret) == 0 {
		return "", errors.New("hub secret unset: cannot decrypt SAML SP key")
	}
	blob, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	aead, err := newSAMLAEAD(hubSecret)
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

// newSAMLAEAD derives a 32-byte AES-256 key from the hub session
// secret with the SAML label. SHA-256 of "lumen/saml/v1" || hubSecret
// gives us a different key from the OIDC derivation (which uses
// "lumen/oidc/v1") so the same hub secret doesn't produce the same
// AEAD key across protocols.
func newSAMLAEAD(hubSecret []byte) (cipher.AEAD, error) {
	h := sha256.New()
	h.Write([]byte("lumen/saml/v1"))
	h.Write(hubSecret)
	key := h.Sum(nil)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
