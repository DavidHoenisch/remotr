package capabilitydoc

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func FuzzDocumentCanonicalization(f *testing.F) {
	f.Add([]byte(`{"documentVersion":1,"artifactSchemaVersions":[1,0],"capabilities":[{"id":"resource:package","revision":"package-v1"},{"id":"provider:init/systemd","revision":"1"}],"facts":[{"key":"init","value":"systemd"},{"key":"architecture","value":"x86"}],"agentVersion":"v1.2.3","digest":"ignored"}`))
	f.Add([]byte(`{"documentVersion":1,"artifactSchemaVersions":[1],"capabilities":[{"id":"resource:file","revision":"file-v1"}],"facts":[],"agentVersion":"dev","digest":"ignored"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > MaxDocumentBytes {
			raw = raw[:MaxDocumentBytes]
		}
		document, err := Decode(raw)
		if err != nil {
			return
		}
		document, err = document.WithCanonicalDigest()
		if err != nil || document.Validate() != nil {
			return
		}
		canonical, err := document.CanonicalBody()
		if err != nil {
			t.Fatal(err)
		}
		digest, err := document.CanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}

		reordered := document
		reordered.ArtifactSchemaVersions = append([]int(nil), document.ArtifactSchemaVersions...)
		reordered.Capabilities = append([]Capability(nil), document.Capabilities...)
		reordered.Facts = append([]Fact(nil), document.Facts...)
		slices.Reverse(reordered.ArtifactSchemaVersions)
		slices.Reverse(reordered.Capabilities)
		slices.Reverse(reordered.Facts)
		reorderedCanonical, err := reordered.CanonicalBody()
		if err != nil {
			t.Fatal(err)
		}
		reorderedDigest, err := reordered.CanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonical, reorderedCanonical) || digest != reorderedDigest {
			t.Fatalf("canonical encoding or digest changed after set reordering")
		}

		duplicate := document
		duplicate.Capabilities = append(append([]Capability(nil), document.Capabilities...), document.Capabilities[0])
		duplicate, err = duplicate.WithCanonicalDigest()
		if err != nil {
			t.Fatal(err)
		}
		var validation *ValidationError
		if err := duplicate.Validate(); !errors.As(err, &validation) || validation.Code != "duplicate_capability" {
			t.Fatalf("duplicate capability error = %v", err)
		}
		if len(document.Facts) > 0 {
			duplicateFact := document
			duplicateFact.Facts = append(append([]Fact(nil), document.Facts...), document.Facts[0])
			duplicateFact, err = duplicateFact.WithCanonicalDigest()
			if err != nil {
				t.Fatal(err)
			}
			if err := duplicateFact.Validate(); !errors.As(err, &validation) || validation.Code != "duplicate_fact" {
				t.Fatalf("duplicate fact error = %v", err)
			}
		}
	})
}

func FuzzGeneratorFactNormalization(f *testing.F) {
	generator, err := NewDefaultGenerator([]int{0, 1})
	if err != nil {
		f.Fatal(err)
	}
	f.Add("Ubuntu", "24.04", "x86", "systemd", "apt", "dconf", "firefox")
	f.Add("ubuntu", "24.04", "ARM", "systemd", "apt", "DCONF", "FIREFOX")
	f.Fuzz(func(t *testing.T, distro, version, architecture, init, pkg, desktop, browser string) {
		bounded := func(value string) string {
			if len(value) > MaxFactValueBytes*2 {
				return value[:MaxFactValueBytes*2]
			}
			return value
		}
		document, err := generator.Generate(facts.Facts{
			Distro: types.Distro(bounded(distro)), DistroVersion: bounded(version),
			Arch: types.Architecture(bounded(architecture)), Init: facts.InitBackend(bounded(init)),
			Package: types.PackageManager(bounded(pkg)),
			Desktop: []facts.DesktopBackend{facts.DesktopBackend(bounded(desktop)), facts.DesktopBackend(strings.ToLower(bounded(desktop)))},
			Browser: []facts.BrowserBackend{facts.BrowserBackend(bounded(browser)), facts.BrowserBackend(strings.ToLower(bounded(browser)))},
		}, "fuzz-agent")
		if err != nil {
			if len(err.Error()) > MaxDiagnosticBytes {
				t.Fatalf("generation diagnostic is unbounded: %d", len(err.Error()))
			}
			return
		}
		if err := document.Validate(); err != nil {
			t.Fatalf("generator returned invalid document: %v", err)
		}
		seen := make(map[Fact]bool, len(document.Facts))
		for _, fact := range document.Facts {
			if seen[fact] || fact.Key != strings.ToLower(fact.Key) || fact.Value != strings.ToLower(fact.Value) {
				t.Fatalf("facts are not normalized and duplicate-free: %+v", document.Facts)
			}
			seen[fact] = true
		}
	})
}
