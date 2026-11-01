package view

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// The game log page's companion section (06 §2.10, 03 §6.7.4): what Archo Monitor (or
// RimCrow Companion) wrote during the run of the selected Player.log, one row per entry.

// MonitorEntry is one entry of the companion log.
type MonitorEntry struct {
	PhaseStart string     `json:"phaseStart,omitempty"`
	Key        string     `json:"key"`
	Time       string     `json:"time"` // 17:51:45
	Status     int        `json:"status"`
	Kind       string     `json:"kind,omitempty"` // inferredType in a few words: patch error
	Text       string     `json:"text"`           // the message's first line, rich-text tags removed
	Spans      []TextSpan `json:"spans"`          // with the mod's rich text (logtext.go)
	Level      string     `json:"level"`
	Count      int        `json:"count"`
	Mods       []string   `json:"mods,omitempty"`      // related mods by name
	FilePaths  []string   `json:"filePaths,omitempty"` // full paths, aligned with shortened Files
	Files      []string   `json:"files,omitempty"`     // related files: inside a mod relative to it
	Details    string     `json:"details,omitempty"`
	Cluster    string     `json:"cluster,omitempty"` // the Player.log cluster it matches (E3)
	Only       bool       `json:"only,omitempty"`    // the game had stopped writing Player.log
}

