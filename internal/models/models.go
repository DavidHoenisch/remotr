package models

import (
	"github.com/DavidHoenisch/remotr/internal/types"
	"time"
)

type Package struct {
	Name    string
	Present bool
	Arch    types.Architecture
	PM      types.PackageManager
}

type File struct {
	Name           string
	Path           string
	UpdateExisting bool
	WithRegx       string
	Content        string
	Mode           []int
}

type Configuration struct {
	Name          string
	Description   string
	LastUpdated   time.Time
	TargetDistros []types.Distro
	Packages      []Package
	Files         []File
}

type State struct {
	Configurations []Configuration
}
