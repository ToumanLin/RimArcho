package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// The slow-check report (16 §2.14 "Slow checks"): after a can-it-run check that took longer
// than 30 s, the window or the terminal offers to send its timings, which the relay appends
// as a comment to one fixed issue (feedback-relay/README.md). Timings is all that goes. It
// names the slowest Mods by packageId and Workshop id, with their patch files relative to
// the Mod's folder (the sender stays anonymous), but no local path, no player and no Steam
// account: Body removes anything that still looks like a path on this computer.
type Timings struct {
	Version   string `json:"version"`        // RimArcho's
	OS        string `json:"os"`             // System()
	Game      string `json:"game,omitempty"` // the game version ("1.6")
	Active    int    `json:"active_mods"`
	Total     int    `json:"total_mods"`
	Automatic bool   `json:"automatic"` // an automatic check: low priority, two threads (16 §8.9)
	Priority  string `json:"priority"`  // low (automatic) or normal (the player asked)
	WallMs    int64  `json:"wall_ms"`   // from the start of the check to the prompt
	// Phases tile the wall time in order; other is what no phase covers
	Phases  Phases         `json:"phases"`
	Run     *TimingRun     `json:"run,omitempty"`
	Patches *TimingPatches `json:"patches,omitempty"`
	Harmony *TimingHarmony `json:"harmony,omitempty"`
	CPU     string         `json:"cpu,omitempty"` // the processor's model string
	Threads int            `json:"cpu_threads,omitempty"`
	RAMGB   int            `json:"ram_gb,omitempty"`
	Disk    string         `json:"disk,omitempty"` // ssd, hdd: the disk of the game's Mods folder
}

// TimingPhase is one phase of a check and its milliseconds.
type TimingPhase struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

// Phases are sent as one object, {"read":33,"defs":1000}, in their order.
type Phases []TimingPhase

