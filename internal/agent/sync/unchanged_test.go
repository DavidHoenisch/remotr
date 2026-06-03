package sync

import "testing"

func TestUnchanged(t *testing.T) {
	const digest = "abc"
	const ref = "deadbeef"

	tests := []struct {
		name        string
		lastDigest  string
		lastRef     string
		serverRef   string
		want        bool
	}{
		{"digest differs", "old", ref, ref, false},
		{"digest matches ref matches", digest, ref, ref, true},
		{"digest matches ref advanced", digest, "older", ref, false},
		{"digest matches agent never saw ref", digest, "", ref, false},
		{"digest matches server ref empty", digest, ref, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unchanged(tt.lastDigest, digest, tt.lastRef, tt.serverRef)
			if got != tt.want {
				t.Fatalf("Unchanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
