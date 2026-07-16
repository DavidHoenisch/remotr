package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

var version = "dev"

func main() {
	app := NewApp(version)
	applicationOptions := newApplicationOptions(app)
	applicationOptions.OnStartup = app.startup
	applicationOptions.OnShutdown = app.shutdown
	err := wails.Run(applicationOptions)
	if err != nil {
		fmt.Println("Error:", err)
	}
}
