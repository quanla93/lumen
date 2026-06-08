// s3_download.go — small adapter to download a single object from S3.
//
// Lives outside target_s3.go to keep the hot Put path lean (we don't
// want every backup to import the GetObject code into the binary's
// code path). This is only used by restore.go.

package backup

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MaxDownloadBytes caps the size of a single restore blob. A Lumen
// db that grows past this is unusual; a runaway is a misconfiguration
// the operator should see, not OOM the hub.
const MaxDownloadBytes = 512 * 1024 * 1024 // 512 MB

func s3Download(ctx context.Context, t *S3Target, name string) ([]byte, error) {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return nil, fmt.Errorf("backup: s3 download: invalid name %q", name)
	}
	out, err := t.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(t.cfg.Bucket),
		Key:    aws.String(t.key(name)),
	})
	if err != nil {
		return nil, fmt.Errorf("backup: s3 download: get %s: %w", name, err)
	}
	defer out.Body.Close()
	return ReadAllAtMost(out.Body, MaxDownloadBytes)
}
