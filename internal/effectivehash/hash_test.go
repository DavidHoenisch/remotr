package effectivehash_test

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
)

func TestCanonicalResourceHashMatchesVersionOneContract(t *testing.T) {
	input := effectivehash.Input{
		ResourceAddress: "base/network",
		ResourceKind:    "networkProfile",
		Provider: effectivehash.ProviderIdentity{
			ID:               "network-manager",
			ContractRevision: "network-profile-v3",
		},
		Desired: effectivehash.Object{
			"steps":   effectivehash.List{effectivehash.String("prepare"), effectivehash.String("apply")},
			"ports":   effectivehash.Set{effectivehash.Integer(443), effectivehash.Integer(22)},
			"enabled": effectivehash.Boolean(false),
			"metadata": effectivehash.Object{
				"labels": effectivehash.Object{
					"z": effectivehash.String("last"),
					"a": effectivehash.String("first"),
				},
			},
			"unmanaged": effectivehash.Null{},
		},
		Secrets: []effectivehash.SecretIdentity{{
			Path: "credentialRef", Provider: "remotr", Name: "wifi/office",
			Version: "0002", ActivationGeneration: 8, Purpose: "network-credential",
		}},
	}

	canonical, err := effectivehash.Canonical(input)
	if err != nil {
		t.Fatal(err)
	}
	const wantCanonical = `{"schemaVersion":1,"resource":{"address":"base/network","kind":"networkProfile","provider":{"id":"network-manager","contractRevision":"network-profile-v3"},"desired":{"enabled":false,"metadata":{"labels":{"a":"first","z":"last"}},"ports":{"$set":[22,443]},"steps":["prepare","apply"],"unmanaged":null},"secrets":[{"path":"credentialRef","provider":"remotr","name":"wifi/office","version":"0002","activationGeneration":8,"purpose":"network-credential"}]}}`
	if string(canonical) != wantCanonical {
		t.Fatalf("canonical document = %s\nwant               = %s", canonical, wantCanonical)
	}

	digest, err := effectivehash.Sum(input)
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:de5cc1aa73eab14fb8ca6c1561b085fc85f3965ab42247bfbd8baa5cc3c02265"
	if digest != wantDigest {
		t.Fatalf("digest = %q, want %q", digest, wantDigest)
	}
}

func TestCanonicalScalarDomainIsPortableAndFinite(t *testing.T) {
	input := effectivehash.Input{
		ResourceAddress: "base/browser", ResourceKind: "browserPolicy",
		Provider: effectivehash.ProviderIdentity{ID: "chromium", ContractRevision: "browser-policy-v1"},
		Desired: effectivehash.Object{
			"maximum": effectivehash.Unsigned(math.MaxUint64),
			"ratio":   effectivehash.Float(1.5),
		},
	}
	canonical, err := effectivehash.Canonical(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canonical), `"maximum":18446744073709551615,"ratio":1.5`) {
		t.Fatalf("canonical scalar encoding = %s", canonical)
	}

	input.Desired["ratio"] = effectivehash.Float(math.NaN())
	if _, err := effectivehash.Canonical(input); err == nil {
		t.Fatal("NaN was accepted into canonical JSON")
	}
	input.Desired["ratio"] = effectivehash.Float(math.Inf(1))
	if _, err := effectivehash.Canonical(input); err == nil {
		t.Fatal("infinity was accepted into canonical JSON")
	}
}

func TestCanonicalRejectsNilAndUnsupportedClosedValues(t *testing.T) {
	base := effectivehash.Input{
		ResourceAddress: "base/browser", ResourceKind: "browserPolicy",
		Provider: effectivehash.ProviderIdentity{ID: "chromium", ContractRevision: "browser-policy-v1"},
		Desired:  effectivehash.Object{"enabled": effectivehash.Boolean(true)},
	}

	t.Run("nil", func(t *testing.T) {
		input := base
		input.Desired = effectivehash.Object{"invalid": nil}
		if _, err := effectivehash.Canonical(input); err == nil || !strings.Contains(err.Error(), "canonical value is nil") {
			t.Fatalf("Canonical() error = %v, want nil canonical value refusal", err)
		}
	})

	t.Run("unsupported pointer", func(t *testing.T) {
		input := base
		object := effectivehash.Object{"nested": effectivehash.String("value")}
		input.Desired = effectivehash.Object{"invalid": &object}
		if _, err := effectivehash.Canonical(input); err == nil || !strings.Contains(err.Error(), "unsupported canonical value") {
			t.Fatalf("Canonical() error = %v, want unsupported closed value refusal", err)
		}
	})
}

