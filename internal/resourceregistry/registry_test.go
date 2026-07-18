package resourceregistry_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/authorizedkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/browserpolicy"
	"github.com/DavidHoenisch/remotr/internal/applicators/certificates"
	"github.com/DavidHoenisch/remotr/internal/applicators/desktopsettings"
	"github.com/DavidHoenisch/remotr/internal/applicators/knownhosts"
	"github.com/DavidHoenisch/remotr/internal/applicators/loginpolicy"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkfiles"
	servicecontracts "github.com/DavidHoenisch/remotr/internal/applicators/services"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/interactiveuser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"gopkg.in/yaml.v3"
)

func TestRegistryRoutesFileBackedNetworkProfileProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: networkProfile\nname: uplink\nprovider: netplan\nselector: {name: eth0}\nprofileName: office\nprofileType: ethernet\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{StateDir: "/var/lib/remotr", Facts: facts.Facts{Network: facts.NetworkNetplan}})
	provider, ok := handler.(*networkfiles.Applicator)
	if err != nil || !ok || provider.StateDir != "/var/lib/remotr" || provider.Resource.Provider != models.NetworkProviderNetplan {
		t.Fatalf("NewProvider() = %#v, %v", handler, err)
	}
}

func TestDefaultRegistryCoversEveryCurrentResourceContract(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := map[models.ResourceKind]bool{
		models.ResourceKindPackage: false, models.ResourceKindAPTSigningKey: false, models.ResourceKindAPTRepository: false, models.ResourceKindSysctl: false, models.ResourceKindKernelModule: false, models.ResourceKindHostname: false, models.ResourceKindHostLocale: false, models.ResourceKindTimeSync: false, models.ResourceKindMount: false, models.ResourceKindSwap: false, models.ResourceKindFile: false,
		models.ResourceKindDirectory:     false,
		models.ResourceKindLink:          false,
		models.ResourceKindGroup:         false,
		models.ResourceKindAuthorizedKey: false,
		models.ResourceKindKnownHost:     false,
		models.ResourceKindSudo:          false,
		models.ResourceKindUserFile:      false, models.ResourceKindDesktopSetting: false, models.ResourceKindSessionPolicy: false, models.ResourceKindBrowserPolicy: false, models.ResourceKindDownload: false,
		models.ResourceKindUser: false, models.ResourceKindSystemd: false,
		models.ResourceKindEndpointSchedule: false,
		models.ResourceKindSystemdUser:      false, models.ResourceKindBootstrap: false,
		models.ResourceKindService:      false,
		models.ResourceKindSystemdUnit:  false,
		models.ResourceKindReboot:       false,
		models.ResourceKindAgentInstall: false, models.ResourceKindFirewall: false, models.ResourceKindHostsEntry: false, models.ResourceKindDNSResolver: false, models.ResourceKindRoute: false, models.ResourceKindNetworkProfile: false,
		models.ResourceKindCertificate:     false,
		models.ResourceKindTrustAnchor:     false,
		models.ResourceKindAppArmorProfile: false,
		models.ResourceKindAuditRules:      false,
		models.ResourceKindAccountLimit:    false,
		models.ResourceKindLoginPolicy:     false,
		models.ResourceKindJournald:        false,
		models.ResourceKindLogrotate:       false,
		models.ResourceKindCommand:         false,
	}
	for _, definition := range registry.Definitions() {
		if _, expected := wantKinds[definition.Kind]; !expected {
			t.Fatalf("unexpected registered kind %q", definition.Kind)
		}
		wantKinds[definition.Kind] = true
		if definition.Decode == nil || definition.Validate == nil || definition.Metadata == nil ||
			definition.DefaultRisk == nil || definition.PlanDescriptor == nil || definition.ProviderFactory == nil ||
			definition.OrderingTier == nil || definition.LockDomains == nil || !definition.Sensitivity.Valid() || len(definition.FieldDescriptors) == 0 {
			t.Fatalf("kind %q has incomplete registry contract: %#v", definition.Kind, definition)
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Errorf("kind %q is not registered", kind)
		}
	}
}

