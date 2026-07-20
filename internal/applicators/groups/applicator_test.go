package groups_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/groups"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-LIA-001: group creation uses fixed argv and records both the requested
// fixed GID and system-group class through the provider boundary.
func TestGroupApplyCreatesFixedSystemGroup(t *testing.T) {
	system := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"getent [group operators]":                   {Err: errors.New("not found")},
		"groupadd [--system --gid 200 -- operators]": {},
	}}
	provider := groups.New(models.GroupResource{
		Name:   "operators",
		Group:  "operators",
		GID:    200,
		System: &system,
		ResourceMeta: models.ResourceMeta{
			Lifecycle: models.LifecyclePresent,
		},
	}, runner)

	if _, met := provider.State(context.Background()); met {
		t.Fatal("missing group must drift")
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := runner.Calls
	want := []executil.MockCall{
		{Name: "getent", Args: []string{"group", "operators"}},
		{Name: "getent", Args: []string{"group", "operators"}},
		{Name: "groupadd", Args: []string{"--system", "--gid", "200", "--", "operators"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

// OS-LIA-001: an existing group may change GID only when configuration opts
// into the dangerous reassignment explicitly.
func TestGroupApplyRequiresExplicitGIDReassignment(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"getent [group operators]": {Stdout: []byte("operators:x:1000:\n")},
	}}
	provider := groups.New(models.GroupResource{
		Name: "operators", Group: "operators", GID: 2000,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}, runner)
	if err := provider.Apply(context.Background()); err == nil {
		t.Fatal("expected explicit reassignment requirement")
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("calls = %#v, want lookup only", runner.Calls)
	}

	provider.Resource.AllowGIDReassignment = true
	runner.Next["groupmod [--gid 2000 -- operators]"] = executil.MockResult{}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := runner.Calls[len(runner.Calls)-1]; got.Name != "groupmod" || !reflect.DeepEqual(got.Args, []string{"--gid", "2000", "--", "operators"}) {
		t.Fatalf("group modification = %#v", got)
	}
}

// OS-AEC-098: a malformed native account-database observation is a probe
// failure, not evidence that the group is absent and safe to create.
func TestGroupMalformedLookupDoesNotCreate(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"getent [group operators]": {Stdout: []byte("malformed native output\n")},
	}}
	provider, err := contract.New(groups.New(models.GroupResource{
		Name: "operators", Group: "operators",
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
	}, runner))
	if err != nil {
		t.Fatal(err)
	}

	result := provider.Apply(context.Background())
	if result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("Apply = %+v, want failed native probe", result)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "getent" {
		t.Fatalf("malformed lookup triggered mutation: %+v", runner.Calls)
	}
}
