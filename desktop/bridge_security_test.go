package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestExternalLinksUseNativeHTTPSHandoff(t *testing.T) {
	var opened []string
	app := NewApp("test", WithExternalLinkOpener(func(_ context.Context, target string) error {
		opened = append(opened, target)
		return nil
	}))

	const allowed = "https://docs.remotr.example/operator-guide#profiles"
	if err := app.OpenExternalLink(allowed); err != nil {
		t.Fatalf("open allowed external link: %v", err)
	}
	if !slices.Equal(opened, []string{allowed}) {
		t.Fatalf("native handoff targets = %v, want [%s]", opened, allowed)
	}

	for _, target := range []string{
		"http://docs.remotr.example/insecure",
		"file:///tmp/local.html",
		"javascript:alert('remote')",
		"//docs.remotr.example/protocol-relative",
		"https://operator:secret@docs.remotr.example/credential",
	} {
		t.Run(target, func(t *testing.T) {
			if err := app.OpenExternalLink(target); err == nil {
				t.Fatal("unsafe external link was accepted")
			}
		})
	}
	if !slices.Equal(opened, []string{allowed}) {
		t.Fatalf("unsafe links reached native handoff: %v", opened)
	}
}

func TestWailsBindingAllowlist(t *testing.T) {
	app := NewApp("test")
	options := newApplicationOptions(app)
	if len(options.Bind) != 1 || options.Bind[0] != app {
		t.Fatalf("bound objects = %#v, want only the typed application service", options.Bind)
	}

	boundType := reflect.TypeOf(options.Bind[0])
	methods := make([]string, 0, boundType.NumMethod())
	for index := 0; index < boundType.NumMethod(); index++ {
		method := boundType.Method(index)
		methods = append(methods, method.Name)
		for output := 0; output < method.Type.NumOut(); output++ {
			assertSafeBridgeOutputType(t, method.Name, method.Type.Out(output), map[reflect.Type]bool{})
		}
	}
	slices.Sort(methods)
	want := []string{
		"ActivateSecretVersion",
		"AddDesktopRBACRule",
		"AuthorizeChangeRequest",
		"BootstrapProfile",
		"BuildLocalPackage",
		"ChangeRequestLifecycle",
		"ChooseAppPackageArchive",
		"ChooseBaselineAdoptionPlan",
		"ChooseLocalPackageSource",
		"ClearDeploymentToken",
		"ClearEnrollmentToken",
		"ConnectProfile",
		"CopyDeploymentToken",
		"CopyEnrollmentToken",
		"CreateBaselineAdoption",
		"CreateDeploymentToken",
		"CreateDesktopRBACRole",
		"CreateEnrollmentToken",
		"CreateLocalPackage",
		"DeleteAppPackage",
		"DeleteDesktopRBACRole",
		"GetApplicationInfo",
		"GetDesktopRBACRole",
		"GetDiagnosticCapabilities",
		"ListAppPackages",
		"ListDeploymentTokens",
		"ListDesktopRBACOperators",
		"ListDesktopRBACRoles",
		"ListSecretVersions",
		"LoadActivityPage",
		"LoadAppPackage",
		"LoadAssetInventory",
		"LoadAuditExportInfo",
		"LoadChangeRequestDetail",
		"LoadDeploymentToken",
		"LoadDiagnosticRequest",
		"LoadEndpointDetail",
		"LoadFirewallReport",
		"LoadFleetDetail",
		"LoadFleetOperationalReports",
		"LoadProfiles",
		"LoadWorkspace",
		"OpenExternalLink",
		"PromoteChangeBaseline",
		"PublishAppPackage",
		"RemoveDesktopRBACRule",
		"RemoveEndpoint",
		"RemoveEndpointLabel",
		"RequestDiagnosticCollection",
		"RequestEndpointAgentUpgrade",
		"RequestFleetAgentUpgrade",
		"RequestGitSync",
		"RevokeDeploymentToken",
		"RevokeSecretVersion",
		"SaveAssetInventory",
		"SaveDeploymentToken",
		"SaveDiagnosticBundle",
		"SaveFirewallReport",
		"SaveProfile",
		"SetDesktopOperatorRoles",
		"SetEndpointLabel",
		"StampDesktopOperatorCredential",
		"UploadSecretVersion",
	}
	if !slices.Equal(methods, want) {
		t.Fatalf("bound method inventory = %v, want %v", methods, want)
	}
}