// OS-AEC-083: every field accepted by strict resource decoding must have an
// explicit sensitivity descriptor before the resource kind can register.
func TestRegistryRejectsUnclassifiedAcceptedField(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	fields := make(resourceregistry.FieldDescriptors, len(definition.FieldDescriptors))
	for path, descriptor := range definition.FieldDescriptors {
		fields[path] = descriptor
	}
	delete(fields, "content")
	definition.FieldDescriptors = fields

	if _, err := resourceregistry.New(definition); err == nil || !strings.Contains(err.Error(), `field "content"`) {
		t.Fatalf("New() error = %v, want unclassified accepted content field", err)
	}
}

// OS-AEC-086: every registered resource/provider boundary must own its plan
// evidence; callers cannot fill a missing descriptor with authoritative data.
func TestRegistryRejectsMissingProviderPlanDescriptor(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	definition.PlanDescriptor = nil

	if _, err := resourceregistry.New(definition); err == nil || !strings.Contains(err.Error(), "incomplete definition") {
		t.Fatalf("New() error = %v, want incomplete provider plan descriptor", err)
	}
}

func TestResourceRejectsUnsafeRegisteredProviderPlanEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	definition.PlanDescriptor = func(any, string) (providercontract.PlanDescriptor, error) {
		return providercontract.PlanDescriptor{
			Effects: []providercontract.PlanEffect{{
				Code: providercontract.EffectResourceUpdate,
				Details: executor.SafeSummary{Fields: []executor.SafeField{{
					Path:        "content",
					Sensitivity: executor.SafeSecret,
					Projection:  executor.SafeValue,
					Text:        "secret-canary-provider-plan",
				}}},
			}},
			RollbackClass: providercontract.RollbackTransactional,
		}, nil
	}
	unsafeRegistry, err := resourceregistry.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := unsafeRegistry.Resources(&models.Configuration{Files: []models.File{{Name: "secret"}}})
	if err != nil || len(resources) != 1 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}
	if _, err := resources[0].PlanDescriptor("file"); err == nil || !strings.Contains(err.Error(), "plan effect details") {
		t.Fatalf("PlanDescriptor() error = %v, want secret-bearing evidence rejection", err)
	}
}

func TestSudoProviderPlanDescriptorOwnsTypedEffectAndRollbackEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{Sudo: []models.SudoResource{{
		ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindSudo},
		Name:         "operators",
	}}}
	resources, err := registry.Resources(&configuration)
	if err != nil || len(resources) != 1 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}

	descriptor, err := resources[0].PlanDescriptor("sudo")
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptor.Effects) != 1 || descriptor.Effects[0].Code != providercontract.EffectSudoPolicyReplace {
		t.Fatalf("effects = %+v, want typed sudo policy replacement", descriptor.Effects)
	}
	if descriptor.RollbackClass != providercontract.RollbackTransactional || !descriptor.BaselineEligible {
		t.Fatalf("descriptor = %+v, want transactional baseline-eligible plan", descriptor)
	}
}

func TestBrowserProviderPlanDescriptorPredictsTypedActivationTarget(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{BrowserPolicies: []models.BrowserPolicyResource{{
		ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindBrowserPolicy},
		Name:         "homepage",
		Browser:      models.BrowserChromium,
	}}}
	resources, err := registry.Resources(&configuration)
	if err != nil || len(resources) != 1 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}

	descriptor, err := resources[0].PlanDescriptor("chromium-policy")
	if err != nil {
		t.Fatal(err)
	}
	wantActivation := providercontract.ActivationTarget{
		Kind: providercontract.ActivationApplicationRestart, Target: "chromium",
	}
	if len(descriptor.ActivationTargets) != 1 || descriptor.ActivationTargets[0] != wantActivation {
		t.Fatalf("activation targets = %+v, want %+v", descriptor.ActivationTargets, wantActivation)
	}
	if descriptor.RollbackClass != providercontract.RollbackTransactional || !descriptor.BaselineEligible {
		t.Fatalf("descriptor = %+v, want transactional baseline-eligible plan", descriptor)
	}
}

