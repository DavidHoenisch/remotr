package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/endpointlabel"
	"github.com/DavidHoenisch/remotr/internal/hubcatalog"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/charmbracelet/huh"
)

const endpointPickerHint = "↑/↓ navigate · / filter · enter confirm"

const endpointMultiPickerHint = "↑/↓ navigate · space select · / filter · enter confirm"

const appPackagePickerHint = endpointPickerHint

const secretPickerHint = endpointPickerHint

func secretPickerLabel(secret admin.LogicalSecretSummary) string {
	active := secret.ActiveVersion
	if active == "" {
		active = "inactive"
	} else {
		active = "active " + active
	}
	return fmt.Sprintf("%s  (%s · %s)", secret.Name, secret.Scope, active)
}

func secretPickerOptions(items []admin.LogicalSecretSummary) []huh.Option[string] {
	sorted := append([]admin.LogicalSecretSummary(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	opts := make([]huh.Option[string], 0, len(sorted))
	for _, item := range sorted {
		opts = append(opts, huh.NewOption(secretPickerLabel(item), item.Name))
	}
	return opts
}

func promptSecretSelect(selected *string, items []admin.LogicalSecretSummary) error {
	if !isInteractive() || strings.TrimSpace(*selected) != "" {
		return nil
	}
	opts := secretPickerOptions(items)
	if len(opts) == 0 {
		return fmt.Errorf("no secrets available")
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Secret").
			Description(secretPickerHint).
			Options(opts...).
			Value(selected).
			Validate(func(value string) error {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func endpointPickerLabel(ep admin.Endpoint) string {
	parts := make([]string, 0, 2)
	if ep.Fleet != "" {
		parts = append(parts, ep.Fleet)
	}
	if usernames := formatUsernames(ep.Usernames); usernames != "" {
		parts = append(parts, usernames)
	}
	if len(parts) == 0 {
		return ep.ID
	}
	return fmt.Sprintf("%s  (%s)", ep.ID, strings.Join(parts, " · "))
}

func endpointPickerOptions(endpoints []admin.Endpoint) []huh.Option[string] {
	sorted := append([]admin.Endpoint(nil), endpoints...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	opts := make([]huh.Option[string], 0, len(sorted))
	for _, ep := range sorted {
		opts = append(opts, huh.NewOption(endpointPickerLabel(ep), ep.ID))
	}
	return opts
}

func appPackagePickerValue(name, version string) string {
	return name + "\x00" + version
}

func parseAppPackagePickerValue(v string) (name, version string) {
	name, version, _ = strings.Cut(v, "\x00")
	return name, version
}

func appPackagePickerLabel(pkg admin.AppPackage) string {
	return fmt.Sprintf("%s@%s", pkg.Name, pkg.Version)
}

func appPackagePickerOptions(items []admin.AppPackage) []huh.Option[string] {
	sorted := append([]admin.AppPackage(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Version < sorted[j].Version
	})
	opts := make([]huh.Option[string], 0, len(sorted))
	for _, item := range sorted {
		opts = append(opts, huh.NewOption(appPackagePickerLabel(item), appPackagePickerValue(item.Name, item.Version)))
	}
	return opts
}

func promptAppPackageSelect(selected *string, items []admin.AppPackage) error {
	if !isInteractive() || strings.TrimSpace(*selected) != "" {
		return nil
	}
	opts := appPackagePickerOptions(items)
	if len(opts) == 0 {
		return fmt.Errorf("no app packages published")
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("App package").
			Description(appPackagePickerHint).
			Options(opts...).
			Value(selected).
			Validate(func(s string) error {
				name, version := parseAppPackagePickerValue(s)
				if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptAppPackageNameVersion(name, version *string) error {
	if !isInteractive() {
		return nil
	}
	if strings.TrimSpace(*name) != "" && strings.TrimSpace(*version) != "" {
		return nil
	}
	var fields []huh.Field
	if strings.TrimSpace(*name) == "" {
		fields = append(fields, huh.NewInput().
			Title("Package name").
			Placeholder("internal/mycli").
			Value(name).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}))
	}
	if strings.TrimSpace(*version) == "" {
		fields = append(fields, huh.NewInput().
			Title("Package version").
			Placeholder("1.0.0").
			Value(version).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}))
	}
	if len(fields) == 0 {
		return nil
	}
	return huh.NewForm(huh.NewGroup(fields...)).Run()
}

const hubSnippetPickerHint = endpointPickerHint

func hubSnippetPickerLabel(entry hubcatalog.Entry) string {
	category := strings.TrimSpace(entry.Category)
	if category == "" {
		return fmt.Sprintf("%s  (%s)", entry.Title, entry.ID)
	}
	return fmt.Sprintf("%s  (%s · %s)", entry.Title, category, entry.ID)
}

func hubSnippetPickerOptions(entries []hubcatalog.Entry) []huh.Option[string] {
	importable := make([]hubcatalog.Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.SnippetPath) == "" {
			continue
		}
		importable = append(importable, entry)
	}
	sorted := append([]hubcatalog.Entry(nil), importable...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Featured != sorted[j].Featured {
			return sorted[i].Featured
		}
		if sorted[i].Title != sorted[j].Title {
			return sorted[i].Title < sorted[j].Title
		}
		return sorted[i].ID < sorted[j].ID
	})
	opts := make([]huh.Option[string], 0, len(sorted))
	for _, entry := range sorted {
		opts = append(opts, huh.NewOption(hubSnippetPickerLabel(entry), entry.ID))
	}
	return opts
}

func promptHubSnippetSelect(entryID *string, entries []hubcatalog.Entry) error {
	if !isInteractive() || strings.TrimSpace(*entryID) != "" {
		return nil
	}
	opts := hubSnippetPickerOptions(entries)
	if len(opts) == 0 {
		return fmt.Errorf("hub catalog has no importable snippets")
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Hub snippet").
			Description(hubSnippetPickerHint).
			Options(opts...).
			Value(entryID).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptEndpointSelect(endpointID *string, endpoints []admin.Endpoint) error {
	if !isInteractive() || strings.TrimSpace(*endpointID) != "" {
		return nil
	}
	opts := endpointPickerOptions(endpoints)
	if len(opts) == 0 {
		return fmt.Errorf("no endpoints enrolled")
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Endpoint").
			Description(endpointPickerHint).
			Options(opts...).
			Value(endpointID).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptEndpointMultiSelect(endpointIDs *[]string, endpoints []admin.Endpoint) error {
	if !isInteractive() {
		return nil
	}
	opts := endpointPickerOptions(endpoints)
	if len(opts) == 0 {
		return fmt.Errorf("no endpoints enrolled")
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Endpoints").
			Description(endpointMultiPickerHint).
			Options(opts...).
			Filterable(true).
			Value(endpointIDs).
			Validate(func(values []string) error {
				if len(values) == 0 {
					return fmt.Errorf("select at least one endpoint")
				}
				return nil
			}),
	)).Run()
}

func endpointLabelKeyOptions(labels map[string]string) []huh.Option[string] {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	opts := make([]huh.Option[string], 0, len(keys))
	for _, k := range keys {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s=%s", k, labels[k]), k))
	}
	return opts
}

func promptEndpointLabelPairFields(key, value *string, keySet, valueSet bool) error {
	if !isInteractive() {
		return nil
	}
	if keySet && valueSet {
		return nil
	}
	var fields []huh.Field
	if !keySet {
		fields = append(fields, huh.NewInput().
			Title("Label key").
			Placeholder("site").
			Value(key).
			Validate(func(s string) error {
				return endpointlabel.ValidateKey(s)
			}))
	}
	if !valueSet {
		fields = append(fields, huh.NewInput().
			Title("Label value").
			Placeholder("berlin").
			Value(value).
			Validate(func(s string) error {
				return endpointlabel.ValidateValue(s)
			}))
	}
	if len(fields) == 0 {
		return nil
	}
	return huh.NewForm(huh.NewGroup(fields...)).Run()
}

func promptEndpointLabelKey(key *string, existingLabels map[string]string) error {
	if !isInteractive() || strings.TrimSpace(*key) != "" {
		return nil
	}
	opts := endpointLabelKeyOptions(existingLabels)
	if len(opts) > 0 {
		return huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Label key").
				Description(endpointPickerHint).
				Options(opts...).
				Value(key).
				Validate(func(s string) error {
					return endpointlabel.ValidateKey(s)
				}),
		)).Run()
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Label key").
			Placeholder("site").
			Value(key).
			Validate(func(s string) error {
				return endpointlabel.ValidateKey(s)
			}),
	)).Run()
}

func promptBootstrapInputs(settings *opconfig.Settings, token *string) error {
	if !isInteractive() {
		return nil
	}
	var fields []huh.Field
	if strings.TrimSpace(settings.ServerURL) == "" {
		fields = append(fields, huh.NewInput().
			Title("Server URL").
			Placeholder("https://remotr.example:8443").
			Value(&settings.ServerURL).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}))
	}
	if strings.TrimSpace(settings.CA) == "" {
		fields = append(fields, huh.NewInput().
			Title("CA certificate path").
			Placeholder("/etc/remotr/ca.crt").
			Value(&settings.CA).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				if _, err := os.Stat(strings.TrimSpace(s)); err != nil {
					return fmt.Errorf("file not found")
				}
				return nil
			}))
	}
	if strings.TrimSpace(*token) == "" {
		fields = append(fields, huh.NewInput().
			Title("Bootstrap token").
			EchoMode(huh.EchoModePassword).
			Value(token).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}))
	}
	if len(fields) == 0 {
		return nil
	}
	return huh.NewForm(huh.NewGroup(fields...)).Run()
}

