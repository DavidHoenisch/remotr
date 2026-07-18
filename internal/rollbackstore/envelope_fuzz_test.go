package rollbackstore

import (
	"encoding/json"
	"reflect"
	"testing"
)

// FuzzTransactionEnvelopeDecoder proves that bounded arbitrary transaction
// envelopes either fail strict decoding or have a stable, decodable canonical
// representation. Unknown fields and trailing values remain rejection seeds.
func FuzzTransactionEnvelopeDecoder(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"header":{"version":2,"metadata":{}},"ciphertext":""}`))
	f.Add([]byte(`{"header":{"version":2,"metadata":{}},"ciphertext":"","unknown":true}`))
	f.Add([]byte(`{"header":{"version":2,"metadata":{}},"ciphertext":""} {}`))
	f.Add([]byte(`not-json`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 64*1024 {
			return
		}
		envelope, err := decodeEnvelope(raw)
		if err != nil {
			return
		}
		canonical, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("accepted envelope could not be encoded: %v", err)
		}
		if !json.Valid(canonical) {
			t.Fatalf("accepted envelope encoded invalid JSON: %q", canonical)
		}
		roundTripped, err := decodeEnvelope(canonical)
		if err != nil {
			t.Fatalf("accepted envelope did not decode after canonical encoding: %v", err)
		}
		if !reflect.DeepEqual(envelope, roundTripped) {
			t.Fatalf("canonical round trip changed envelope: before=%+v after=%+v", envelope, roundTripped)
		}
	})
}
