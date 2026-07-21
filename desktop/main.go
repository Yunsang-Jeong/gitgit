package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application := NewDesktopApp()
	if err := wails.Run(&options.App{
		Title:                    "GitGit",
		Width:                    1568,
		Height:                   1004,
		MinWidth:                 1120,
		MinHeight:                720,
		BackgroundColour:         &options.RGBA{R: 10, G: 17, B: 21, A: 255},
		StartHidden:              browserDevelopmentMode(os.Getenv("GITGIT_BROWSER_DEV")),
		DisableResize:            false,
		EnableDefaultContextMenu: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.startup,
		OnShutdown: application.shutdown,
		Bind: []interface{}{
			application,
		},
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About: &mac.AboutInfo{
				Title:   "GitGit",
				Message: "Flexible Git history and worktree workflows",
			},
		},
	}); err != nil {
		log.Fatal(err)
	}
}

func browserDevelopmentMode(value string) bool {
	return value == "1"
}
