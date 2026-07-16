package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

func main() {
	err := wails.Run(newApplicationOptions(NewApp(version)))
	if err != nil {
		fmt.Println("Error:", err)
	}
}
