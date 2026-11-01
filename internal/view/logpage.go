package view

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// The game log page (the Game log tab): one log's error clusters, each with the mods it is tied
// to; the interfaces show a cluster's evidence with `archo log --cluster`.

// LogFile is a log the page can show. A companion log (ArchoMonitor.log, RimCrow's) has Of
// set: the Player.log it belongs to, listed right before it; it shows as that log's
// companion section.
type LogFile struct {
	Label string `json:"label"` // "Player.log" / "Player-prev.log" / "ArchoMonitor.log"
	Path  string `json:"path"`
	When  string `json:"when"`
	Of    string `json:"of,omitempty"`
}

// LogRow is one cluster.
type LogRow struct {
	PhaseStart string `json:"phaseStart,omitempty"` // quiet boundary above this row
	ID         string `json:"id"`                   // E3
	Class      string `json:"class"`
	Count      int    `json:"count"`
	Text       string `json:"text"` // the first line, rich-text tags removed
	// Spans is Text with the mods' rich text; Level the line's colour (logtext.go).
	Spans   []TextSpan `json:"spans"`
	Level   string     `json:"level"`
	Mods    string     `json:"mods"` // "Harmony (patcher), +2"
	Status  int        `json:"status"`
	Info    bool       `json:"info,omitempty"`
	Cluster string     `json:"cluster"` // the argument of log --cluster
	// Line is the first line, normalised: it finds this cluster from a reading whose ids
	// differ (the running game's items, whose clusters are numbered as they arrive).
	Line string `json:"line,omitempty"`
}

// LogPage is one analysed log.
type LogPage struct {
	Files    []LogFile `json:"files"`
	Path     string    `json:"path"`
	Facts    string    `json:"facts"` // "1.6.4633  12 840 lines"
	Rows     []LogRow  `json:"rows"`
	Selected bool      `json:"selected"` // its errors are in Can it run
	// Live: the running game still writes it; its errors join Can it run when the game exits.
	Live bool `json:"live,omitempty"`
	// Pending is the game version whose analysis is still running: no rows yet, ask again.
	Pending string `json:"pending,omitempty"`
	// Monitor is the companion log of this run (Archo Monitor's, else RimCrow's).
	Monitor *MonitorLog `json:"monitor,omitempty"`
	// NoMonitor: the current plan has no Archo Monitor, in one line; AddMonitor: it can be
	// added (an installed monitor and Harmony in the plan).
	NoMonitor  string `json:"noMonitor,omitempty"`
	AddMonitor bool   `json:"addMonitor,omitempty"`
	// Links maps a normalised first line to its cluster, for LiveMonitor.
	Links map[string]string `json:"-"`
}

// LogFiles lists the environment's current and previous Player.log, each followed by its
// companion log when it has one (03 §6.7.4).
func LogFiles(a *app.App, st *app.LoadoutState) []LogFile {
	var out []LogFile
	cur, prev := a.LogPaths(st)
	when := func(p string) string {
		if fi, err := os.Stat(p); err == nil {
			return ShortDate(fi.ModTime()) + " " + fi.ModTime().In(a.Loc).Format("15:04")
		}
		return ""
	}
	for i, p := range []string{cur, prev} {
		if p == "" {
			continue
		}
		out = append(out, LogFile{Label: []string{"Player.log", "Player-prev.log"}[i], Path: p, When: when(p)})
		if c := companionOf(p); c != "" {
			out = append(out, LogFile{Label: filepath.Base(c), Path: c, When: when(c), Of: p})
		}
	}
	return out
}

// Companion files can appear or rotate while Player.log stays unchanged.
func companionOf(path string) string { return app.CompanionFor(path) }

