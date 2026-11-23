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
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"

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

// postOnly refuses a call to the app's functions that does not come from
// the window's own code. That code posts, from the app's own page. An
// address written in the text the window shows (a link, an image) can only
// be fetched with a GET; and a page from somewhere else, should the window
// ever be taken to one (a link opened from the system's own menu, a file
// dropped on it), says where it is from when it posts. Neither may act as
// the user: approve a command, change the provider, switch approvals off.
var urlGuard = &gui.URLGuard{}

func postOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wails/runtime" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if !ownPage(r) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// Opening a link is for web pages only: the system would open
			// file://, smb:// or another app's scheme just the same.
			if !urlGuard.Allow(r) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ownPage reports a request the app's own page can have made. A page from
// the web says so in two ways, and either is enough to refuse it: the
// webview marks the request as coming from another site, or its Origin is
// a web address that is not the app's (the app's page is wails://… on
// macOS and Linux, http://wails.localhost on Windows, localhost in the
// capture build). A request with no Origin, or with the origin of a custom
// scheme, is the app's own.
func ownPage(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	u, err := url.Parse(r.Header.Get("Origin"))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return true
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "wails.localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.EqualFold(u.Host, r.Host)
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