func TestProviderPlanDescriptorsExcludeOneShotAndDestructiveResourcesFromBaselines(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{
		Packages: []models.Package{{ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindPackage}, Name: "curl"}},
		Reboots:  []models.RebootResource{{ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindReboot}, Name: "kernel"}},
		Commands: []models.CommandResource{{ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindCommand}, Name: "escape-hatch"}},
	}
	resources, err := registry.Resources(&configuration)
	if err != nil {
		t.Fatal(err)
	}
	eligible := make(map[models.ResourceKind]bool, len(resources))
	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("registered-provider")
		if err != nil {
			t.Fatalf("%s PlanDescriptor(): %v", resource.Kind(), err)
		}
		eligible[resource.Kind()] = descriptor.BaselineEligible
	}
	if !eligible[models.ResourceKindPackage] {
		t.Fatal("ordinary package provider is not baseline eligible")
	}
	if eligible[models.ResourceKindReboot] || eligible[models.ResourceKindCommand] {
		t.Fatalf("one-shot/destructive baseline eligibility = reboot:%t command:%t", eligible[models.ResourceKindReboot], eligible[models.ResourceKindCommand])
	}
}

func TestProtectedProvidersAdvertiseTransactionalPlanRollback(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	audit := false
	configuration := models.Configuration{
		APTSigningKeys:  []models.APTSigningKey{{Name: "key"}},
		APTRepositories: []models.APTRepository{{Name: "repo"}},
		Files:           []models.File{{Name: "file"}},
		AuthorizedKeys:  []models.AuthorizedKeyResource{{Name: "authorized-key"}},
		KnownHosts:      []models.KnownHostResource{{Name: "known-host"}},
		Sudo:            []models.SudoResource{{Name: "sudo"}},
		Downloads:       []models.DownloadResource{{Name: "download"}},
		BrowserPolicies: []models.BrowserPolicyResource{{Name: "browser", Browser: models.BrowserChromium}},
		HostsEntries:    []models.HostsEntryResource{{Name: "hosts"}},
		NetworkProfiles: []models.NetworkProfileResource{{Name: "network", Audit: &audit}},
		Certificates:    []models.CertificateResource{{Name: "certificate"}},
		TrustAnchors:    []models.TrustAnchorResource{{Name: "trust-anchor"}},
		AccountLimits:   []models.AccountLimitResource{{Name: "account-limit"}},
		LoginPolicies:   []models.LoginPolicyResource{{Name: "login-policy"}},
		Journald:        []models.JournaldResource{{Name: "journald"}},
		Logrotate:       []models.LogrotateResource{{Name: "logrotate"}},
	}
	resources, err := registry.Resources(&configuration)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("registered-provider")
		if err != nil {
			t.Fatalf("%s PlanDescriptor(): %v", resource.Kind(), err)
		}
		if descriptor.RollbackClass != providercontract.RollbackTransactional {
			t.Errorf("%s rollback class = %q, want transactional", resource.Kind(), descriptor.RollbackClass)
		}
	}
}

