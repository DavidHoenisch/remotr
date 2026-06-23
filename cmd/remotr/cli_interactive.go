package main

import (
	"fmt"
	"os"
	"strings"

	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	"github.com/charmbracelet/huh"
)

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
