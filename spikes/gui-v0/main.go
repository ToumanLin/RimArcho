// Spike: GUI V0 (08 §V0). A Wails v3 window with plain HTML/CSS/JS (no npm, no generated
// bindings) over the real internal/app: three columns of 1354 mods, a detail with the
// thumbnail, then measure memory (archo + WebView2 processes) and binary size.
//
//	go build -o spikes/gui-v0/gui-v0.exe ./spikes/gui-v0 && spikes\gui-v0\gui-v0.exe -home <copy of the data dir>
package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ToumanLin/RimArcho/internal/app"
)

//go:embed assets
var assets embed.FS

// Svc is the bound service. The app is not safe for concurrent use, so every call locks.
type Svc struct {
	mu sync.Mutex
	ap *app.App
}

// Row is one mod in a column.
type Row struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Pos    int    `json:"pos"` // 1-based position in the enabled list, 0 when not enabled
	Status string `json:"status"`
}

// Rows returns every live mod with its position in the workspace.
func (s *Svc) Rows() ([]Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.ap.Loadout()
	if err != nil {
		return nil, err
	}
	pos := map[string]int{}
	for i, e := range st.List {
		if e.Instance != "" {
			pos[e.Instance] = i + 1
		}
	}
	rows := make([]Row, 0, len(st.Ix.Mods))
	for _, m := range st.Ix.Mods {
		if m.Status != "live" && pos[m.ID] == 0 {
			continue
		}
		name := m.Name
		if m.Alias != "" {
			name = m.Alias
		}
		rows = append(rows, Row{ID: m.ID, Name: name, Kind: string(m.Kind), Pos: pos[m.ID], Status: m.Status})
	}
	return rows, nil
}

// Detail is the minimal detail of one mod.
type Detail struct {
	Name, Package, Authors, Path string
	Tags                         []string
	Preview                      bool
}

// Detail returns the facts the V0 detail shows.
func (s *Svc) Detail(id string) (*Detail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ix, err := s.ap.Index()
	if err != nil {
		return nil, err
	}
	m := ix.ByID[id]
	if m == nil {
		return nil, nil
	}
	_, _, _, perr := s.ap.Preview(ix, m)
	return &Detail{Name: m.Name, Package: m.PackageID, Authors: m.Authors, Path: m.RealPath, Tags: m.Tags, Preview: perr == nil}, nil
}

// preview serves /preview/<id> from the app's thumbnail cache.
func (s *Svc) preview(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/preview/")
	s.mu.Lock()
	ix, err := s.ap.Index()
	var path string
	if err == nil && ix.ByID[id] != nil {
		path, _, _, err = s.ap.Preview(ix, ix.ByID[id])
	}
	s.mu.Unlock()
	if err != nil || path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeFile(w, r, path)
}

func main() {
	home := flag.String("home", "", "data directory (a copy)")
	flag.Parse()
	ap, err := app.Open(*home)
	if err != nil {
		log.Fatal(err)
	}
	defer ap.Close()
	svc := &Svc{ap: ap}
	files := application.BundledAssetFileServer(mustSub(assets, "assets"))
	mux := http.NewServeMux()
	mux.HandleFunc("/preview/", svc.preview)
	mux.Handle("/", files)

	a := application.New(application.Options{
		Name:     "RimArcho",
		Services: []application.Service{application.NewService(svc)},
		Assets:   application.AssetOptions{Handler: mux},
	})
	a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "RimArcho", Width: 1400, Height: 860, URL: "/",
		BackgroundColour: application.NewRGB(0x0f, 0x10, 0x11),
	})
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