func TestFirewallPlanDescriptorReflectsAuditAndTransactionalProviderModes(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	enforced := false
	configuration := models.Configuration{Firewall: []models.FirewallResource{
		{Name: "audit"},
		{Name: "enforced", Audit: &enforced},
	}}
	resources, err := registry.Resources(&configuration)
	if err != nil || len(resources) != 2 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}

	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("firewall")
		if err != nil {
			t.Fatalf("%s PlanDescriptor(): %v", resource.Name(), err)
		}
		if len(descriptor.Effects) != 1 || descriptor.Effects[0].Code != providercontract.EffectFirewallPolicyReplace {
			t.Fatalf("%s effects = %+v", resource.Name(), descriptor.Effects)
		}
		wantRollback := providercontract.RollbackNone
		if resource.Name() == "enforced" {
			wantRollback = providercontract.RollbackTransactional
		}
		if descriptor.RollbackClass != wantRollback {
			t.Errorf("%s rollback = %q, want %q", resource.Name(), descriptor.RollbackClass, wantRollback)
		}
	}
}

func TestProviderPlanDescriptorDerivesTypedActivationFromResourceMetadata(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{Packages: []models.Package{{
		ResourceMeta: models.ResourceMeta{Notifications: []models.Notification{{
			Type: models.NotificationTryRestart, Target: "telemetry.service",
		}}},
		Name: "telemetry",
	}}}
	resources, err := registry.Resources(&configuration)
	if err != nil || len(resources) != 1 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}
	descriptor, err := resources[0].PlanDescriptor("apt")
	if err != nil {
		t.Fatal(err)
	}
	want := providercontract.ActivationTarget{Kind: providercontract.ActivationTryRestart, Target: "telemetry.service"}
	if len(descriptor.ActivationTargets) != 1 || descriptor.ActivationTargets[0] != want {
		t.Fatalf("activation targets = %+v, want %+v", descriptor.ActivationTargets, want)
	}
}

func TestProviderPlanDescriptorsPredictProviderOwnedActivationTargets(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{
		Sysctls:         []models.SysctlResource{{Name: "boot-tuning", Activation: models.SysctlNextBoot}},
		TimeSync:        []models.TimeSyncResource{{Name: "ntp", Servers: []string{"time.example.test"}}},
		SessionPolicies: []models.SessionPolicyResource{{Name: "session"}},
		SystemdUnits:    []models.SystemdUnitResource{{Name: "telemetry-unit"}},
		AccountLimits:   []models.AccountLimitResource{{Name: "limits"}},
		Journald:        []models.JournaldResource{{Name: "journal"}},
	}
	resources, err := registry.Resources(&configuration)
	if err != nil {
		t.Fatal(err)
	}
	want := map[models.ResourceKind]providercontract.ActivationTarget{
		models.ResourceKindSysctl:        {Kind: providercontract.ActivationNextBoot},
		models.ResourceKindTimeSync:      {Kind: providercontract.ActivationRestart, Target: "systemd-timesyncd.service"},
		models.ResourceKindSessionPolicy: {Kind: providercontract.ActivationLogoutRequired},
		models.ResourceKindSystemdUnit:   {Kind: providercontract.ActivationDaemonReload},
		models.ResourceKindAccountLimit:  {Kind: providercontract.ActivationLogoutRequired},
		models.ResourceKindJournald:      {Kind: providercontract.ActivationRestart, Target: "systemd-journald.service"},
	}
	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("registered-provider")
		if err != nil {
			t.Fatalf("%s PlanDescriptor(): %v", resource.Kind(), err)
		}
		expected, ok := want[resource.Kind()]
		if !ok {
			continue
		}
		if len(descriptor.ActivationTargets) != 1 || descriptor.ActivationTargets[0] != expected {
			t.Errorf("%s activation targets = %+v, want %+v", resource.Kind(), descriptor.ActivationTargets, expected)
		}
	}
}

