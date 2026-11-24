// releaseserver is a test release service for the update rehearsal (17 §5): it answers the
// GitHub Releases calls RimArcho's self-update makes (/releases/latest, /releases/tags/v<v>)
// with one release made of the zip and .sha256 in -dir, and serves those two files. The
// candidate runs with RIMARCHO_UPDATE_API=http://<addr>, so its check, download, checksum,
// extract, replace and restart are a player's; only the API root differs. Test only: it runs
// in the VM guest and is never shipped. Every request is written to -log.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8471", "listen address")
	dir := flag.String("dir", "", "folder with archo-<version>-windows-amd64.zip and its .sha256")
	logFile := flag.String("log", "", "request log (default: stderr)")
	flag.Parse()
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		log.SetOutput(f)
	}
	h, err := newHandler(*dir, "http://"+*addr)
	if err != nil {
		log.Println(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log.Printf("serving %s %s on %s", h.version, h.zip, *addr)
	if err := http.ListenAndServe(*addr, h); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

var zipName = regexp.MustCompile(`^archo-(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)-windows-amd64\.zip$`)

type handler struct {
	dir, zip, version, root string
}

func newHandler(dir, root string) (*handler, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	h := &handler{dir: dir, root: root}
	for _, e := range entries {
		if m := zipName.FindStringSubmatch(e.Name()); m != nil {
			if h.zip != "" {
				return nil, fmt.Errorf("%s holds more than one release zip", dir)
			}
			h.zip, h.version = e.Name(), m[1]
		}
	}
	if h.zip == "" {
		return nil, fmt.Errorf("no archo-<version>-windows-amd64.zip in %s", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, h.zip+".sha256")); err != nil {
		return nil, errors.New("the zip has no .sha256 beside it")
	}
	return h, nil
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

func (h *handler) release() (map[string]any, error) {
	var assets []asset
	for _, name := range []string{h.zip, h.zip + ".sha256"} {
		st, err := os.Stat(filepath.Join(h.dir, name))
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset{Name: name, URL: h.root + "/download/" + name, Size: st.Size()})
	}
	return map[string]any{
		"tag_name": "v" + h.version, "name": "RimArcho " + h.version + " (update rehearsal)",
		"body":         "Update rehearsal build; never published.",
		"html_url":     h.root + "/releases/tag/v" + h.version,
		"published_at": time.Now().UTC().Format(time.RFC3339),
		"prerelease":   strings.Contains(h.version, "-"), "draft": false, "assets": assets,
	}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)
	p := r.URL.Path
	switch {
	case p == "/releases/latest" || p == "/releases/tags/v"+h.version:
		rel, err := h.release()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	case p == "/releases":
		rel, err := h.release()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{rel})
	case p == "/download/"+h.zip || p == "/download/"+h.zip+".sha256":
		http.ServeFile(w, r, filepath.Join(h.dir, strings.TrimPrefix(p, "/download/")))
	default:
		http.NotFound(w, r)
	}
}
