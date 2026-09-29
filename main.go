//go:build gtk3

package main

import (
	"embed"
	"log"

	"github.com/juniorgs/hyprmod/internal/api"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service := api.NewService()
	app := application.New(application.Options{
		Name:        "HyprMod",
		Description: "Visual configuration editor for Hyprland",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "HyprMod",
		Width:            1320,
		Height:           880,
		MinWidth:         980,
		MinHeight:        640,
		BackgroundColour: application.NewRGB(20, 22, 29),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
