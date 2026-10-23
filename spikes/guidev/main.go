// Spike: a development harness for the GUI page (08). It serves the embedded page on
// 127.0.0.1 with a stand-in for /wails/runtime.js that calls the same gui.API over HTTP,
// so the page can be driven in a browser during development. Never part of a release: the
// product opens no port (01 §4).
//
//	go run ./spikes/guidev -home <copy of the data dir> -addr 127.0.0.1:8765
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/view"
)

const shim = `
const prefix = "github.com/ToumanLin/RimArcho/internal/surface/gui.API.";
export const Call = {
  ByName: async (name, ...args) => {
    const r = await fetch("/call", { method: "POST", body: JSON.stringify({ m: name.slice(prefix.length), a: args }) });
    const j = await r.json();
    if (j.error) throw new Error(j.error);
    return j.result;
  },
};
const subs = {};
const es = new EventSource("/events");
es.onmessage = (m) => { const e = JSON.parse(m.data); (subs[e.name] || []).forEach((f) => f({ name: e.name, data: e.data })); };
export const Events = { On: (n, f) => { (subs[n] = subs[n] || []).push(f); } };
`

// escCheck is G5's check of esc on every page, run in the page (esccheck.js says how).
//
//go:embed esccheck.js
var escCheck string

//go:embed pagecheck.html pagecheck.js pagecheck-runtime.js
var pageCheck embed.FS

type hub struct {
	mu   sync.Mutex
	subs []chan []byte
}

func (h *hub) emit(name string, data any) {
	b, _ := json.Marshal(map[string]any{"name": name, "data": data})
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.subs {
		select {
		case c <- b:
		default:
		}
	}
}

func main() {
	home := flag.String("home", "", "data directory (a copy)")
	addr := flag.String("addr", "127.0.0.1:8765", "listen address (loopback only)")
	lang := flag.String("lang", "", "zh or en (default: the setting)")
	exe := flag.String("exe", "bin/archo.exe", "the archo executable for scan and analyze children")
	assets := flag.String("assets", "", "serve the page's files from this folder (edits show on reload)")
	fake := flag.String("fakegame", "", "while this file exists the current environment's game counts as running (started at its time)")
	flag.Parse()
	if !strings.HasPrefix(*addr, "127.0.0.1:") {
		log.Fatal("loopback only")
	}
	a, err := app.Open(*home)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	l := *lang
	if l == "" {
		l = cmds.Setting(a, "ui.lang")
	}
	view.SetLang(l)
	a.LazyLog, a.LogCacheOnly = true, true
	a.Launcher = func(string, []string, string) error { return fmt.Errorf("launching is off in the dev harness") }
	if *fake != "" {
		// the sidebar's running game (06 §2.1, G17) without starting RimWorld
		a.GameProbe = func(string) *app.GameRun {
			fi, err := os.Stat(*fake)
			if err != nil {
				return nil
			}
			return &app.GameRun{PID: 4242, Started: fi.ModTime()}
		}
	}
	h := &hub{}
	api := gui.NewAPI(a, h.emit)
	defer api.Close()
	api.Exe, _ = filepath.Abs(*exe)
	page, err := gui.PageHandler(api)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/dev/pagecheck", http.RedirectHandler("/dev/pagecheck.html", http.StatusTemporaryRedirect))
	for _, name := range []string{"pagecheck.html", "pagecheck.js", "pagecheck-runtime.js"} {
		mux.HandleFunc("/dev/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if *assets != "" {
				http.ServeFile(w, r, filepath.Join("spikes", "guidev", name))
				return
			}
			http.StripPrefix("/dev/", http.FileServer(http.FS(pageCheck))).ServeHTTP(w, r)
		})
	}
	mux.HandleFunc("/wails/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, shim)
	})
	mux.HandleFunc("/dev/esccheck.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-store")
		if *assets != "" {
			// with -assets the page's files come from disk; so does this one
			if b, err := os.ReadFile(filepath.Join("spikes", "guidev", "esccheck.js")); err == nil {
				w.Write(b)
				return
			}
		}
		fmt.Fprint(w, escCheck)
	})
	mux.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			M string            `json:"m"`
			A []json.RawMessage `json:"a"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		res, err := call(api, req.M, req.A)
		out := map[string]any{"result": res}
		if err != nil {
			out = map[string]any{"error": err.Error()}
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		c := make(chan []byte, 16)
		h.mu.Lock()
		h.subs = append(h.subs, c)
		h.mu.Unlock()
		fl := w.(http.Flusher)
		fl.Flush()
		for {
			select {
			case b := <-c:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	if *assets != "" {
		disk := http.FileServer(http.Dir(*assets))
		embedded := page
		page = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := r.URL.Path; p == "/" || p == "/index.html" || p == "/style.css" || strings.HasPrefix(p, "/js/") {
				w.Header().Set("Cache-Control", "no-store")
				disk.ServeHTTP(w, r)
				return
			}
			embedded.ServeHTTP(w, r)
		})
	}
	mux.Handle("/", page)
	log.Printf("http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// call invokes an exported API method with JSON arguments.
func call(api *gui.API, name string, args []json.RawMessage) (any, error) {
	m := reflect.ValueOf(api).MethodByName(name)
	if !m.IsValid() {
		return nil, fmt.Errorf("no method %s", name)
	}
	t := m.Type()
	if len(args) != t.NumIn() {
		return nil, fmt.Errorf("%s takes %d arguments, got %d", name, t.NumIn(), len(args))
	}
	in := make([]reflect.Value, t.NumIn())
	for i := range in {
		p := reflect.New(t.In(i))
		if err := json.Unmarshal(args[i], p.Interface()); err != nil {
			return nil, fmt.Errorf("argument %d: %v", i, err)
		}
		in[i] = p.Elem()
	}
	out := m.Call(in)
	var res any
	for _, o := range out {
		if o.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if !o.IsNil() {
				return nil, o.Interface().(error)
			}
			continue
		}
		res = o.Interface()
	}
	return res, nil
}