func promptServerURL(settings *opconfig.Settings) error {
	if !isInteractive() || strings.TrimSpace(settings.ServerURL) != "" {
		return nil
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Server URL").
			Placeholder("https://remotr.example:8443").
			Value(&settings.ServerURL).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptFleet(fleet *string) error {
	if !isInteractive() || strings.TrimSpace(*fleet) != "" {
		return nil
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Fleet name").
			Placeholder("engineering").
			Value(fleet).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptEndpointID(endpointID *string) error {
	if !isInteractive() || strings.TrimSpace(*endpointID) != "" {
		return nil
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Endpoint ID").
			Placeholder("laptop-engineering-01").
			Value(endpointID).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptLabel(label *string, title string) error {
	if !isInteractive() || strings.TrimSpace(*label) != "" {
		return nil
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(title).
			Value(label).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			}),
	)).Run()
}

func promptConfirm(message string) (bool, error) {
	if !isInteractive() {
		return false, nil
	}
	confirm := false
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(message).
			Affirmative("Yes").
			Negative("No").
			Value(&confirm),
	)).Run()
	if err != nil {
		return false, err
	}
	return confirm, nil
}

func promptConfirmResource(resourceID string) (bool, error) {
	return promptConfirm(fmt.Sprintf("Remove %q permanently?", resourceID))
}
