package main

import (
	"embed"
	"flag"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"keepsake/internal/log"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	debug := flag.Bool("debug", false, "enable verbose debug logging")
	demo := flag.Bool("demo", false, "demo mode: pixelate all media and blur names for safe screen recording")
	flag.Parse()
	log.SetDebug(*debug)

	// Create an instance of the app structure
	app := NewApp()
	app.demo = *demo

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Keepsake",
		Width:  1100,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
