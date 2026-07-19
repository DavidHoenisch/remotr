package capabilitydoc

import "testing"

func FuzzDocumentValidation(f *testing.F) {
	f.Add([]byte(`{"documentVersion":1}`))
	f.Add([]byte(`{"documentVersion":99,"capabilities":[]}`))
	f.Add([]byte(`{"documentVersion":1,"capabilities":[{"id":"provider:init/systemd","revision":"1"},{"id":"provider:init/systemd","revision":"2"}]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > MaxDocumentBytes*2 {
			raw = raw[:MaxDocumentBytes*2]
		}
		document, err := Decode(raw)
		if err != nil {
			if len(err.Error()) > MaxDiagnosticBytes {
				t.Fatalf("decode diagnostic is unbounded: %d", len(err.Error()))
			}
			return
		}
		if err := document.Validate(); err != nil && len(err.Error()) > MaxDiagnosticBytes {
			t.Fatalf("validation diagnostic is unbounded: %d", len(err.Error()))
		}
	})
}
