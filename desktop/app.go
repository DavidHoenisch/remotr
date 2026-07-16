package main

type App struct {
	version string
}

type ApplicationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewApp(version string) *App {
	return &App{version: version}
}

func (a *App) GetApplicationInfo() ApplicationInfo {
	return ApplicationInfo{
		Name:    "Remotr Desktop",
		Version: a.version,
	}
}
