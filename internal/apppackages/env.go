package apppackages

import (
	"os"
	"strings"
)

// S3ConfigFromEnv resolves S3 settings from Remotr and standard AWS/Fly Tigris variables.
//
// Bucket: REMOTR_S3_BUCKET, then BUCKET_NAME (set by fly storage create).
// Region: REMOTR_S3_REGION, then AWS_REGION, then us-east-1.
// Endpoint: REMOTR_S3_ENDPOINT, then AWS_ENDPOINT_URL_S3 (Fly Tigris).
func S3ConfigFromEnv() (S3Config, bool) {
	bucket := firstNonEmpty(
		os.Getenv("REMOTR_S3_BUCKET"),
		os.Getenv("BUCKET_NAME"),
	)
	if bucket == "" {
		return S3Config{}, false
	}
	region := firstNonEmpty(
		os.Getenv("REMOTR_S3_REGION"),
		os.Getenv("AWS_REGION"),
	)
	endpoint := firstNonEmpty(
		os.Getenv("REMOTR_S3_ENDPOINT"),
		os.Getenv("AWS_ENDPOINT_URL_S3"),
	)
	return S3Config{
		Bucket:   bucket,
		Region:   region,
		Endpoint: endpoint,
	}, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