// BuildLogPage analyses path (the newest log when empty; a companion log means its
// Player.log); the analysis is cached by the app, so a second look at the same file is
// quick. live, when it follows path, supplies the companion section as read so far.
func BuildLogPage(a *app.App, st *app.LoadoutState, path string, live *LiveTracker) (*LogPage, error) {
	p := &LogPage{Files: LogFiles(a, st)}
	matched := false
	for _, f := range p.Files {
		if strings.EqualFold(f.Path, path) {
			matched = true
			if f.Of != "" {
				path = f.Of
			}
			break
		}
	}
	// A previous environment's selection must not override this environment's logs.
	if !matched {
		path = ""
	}
	if path == "" && len(p.Files) > 0 {
		path = p.Files[0].Path
	}
	if path == "" {
		return p, nil
	}
	if ms, err := a.MonitorState(); err == nil && ms.InPlan == nil {
		p.NoMonitor = T("this list has no Archo Monitor: errors will not say which file or def was loading")
		p.AddMonitor = ms.Newest != nil
		if !p.AddMonitor {
			p.NoMonitor = T("Archo Monitor is not installed: errors will not say which file or def was loading (archo monitor install)")
		}
	}
	if err := a.EnsureLogFileAnalysis(st, path, nil); errors.Is(err, app.ErrStillComputing) {
		p.Path, p.Pending = path, app.LogMM(app.LogVersionOf(path))
		return p, nil
	} else if err != nil {
		return nil, err
	}
	r, err := a.AnalyzeLog(path)
	if err != nil {
		return nil, err
	}
	p.Path, p.Selected = path, a.ProblemLog(st) == path
	p.Live = !p.Selected && path == a.LiveLog(st)
	var facts []string
	if r.Version != "" {
		facts = append(facts, r.Version)
	}
	// the file picker beside it already says when the log was written
	facts = append(facts, Tf("%d lines", r.Lines))
	if r.AnalysedAs != "" {
		facts = append(facts, Tf("read as %s", r.AnalysedAs))
	}
	p.Facts = strings.Join(facts, "  ")
	p.Links = logLinks(r.Clusters)
	if m := LiveMonitor(a, st, live, path, p.Links); m != nil {
		p.Monitor = m
	} else if c := companionOf(path); c != "" {
		p.Monitor = MonitorOf(a, st, readCompanion(c), p.Links, false)
	}
	if p.Monitor != nil {
		// this run had the monitor: its tab is the answer, not a line saying it is missing
		p.NoMonitor, p.AddMonitor = "", false
	}
	var prev *logs.Phase
	first := true
	for _, lc := range r.Clusters {
		row := LogRow{ID: lc.ID, Class: T(lc.Class), Count: lc.Count, Spans: LogSpans(lc.First.Message, 240), Cluster: lc.ID, Line: lineKey(lc.First.Message), Info: logs.Informational(lc.Class)}
		if r.HasPhases && (first || !samePhase(prev, lc.Phase)) {
			row.PhaseStart = phaseLabel(lc.Phase, a.Loc)
		}
		prev, first = lc.Phase, false
		row.Text = SpanText(row.Spans)
		switch {
		case row.Info:
			row.Status = StatusNone
		case lc.Class == logs.ClassTexture || lc.Class == logs.ClassMaterial:
			row.Status = StatusWarn
		default:
			row.Status = StatusErr
		}
		lvl := lc.Level
		if lvl == "" {
			lvl = lc.First.Level
		}
		row.Level = LevelOf(lvl, row.Status)
		if len(lc.Attr) > 0 {
			at := lc.Attr[0]
			name := at.Instance
			if m := st.Ix.ByID[at.Instance]; m != nil {
				name = m.DisplayName()
			}
			row.Mods = Tf("%s (%s)", name, T(at.Role))
			if len(lc.Attr) > 1 {
				row.Mods += Tf(" +%d", len(lc.Attr)-1)
			}
		}
		p.Rows = append(p.Rows, row)
	}
	if !r.HasPhases {
		// Keep the existing ordering for logs from older monitors.
		order := func(row LogRow) int {
			if row.Info {
				return 2
			}
			if row.Status == StatusWarn {
				return 1
			}
			return 0
		}
		sort.SliceStable(p.Rows, func(i, j int) bool {
			x, y := p.Rows[i], p.Rows[j]
			if order(x) != order(y) {
				return order(x) < order(y)
			}
			return x.Count > y.Count
		})
	}
	return p, nil
}
