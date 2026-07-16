package main

import (
	"net/http"
	"net/url"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	linuxoptions "github.com/wailsapp/wails/v2/pkg/options/linux"
)

func newApplicationOptions(bindings ...interface{}) *options.App {
	return &options.App{
		Title:     "Remotr Desktop",
		Width:     1440,
		Height:    900,
		MinWidth:  1100,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: releaseAssetPolicy,
		},
		EnableDefaultContextMenu: false,
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
		BindingsAllowedOrigins: "",
		Bind:                   bindings,
		DragAndDrop: &options.DragAndDrop{
			DisableWebViewDrop: true,
		},
		Linux: &linuxoptions.Options{
			ProgramName:      "remotr-desktop",
			WebviewGpuPolicy: linuxoptions.WebviewGpuPolicyNever,
		},
	}
}

func releaseAssetPolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isEmbeddedAssetURL(r.URL) {
			http.Error(w, "remote application content is forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; font-src 'self'; form-action 'none'; frame-ancestors 'none'; frame-src 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self'")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func isEmbeddedAssetURL(target *url.URL) bool {
	if target == nil {
		return false
	}
	if target.Scheme == "" && target.Host == "" {
		return true
	}
	return target.Scheme == "wails" && target.Host == "wails"
}