func TestDownloadPlanDescriptorRejectsFreeFormActivationEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{Downloads: []models.DownloadResource{
		{Name: "typed", ReloadExec: []string{"systemctl", "reload", "auditd.service"}},
		{Name: "free-form", ReloadExec: []string{"sh", "-c", "secret-canary-provider-plan"}},
	}}
	resources, err := registry.Resources(&configuration)
	if err != nil || len(resources) != 2 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}
	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("download")
		if resource.Name() == "free-form" {
			if err == nil || !strings.Contains(err.Error(), "unrepresentable reloadExec") {
				t.Fatalf("free-form PlanDescriptor() = %+v, %v", descriptor, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		want := providercontract.ActivationTarget{Kind: providercontract.ActivationReload, Target: "auditd.service"}
		if len(descriptor.ActivationTargets) != 1 || descriptor.ActivationTargets[0] != want {
			t.Fatalf("typed activation targets = %+v, want %+v", descriptor.ActivationTargets, want)
		}
	}
}

func TestNetworkPlanDescriptorsUseBoundedTypedEffectCodes(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	configuration := models.Configuration{
		DNSResolvers: []models.DNSResolverResource{{Name: "resolver"}},
		Routes:       []models.RouteResource{{Name: "default", Destination: "0.0.0.0/0"}},
	}
	resources, err := registry.Resources(&configuration)
	if err != nil {
		t.Fatal(err)
	}
	want := map[models.ResourceKind]providercontract.EffectCode{
		models.ResourceKindDNSResolver: providercontract.EffectNetworkDNSReplace,
		models.ResourceKindRoute:       providercontract.EffectDefaultRouteReplace,
	}
	for _, resource := range resources {
		descriptor, err := resource.PlanDescriptor("network-provider")
		if err != nil {
			t.Fatalf("%s PlanDescriptor(): %v", resource.Kind(), err)
		}
		if len(descriptor.Effects) != 1 || descriptor.Effects[0].Code != want[resource.Kind()] {
			t.Errorf("%s effects = %+v, want %q", resource.Kind(), descriptor.Effects, want[resource.Kind()])
		}
	}
}

func TestTrustAnchorPlanDescriptorPredictsSelectedProviderRefresh(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{
		TrustAnchors: []models.TrustAnchorResource{{Name: "corporate"}},
	})
	if err != nil || len(resources) != 1 {
		t.Fatalf("Resources() = %+v, %v", resources, err)
	}
	descriptor, err := resources[0].PlanDescriptor("debian")
	if err != nil {
		t.Fatal(err)
	}
	want := providercontract.ActivationTarget{Kind: providercontract.ActivationTrustStoreRefresh, Target: "debian"}
	if len(descriptor.ActivationTargets) != 1 || descriptor.ActivationTargets[0] != want {
		t.Fatalf("activation targets = %+v, want %+v", descriptor.ActivationTargets, want)
	}
}

func TestRegistryReturnsImmutableFieldDescriptorCopies(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	definition.FieldDescriptors["content"] = resourceregistry.FieldDescriptor{}

	registered, ok := registry.Definition(models.ResourceKindFile)
	want := resourceregistry.FieldDescriptor{Sensitivity: resourceregistry.SensitivitySecret, Projection: resourceregistry.ProjectOmit}
	if !ok || registered.FieldDescriptors["content"] != want {
		t.Fatalf("registered content descriptor was mutated: %+v", registered.FieldDescriptors["content"])
	}
}

