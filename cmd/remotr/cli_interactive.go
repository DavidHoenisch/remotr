package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/charmbracelet/huh"
)

const endpointPickerHint = "↑/↓ navigate · / filter · enter confirm"

const endpointMultiPickerHint = "↑/↓ navigate · space select · / filter · enter confirm"

func endpointPickerOptions(endpoints []admin.Endpoint) []huh.Option[string] {
	sorted := append([]admin.Endpoint(nil), endpoints...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	opts := make([]huh.Option[string], 0, len(sorted))
	for _, ep := range sorted {
		label := ep.ID
		if ep.Fleet != "" {
			label = fmt.Sprintf("%s  (%s)", ep.ID, ep.Fleet)
		}
		opts = append(opts, huh.NewOption(label, ep.ID))
	}
	return opts
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
