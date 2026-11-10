//go:build gui

// Command omega-gui is omega's desktop app: the same agent, tasks, safety
// policy and configuration as the omega command, in a window.
//
// It is built apart from the command-line tool (the gui build tag) because
// the window needs the system's webview, and so cgo on macOS and Linux.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/lcajigasm/omega/internal/cli"
	"github.com/lcajigasm/omega/internal/gui"
	"github.com/lcajigasm/omega/internal/sudo"
)

// Set by the release build via -ldflags.
var version = "dev"

//go:embed frontend
var frontend embed.FS

// The window and taskbar icon on Windows and Linux. macOS takes it from the
// app bundle.
//
//go:embed icon.png
var icon []byte

func main() {
	// sudo runs this binary as its askpass helper; see internal/sudo.
	if sudo.IsHelper(os.Args) {
		os.Exit(sudo.RunHelper(os.Args))
	}
	cli.Init(version)

	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	svc := &App{demo: slices.Contains(os.Args[1:], "--demo") || os.Getenv("OMEGA_DEMO") == "1"}
	app := application.New(application.Options{
		Name:        "omega",
		Description: "AI-driven maintenance",
		Icon:        icon,
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		OnShutdown:  svc.shutdown,
	})
	svc.emit = func(m gui.Message) { app.Event.Emit("omega", m) }

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "omega",
		Width:            1040,
		Height:           760,
		MinWidth:         640,
		MinHeight:        480,
		URL:              "/",
		BackgroundColour: application.NewRGB(18, 18, 20),
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