func TestRegistryRejectsInvalidAndUnknownFieldDescriptors(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	base, ok := registry.Definition(models.ResourceKindFile)
	if !ok {
		t.Fatal("file definition is not registered")
	}
	for _, test := range []struct {
		name    string
		mutate  func(resourceregistry.FieldDescriptors)
		message string
	}{
		{
			name: "invalid classification",
			mutate: func(fields resourceregistry.FieldDescriptors) {
				fields["content"] = resourceregistry.FieldDescriptor{}
			},
			message: `field "content"`,
		},
		{
			name: "unknown field",
			mutate: func(fields resourceregistry.FieldDescriptors) {
				fields["notAccepted"] = resourceregistry.FieldDescriptor{Sensitivity: resourceregistry.SensitivityPublic}
			},
			message: `unknown field "notAccepted"`,
		},
		{
			name: "secret emitted as value",
			mutate: func(fields resourceregistry.FieldDescriptors) {
				fields["content"] = resourceregistry.FieldDescriptor{
					Sensitivity: resourceregistry.SensitivitySecret,
					Projection:  resourceregistry.ProjectValue,
				}
			},
			message: `field "content"`,
		},
		{
			name: "public field omitted",
			mutate: func(fields resourceregistry.FieldDescriptors) {
				fields["mode[]"] = resourceregistry.FieldDescriptor{
					Sensitivity: resourceregistry.SensitivityPublic,
					Projection:  resourceregistry.ProjectOmit,
				}
			},
			message: `field "mode[]"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := base
			definition.FieldDescriptors = make(resourceregistry.FieldDescriptors, len(base.FieldDescriptors)+1)
			for path, descriptor := range base.FieldDescriptors {
				definition.FieldDescriptors[path] = descriptor
			}
			test.mutate(definition.FieldDescriptors)
			if _, err := resourceregistry.New(definition); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("New() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestDefaultRegistryClassifiesNestedFieldsAndSafeProjections(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind        models.ResourceKind
		path        string
		sensitivity resourceregistry.Sensitivity
		projection  resourceregistry.SafeProjection
	}{
		{models.ResourceKindFile, "kind", resourceregistry.SensitivityPublic, resourceregistry.ProjectValue},
		{models.ResourceKindFile, "content", resourceregistry.SensitivitySecret, resourceregistry.ProjectOmit},
		{models.ResourceKindEndpointSchedule, "environment[].secretRef", resourceregistry.SensitivitySecret, resourceregistry.ProjectReference},
		{models.ResourceKindEndpointSchedule, "environment[].value", resourceregistry.SensitivitySecret, resourceregistry.ProjectOmit},
		{models.ResourceKindAuthorizedKey, "entries[].key", resourceregistry.SensitivitySensitiveMetadata, resourceregistry.ProjectOmit},
		{models.ResourceKindAuthorizedKey, "entries[].fingerprint", resourceregistry.SensitivitySensitiveMetadata, resourceregistry.ProjectFingerprint},
		{models.ResourceKindAPTRepository, "credentialRef", resourceregistry.SensitivitySecret, resourceregistry.ProjectReference},
		{models.ResourceKindCertificate, "privateKeyRef", resourceregistry.SensitivitySecret, resourceregistry.ProjectReference},
		{models.ResourceKindCommand, "providerOptions.*.*", resourceregistry.SensitivitySecret, resourceregistry.ProjectOmit},
	} {
		t.Run(string(test.kind)+"/"+test.path, func(t *testing.T) {
			definition, ok := registry.Definition(test.kind)
			if !ok {
				t.Fatalf("kind %q is not registered", test.kind)
			}
			got, ok := definition.FieldDescriptors[test.path]
			if !ok || got.Sensitivity != test.sensitivity || got.Projection != test.projection {
				t.Fatalf("descriptor = %+v present=%t, want sensitivity=%q projection=%q", got, ok, test.sensitivity, test.projection)
			}
		})
	}
}

func TestRegistryBuildsOnlyDebianFamilyLoginPolicyProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: loginPolicy\nname: baseline\nprovider: pam-auth-update\nrecoveryPrincipals: [recovery]\nrules:\n  - {section: auth, control: required, module: pam_faillock.so}\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Distro: types.Debian}})
	if _, ok := handler.(*loginpolicy.Applicator); err != nil || !ok {
		t.Fatalf("Debian NewProvider() = %T, %v", handler, err)
	}
	handler, err = resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Distro: types.Arch}})
	if err != nil {
		t.Fatal(err)
	}
	if check := executor.Check(t.Context(), handler); check.Status != executor.Unsupported {
		t.Fatalf("Arch Check() = %+v", check)
	}
}

func TestRegistryBuildsCertificateProviderAtSensitiveExecutionSeam(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: certificate\nname: service\ncertificatePath: /etc/service/tls.crt\nprivateKeyPath: /etc/service/tls.key\ncertificateRef: remotr:certificates/service@active\nprivateKeyRef: remotr:private-keys/service@7\nrenewalPolicy: provider\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	provider, ok := handler.(*certificates.Applicator)
	if err != nil || !ok || resource.Sensitivity() != resourceregistry.SensitivitySecret || resource.DefaultRisk() != models.RiskSensitive || provider.Resource.Name != "service" {
		t.Fatalf("certificate provider = %T resource=%#v err=%v", handler, resource, err)
	}
}

func TestRegistryBuildsCapabilityGatedDesktopSettingProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: desktopSetting\nname: animations\nprovider: dconf\nscope: user\nselector: {mode: all-interactive}\npath: /org/gnome/desktop/interface/enable-animations\nvalue: {type: boolean, value: false}\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopDconf}}, StateDir: "/var/lib/remotr", ResourceAddress: "workstation/animations"})
	provider, ok := handler.(*desktopsettings.Applicator)
	if err != nil || !ok || provider.StateDir != "/var/lib/remotr" || provider.StateKey != "workstation/animations" {
		t.Fatalf("dconf provider = %T, %v", handler, err)
	}
	handler, err = resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Desktop: []facts.DesktopBackend{facts.DesktopGSettings}}})
	if err != nil {
		t.Fatal(err)
	}
	if check := executor.Check(t.Context(), handler); check.Status != executor.Unsupported {
		t.Fatalf("mismatched desktop provider Check() = %+v", check)
	}
}

func TestRegistryBuildsCapabilityGatedBrowserPolicyProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: browserPolicy\nname: homepage\nbrowser: chromium\npolicyName: HomepageLocation\nscope: system\nlevel: mandatory\nvalue: {type: string, value: 'https://example.test'}\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Browser: []facts.BrowserBackend{facts.BrowserChromium}}})
	if _, ok := handler.(*browserpolicy.Applicator); err != nil || !ok {
		t.Fatalf("browser policy provider = %T, %v", handler, err)
	}
	handler, err = resource.NewProvider(resourceregistry.FactoryContext{Facts: facts.Facts{Browser: []facts.BrowserBackend{facts.BrowserFirefox}}})
	if err != nil {
		t.Fatal(err)
	}
	if check := executor.Check(t.Context(), handler); check.Status != executor.Unsupported {
		t.Fatalf("mismatched browser provider Check() = %+v", check)
	}
}

func TestRegistryDoesNotAdvertiseDeferredServiceProviders(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: service\nname: ssh\nprovider: openrc\nscope: system\nservice: sshd\nactive: true\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if handler != nil || err == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
	}
	var unavailable servicecontracts.ProviderNotAdvertisedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("NewProvider() error = %T %v", err, err)
	}
}

func TestRegistryAdaptsProviderNeutralServiceScopesToSystemdProviders(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, yaml string
		assert     func(t *testing.T, handler any)
	}{
		{
			name: "system",
			yaml: "kind: service\nname: ssh\nprovider: systemd\nscope: system\nservice: ssh.service\nenabled: true\nactive: true\nmasked: false\n",
			assert: func(t *testing.T, handler any) {
				provider, ok := handler.(*systemd.Applicator)
				if !ok || provider.Resource.Unit != "ssh.service" || provider.Resource.Masked == nil {
					t.Fatalf("system provider = %#v", handler)
				}
			},
		},
		{
			name: "user",
			yaml: "kind: service\nname: desktop-agent\nprovider: systemd\nscope: user\nservice: desktop-agent.service\nusers: interactive\nlinger: true\nenabled: true\nactive: true\nmasked: false\n",
			assert: func(t *testing.T, handler any) {
				provider, ok := handler.(*systemduser.Applicator)
				if !ok || provider.Resource.Unit != "desktop-agent.service" || provider.Resource.Masked == nil {
					t.Fatalf("user provider = %#v", handler)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(test.yaml), &node); err != nil {
				t.Fatal(err)
			}
			resource, err := registry.Decode(node.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := resource.Validate(); err != nil {
				t.Fatal(err)
			}
			handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
			if err != nil {
				t.Fatal(err)
			}
			test.assert(t, handler)
		})
	}
}

func TestRegistryBuildsHostLocaleProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: hostLocale\nname: utc\ntimezone: UTC\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind() != models.ResourceKindHostLocale || resource.Name() != "utc" {
		t.Fatalf("decoded identity = %q/%q", resource.Kind(), resource.Name())
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if err != nil || handler == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
	}
}

func TestRegistryDecodesValidatesAndBuildsProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: file\nname: motd\npath: /etc/motd\ncontent: managed\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind() != models.ResourceKindFile || resource.Name() != "motd" {
		t.Fatalf("decoded identity = %q/%q", resource.Kind(), resource.Name())
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if resource.Sensitivity() != resourceregistry.SensitivityPublic || resource.DefaultRisk() != models.RiskNormal {
		t.Fatalf("classification = %q/%q", resource.Sensitivity(), resource.DefaultRisk())
	}
	if resource.OrderingTier() != 1 || len(resource.LockDomains()) != 0 {
		t.Fatalf("ordering/locks = %d/%v", resource.OrderingTier(), resource.LockDomains())
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if err != nil || handler == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
	}
}

// OS-LIA-009: known_hosts entries manage outbound host trust, so unlike
// authoritative SSH authorization they are normal-risk and may converge
// without an access-recovery preflight.
func TestKnownHostDefaultsToNormalRisk(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	data := []byte("kind: knownHost\nname: git\nlifecycle: present\nownership: named\nscope: system\nhosts: [git.example]\ntype: ssh-ed25519\nkey: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W\nfingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4\nhashing: plain\n")
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	if resource.DefaultRisk() != models.RiskNormal {
		t.Fatalf("knownHost default risk = %q, want %q", resource.DefaultRisk(), models.RiskNormal)
	}
}

func TestRegistryConfiguresAuthorizedKeyProtectedRollback(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	data := []byte("kind: authorizedKey\nname: restart-access\nuser: admin\nlifecycle: present\nownership: merge\nentries:\n  - type: ssh-ed25519\n    key: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W\n    fingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4\n")
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{
		StateDir: t.TempDir(), ResourceAddress: "authorizedKey.restart-access", ArtifactDigest: "artifact-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := handler.(*authorizedkeys.Applicator)
	if !ok {
		t.Fatalf("provider = %T, want *authorizedkeys.Applicator", handler)
	}
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider.LookupUser = func(string) (interactiveuser.Account, error) {
		return interactiveuser.Account{Username: "admin", UID: os.Getuid(), GID: os.Getgid(), HomeDir: home}, nil
	}
	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("registry provider ApplyResult = %+v, want changed transactional", result)
	}
}

func TestRegistryConfiguresKnownHostProtectedRollback(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	data := []byte("kind: knownHost\nname: restart-host\nlifecycle: present\nownership: named\nscope: system\nhosts: [git.example]\ntype: ssh-ed25519\nkey: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W\nfingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4\nhashing: plain\n")
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{
		StateDir: t.TempDir(), ResourceAddress: "knownHost.restart-host", ArtifactDigest: "artifact-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := handler.(*knownhosts.Applicator)
	if !ok {
		t.Fatalf("provider = %T, want *knownhosts.Applicator", handler)
	}
	provider.SystemPath = filepath.Join(t.TempDir(), "ssh_known_hosts")
	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional || result.Err != nil {
		t.Fatalf("registry provider ApplyResult = %+v, want changed transactional", result)
	}
}