func TestBindingInventoryExcludesRemainingDeferredAuthority(t *testing.T) {
	boundType := reflect.TypeOf(newApplicationOptions(NewApp("test")).Bind[0])
	forbiddenFragments := []string{
		"commit",
		"configrepo",
		"configurationrepository",
		"desiredstate",
		"deployableartifact",
		"hubsnippet",
		"merge",
		"push",
		"repositorywrite",
		"stage",
	}

	for index := 0; index < boundType.NumMethod(); index++ {
		method := boundType.Method(index)
		normalized := strings.ToLower(method.Name)
		for _, fragment := range forbiddenFragments {
			if strings.Contains(normalized, fragment) {
				t.Errorf("binding %s exposes remaining deferred authority %q", method.Name, fragment)
			}
		}
	}
}

func TestBridgeViewModelsExcludeCredentialCanaries(t *testing.T) {
	const (
		tokenCanary       = "bridge-bootstrap-token-canary"
		privateKeyCanary  = "bridge-private-key-pem-canary"
		certificateCanary = "bridge-certificate-pem-canary"
	)
	models := []any{
		ApplicationInfo{Name: "Remotr Desktop", Version: "test"},
		ConnectionProfile{
			Name:         "Production",
			ServerURL:    "https://remotr.example:8443",
			StateDir:     "/home/operator/.config/remotr",
			CAPath:       "/home/operator/.config/remotr/ca.crt",
			DefaultFleet: "production",
		},
		ConnectionView{
			ProfileName: "Production",
			ServerURL:   "https://remotr.example:8443",
			OperatorID:  "operator-safe-id",
			Roles:       []string{"operator"},
		},
		ConnectionFailure{
			Kind:     ConnectionUnknownCA,
			Message:  "The Remotr server certificate is not trusted.",
			Guidance: "Verify the CA reference.",
		},
		BootstrapFailure{
			Kind:     BootstrapRejected,
			Message:  "The bootstrap token was rejected.",
			Guidance: "Request a new one-time token.",
		},
	}
	encoded, err := json.Marshal(models)
	if err != nil {
		t.Fatalf("encode bridge view models: %v", err)
	}
	for _, canary := range []string{tokenCanary, privateKeyCanary, certificateCanary, "BEGIN PRIVATE KEY", "BEGIN CERTIFICATE"} {
		if strings.Contains(string(encoded), canary) {
			t.Errorf("bridge view models disclosed credential canary %q", canary)
		}
	}
}

func TestBridgeSecurityRejectsRemoteNavigationAndContent(t *testing.T) {
	options := newApplicationOptions(NewApp("test"))
	if options.AssetServer == nil || options.AssetServer.Middleware == nil {
		t.Fatal("release asset middleware is not configured")
	}
	handler := options.AssetServer.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	for _, target := range []string{
		"http://untrusted.example/app.js",
		"https://untrusted.example/app.js",
		"//untrusted.example/app.js",
		"file:///tmp/untrusted.html",
		"data:text/html,<script>untrusted</script>",
	} {
		t.Run(target, func(t *testing.T) {
			parsed, err := url.Parse(target)
			if err != nil {
				t.Fatalf("parse remote target: %v", err)
			}
			request := &http.Request{Method: http.MethodGet, URL: parsed}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("remote target status = %d, want 403", response.Code)
			}
		})
	}
}

func assertSafeBridgeOutputType(t *testing.T, method string, output reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	if output == reflect.TypeFor[error]() {
		return
	}
	for output.Kind() == reflect.Pointer || output.Kind() == reflect.Slice || output.Kind() == reflect.Array {
		output = output.Elem()
	}
	if output.Kind() != reflect.Struct || seen[output] {
		return
	}
	seen[output] = true

	for index := 0; index < output.NumField(); index++ {
		field := output.Field(index)
		normalizedName := strings.ToLower(field.Name)
		for _, forbidden := range []string{"token", "privatekey", "keypem", "certpem", "secret", "httpclient", "tlsconfig", "raw", "diagnosticbytes"} {
			if strings.Contains(normalizedName, forbidden) {
				if method == "CreateEnrollmentToken" && output == reflect.TypeFor[EnrollmentTokenResult]() && field.Name == "Token" {
					continue
				}
				if method == "CreateDeploymentToken" && output == reflect.TypeFor[DeploymentTokenCreateResult]() && field.Name == "Token" {
					continue
				}
				t.Errorf("bound method %s returns forbidden field %s.%s", method, output.Name(), field.Name)
			}
		}
		assertSafeBridgeOutputType(t, method, field.Type, seen)
	}
}
