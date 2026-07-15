package models_test

import (
	"strings"
	"testing"

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
		{name: "path injection", mutate: func(r *models.LogrotateResource) { r.Paths = []string{"/var/log/app {\nrotate 0"} }},
		{name: "cadence", mutate: func(r *models.LogrotateResource) { r.Cadence = "sometimes" }},
		{name: "retention", mutate: func(r *models.LogrotateResource) { value := -1; r.Retention = &value }},
		{name: "create mode", mutate: func(r *models.LogrotateResource) {
			r.Create = &models.LogrotateCreate{Mode: "6666", Owner: "root", Group: "adm"}
		}},
		{name: "relative executable", mutate: func(r *models.LogrotateResource) {
			r.PostRotate = &models.LogrotateScript{Command: []string{"systemctl", "reload"}}
		}},
		{name: "script newline", mutate: func(r *models.LogrotateResource) {
			r.PostRotate = &models.LogrotateScript{Command: []string{"/usr/bin/logger", "bad\ncommand"}}
		}},
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
