package croncatalog_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/croncatalog"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestDebianUpgradeBuiltinExecutesPackageUpdates(t *testing.T) {
	for _, distro := range []types.Distro{types.Debian, types.Ubuntu, types.PopOS} {
		t.Run(string(distro), func(t *testing.T) { testUpgradeBuiltinExecutesPackageUpdates(t, distro) })
	}
}

func testUpgradeBuiltinExecutesPackageUpdates(t *testing.T, distro types.Distro) {
	state, err := croncatalog.Resolve("", models.CronState{Crons: []models.CronJob{{Use: "builtin/system-upgrade"}}})
	if err != nil {
		t.Fatal(err)
	}
	f := facts.Facts{Distro: distro, Arch: types.X86}
	configs := make([]models.Configuration, 0, len(state.Crons))
	for _, job := range state.Crons {
		configs = append(configs, job.ToConfiguration())
	}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"apt-get [update -y]": {}, "apt-get [upgrade -y -q]": {},
		"/usr/bin/false []": {Err: errors.New("exit status 1")},
	}}
	eng, err := engine.New(resolve.Resolve(models.State{Configurations: configs}, f), f, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed != nil {
		t.Fatalf("cron apply failed: %+v", result.Failed)
	}
	var got []executil.MockCall
	for _, call := range runner.Calls {
		if call.Name == "apt-get" {
			got = append(got, call)
		}
	}
	want := []executil.MockCall{{Name: "apt-get", Args: []string{"update", "-y"}}, {Name: "apt-get", Args: []string{"upgrade", "-y", "-q"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("package update commands = %#v, want %#v; skipped = %v", got, want, result.Skipped)
	}
	runner.Calls = nil
	result = eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed != nil {
		t.Fatalf("next scheduled occurrence failed: %+v", result.Failed)
	}
	got = nil
	for _, call := range runner.Calls {
		if call.Name == "apt-get" {
			got = append(got, call)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("next occurrence commands = %#v, want %#v", got, want)
	}
	runner.Calls = nil
	runner.Next["apt-get [update -y]"] = executil.MockResult{Err: errors.New("repository unavailable")}
	result = eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed == nil {
		t.Fatal("failed metadata refresh reported success")
	}
	for _, call := range runner.Calls {
		if call.Name == "apt-get" && reflect.DeepEqual(call.Args, []string{"upgrade", "-y", "-q"}) {
			t.Fatal("upgrade ran after metadata refresh failed")
		}
	}
}

func TestClamAVBuiltinExecutesScheduledScan(t *testing.T) {
	state, err := croncatalog.Resolve("", models.CronState{Crons: []models.CronJob{{Use: "builtin/clamav-scan-debian"}}})
	if err != nil {
		t.Fatal(err)
	}
	f := facts.Facts{Distro: types.Debian, Arch: types.X86}
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"/usr/bin/false []": {Err: errors.New("exit status 1")}, "freshclam [--quiet]": {}, "clamdscan [-r /home]": {},
	}}
	eng, err := engine.New(resolve.Resolve(models.State{Configurations: []models.Configuration{state.Crons[0].ToConfiguration()}}, f), f, runner, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed != nil {
		t.Fatalf("scan failed: %+v", result.Failed)
	}
	var got []executil.MockCall
	for _, call := range runner.Calls {
		if call.Name != "/usr/bin/false" {
			got = append(got, call)
		}
	}
	want := []executil.MockCall{{Name: "freshclam", Args: []string{"--quiet"}}, {Name: "clamdscan", Args: []string{"-r", "/home"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scan commands = %#v, want %#v", got, want)
	}
}
