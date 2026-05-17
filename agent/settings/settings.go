package settings

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Default path to remotr settings file
const DefaultSettingsFilePath string = "/etc/remotr/settings.yaml"

func SettingsFromDefault() (*Settings, error) {
	var settings Settings

	dat, err := os.ReadFile(DefaultSettingsFilePath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(dat, settings)
	if err != nil {
		return nil, err
	}

	return &settings, nil
}
