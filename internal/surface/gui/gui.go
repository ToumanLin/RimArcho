// Package gui is the desktop interface (08): a Wails v3 window with plain HTML, CSS and
// JavaScript (assets/, embedded), over the same internal/view models as the TUI.
package gui

import (
	"embed"
	"io/fs"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/icon"
	"github.com/ToumanLin/RimArcho/internal/view"
)

//go:embed assets
var assets embed.FS

// Options for Run.
type Options struct {
	App *app.App
}

// Run opens the window and returns when it is closed.
func Run(o Options) error {
	a := o.App
	view.SetLang(cmds.Setting(a, "ui.lang"))
	a.LazyLog = true // the newest log is parsed when 需要处理 opens (as in the TUI)
	a.LogCacheOnly = true
	tuneProcess(a)
	api := newAPI(a)
	defer api.Close()
	api.free.on = true
	page, err := PageHandler(api)
	if err != nil {
		return err
	}
	ico := windowIcon()
	debug.FreeOSMemory() // the drawing's buffers
	w := application.New(application.Options{
		Name:     "RimArcho",
		Icon:     ico, // used when the exe has no icon resource (development builds, 06 §8.6)
		Services: []application.Service{application.NewService(api)},
		Assets:   application.AssetOptions{Handler: page},
		Windows: application.WindowsOptions{
			// WebView2's profile lives in RimArcho's data directory, not %APPDATA%\RimArcho.exe.
			WebviewUserDataPath:   filepath.Join(a.Home, "webview2"),
			AdditionalBrowserArgs: BrowserArgs(cmds.Setting(a, "ui.gpu") != "off"),
		},
	})
	api.emit = func(name string, data any) { w.Event.Emit(name, data) }
	api.x.native = wailsNative{w}
	api.quit = w.Quit
	win := w.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "RimArcho", Width: 1440, Height: 880, MinWidth: 720, MinHeight: 480, URL: "/",
		BackgroundColour:           application.NewRGB(0x0f, 0x10, 0x11),
		DefaultContextMenuDisabled: true, // the page draws its own (06 §8.5)
	})
	lowMemoryWhenMinimised(win)
	return w.Run()
}

// BrowserArgs are WebView2's command-line switches (07 §8.3). Always: the network service
// runs inside the browser process (the page loads nothing from the network, 06 §8.2) and
// there is one renderer, which saves a process of about 7 MB. Without GPU acceleration
// (ui.gpu off, 06 §2.14) the GPU process goes too and the page is drawn in software.
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS has no effect under Wails, so they go here.
func BrowserArgs(gpu bool) []string {
	args := []string{"--enable-features=NetworkServiceInProcess2", "--renderer-process-limit=1"}
	if !gpu {
		args = append(args, "--disable-gpu", "--disable-gpu-compositing", "--in-process-gpu")
	}
	return args
}

// NewAPI makes the service for an app; emit receives its events (scan, analyze). The
// window uses it, and so does the development harness (spikes/guidev).
func NewAPI(a *app.App, emit func(name string, data any)) *API {
	api := newAPI(a)
	api.emit = emit
	return api
}

// PageHandler serves the page, the thumbnails (/preview/<id>) and the description images
// (/img?u=<url>) from the local caches; nothing remote is loaded by the page (06 §8.2).
func PageHandler(api *API) (http.Handler, error) {
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	return handler(api, files), nil
}

func handler(api *API, files fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/preview/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/preview/")
		api.mu.Lock()
		st := api.boardState()
		var path string
		var err error = app.ErrNoPreview
		if st != nil && st.Ix.ByID[id] != nil {
			path, _, _, err = api.a.Preview(st.Ix, st.Ix.ByID[id])
		}
		api.mu.Unlock()
		serveFile(w, r, path, err)
	})
	mux.HandleFunc("/img", func(w http.ResponseWriter, r *http.Request) {
		if !api.a.WorkshopImages() {
			http.NotFound(w, r)
			return
		}
		path, _, _, err := api.a.DescImage(r.URL.Query().Get("u")) // downloads into the cache once
		serveFile(w, r, path, err)
	})
	mux.HandleFunc("/bg", api.serveBg)
	for path, size := range map[string]int{"/icon.svg": 256, "/icon-small.svg": 16} {
		svg := []byte(icon.SVG(size))
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write(svg)
		})
	}
	files2 := application.BundledAssetFileServer(files)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		files2.ServeHTTP(w, r)
	}))
	return mux
}

func serveFile(w http.ResponseWriter, r *http.Request, path string, err error) {
	if err != nil || path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFile(w, r, path)
}

// openURL opens a link in the system browser.
// A variable so tests can never start the real program (noopen_test.go).
var openURL = func(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}
