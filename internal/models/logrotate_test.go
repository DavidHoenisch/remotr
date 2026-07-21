package models_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalStructuredLogrotateFragment(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: logging
    resources:
      - kind: logrotate
        name: remotr-agent
        paths: [/var/log/remotr/*.log]
        cadence: daily
        retention: 14
        compress: true
        create: {mode: "0640", owner: root, group: adm}
        sharedScripts: true
        preRotate: {command: [/usr/bin/test, -d, /var/log/remotr]}
        postRotate: {command: [/usr/bin/systemctl, reload, remotr-agent.service]}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Logrotate) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].Logrotate[0]
	if resource.Kind != models.ResourceKindLogrotate || resource.Retention == nil || *resource.Retention != 14 ||
		resource.Create == nil || resource.Create.Mode != "0640" || resource.PostRotate == nil || len(resource.PostRotate.Command) != 3 {
		t.Fatalf("logrotate fragment = %#v", resource)
	}
}

func TestLogrotateValidationRejectsUnsafeStructuredFields(t *testing.T) {
	retention := 7
	base := func() models.LogrotateResource {
		return models.LogrotateResource{Name: "policy", Paths: []string{"/var/log/app/*.log"}, Cadence: models.LogrotateDaily, Retention: &retention}
	}
	for _, test := range []struct {
		name   string
		mutate func(*models.LogrotateResource)
	}{
		{name: "empty name", mutate: func(r *models.LogrotateResource) { r.Name = "" }},
		{name: "oversized name", mutate: func(r *models.LogrotateResource) { r.Name = strings.Repeat("a", 128) }},
		{name: "missing paths", mutate: func(r *models.LogrotateResource) { r.Paths = nil }},
		{name: "path injection", mutate: func(r *models.LogrotateResource) { r.Paths = []string{"/var/log/app {\nrotate 0"} }},
		{name: "duplicate path", mutate: func(r *models.LogrotateResource) {
			r.Paths = []string{"/var/log/app/*.log", "/var/log/app/*.log"}
		}},
		{name: "cadence", mutate: func(r *models.LogrotateResource) { r.Cadence = "sometimes" }},
		{name: "retention", mutate: func(r *models.LogrotateResource) { value := -1; r.Retention = &value }},
		{name: "retention upper bound", mutate: func(r *models.LogrotateResource) { value := 10001; r.Retention = &value }},
		{name: "create mode", mutate: func(r *models.LogrotateResource) {
			r.Create = &models.LogrotateCreate{Mode: "6666", Owner: "root", Group: "adm"}
		}},
		{name: "create principal", mutate: func(r *models.LogrotateResource) {
			r.Create = &models.LogrotateCreate{Mode: "0640", Owner: "root user", Group: "adm"}
		}},
		{name: "relative executable", mutate: func(r *models.LogrotateResource) {
			r.PostRotate = &models.LogrotateScript{Command: []string{"systemctl", "reload"}}
		}},
		{name: "empty script argument", mutate: func(r *models.LogrotateResource) {
			r.PostRotate = &models.LogrotateScript{Command: []string{"/usr/bin/logger", ""}}
		}},
		{name: "script newline", mutate: func(r *models.LogrotateResource) {
			r.PostRotate = &models.LogrotateScript{Command: []string{"/usr/bin/logger", "bad\ncommand"}}
		}},
		{name: "absent with settings", mutate: func(r *models.LogrotateResource) { r.Lifecycle = models.LifecycleAbsent }},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource := base()
			test.mutate(&resource)
			if err := resource.Validate(); err == nil {
				t.Fatalf("Validate(%#v) succeeded", resource)
			}
		})
	}
}

func TestLogrotateValidationAcceptsBoundaryValues(t *testing.T) {
	zero := 0
	maximum := 10000
	for _, resource := range []models.LogrotateResource{
		{Name: "zero", Paths: []string{"/var/log/app/*.log"}, Cadence: models.LogrotateHourly, Retention: &zero, Create: &models.LogrotateCreate{Mode: "640", Owner: "root", Group: "adm"}},
		{Name: strings.Repeat("a", 127), Paths: []string{"/var/log/app/*.log"}, Cadence: models.LogrotateYearly, Retention: &maximum, Create: &models.LogrotateCreate{Mode: "0640", Owner: "root", Group: "adm"}},
		{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "removed"},
	} {
		if err := resource.Validate(); err != nil {
			t.Fatalf("Validate(%#v) = %v", resource, err)
		}
	}
}

func FuzzParseCanonicalLogrotatePolicy(f *testing.F) {
	f.Add("/var/log/remotr/*.log", "daily", 14, "0640", "root", "adm", "/usr/bin/logger", "rotated")
	f.Add("/var/log/app.log", "hourly", 0, "640", "root", "root", "/usr/bin/true", "ok")
	f.Add("relative", "sometimes", -1, "6666", "bad user", "bad group", "logger", "bad\nargument")
	f.Fuzz(func(t *testing.T, path, cadence string, retention int, mode, owner, group, executable, argument string) {
		for _, value := range []string{path, cadence, mode, owner, group, executable, argument} {
			if len(value) > 256 || !utf8.ValidString(value) {
				return
			}
		}
		document := fmt.Sprintf(`schemaVersion: 1
configurations:
  - name: fuzz
    resources:
      - kind: logrotate
        name: fuzz
        paths: [%s]
        cadence: %s
        retention: %d
        create: {mode: %s, owner: %s, group: %s}
        postRotate: {command: [%s, %s]}
`, strconv.Quote(path), strconv.Quote(cadence), retention, strconv.Quote(mode), strconv.Quote(owner), strconv.Quote(group), strconv.Quote(executable), strconv.Quote(argument))
		state, err := models.ParseState(strings.NewReader(document))
		if err != nil {
			if len(err.Error()) > 1024 {
				t.Fatalf("logrotate parser diagnostic is unbounded: %d bytes", len(err.Error()))
			}
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].Logrotate) != 1 {
			t.Fatalf("accepted canonical logrotate shape = %#v", state.Configurations)
		}
		resource := state.Configurations[0].Logrotate[0]
		if len(resource.Paths) != 1 || resource.Paths[0] != path || string(resource.Cadence) != cadence ||
			resource.Retention == nil || *resource.Retention != retention || resource.Create == nil ||
			resource.Create.Mode != mode || resource.Create.Owner != owner || resource.Create.Group != group ||
			resource.PostRotate == nil || len(resource.PostRotate.Command) != 2 ||
			resource.PostRotate.Command[0] != executable || resource.PostRotate.Command[1] != argument {
			t.Fatalf("accepted logrotate fields changed = %#v", resource)
		}
		if err := resource.Validate(); err != nil {
			t.Fatalf("parser accepted invalid logrotate policy: %v", err)
		}
	})
}
