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
	Label     string `json:"label"` // "Player.log" / "Player-prev.log" / "ArchoMonitor.log"
	Path      string `json:"path"`
	When      string `json:"when"`
	Of        string `json:"of,omitempty"`
	Oversized string `json:"oversized,omitempty"`
	bytes     int64
}

// LogRow is one cluster.
type LogRow struct {
	PhaseStart string `json:"phaseStart,omitempty"` // quiet boundary above this row
	ID         string `json:"id"`                   // E3
	Class      string `json:"class"`                // the kind in plain words (logClassTitle)
	Code       string `json:"code"`                 // the class as the log parser names it (06 §2), on hover
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
	Files []LogFile `json:"files"`
	Path  string    `json:"path"`
	// Notice says how much of an oversized or trimmed log was analysed (06 §1); the GUI
	// draws it as a coverage mark with the sentence as its name and tooltip (16 §2.10).
	Notice   string   `json:"notice,omitempty"`
	Trimmed  bool     `json:"trimmed,omitempty"`
	Facts    string   `json:"facts"` // "1.6.4633  12 840 lines"
	Rows     []LogRow `json:"rows"`
	Selected bool     `json:"selected"` // its errors are in Can it run
	// Live: the running game still writes it; its errors join Can it run when the game exits.
	Live bool `json:"live,omitempty"`
	// Pending is the game version whose analysis is still running: no rows yet, ask again.
	Pending string `json:"pending,omitempty"`
	// Monitor is the companion log of this run (Archo Monitor's, else RimCrow's).
	Monitor *MonitorLog `json:"monitor,omitempty"`
	// NoMonitor: the current plan has no Archo Monitor, in one line; AddMonitor: it can be
	// added (an installed monitor and Harmony in the plan); InstallMonitor: it is not
	// installed, and the GUI's button installs it (16 §2.9).
	NoMonitor      string `json:"noMonitor,omitempty"`
	AddMonitor     bool   `json:"addMonitor,omitempty"`
	InstallMonitor bool   `json:"installMonitor,omitempty"`
	// Links maps a normalised first line to its cluster, for LiveMonitor.
	Links map[string]string `json:"-"`
}

// LogFiles lists the environment's current and previous Player.log, each followed by its
// companion log when it has one (06 §7).
func LogFiles(a *app.App, st *app.LoadoutState) []LogFile {
	var out []LogFile
	for _, row := range playerLogFiles(a, st) {
		out = append(out, row)
		if c := companionOf(row.Path); c != "" {
			comp := LogFile{Label: filepath.Base(c), Path: c, Of: row.Path}
			if fi, err := os.Stat(c); err == nil {
				comp.When = logFileDate(a, fi)
			}
			out = append(out, comp)
		}
	}
	return out
}

func logFileDate(a *app.App, fi os.FileInfo) string {
	return ShortDate(fi.ModTime()) + " " + fi.ModTime().In(a.Loc).Format("15:04")
}

func playerLogFiles(a *app.App, st *app.LoadoutState) []LogFile {
	cur, prev := a.LogPaths(st)
	var out []LogFile
	for i, p := range []string{cur, prev} {
		if p == "" {
			continue
		}
		row := LogFile{Label: []string{"Player.log", "Player-prev.log"}[i], Path: p}
		if fi, err := os.Lstat(p); err == nil {
			row.When = logFileDate(a, fi)
			if fi.Mode().IsRegular() && fi.Size() > app.LogSizeLimit {
				row.Oversized, row.bytes = SizeText(fi.Size()), fi.Size()
			}
		}
		out = append(out, row)
	}
	return out
}

// OversizedSize is the oversized logs' size together, for the Launch page's size badge.
func OversizedSize(files []LogFile) string {
	var n int64
	for _, f := range files {
		n += f.bytes
	}
	if n == 0 {
		return ""
	}
	return SizeText(n)
}

// OversizedLogs returns the Player logs eligible for a cleanup preview.
func OversizedLogs(a *app.App, st *app.LoadoutState) []LogFile {
	var out []LogFile
	for _, f := range playerLogFiles(a, st) {
		if f.Oversized != "" {
			out = append(out, f)
		}
	}
	return out
}

func logCoverage(omitted int64, trimmed bool) string {
	if omitted <= 0 {
		return ""
	}
	if trimmed {
		return Tf("trimmed: %s was removed from the middle; the rest was analysed", SizeText(omitted))
	}
	return Tf("too large to read in full: the beginning and end were analysed, %s in the middle skipped", SizeText(omitted))
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
			p.NoMonitor, p.InstallMonitor = T("Install the in-game helper to see which file caused an error."), true
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
	if r.OmittedBytes > 0 && !r.Trimmed {
		facts = append(facts, Tf("%d lines analysed", r.Lines))
	} else {
		facts = append(facts, Tf("%d lines", r.Lines))
	}
	if r.AnalysedAs != "" {
		facts = append(facts, Tf("read as %s", r.AnalysedAs))
	}
	p.Facts = strings.Join(facts, "  ")
	p.Notice, p.Trimmed = logCoverage(r.OmittedBytes, r.Trimmed), r.Trimmed && r.OmittedBytes > 0
	p.Links = logLinks(r.Clusters)
	if m := LiveMonitor(a, st, live, path, p.Links); m != nil {
		p.Monitor = m
	} else if c := companionOf(path); c != "" {
		p.Monitor = MonitorOf(a, st, readCompanion(c), p.Links, false)
	}
	if p.Monitor != nil {
		// this run had the monitor: its tab is the answer, not a line saying it is missing
		p.NoMonitor, p.AddMonitor, p.InstallMonitor = "", false, false
	}
	var prev *logs.Phase
	first := true
	for _, lc := range r.Clusters {
		row := LogRow{ID: lc.ID, Class: logClassTitle(lc.Class), Code: lc.Class, Count: lc.Count, Spans: LogSpans(lc.First.Message, 240), Cluster: lc.ID, Line: lineKey(lc.First.Message), Info: logs.Informational(lc.Class)}
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

// logClassTitles name a cluster's class in a player's words (16 §2.9); the parser's code
// (harmony-stack, xref-missing…) stays in LogRow.Code and in CLI and MCP output.
var logClassTitles = map[string]string{
	logs.ClassHarmony:     "error in the game's code",
	logs.ClassXrefMissing: "a missing item",
	logs.ClassPatchFailed: "a patch that failed",
	logs.ClassException:   "an exception",
	logs.ClassXMLLoad:     "content that did not load",
	logs.ClassTexture:     "a missing texture or sound",
	logs.ClassMaterial:    "a material error",
	logs.ClassSave:        "a save data error",
	logs.ClassGameError:   "a game error naming Mod code",
	logs.ClassModReport:   "a Mod's own report",
	logs.ClassOther:       "other",
	logs.ClassNoise:       "a known harmless line",
}

func logClassTitle(class string) string {
	if t, ok := logClassTitles[class]; ok {
		return T(t)
	}
	return T(class)
}
