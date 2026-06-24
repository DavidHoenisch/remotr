package apppackages

import (
	"testing"
)

func TestS3ConfigFromEnv(t *testing.T) {
	t.Setenv("REMOTR_S3_BUCKET", "")
	t.Setenv("BUCKET_NAME", "")
	t.Setenv("REMOTR_S3_REGION", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("REMOTR_S3_ENDPOINT", "")
	t.Setenv("AWS_ENDPOINT_URL_S3", "")

	if _, ok := S3ConfigFromEnv(); ok {
		t.Fatal("expected disabled when no bucket env")
	}

	t.Setenv("BUCKET_NAME", "fly-bucket")
	t.Setenv("AWS_REGION", "auto")
	t.Setenv("AWS_ENDPOINT_URL_S3", "https://fly.storage.tigris.dev")

	cfg, ok := S3ConfigFromEnv()
	if !ok {
		t.Fatal("expected enabled from BUCKET_NAME")
	}
	if cfg.Bucket != "fly-bucket" {
		t.Fatalf("bucket = %q", cfg.Bucket)
	}
	if cfg.Region != "auto" {
		t.Fatalf("region = %q", cfg.Region)
	}
	if cfg.Endpoint != "https://fly.storage.tigris.dev" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}

	t.Setenv("REMOTR_S3_BUCKET", "remotr-bucket")
	t.Setenv("REMOTR_S3_REGION", "eu-west-1")
	t.Setenv("REMOTR_S3_ENDPOINT", "http://minio:9000")

	cfg, ok = S3ConfigFromEnv()
	if !ok {
		t.Fatal("expected enabled")
	}
	if cfg.Bucket != "remotr-bucket" {
		t.Fatalf("bucket = %q", cfg.Bucket)
	}
	if cfg.Region != "eu-west-1" {
		t.Fatalf("region = %q", cfg.Region)
	}
	if cfg.Endpoint != "http://minio:9000" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
}
