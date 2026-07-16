package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	err := wails.Run(&options.App{
		Title: "Remotr Desktop",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
	})
	if err != nil {
		fmt.Println("Error:", err)
	}
}
