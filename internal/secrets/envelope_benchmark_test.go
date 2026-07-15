package secrets

import (
	"bytes"
	"fmt"
	"testing"
)

var benchmarkEncryptedRecord EncryptedRecord

func BenchmarkEnvelopeEncryptAndRewrap(b *testing.B) {
	oldKey := bytes.Repeat([]byte{0x41}, 32)
	newKey := bytes.Repeat([]byte{0x42}, 32)
	sourceKeyring, err := NewKeyring("old", map[string][]byte{"old": oldKey})
	if err != nil {
		b.Fatal(err)
	}
	source, err := NewEnvelope(sourceKeyring)
	if err != nil {
		b.Fatal(err)
	}
	targetKeyring, err := NewKeyring("new", map[string][]byte{"old": oldKey, "new": newKey})
	if err != nil {
		b.Fatal(err)
	}
	target, err := NewEnvelope(targetKeyring)
	if err != nil {
		b.Fatal(err)
	}

	for _, size := range []int{32, 4 << 10, 64 << 10} {
		plaintext := bytes.Repeat([]byte{0x5a}, size)
		scope := ScopeMetadata{Name: "benchmark/value", Version: "1", Fleet: "benchmark"}
		record, err := source.Encrypt(scope, plaintext)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("encrypt/bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkEncryptedRecord, err = source.Encrypt(scope, plaintext)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("rewrap/bytes=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkEncryptedRecord, err = target.Rewrap(record)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
