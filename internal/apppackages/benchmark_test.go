package apppackages

import "testing"

var benchmarkManifest Manifest

func BenchmarkParseManifestSchemaVersion(b *testing.B) {
	variants := []struct {
		name    string
		raw     []byte
		wantErr bool
	}{
		{
			name: "v1",
			raw:  []byte("schemaVersion: 1\nname: benchmark/tool\nversion: v1\ninstall:\n  mode: script\n  script: [install]\n"),
		},
		{
			name:    "v0",
			raw:     []byte("schemaVersion: 0\nname: benchmark/tool\nversion: v1\ninstall:\n  mode: script\n  script: [install]\n"),
			wantErr: true,
		},
		{
			name:    "v2",
			raw:     []byte("schemaVersion: 2\nname: benchmark/tool\nversion: v1\ninstall:\n  mode: script\n  script: [install]\n"),
			wantErr: true,
		},
	}

	for _, variant := range variants {
		b.Run(variant.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(variant.raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				manifest, err := ParseManifest(variant.raw)
				if (err != nil) != variant.wantErr {
					b.Fatalf("ParseManifest() error = %v, wantErr %v", err, variant.wantErr)
				}
				benchmarkManifest = manifest
			}
		})
	}
}
