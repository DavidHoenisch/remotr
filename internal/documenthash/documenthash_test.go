package documenthash

import (
	"encoding/json"
	"strings"
	"testing"
)

const validHash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func TestDigestIsVersionedAndDocumentTypeDomainSeparated(t *testing.T) {
	got, err := Digest(Capability, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:8f14a4aa5df6bc3fd700928b418ad2f31615bec83cf4cf08a3578383327480a3"
	if got != want {
		t.Fatalf("Digest(capability, {}) = %q, want %q", got, want)
	}
	other, err := Digest(SystemInformation, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if other == got {
		t.Fatal("equal semantic bytes aliased across document domains")
	}
}

func TestDecodeRejectsMalformedOrUnboundedSummary(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unsupported version", raw: `{"version":2,"documents":{"capability":"` + validHash + `"}}`},
		{name: "empty documents", raw: `{"version":1,"documents":{}}`},
		{name: "unknown document", raw: `{"version":1,"documents":{"secret":"` + validHash + `"}}`},
		{name: "upper case hash", raw: `{"version":1,"documents":{"capability":"sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`},
		{name: "short hash", raw: `{"version":1,"documents":{"capability":"sha256:00"}}`},
		{name: "unknown field", raw: `{"version":1,"documents":{"capability":"` + validHash + `"},"extra":true}`},
		{name: "trailing value", raw: `{"version":1,"documents":{"capability":"` + validHash + `"}} {}`},
		{name: "oversize", raw: strings.Repeat(" ", MaxSummaryBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.raw)); err == nil {
				t.Fatal("Decode() unexpectedly accepted invalid summary")
			}
		})
	}
}

func TestDecodeAcceptsAllBoundedDocumentDomains(t *testing.T) {
	raw := `{"version":1,"documents":{"capability":"` + validHash +
		`","systemInformation":"` + validHash + `","delivery":"` + validHash +
		`","targeting":"` + validHash + `"}}`
	summary, err := Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Documents) != 4 {
		t.Fatalf("document count = %d, want 4", len(summary.Documents))
	}
}

func TestCanonicalDeliveryAndTargetingAreStableAndBounded(t *testing.T) {
	delivery, err := CanonicalDelivery("release-1", "digest-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(delivery) != `{"releaseRef":"release-1","digest":"digest-1"}` {
		t.Fatalf("delivery canonical bytes = %s", delivery)
	}
	left, err := CanonicalTargeting(map[string]string{"distro": "ubuntu", "arch": "x86"}, []string{"bob", "alice", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CanonicalTargeting(map[string]string{"arch": "x86", "distro": "ubuntu"}, []string{"alice", "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatalf("equal targeting semantics differ: %s != %s", left, right)
	}
	tooMany := make(map[string]string, MaxTargetLabels+1)
	for i := range MaxTargetLabels + 1 {
		tooMany[string(rune('a'+i))] = "value"
	}
	if _, err := CanonicalTargeting(tooMany, nil); err == nil {
		t.Fatal("oversized targeting document was accepted")
	}
}

func FuzzDecodeSummaryBoundedRoundTrip(f *testing.F) {
	f.Add([]byte(`{"version":1,"documents":{"capability":"` + validHash + `"}}`))
	f.Add([]byte(`{"version":0,"documents":{}}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		summary, err := Decode(raw)
		if err != nil {
			return
		}
		if len(raw) > MaxSummaryBytes || summary.Validate() != nil {
			t.Fatal("Decode accepted input outside its documented bounds")
		}
		encoded, err := json.Marshal(summary)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(encoded); err != nil {
			t.Fatalf("accepted summary did not round trip: %v", err)
		}
	})
}
