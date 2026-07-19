package effectivehash_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
)

// FuzzCanonicalHashIsOrderInvariantAndSecretSafe proves the version-one
// canonical contract across bounded generated maps, sets, defaults, and secret
// identities. Ordered lists and safe secret-version identity remain hash
// significant while secret material has no route into the input type.
func FuzzCanonicalHashIsOrderInvariantAndSecretSafe(f *testing.F) {
	f.Add([]byte("alpha"), []byte("beta"), uint64(7), false)
	f.Add([]byte{}, []byte{0xff, 0x00}, ^uint64(0), true)
	f.Add([]byte("same"), []byte("same"), uint64(0), false)

	f.Fuzz(func(t *testing.T, rawFirst, rawSecond []byte, number uint64, enabled bool) {
		if len(rawFirst) > 128 || len(rawSecond) > 128 {
			return
		}
		first := hex.EncodeToString(rawFirst)
		second := hex.EncodeToString(rawSecond)
		left := fuzzHashInput(first, second, number, enabled, false)
		right := fuzzHashInput(first, second, number, enabled, true)

		leftCanonical, err := effectivehash.Canonical(left)
		if err != nil {
			t.Fatal(err)
		}
		rightCanonical, err := effectivehash.Canonical(right)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(leftCanonical, rightCanonical) {
			t.Fatalf("unordered input permutation changed canonical document:\nleft:  %s\nright: %s", leftCanonical, rightCanonical)
		}
		if !json.Valid(leftCanonical) {
			t.Fatalf("canonical document is not valid JSON: %q", leftCanonical)
		}
		const secretCanary = "OS-AEC-085-FUZZ-SECRET-CANARY"
		if bytes.Contains(leftCanonical, []byte(secretCanary)) {
			t.Fatalf("canonical document contains secret material: %s", leftCanonical)
		}

		leftHash, err := effectivehash.Sum(left)
		if err != nil {
			t.Fatal(err)
		}
		rightHash, err := effectivehash.Sum(right)
		if err != nil {
			t.Fatal(err)
		}
		if leftHash != rightHash {
			t.Fatalf("unordered input permutation changed hash: left=%q right=%q", leftHash, rightHash)
		}
		if err := effectivehash.Validate(leftHash); err != nil {
			t.Fatalf("Sum returned a non-canonical digest: %v", err)
		}
		wantDigest := sha256.Sum256(leftCanonical)
		if leftHash != "sha256:"+hex.EncodeToString(wantDigest[:]) {
			t.Fatalf("hash %q does not match canonical document", leftHash)
		}

		changedSecret := fuzzHashInput(first, second, number, enabled, false)
		changedSecret.Secrets[0].Version += "-next"
		changedSecretHash, err := effectivehash.Sum(changedSecret)
		if err != nil {
			t.Fatal(err)
		}
		if changedSecretHash == leftHash {
			t.Fatalf("secret version identity change retained hash %q", leftHash)
		}

		if first != second {
			changedOrder := fuzzHashInput(first, second, number, enabled, false)
			changedOrder.Desired["ordered"] = effectivehash.List{
				effectivehash.String("value:" + second),
				effectivehash.String("value:" + first),
			}
			changedOrderHash, err := effectivehash.Sum(changedOrder)
			if err != nil {
				t.Fatal(err)
			}
			if changedOrderHash == leftHash {
				t.Fatalf("ordered list permutation retained hash %q", leftHash)
			}
		}
	})
}

func fuzzHashInput(first, second string, number uint64, enabled, reverse bool) effectivehash.Input {
	labels := make(effectivehash.Object, 2)
	if reverse {
		labels["key:"+second] = effectivehash.String("value:" + second)
		labels["key:"+first] = effectivehash.String("value:" + first)
	} else {
		labels["key:"+first] = effectivehash.String("value:" + first)
		labels["key:"+second] = effectivehash.String("value:" + second)
	}
	settings := make(effectivehash.Object, 2)
	if reverse {
		settings["enabled"] = effectivehash.Boolean(enabled)
		settings["number"] = effectivehash.Unsigned(number)
	} else {
		settings["number"] = effectivehash.Unsigned(number)
		settings["enabled"] = effectivehash.Boolean(enabled)
	}
	set := effectivehash.Set{
		effectivehash.String("value:" + first),
		effectivehash.String("value:" + second),
		effectivehash.Unsigned(number),
	}
	secrets := []effectivehash.SecretIdentity{
		{Path: "credentials.primary", Provider: "remotr", Name: "primary", Version: "version:" + first, Purpose: "api-token"},
		{Path: "credentials.backup", Provider: "remotr", Name: "backup", Version: "version:" + second, ActivationGeneration: number, Purpose: "api-token"},
	}
	if reverse {
		set[0], set[2] = set[2], set[0]
		secrets[0], secrets[1] = secrets[1], secrets[0]
	}
	return effectivehash.Input{
		ResourceAddress: "base/fuzz", ResourceKind: "service",
		Provider: effectivehash.ProviderIdentity{ID: "systemd", ContractRevision: "service-v1"},
		Desired: effectivehash.Object{
			"labels":  labels,
			"members": set,
			"ordered": effectivehash.List{
				effectivehash.String("value:" + first),
				effectivehash.String("value:" + second),
			},
		},
		Defaults: effectivehash.Object{"settings": settings},
		Secrets:  secrets,
	}
}
