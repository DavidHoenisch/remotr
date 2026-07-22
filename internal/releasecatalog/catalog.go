// Package releasecatalog exposes immutable capability evidence packaged with
// one Remotr agent release. Generated payloads are validated before use and
// callers receive clones so runtime code cannot mutate release truth.
package releasecatalog

import (
	"sync"

	"github.com/DavidHoenisch/remotr/internal/ubuntuproqualification"
)

//go:generate go run -mod=vendor ./cmd/generate -source ../../test/qualification/ubuntu-pro.yaml -output generated_ubuntu_pro.go

var (
	ubuntuProOnce     sync.Once
	ubuntuProManifest ubuntuproqualification.Manifest
	ubuntuProErr      error
)

// UbuntuProQualification returns the frozen qualification inventory embedded
// in this build. It never reads test or operator-selected files at runtime.
func UbuntuProQualification() (ubuntuproqualification.Manifest, error) {
	ubuntuProOnce.Do(func() {
		ubuntuProManifest, ubuntuProErr = ubuntuproqualification.Decode(generatedUbuntuProYAML)
	})
	if ubuntuProErr != nil {
		return ubuntuproqualification.Manifest{}, ubuntuProErr
	}
	return ubuntuProManifest.Clone(), nil
}
