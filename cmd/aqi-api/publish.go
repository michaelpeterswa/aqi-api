package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob" // file:// for local runs
	_ "gocloud.dev/blob/gcsblob"  // gs:// via Application Default Credentials

	"github.com/michaelpeterswa/aqi-api/internal/config"
	"github.com/michaelpeterswa/aqi-api/internal/publish"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
)

// runPublish computes one snapshot and uploads it to the bucket. It is meant
// to run as a cron job: one process, one snapshot, exit.
func runPublish(ctx context.Context, c *config.Config) error {
	if c.PublishBucketURL == "" {
		return errors.New("PUBLISH_BUCKET_URL is required for publish")
	}

	// No dragonfly here: the publisher wants fresh data, not the API's
	// response cache, and a cron job should not depend on the cache being up.
	timescaleClient, err := timescale.NewTimescaleClient(ctx, c.TimescaleDSN, c.TimescaleTable, c.QueryTimeout)
	if err != nil {
		return fmt.Errorf("create timescale client: %w", err)
	}
	defer timescaleClient.Close()

	bucket, err := blob.OpenBucket(ctx, c.PublishBucketURL)
	if err != nil {
		return fmt.Errorf("open bucket %s: %w", c.PublishBucketURL, err)
	}
	defer func() { _ = bucket.Close() }()

	publisher := &publish.Publisher{
		Reader:       timescaleClient,
		Bucket:       bucket,
		Prefix:       c.PublishPrefix,
		CacheControl: c.PublishCacheControl,
	}

	slog.Info("publishing snapshot", slog.String("bucket", c.PublishBucketURL), slog.String("prefix", c.PublishPrefix))
	return publisher.Run(ctx)
}