func TestDefaultsNormalizeButUnmanagedOmissionRemainsDistinct(t *testing.T) {
	base := effectivehash.Input{
		ResourceAddress: "base/service",
		ResourceKind:    "service",
		Provider: effectivehash.ProviderIdentity{
			ID: "systemd", ContractRevision: "service-state-v1",
		},
		Desired: effectivehash.Object{"name": effectivehash.String("sshd")},
		Defaults: effectivehash.Object{
			"enabled": effectivehash.Boolean(true),
			"options": effectivehash.Object{"restart": effectivehash.Boolean(false)},
		},
	}

	omittedDefault, err := effectivehash.Sum(base)
	if err != nil {
		t.Fatal(err)
	}
	explicitDefault := base
	explicitDefault.Desired = effectivehash.Object{
		"name":    effectivehash.String("sshd"),
		"enabled": effectivehash.Boolean(true),
		"options": effectivehash.Object{"restart": effectivehash.Boolean(false)},
	}
	explicitDefaultHash, err := effectivehash.Sum(explicitDefault)
	if err != nil {
		t.Fatal(err)
	}
	if omittedDefault != explicitDefaultHash {
		t.Fatalf("omitted declared default hash = %q, explicit default hash = %q", omittedDefault, explicitDefaultHash)
	}

	explicitUnmanaged := base
	explicitUnmanaged.Desired = effectivehash.Object{
		"name": effectivehash.String("sshd"), "masked": effectivehash.Boolean(false),
	}
	explicitUnmanagedHash, err := effectivehash.Sum(explicitUnmanaged)
	if err != nil {
		t.Fatal(err)
	}
	if omittedDefault == explicitUnmanagedHash {
		t.Fatal("omitted unmanaged field hashed like an explicit false value")
	}
}

func TestUnorderedNestedValuesHaveAnOrderInvariantHash(t *testing.T) {
	newInput := func(random *rand.Rand) effectivehash.Input {
		labels := []struct{ key, value string }{{"z", "last"}, {"a", "first"}, {"m", "middle"}}
		ports := []int64{8443, 22, 443}
		secrets := []effectivehash.SecretIdentity{
			{Path: "credentials.primary", Provider: "remotr", Name: "service/primary", Version: "0003", ActivationGeneration: 4, Purpose: "api-token"},
			{Path: "credentials.backup", Provider: "remotr", Name: "service/backup", Version: "0007", ActivationGeneration: 2, Purpose: "api-token"},
		}
		random.Shuffle(len(labels), func(i, j int) { labels[i], labels[j] = labels[j], labels[i] })
		random.Shuffle(len(ports), func(i, j int) { ports[i], ports[j] = ports[j], ports[i] })
		random.Shuffle(len(secrets), func(i, j int) { secrets[i], secrets[j] = secrets[j], secrets[i] })
		labelObject := make(effectivehash.Object, len(labels))
		for _, label := range labels {
			labelObject[label.key] = effectivehash.String(label.value)
		}
		portSet := make(effectivehash.Set, len(ports))
		for index, port := range ports {
			portSet[index] = effectivehash.Integer(port)
		}
		return effectivehash.Input{
			ResourceAddress: "base/proxy", ResourceKind: "service",
			Provider: effectivehash.ProviderIdentity{ID: "systemd", ContractRevision: "service-state-v1"},
			Desired: effectivehash.Object{
				"metadata": effectivehash.Object{"labels": labelObject},
				"ports":    portSet,
				"steps":    effectivehash.List{effectivehash.String("prepare"), effectivehash.String("apply")},
			},
			Secrets: secrets,
		}
	}

	want, err := effectivehash.Sum(newInput(rand.New(rand.NewSource(1))))
	if err != nil {
		t.Fatal(err)
	}
	for seed := int64(2); seed <= 100; seed++ {
		got, err := effectivehash.Sum(newInput(rand.New(rand.NewSource(seed))))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if got != want {
			t.Fatalf("seed %d reordered an unordered hash: got %q, want %q", seed, got, want)
		}
	}

	reorderedList := newInput(rand.New(rand.NewSource(1)))
	reorderedList.Desired["steps"] = effectivehash.List{effectivehash.String("apply"), effectivehash.String("prepare")}
	got, err := effectivehash.Sum(reorderedList)
	if err != nil {
		t.Fatal(err)
	}
	if got == want {
		t.Fatal("ordered list permutation did not change the hash")
	}
}

func TestProviderAndSecretIdentityChangesInvalidateHashWithoutSecretBytes(t *testing.T) {
	const secretCanary = "OS-AEC-085-SECRET-MATERIAL"
	secretMaterial := []byte(secretCanary)
	if len(secretMaterial) == 0 {
		t.Fatal("invalid test setup")
	}
	base := effectivehash.Input{
		ResourceAddress: "office/wifi", ResourceKind: "networkProfile",
		Provider: effectivehash.ProviderIdentity{ID: "network-manager", ContractRevision: "network-profile-v2"},
		Desired:  effectivehash.Object{"ssid": effectivehash.String("corp")},
		Secrets: []effectivehash.SecretIdentity{{
			Path: "credentialRef", Provider: "remotr", Name: "wifi/office",
			Version: "0001", ActivationGeneration: 5, Purpose: "network-credential",
		}},
	}
	baseHash, err := effectivehash.Sum(base)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := effectivehash.Canonical(base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(canonical), secretCanary) {
		t.Fatalf("canonical document contains secret bytes: %s", canonical)
	}

	tests := []struct {
		name   string
		mutate func(*effectivehash.Input)
	}{
		{"provider contract revision", func(input *effectivehash.Input) { input.Provider.ContractRevision = "network-profile-v3" }},
		{"secret version with identical bytes", func(input *effectivehash.Input) { input.Secrets[0].Version = "0002" }},
		{"secret activation generation", func(input *effectivehash.Input) { input.Secrets[0].ActivationGeneration++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			changed.Secrets = append([]effectivehash.SecretIdentity(nil), base.Secrets...)
			test.mutate(&changed)
			changedHash, err := effectivehash.Sum(changed)
			if err != nil {
				t.Fatal(err)
			}
			if changedHash == baseHash {
				t.Fatalf("identity change retained prior hash %q", baseHash)
			}
		})
	}
}
