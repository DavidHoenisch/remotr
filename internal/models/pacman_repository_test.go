package models

import (
	"slices"
	"strings"
	"testing"
)

func TestParseCanonicalPacmanTrustAndRepositoryRoundTrip(t *testing.T) {
	const fingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"
	state, err := ParseState(strings.NewReader(`
schemaVersion: 1
configurations:
  - name: base
    targetDistros: [Arch]
    resources:
      - kind: pacmanSigningKey
        name: vendor
        lifecycle: present
        ownership: named
        source: https://keys.example.test/vendor.asc
        fingerprint: ` + fingerprint + `
      - kind: pacmanRepository
        name: vendor-repository
        lifecycle: disabled
        ownership: fragment
        dependsOn: [base/vendor]
        servers:
          - https://mirror.example.test/$repo/os/$arch
        architecture: x86_64
        signatureLevel: required
        signingKeys: [vendor]
        credentialRef: file:/run/remotr/pacman-vendor
`))
	if err != nil {
		t.Fatalf("ParseState() = %v", err)
	}
	configuration := state.Configurations[0]
	if len(configuration.PacmanSigningKeys) != 1 || len(configuration.PacmanRepositories) != 1 {
		t.Fatalf("Pacman resources = keys:%+v repositories:%+v", configuration.PacmanSigningKeys, configuration.PacmanRepositories)
	}
	key := configuration.PacmanSigningKeys[0]
	if key.Kind != ResourceKindPacmanSigningKey || key.Lifecycle != LifecyclePresent || key.Ownership != OwnershipNamed ||
		key.Name != "vendor" || key.Source != "https://keys.example.test/vendor.asc" || key.Fingerprint != fingerprint {
		t.Fatalf("Pacman signing key = %+v", key)
	}
	repository := configuration.PacmanRepositories[0]
	if repository.Kind != ResourceKindPacmanRepository || repository.Lifecycle != LifecycleDisabled || repository.Ownership != OwnershipFragment ||
		repository.Name != "vendor-repository" || repository.Architecture != "x86_64" || repository.SignatureLevel != PacmanSignatureRequired ||
		!slices.Equal(repository.DependsOn, []string{"base/vendor"}) || !slices.Equal(repository.SigningKeys, []string{"vendor"}) ||
		!slices.Equal(repository.Servers, []string{"https://mirror.example.test/$repo/os/$arch"}) || repository.CredentialRef != "file:/run/remotr/pacman-vendor" {
		t.Fatalf("Pacman repository = %+v", repository)
	}
}

func TestPacmanTrustAndRepositoryValidationBoundaries(t *testing.T) {
	validKey := PacmanSigningKey{
		Name: "vendor", Source: "https://keys.example.test/vendor.asc",
		Fingerprint: "0123456789ABCDEF0123456789ABCDEF01234567",
	}
	validRepository := PacmanRepository{
		Name: "vendor-repository", Servers: []string{"https://mirror.example.test/$repo/os/$arch"},
		Architecture: "x86_64", SignatureLevel: PacmanSignatureRequired, SigningKeys: []string{"vendor"},
	}
	tests := []struct {
		name string
		err  error
	}{
		{name: "valid key", err: validKey.Validate()},
		{name: "valid repository", err: validRepository.Validate()},
		{name: "absent key omits material", err: (PacmanSigningKey{Name: "vendor", ResourceMeta: ResourceMeta{Lifecycle: LifecycleAbsent}}).Validate()},
		{name: "absent repository omits fields", err: (PacmanRepository{Name: "vendor-repository", ResourceMeta: ResourceMeta{Lifecycle: LifecycleAbsent}}).Validate()},
		{name: "short key fingerprint", err: func() error { value := validKey; value.Fingerprint = "0123"; return value.Validate() }()},
		{name: "key URL credentials", err: func() error {
			value := validKey
			value.Source = "https://user:pass@keys.example.test/vendor.asc"
			return value.Validate()
		}()},
		{name: "key wrong ownership", err: func() error { value := validKey; value.Ownership = OwnershipFragment; return value.Validate() }()},
		{name: "repository wrong ownership", err: func() error { value := validRepository; value.Ownership = OwnershipNamed; return value.Validate() }()},
		{name: "weak signature policy", err: func() error { value := validRepository; value.SignatureLevel = "optional"; return value.Validate() }()},
		{name: "server URL credentials", err: func() error {
			value := validRepository
			value.Servers = []string{"https://user:pass@mirror.example.test/$repo"}
			return value.Validate()
		}()},
		{name: "invalid architecture", err: func() error { value := validRepository; value.Architecture = "x86_64;Include"; return value.Validate() }()},
		{name: "duplicate server", err: func() error {
			value := validRepository
			value.Servers = []string{validRepository.Servers[0], validRepository.Servers[0]}
			return value.Validate()
		}()},
		{name: "duplicate signing key", err: func() error {
			value := validRepository
			value.SigningKeys = []string{"vendor", "vendor"}
			return value.Validate()
		}()},
		{name: "invalid credential ref", err: func() error {
			value := validRepository
			value.CredentialRef = "literal-secret"
			return value.Validate()
		}()},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if index < 4 && test.err != nil {
				t.Fatalf("Validate() = %v, want valid boundary", test.err)
			}
			if index >= 4 && test.err == nil {
				t.Fatal("Validate() succeeded for unsafe boundary")
			}
		})
	}
}

func FuzzPacmanTrustAndRepositoryValidation(f *testing.F) {
	f.Add("vendor", "https://keys.example.test/vendor.asc", "0123456789ABCDEF0123456789ABCDEF01234567", "https://mirror.example.test/$repo/os/$arch", "x86_64", "required", "vendor", "")
	f.Add("bad;name", "http://keys.invalid/key", "short", "https://user:pass@mirror.invalid/repo", "x86_64;bad", "optional", "bad key", "literal")
	f.Fuzz(func(t *testing.T, name, source, fingerprint, server, architecture, signature, signingKey, credential string) {
		for _, value := range []string{name, source, fingerprint, server, architecture, signature, signingKey, credential} {
			if len(value) > 512 {
				// test-exception: EXC-031
				t.Skip()
			}
		}
		key := PacmanSigningKey{Name: name, Source: source, Fingerprint: fingerprint}
		if err := key.Validate(); err == nil {
			if key.Name == "" || key.NormalizedFingerprint() == "" || !strings.HasPrefix(key.Source, "https://") {
				t.Fatalf("accepted invalid Pacman signing key: %+v", key)
			}
		}
		repository := PacmanRepository{
			Name: name, Servers: []string{server}, Architecture: architecture,
			SignatureLevel: PacmanSignatureLevel(signature), SigningKeys: []string{signingKey}, CredentialRef: credential,
		}
		if err := repository.Validate(); err == nil {
			if repository.Name == "" || len(repository.Servers) != 1 || !repository.SignatureLevel.Valid() || len(repository.SigningKeys) != 1 {
				t.Fatalf("accepted invalid Pacman repository: %+v", repository)
			}
		}
	})
}
