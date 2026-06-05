package backup

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// testParams keeps the suite fast. Production uses DefaultArgon2Params
// (~1s/key on a Raspberry Pi 4) which would balloon the test run.
var testParams = Argon2Params{
	Time:    1,
	Memory:  8 * 1024, // 8 MiB
	Threads: 2,
	KeyLen:  32,
}

func TestSealOpenRoundTrip(t *testing.T) {
	plaintext := []byte("LUMEN backup payload — could be gzipped SQLite bytes")
	pass := []byte("correct horse battery staple")

	blob, err := SealWithParams(pass, plaintext, testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(blob) < HeaderSize {
		t.Fatalf("blob shorter than header: got %d, want >= %d", len(blob), HeaderSize)
	}
	if string(blob[:len(Magic)]) != Magic {
		t.Fatalf("magic prefix missing: got %q", blob[:len(Magic)])
	}
	if blob[len(Magic)] != Version1 {
		t.Fatalf("version byte = %d, want %d", blob[len(Magic)], Version1)
	}

	got, err := OpenWithParams(pass, blob, testParams)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext mismatch:\n got  %q\n want %q", got, plaintext)
	}
}

func TestOpenWrongPassphrase(t *testing.T) {
	blob, err := SealWithParams([]byte("right"), []byte("payload"), testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	_, err = OpenWithParams([]byte("wrong"), blob, testParams)
	if !errors.Is(err, ErrWrongPass) {
		t.Fatalf("want ErrWrongPass, got %v", err)
	}
}

func TestOpenTamperedCiphertext(t *testing.T) {
	blob, err := SealWithParams([]byte("pw"), []byte("payload"), testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	// Flip the last byte (inside the AEAD tag). GCM authentication must reject.
	tampered := append([]byte{}, blob...)
	tampered[len(tampered)-1] ^= 0x01
	_, err = OpenWithParams([]byte("pw"), tampered, testParams)
	if !errors.Is(err, ErrWrongPass) {
		t.Fatalf("want ErrWrongPass on tampered tag, got %v", err)
	}

	// Flip a byte inside the ciphertext body (after the header, before the tag).
	tampered2 := append([]byte{}, blob...)
	tampered2[HeaderSize] ^= 0x01
	_, err = OpenWithParams([]byte("pw"), tampered2, testParams)
	if !errors.Is(err, ErrWrongPass) {
		t.Fatalf("want ErrWrongPass on tampered body, got %v", err)
	}
}

func TestOpenBadMagic(t *testing.T) {
	blob, err := SealWithParams([]byte("pw"), []byte("payload"), testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	// Corrupt the first byte of the magic prefix.
	bad := append([]byte{}, blob...)
	bad[0] = 'X'
	_, err = OpenWithParams([]byte("pw"), bad, testParams)
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("want ErrBadMagic, got %v", err)
	}

	// A blob that's just garbage of header length.
	_, err = OpenWithParams([]byte("pw"), bytes.Repeat([]byte{0xFF}, HeaderSize+8), testParams)
	if !errors.Is(err, ErrBadMagic) {
		t.Fatalf("want ErrBadMagic on garbage prefix, got %v", err)
	}
}

func TestOpenFutureVersion(t *testing.T) {
	blob, err := SealWithParams([]byte("pw"), []byte("payload"), testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	bad := append([]byte{}, blob...)
	bad[len(Magic)] = 0x02 // a hypothetical v2 file
	_, err = OpenWithParams([]byte("pw"), bad, testParams)
	if !errors.Is(err, ErrFutureVersion) {
		t.Fatalf("want ErrFutureVersion, got %v", err)
	}
	if !strings.Contains(err.Error(), "version=2") {
		t.Fatalf("error should name the version byte, got %v", err)
	}
}

func TestOpenTruncated(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("LUMEN_BAK"),                 // 9 bytes, one short of magic
		bytes.Repeat([]byte{0}, HeaderSize-1), // exactly one short of full header
	}
	for i, blob := range cases {
		_, err := OpenWithParams([]byte("pw"), blob, testParams)
		if !errors.Is(err, ErrTruncated) {
			t.Errorf("case %d: want ErrTruncated, got %v", i, err)
		}
	}
}

func TestSealEmptyPassphrase(t *testing.T) {
	_, err := SealWithParams(nil, []byte("payload"), testParams)
	if err == nil || !strings.Contains(err.Error(), "empty passphrase") {
		t.Fatalf("want empty-passphrase error, got %v", err)
	}
}

func TestSealRandomness(t *testing.T) {
	// Two seals of the same plaintext under the same passphrase must
	// produce different blobs (different salt + nonce). If they collide,
	// either rand.Reader is broken or we're not regenerating per call.
	pass := []byte("pw")
	plaintext := []byte("payload")
	a, err := SealWithParams(pass, plaintext, testParams)
	if err != nil {
		t.Fatalf("seal a: %v", err)
	}
	b, err := SealWithParams(pass, plaintext, testParams)
	if err != nil {
		t.Fatalf("seal b: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two seals produced identical blobs (salt+nonce not regenerated?)")
	}
}

func TestSaltFromBlob(t *testing.T) {
	blob, err := SealWithParams([]byte("pw"), []byte("payload"), testParams)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	salt, err := SaltFromBlob(blob)
	if err != nil {
		t.Fatalf("SaltFromBlob: %v", err)
	}
	if len(salt) != SaltSize {
		t.Fatalf("salt size = %d, want %d", len(salt), SaltSize)
	}
	want := blob[len(Magic)+1 : len(Magic)+1+SaltSize]
	if !bytes.Equal(salt, want) {
		t.Fatalf("salt mismatch: got %x, want %x", salt, want)
	}
	// Verify it's a copy: mutating one doesn't bleed into the other.
	salt[0] ^= 0xFF
	if salt[0] == want[0] {
		t.Fatal("SaltFromBlob returned a slice aliasing the blob, want a copy")
	}
}

func TestHeaderSizeConst(t *testing.T) {
	// RFC 0001 spec lock: 10 + 1 + 16 + 12 = 39.
	if HeaderSize != 39 {
		t.Fatalf("HeaderSize = %d, want 39", HeaderSize)
	}
	if len(Magic) != 10 {
		t.Fatalf("len(Magic) = %d, want 10", len(Magic))
	}
}