// MonitorLog is the companion section of the log page.
type MonitorLog struct {
	Title    string         `json:"title"` // "Archo Monitor"
	Label    string         `json:"label"` // ArchoMonitor.log
	Path     string         `json:"path"`
	Facts    string         `json:"facts"` // 17:51–17:55
	Running  bool           `json:"running,omitempty"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
	Entries  []MonitorEntry `json:"entries"`
}

// MonitorKind words an inferredType ("" for none or an unknown one).
func MonitorKind(inferred string) string {
	k := map[string]string{
		"PatchError": "patch error", "DefConfigError": "def config error", "CrossRefError": "cross-reference error",
		"XMLSyntaxError": "XML load error", "AssemblyConflict": "assembly conflict", "TickException": "tick exception",
		"DrawException": "draw exception", "NullReference": "null reference", "OutOfMemory": "out of memory",
		"MissingTexture": "missing texture", "ObserverFailure": "Archo Monitor's own error",
	}[inferred]
	if k == "" {
		return ""
	}
	return T(k)
}

// MonitorOf builds the section from a companion log read so far. links maps a normalised
// first line to its Player.log cluster; running: the game still writes it.
func MonitorOf(a *app.App, st *app.LoadoutState, c *logs.Companion, links map[string]string, running bool) *MonitorLog {
	if c == nil {
		return nil
	}
	m := &MonitorLog{Label: filepath.Base(c.Path), Path: c.Path, Running: running, Entries: []MonitorEntry{}}
	name := "RimCrow Companion"
	if c.Monitor {
		name = "Archo Monitor"
	}
	// The run is the Player.log's: its game version is on the line above, and the
	// monitor's own version tells the player nothing. Only the run's time is new here.
	m.Title = name
	var facts []string
	switch start := c.Started.In(a.Loc).Format("15:04"); {
	case c.Started.IsZero():
	case !c.Ended.IsZero():
		facts = append(facts, start+"–"+c.Ended.In(a.Loc).Format("15:04"))
	case running:
		facts = append(facts, Tf("since %s", start))
	case c.Monitor:
		facts = append(facts, Tf("%s, no exit line (it crashed or was killed)", start))
	}
	if c.Full {
		facts = append(facts, T("full: no new entries after 32 MiB"))
	}
	m.Facts = strings.Join(facts, "  ")
	entries := c.Entries
	if c.HasPhases {
		entries = c.Segments
	}
	var prev *logs.Phase
	first := true
	for _, e := range entries {
		if e.Level == "INFO" && e.Inferred == "" {
			continue
		}
		me := MonitorEntry{Key: e.Key, Kind: MonitorKind(e.Inferred), Spans: LogSpans(e.Message, 240), Count: e.Count, Details: strings.TrimSpace(e.Details)}
		if c.HasPhases && (first || !samePhase(prev, e.Phase)) {
			me.PhaseStart = phaseLabel(e.Phase, a.Loc)
		}
		prev, first = e.Phase, false
		me.Text = SpanText(me.Spans)
		if !e.At.IsZero() {
			me.Time = e.At.In(a.Loc).Format("15:04:05")
		}
		switch e.Level {
		case "ERROR":
			me.Status = StatusErr
			m.Errors++
		case "WARNING":
			me.Status = StatusWarn
			m.Warnings++
		default:
			me.Status = StatusNone
		}
		me.Level = LevelOf(e.Level, me.Status)
		for _, pkg := range e.Mods {
			if st != nil {
				me.Mods = append(me.Mods, TokName(st, "pkg:"+pkg))
			} else {
				me.Mods = append(me.Mods, pkg)
			}
		}
		for _, f := range e.Files {
			me.Files = append(me.Files, shortFile(st, f))
			me.FilePaths = append(me.FilePaths, f)
		}
		if id, ok := links[e.Match]; ok {
			me.Cluster = id
		} else if links != nil && e.Monitor && (e.Level == "ERROR" || e.Level == "WARNING") {
			me.Only = true
		}
		m.Entries = append(m.Entries, me)
	}
	return m
}

// shortFile: a path inside a mod's folder relative to it ("Mod › Defs/a.xml"), else as is.
func shortFile(st *app.LoadoutState, p string) string {
	if st == nil || !filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	k := strings.ToLower(filepath.Clean(p))
	for _, m := range st.Ix.Mods {
		dir := st.Ix.ModDir(m)
		if dir == "" {
			continue
		}
		d := strings.ToLower(filepath.Clean(dir)) + string(filepath.Separator)
		if strings.HasPrefix(k, d) {
			return m.DisplayName() + " › " + filepath.ToSlash(p[len(d):])
		}
	}
	return filepath.ToSlash(p)
}

// lineKey is a message's first line, normalised: how one reading of a log meets a cluster
// of another (the companion log, the running game's clusters, the log page's rows).
func lineKey(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return logs.Normalize(strings.TrimSpace(msg))
}

// logLinks maps each cluster's normalised first line to its id (the first cluster wins).
func logLinks(cs []*app.LogCluster) map[string]string {
	out := map[string]string{}
	for _, lc := range cs {
		if lc.First == nil {
			continue
		}
		k := lineKey(lc.First.Message)
		if _, ok := out[k]; !ok {
			out[k] = lc.ID
		}
	}
	return out
}

// companionCache keeps the companion logs read for the page, by path, size and time.
var companionCache = struct {
	sync.Mutex
	by map[string]*logs.Companion
}{by: map[string]*logs.Companion{}}

// readCompanion reads a finished companion log once per version of the file.
func readCompanion(path string) *logs.Companion {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
	companionCache.Lock()
	defer companionCache.Unlock()
	if c := companionCache.by[key]; c != nil {
		return c
	}
	c, err := logs.ReadCompanion(path)
	if err != nil {
		return nil
	}
	if len(companionCache.by) > 8 {
		companionCache.by = map[string]*logs.Companion{}
	}
	companionCache.by[key] = c
	return c
}

// Companion is the companion log the tracker follows for path, while the game writes it
// (nil otherwise).
func (t *LiveTracker) Companion(path string) *logs.Companion {
	if t == nil || t.lv == nil || !strings.EqualFold(filepath.Clean(t.lv.Path), filepath.Clean(path)) {
		return nil
	}
	return t.lv.CompanionLog()
}

// LiveMonitor is the section of path from the tracker's companion log, for the page's 2 s
// refresh while the game runs (nil when the tracker does not follow path). links: the
// page's (LogPage.Links).
func LiveMonitor(a *app.App, st *app.LoadoutState, t *LiveTracker, path string, links map[string]string) *MonitorLog {
	c := t.Companion(path)
	if c == nil {
		return nil
	}
	paired := app.CompanionForRun(path, a.RunDataRoot(st, t.what))
	if paired == "" {
		return nil
	}
	if !strings.EqualFold(filepath.Clean(c.Path), filepath.Clean(paired)) || c.Session != logs.FirstSession(paired) {
		c = readCompanion(paired)
	}
	return MonitorOf(a, st, c, links, true)
}

// MonitorAdd is the page's "add it" action (the default insertion, 04 §1.5).
func MonitorAdd(a *app.App) (string, error) {
	if _, err := a.AddMonitor(); err != nil {
		return "", err
	}
	return T("added Archo Monitor after Harmony"), nil
}
