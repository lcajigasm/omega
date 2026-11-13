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
	"net/http"
	"os"
	"runtime"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

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
		Assets: application.AssetOptions{Handler: application.BundledAssetFileServer(assets),
			Middleware: postOnly},
		Mac:        application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		ShouldQuit: svc.closing,
		OnShutdown: svc.shutdown,
	})
	svc.emit = func(m gui.Message) { app.Event.Emit("omega", m) }
	svc.quit = app.Quit
	app.Menu.SetApplicationMenu(menu())

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "omega",
		Width:            1040,
		Height:           760,
		MinWidth:         640,
		MinHeight:        480,
		URL:              "/",
		BackgroundColour: application.NewRGB(18, 18, 20),
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !svc.closing() {
			e.Cancel()
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// postOnly refuses a call to the app's functions that is not a POST. The
// window's own code posts; an address written in the text the window shows
// (a link, an image) can only be fetched with a GET, and must not be able
// to act as the user.
func postOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wails/runtime" && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// menu is the default menu without "Reload": the window is the session, and
// drawing it again in the middle of one only gets in the way.
func menu() *application.Menu {
	m := application.NewMenu()
	if runtime.GOOS == "darwin" {
		m.AddRole(application.AppMenu)
	}
	m.AddRole(application.FileMenu)
	m.AddRole(application.EditMenu)
	view := m.AddSubmenu("View")
	view.AddRole(application.ResetZoom)
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	m.AddRole(application.WindowMenu)
	return m
}
