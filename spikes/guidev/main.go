// Spike: a development harness for the GUI page (08). It serves the embedded page on
// 127.0.0.1 with a stand-in for /wails/runtime.js that calls the same gui.API over HTTP,
// so the page can be driven in a browser during development. Never part of a release: the
// product opens no port (01 §5.3).
//
//	go run ./spikes/guidev -home <copy of the data dir> -addr 127.0.0.1:8765
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
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
	h := &hub{}
	api := gui.NewAPI(a, h.emit)
	api.Exe, _ = filepath.Abs(*exe)
	page, err := gui.PageHandler(api)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/wails/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, shim)
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
