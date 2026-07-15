package apppackages

import (
	"context"
	"encoding/json"
	"time"
)

// PackageRecord is one catalog entry.
type PackageRecord struct {
	ID        string
	Name      string
	Version   string
	S3Key     string
	SHA256    string
	Manifest  Manifest
	CreatedAt time.Time
}

// DownloadURL is a presigned fetch grant for an agent.
type DownloadURL struct {
	URL       string
	SHA256    string
	ExpiresAt time.Time
}

// Catalog stores published package metadata.
type Catalog interface {
	Create(ctx context.Context, rec PackageRecord) (PackageRecord, error)
	Get(ctx context.Context, name, version string) (PackageRecord, error)
	List(ctx context.Context, namePrefix string) ([]PackageRecord, error)
	Delete(ctx context.Context, name, version string) error
}

// URLResolver mints download URLs for agents at apply time.
type URLResolver interface {
	DownloadURL(ctx context.Context, name, version string) (DownloadURL, error)
}

// ManifestJSON marshals a manifest for Postgres storage.
func ManifestJSON(m Manifest) ([]byte, error) {
	return json.Marshal(m)
}

// ManifestFromJSON decodes a stored manifest.
func ManifestFromJSON(raw []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, err
	}
	if err := ValidateManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Service combines catalog lookup and presigning.
type Service struct {
	Catalog    Catalog
	Blobs      *BlobStore
	PresignTTL time.Duration
}

func (s *Service) DownloadURL(ctx context.Context, name, version string) (DownloadURL, error) {
	rec, err := s.Catalog.Get(ctx, name, version)
	if err != nil {
		return DownloadURL{}, err
	}
	if s.Blobs == nil {
		return DownloadURL{}, ErrUnavailable
	}
	ttl := s.PresignTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	url, err := s.Blobs.PresignGet(ctx, rec.S3Key, ttl)
	if err != nil {
		return DownloadURL{}, err
	}
	return DownloadURL{
		URL:       url,
		SHA256:    rec.SHA256,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}, nil
}
