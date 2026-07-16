package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	err := wails.Run(newApplicationOptions())
	if err != nil {
		fmt.Println("Error:", err)
	}
}