// MarshalJSON writes the object in order.
func (p Phases) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, ph := range p {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(ph.Name)
		fmt.Fprintf(&b, "%s:%d", k, ph.Ms)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// UnmarshalJSON reads the object, or the array of {name, ms} that 1.1.5 sent.
func (p *Phases) UnmarshalJSON(b []byte) error {
	var arr []TimingPhase
	if json.Unmarshal(b, &arr) == nil {
		*p = arr
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return errors.New("phases: not an object")
	}
	*p = nil
	for d.More() {
		k, err := d.Token()
		if err != nil {
			return err
		}
		var ms int64
		if err := d.Decode(&ms); err != nil {
			return err
		}
		*p = append(*p, TimingPhase{Name: fmt.Sprint(k), Ms: ms})
	}
	return nil
}

// Ms is how long a phase named name took (0 when there is none).
func (p Phases) Ms(name string) int64 {
	for _, ph := range p {
		if ph.Name == name {
			return ph.Ms
		}
	}
	return 0
}

// TimingRun says whether the check was cold and how much it read.
type TimingRun struct {
	Cold bool `json:"cold"` // first, or the list was new or changed
	// First: the first check since RimArcho started; List: new, changed or same (a repeat)
	First      bool   `json:"first"`
	List       string `json:"list"`
	ModsRead   int    `json:"mods_read"`
	ModsCached int    `json:"mods_cached"`
	Workers    int    `json:"workers"`
	GCMs       int64  `json:"gc_ms"` // the garbage collector's CPU time
	PeakHeapMB int    `json:"peak_heap_mb"`
}

// TimingPatches is the patch layer: its size, its parts and its slowest operations.
type TimingPatches struct {
	Ops     int        `json:"ops"`
	Nodes   int        `json:"nodes"`    // the merged XML document
	TooSlow int        `json:"too_slow"` // operations marked patch.too-slow
	ParseMs int64      `json:"parse_ms"`
	SimMs   int64      `json:"sim_ms"`
	FactsMs int64      `json:"facts_ms"`
	Slowest []TimingOp `json:"slowest,omitempty"`
}

// TimingOp is one slow patch operation.
type TimingOp struct {
	Ms      float64 `json:"ms"`
	Pkg     string  `json:"pkg"`
	WS      string  `json:"ws,omitempty"`
	Class   string  `json:"class"`
	XPath   string  `json:"xpath,omitempty"` // at most XPathMax characters
	File    string  `json:"file"`            // relative to the Mod's folder
	TooSlow bool    `json:"too_slow,omitempty"`
}

// XPathMax is the longest XPath a report carries, in characters.
const XPathMax = 300

// TimingHarmony is the Harmony layer: DLLs, patches, and the slowest DLLs.
type TimingHarmony struct {
	DLLs    int         `json:"dlls"`
	Cached  int         `json:"cached"`  // DLLs whose facts came from the cache
	Patches int         `json:"patches"` // static Harmony patches
	Methods int         `json:"methods"` // distinct methods they patch
	Slowest []TimingDLL `json:"slowest,omitempty"`
}

// TimingDLL is one slow DLL.
type TimingDLL struct {
	Ms     float64 `json:"ms"`
	Pkg    string  `json:"pkg"`
	WS     string  `json:"ws,omitempty"`
	File   string  `json:"file"`
	Cached bool    `json:"cached,omitempty"`
}

// Body is the request body: the preview shows these bytes, and Send posts them unchanged.
// One field to a line, each phase list and each slow operation on one line, so the
// preview stays short. Anything that looks like a path on this computer is removed first
// (Scrub).
func (t Timings) Body() string {
	t.Scrub(homeDir())
	b, _ := json.Marshal(t)
	return compactJSON(b)
}

func homeDir() string { h, _ := os.UserHomeDir(); return h }

// Scrub removes local paths: a file that is not relative to its Mod's folder keeps only
// its name, and any other text that names a drive, a user folder or home becomes "".
func (t *Timings) Scrub(home string) {
	clean := func(s string) string {
		if localPath(s, home) {
			return ""
		}
		return s
	}
	file := func(s string) string {
		s = strings.ReplaceAll(s, `\`, "/")
		if localPath(s, home) || strings.HasPrefix(s, "/") || strings.Contains(s, "../") {
			s = s[strings.LastIndex(s, "/")+1:]
		}
		return clean(s)
	}
	t.Version, t.OS, t.Game, t.CPU, t.Disk = clean(t.Version), clean(t.OS), clean(t.Game), clean(t.CPU), clean(t.Disk)
	if t.Patches != nil {
		for i := range t.Patches.Slowest {
			o := &t.Patches.Slowest[i]
			o.Pkg, o.WS, o.Class, o.XPath, o.File = clean(o.Pkg), clean(o.WS), clean(o.Class), clean(clip(o.XPath, XPathMax)), file(o.File)
		}
	}
	if t.Harmony != nil {
		for i := range t.Harmony.Slowest {
			d := &t.Harmony.Slowest[i]
			d.Pkg, d.WS, d.File = clean(d.Pkg), clean(d.WS), file(d.File)
		}
	}
}

var localPathRe = regexp.MustCompile(`(?i)(^|[^a-z])[a-z]:[\\/]|\\\\|(^|[\\/])(users|home|documents and settings)[\\/]`)

func localPath(s, home string) bool {
	if localPathRe.MatchString(s) {
		return true
	}
	if home = strings.Trim(strings.ReplaceAll(home, `\`, "/"), "/"); len(home) > 3 && strings.Contains(strings.ToLower(strings.ReplaceAll(s, `\`, "/")), strings.ToLower(home)) {
		return true
	}
	return false
}

// clip shortens s to n characters, ending with "…".
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// compactJSON lays out a compact JSON object: the top level and its objects one field to
// a line (except phases), arrays one element to a line, everything deeper on its line.
func compactJSON(b []byte) string {
	var top map[string]json.RawMessage
	if json.Unmarshal(b, &top) != nil {
		return string(b)
	}
	var out strings.Builder
	writeObject(&out, b, "", 0)
	return out.String()
}

// writeObject writes the object raw at depth (0 the top), keeping its field order.
func writeObject(out *strings.Builder, raw []byte, indent string, depth int) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.Token() // {
	out.WriteString("{")
	first := true
	for d.More() {
		k, _ := d.Token()
		var v json.RawMessage
		d.Decode(&v)
		if !first {
			out.WriteString(",")
		}
		first = false
		key, _ := json.Marshal(k)
		out.WriteString("\n" + indent + "  " + string(key) + ": ")
		switch {
		case depth == 0 && len(v) > 0 && v[0] == '{' && k != "phases":
			writeObject(out, v, indent+"  ", depth+1)
		case depth == 1 && len(v) > 0 && v[0] == '[':
			var items []json.RawMessage
			json.Unmarshal(v, &items)
			out.WriteString("[")
			for i, it := range items {
				if i > 0 {
					out.WriteString(",")
				}
				out.WriteString("\n" + indent + "    " + string(it))
			}
			out.WriteString("\n" + indent + "  ]")
		default:
			out.Write(v)
		}
	}
	out.WriteString("\n" + indent + "}")
}

// TimingsURL is the relay's timings route beside its report route, "" when there is no relay.
func TimingsURL() string {
	r := RelayURL()
	if r == "" {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(r, "/"), "/report") + "/timings"
}

// ErrNoTimingsRoute: the relay answered 404, so it has no timings route yet (or no issue
// for it). The caller says nothing to the player.
var ErrNoTimingsRoute = errors.New("feedback relay: no timings route")

// SendTimings posts body (Timings.Body) to the relay's timings route at endpoint.
func SendTimings(ctx context.Context, endpoint, agent, body string) error {
	if err := relayAddress(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 16<<10))
	if res.StatusCode == http.StatusNotFound {
		return ErrNoTimingsRoute
	}
	if res.StatusCode != http.StatusOK {
		var out struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &out)
		if out.Error == "" {
			out.Error = http.StatusText(res.StatusCode)
		}
		return &RelayError{Status: res.StatusCode, Code: out.Error}
	}
	return nil
}

// String is a short form for the process log.
func (t Timings) String() string {
	return fmt.Sprintf("%d ms, %d/%d Mods, %d phases", t.WallMs, t.Active, t.Total, len(t.Phases))
}
