package networkresources

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRouteApplicatorReportsEffectiveDriftSeparately(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]":                          {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-g ipv4.routes connection show office]":                              {Stdout: []byte("10.20.0.0/16 192.0.2.1 50, table=254\n")},
		"ip [-json route show exact 10.20.0.0/16 table 254]":                         {Stdout: []byte("[]\n")},
		"ip [route replace 10.20.0.0/16 via 192.0.2.1 dev eth0 metric 50 table 254]": {},
	}}
	resource := models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "private-network", Provider: models.NetworkProviderNetworkManager, Interface: "eth0",
		Destination: "10.20.0.0/16", Gateway: "192.0.2.1", Metric: 50, Table: 254, Configured: true, Effective: true,
	}
	provider := NewRoute(resource, runner)
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted {
		t.Fatalf("Check() = %+v", check)
	}
	report, ok := check.Actual.(RouteStateReport)
	if !ok || !report.Configured.Compliant || report.Effective.Compliant {
		t.Fatalf("route scope report = %#v", check.Actual)
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.Calls {
		if call.Name == "nmcli" && len(call.Args) > 1 && call.Args[0] == "connection" && call.Args[1] == "modify" {
			t.Fatalf("runtime-only drift rewrote persistent route: %+v", call)
		}
	}
}

func TestRouteApplicatorReportsUnadvertisedBackendAsUnsupported(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	provider := NewRoute(models.RouteResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "private-network", Provider: models.NetworkProviderNetplan, Interface: "eth0",
		Destination: "10.20.0.0/16", Configured: true,
	}, runner)

	check := provider.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != executor.ReasonProviderUnavailable {
		t.Fatalf("Check() = %+v, want typed unsupported backend", check)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unsupported route backend crossed process boundary: %+v", runner.Calls)
	}
}
