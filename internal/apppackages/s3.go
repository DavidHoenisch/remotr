package apppackages

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config configures access to a private object store.
type S3Config struct {
	Bucket   string
	Region   string
	Endpoint string
}

// BlobStore uploads and presigns package zips.
type BlobStore struct {
	client *s3.Client
	bucket string
}

// NewBlobStore builds an S3 client from environment and Remotr config.
func NewBlobStore(ctx context.Context, cfg S3Config) (*BlobStore, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if id := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")); id != "" {
		secret := os.Getenv("AWS_SECRET_ACCESS_KEY")
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(id, secret, os.Getenv("AWS_SESSION_TOKEN")),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if ep := strings.TrimSpace(cfg.Endpoint); ep != "" {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		}
	})
	return &BlobStore{client: client, bucket: bucket}, nil
}

// Upload puts a zip object at key.
func (b *BlobStore) Upload(ctx context.Context, key string, r io.Reader, size int64) error {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return fmt.Errorf("s3 key required")
	}
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String("application/zip"),
	})
	if err != nil {
		return fmt.Errorf("s3 put object: %w", err)
	}
	return nil
}

// PresignGet returns a time-limited download URL for key.
func (b *BlobStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return "", fmt.Errorf("s3 key required")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	presigner := s3.NewPresignClient(b.client)
	out, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign get object: %w", err)
	}
	return out.URL, nil
}

// DeleteObject removes key from the bucket.
func (b *BlobStore) DeleteObject(ctx context.Context, key string) error {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" {
		return fmt.Errorf("s3 key required")
	}
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete object: %w", err)
	}
	return nil
}
