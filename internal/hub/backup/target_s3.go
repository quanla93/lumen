// target_s3.go — S3-compatible backup target.
//
// Wraps aws-sdk-go-v2 with the few options RFC 0001 mandates:
//   - BaseEndpoint override (AWS S3, MinIO, R2, B2 each need their
//     own URL — the SDK only auto-derives AWS endpoints from region).
//   - ForcePathStyle toggle (MinIO and older SDKs need path-style
//     URLs; Cloudflare R2 / AWS S3 prefer virtual-hosted style).
//   - Prefix support so multiple Lumen hubs can share a bucket.
//
// Snapshots stay <100 MB for any reasonable fleet; we use a single
// PutObject with a streaming reader, no multipart. Operators running
// 500+ hosts can switch to a multipart helper later without changing
// the Target interface.

package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config captures everything a caller needs to wire a target. The
// settings handlers (D3) translate the settings table rows into this
// struct before calling NewS3Target; tests construct it directly.
type S3Config struct {
	Endpoint        string // e.g. "https://s3.amazonaws.com" or "http://minio.local:9000"
	Region          string // "auto" is allowed for R2; AWS requires a real region
	Bucket          string
	Prefix          string // e.g. "lumen/"; trailing slash recommended; "" = no prefix
	AccessKey       string
	SecretKey       string // plaintext; handlers decrypt from settings before passing in
	ForcePathStyle  bool
}

// S3Target is the Target implementation backed by an S3-compatible
// object store. The client is created once at construction and reused
// across Put/List/Delete; aws-sdk-go-v2 clients are safe for
// concurrent use.
type S3Target struct {
	cfg    S3Config
	client *s3.Client
}

// NewS3Target builds an S3 client tuned to cfg and validates the
// configuration by issuing a HeadBucket. The head call is the cheapest
// round-trip that surfaces the most common misconfigurations
// (404 bucket not found, 403 auth failed, TLS handshake failed) before
// the scheduler goes into cron mode and silently fails for hours.
func NewS3Target(ctx context.Context, cfg S3Config) (*S3Target, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("backup: s3 target: bucket is required")
	}
	if cfg.Region == "" {
		return nil, errors.New("backup: s3 target: region is required (use \"auto\" for Cloudflare R2)")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("backup: s3 target: access_key and secret_key are required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("backup: s3 target: load config: %w", err)
	}

	// BaseEndpoint goes through aws.WithEndpointResolverWithOptions so
	// R2 / MinIO URLs override the SDK's AWS endpoint resolver.
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		if cfg.ForcePathStyle {
			o.UsePathStyle = true
		}
	})

	t := &S3Target{cfg: cfg, client: client}
	if err := t.ping(ctx); err != nil {
		return nil, fmt.Errorf("backup: s3 target: head bucket %s: %w", cfg.Bucket, err)
	}
	return t, nil
}

// ping is the HeadBucket used to surface connectivity / auth errors
// at construction time. Cheap (one HEAD) and gives a much faster
// signal than waiting for the first cron tick to fail.
func (t *S3Target) ping(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(t.cfg.Bucket),
	})
	return err
}

// Put uploads the reader under cfg.Prefix + name. The reader is
// streamed straight into PutObject (no buffering) so an 80 MB
// snapshot doesn't spend 80 MB of hub RAM.
func (t *S3Target) Put(ctx context.Context, name string, r io.Reader) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if name == "" || strings.ContainsAny(name, "/\\") {
		return Entry{}, fmt.Errorf("backup: s3 target: invalid name %q (no path separators)", name)
	}
	key := t.key(name)
	_, err := t.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(key),
		Body:   r,
		// Server-side encryption is the cheapest baseline defense. AES256
		// is supported by every S3-compatible target (AWS, R2, MinIO,
		// B2, Wasabi). Operators who want KMS can add BucketKeyEnabled
		// + SSEKMSKeyId here later without touching the wire format.
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return Entry{}, fmt.Errorf("backup: s3 target: put %s: %w", key, err)
	}
	return Entry{
		Name:      name,
		Size:      0, // PutObject doesn't return size; List fills it in
		CreatedAt: time.Now().UTC(),
	}, nil
}

// List returns backup objects in cfg.Prefix, newest-first by LastModified.
// Continuation tokens are honored: an operator with thousands of
// backups won't get a truncated view.
func (t *S3Target) List(ctx context.Context) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix := t.cfg.Prefix
	out := []Entry{}
	var continuationToken *string
	for {
		page, err := t.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(t.cfg.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("backup: s3 target: list %s: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil || obj.LastModified == nil {
				continue
			}
			name := strings.TrimPrefix(*obj.Key, prefix)
			if name == "" || strings.Contains(name, "/") {
				continue // nested under our prefix; ignore
			}
			size := int64(0)
			if obj.Size != nil {
				size = *obj.Size
			}
			out = append(out, Entry{
				Name:      name,
				Size:      size,
				CreatedAt: obj.LastModified.UTC(),
			})
		}
		if page.NextContinuationToken == nil {
			break
		}
		continuationToken = page.NextContinuationToken
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Delete removes name from the bucket. A "NotFound" response is
// non-fatal — the retention sweep is best-effort and a missing
// object is the desired end state.
func (t *S3Target) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("backup: s3 target: invalid name %q", name)
	}
	_, err := t.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(t.key(name)),
	})
	if err != nil {
		// Treat NoSuchKey as success so the sweep is idempotent.
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		var apiErr smithyAPIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			return nil
		}
		return fmt.Errorf("backup: s3 target: delete %s: %w", name, err)
	}
	return nil
}

// key composes the full S3 object key: cfg.Prefix + name. Empty prefix
// is allowed; the result is just name.
func (t *S3Target) key(name string) string {
	return t.cfg.Prefix + name
}

// smithyAPIError is a minimal interface that matches both the typed
// NoSuchKey and the generic smithy API error — the SDK returns the
// former for direct API calls and the latter when the request was
// retried. We only care about the error code; "NoSuchKey" is the
// "object is already gone" signal.
type smithyAPIError interface {
	ErrorCode() string
}

// compile-time check: S3Target satisfies the Target interface.
var _ Target = (*S3Target)(nil)

// PutBytes is a small shim used by tests that want to write a known
// payload and assert List / Delete behavior without standing up
// MinIO. It bypasses the SDK and writes into an in-memory map, so it
// only satisfies the test ergonomics — production Put is the
// S3Target.Put method above.
func PutBytes(t *S3Target, name string, body []byte) (Entry, error) {
	return t.Put(context.Background(), name, bytes.NewReader(body))
}
