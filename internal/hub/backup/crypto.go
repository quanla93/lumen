// Package backup implements Lumen's backup + restore feature.
//
// A backup is one self-contained file containing the hub's SQLite
// database snapshot, gzipped and encrypted with an operator-chosen
// passphrase. The file format is intentionally simple so a third-party
// tool can decrypt it without Lumen running:
//
//	LUMEN_BAK\x00        (10 bytes magic — "LUMEN_BAK" + null)
//	\x01                 (1 byte version: 1)
//	[16 bytes salt]      (Argon2id salt, random per backup)
//	[12 bytes nonce]     (AES-GCM nonce, random per backup)
//	[ciphertext]         (AES-256-GCM over gzipped SQLite snapshot)
//
// The 39-byte fixed-size header makes inspection trivial:
//
//	dd if=backup.bin bs=1 count=39 | xxd
//
// This file owns only the cryptographic layer: turning a passphrase +
// plaintext bytes into the encrypted blob, and reversing it. The
// snapshot, gzip, target write, and schedule layers live in sibling
// files added on D2/D3.
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Magic is the 10-byte file prefix every Lumen backup starts with.
const Magic = "LUMEN_BAK\x00"

// Version1 is the format version this build writes. Files with a higher
// version byte are rejected with ErrFutureVersion.
const Version1 byte = 0x01

// Sizes of the fixed-width header fields.
const (
	SaltSize   = 16
	NonceSize  = 12
	HeaderSize = len(Magic) + 1 + SaltSize + NonceSize // 39
)

// Argon2Params tunes the key derivation. The defaults aim for ~1s wall
// time on a Raspberry Pi 4 — the slowest realistic operator hardware.
// Tests pass cheaper params to keep the suite fast.
type Argon2Params struct {
	Time    uint32 // number of iterations
	Memory  uint32 // KiB; 64*1024 = 64 MiB
	Threads uint8  // parallelism
	KeyLen  uint32 // output length (32 for AES-256)
}

// DefaultArgon2Params is the production-grade tuning RFC 0001 mandates.
var DefaultArgon2Params = Argon2Params{
	Time:    3,
	Memory:  64 * 1024,
	Threads: 4,
	KeyLen:  32,
}

// Errors returned by Open / parseHeader. Stable enough to be matched
// with errors.Is in the CLI restore path.
var (
	ErrBadMagic      = errors.New("backup: not a Lumen backup file (magic byte mismatch)")
	ErrFutureVersion = errors.New("backup: file version is newer than this binary supports")
	ErrTruncated     = errors.New("backup: file truncated before ciphertext")
	ErrWrongPass     = errors.New("backup: decryption failed (wrong passphrase or tampered file)")
)

// Seal encrypts plaintext under passphrase using DefaultArgon2Params and
// returns the full backup blob (header + ciphertext).
func Seal(passphrase, plaintext []byte) ([]byte, error) {
	return SealWithParams(passphrase, plaintext, DefaultArgon2Params)
}

// SealWithParams is Seal with caller-supplied Argon2id parameters — used
// by tests to dial down cost and keep the suite fast.
func SealWithParams(passphrase, plaintext []byte, p Argon2Params) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("backup: empty passphrase")
	}

	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("backup: salt: %w", err)
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("backup: nonce: %w", err)
	}

	aead, err := newAEAD(passphrase, salt, p)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, HeaderSize+len(plaintext)+aead.Overhead())
	out = append(out, Magic...)
	out = append(out, Version1)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = aead.Seal(out, nonce, plaintext, nil)
	return out, nil
}

// Open parses a backup blob and decrypts it using DefaultArgon2Params.
func Open(passphrase, blob []byte) ([]byte, error) {
	return OpenWithParams(passphrase, blob, DefaultArgon2Params)
}

// OpenWithParams is Open with caller-supplied Argon2id parameters.
func OpenWithParams(passphrase, blob []byte, p Argon2Params) ([]byte, error) {
	salt, nonce, ct, err := parseHeader(blob)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(passphrase, salt, p)
	if err != nil {
		return nil, err
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrWrongPass
	}
	return pt, nil
}

// SaltFromBlob extracts the salt of a backup blob without decrypting —
// useful when the CLI wants to compare the operator's typed passphrase
// against settings.backup.passphrase_hash before doing the slow Argon2id
// derivation of the AEAD key.
func SaltFromBlob(blob []byte) ([]byte, error) {
	salt, _, _, err := parseHeader(blob)
	if err != nil {
		return nil, err
	}
	cp := make([]byte, len(salt))
	copy(cp, salt)
	return cp, nil
}

// parseHeader validates the magic + version + carves out salt / nonce /
// ciphertext slices into the original blob (no copies).
func parseHeader(blob []byte) (salt, nonce, ciphertext []byte, err error) {
	if len(blob) < HeaderSize {
		return nil, nil, nil, ErrTruncated
	}
	if string(blob[:len(Magic)]) != Magic {
		return nil, nil, nil, ErrBadMagic
	}
	ver := blob[len(Magic)]
	if ver != Version1 {
		return nil, nil, nil, fmt.Errorf("%w: version=%d", ErrFutureVersion, ver)
	}
	off := len(Magic) + 1
	salt = blob[off : off+SaltSize]
	off += SaltSize
	nonce = blob[off : off+NonceSize]
	off += NonceSize
	ciphertext = blob[off:]
	return salt, nonce, ciphertext, nil
}

func newAEAD(passphrase, salt []byte, p Argon2Params) (cipher.AEAD, error) {
	key := argon2.IDKey(passphrase, salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backup: aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
