package view

import (
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// The game log page (the 日志 tab): one log's error clusters, each with the mods it is tied
// to; the interfaces show a cluster's evidence with `archo log --cluster`.

// LogFile is a log the page can show.
type LogFile struct {
	Label string `json:"label"` // "Player.log" / "Player-prev.log"
	Path  string `json:"path"`
	When  string `json:"when"`
}

// LogRow is one cluster.
type LogRow struct {
	ID      string `json:"id"` // E3
	Class   string `json:"class"`
	Count   int    `json:"count"`
	Text    string `json:"text"`
	Mods    string `json:"mods"` // "Harmony (patcher), +2"
	Status  int    `json:"status"`
	Info    bool   `json:"info,omitempty"`
	Cluster string `json:"cluster"` // the argument of log --cluster
}

// LogPage is one analysed log.
type LogPage struct {
	Files    []LogFile `json:"files"`
	Path     string    `json:"path"`
	Facts    string    `json:"facts"` // "1.6.4633 · 12 840 lines · 10 月 19 日 21:04"
	Rows     []LogRow  `json:"rows"`
	Selected bool      `json:"selected"` // its errors are in 需要处理
	// Pending is the game version whose analysis is still running: no rows yet, ask again.
	Pending string `json:"pending,omitempty"`
}

// LogFiles lists the environment's current and previous Player.log.
func LogFiles(a *app.App, st *app.LoadoutState) []LogFile {
	var out []LogFile
	cur, prev := a.LogPaths(st)
	for i, p := range []string{cur, prev} {
		if p == "" {
			continue
		}
		f := LogFile{Label: []string{"Player.log", "Player-prev.log"}[i], Path: p}
		if fi, err := os.Stat(p); err == nil {
			f.When = ShortDate(fi.ModTime()) + " " + fi.ModTime().In(a.Loc).Format("15:04")
		}
		out = append(out, f)
	}
	return out
}

// BuildLogPage analyses path (the newest log when empty); the analysis is cached by the
// app, so a second look at the same file is quick.
func BuildLogPage(a *app.App, st *app.LoadoutState, path string) (*LogPage, error) {
	p := &LogPage{Files: LogFiles(a, st)}
	if path == "" && len(p.Files) > 0 {
		path = p.Files[0].Path
	}
	if path == "" {
		return p, nil
	}
	if err := a.EnsureLogAnalysis(st, app.LogVersionOf(path), nil); errors.Is(err, app.ErrStillComputing) {
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
	var facts []string
	if r.Version != "" {
		facts = append(facts, r.Version)
	}
	facts = append(facts, Tf("%d lines", r.Lines))
	if !r.ModTime.IsZero() {
		facts = append(facts, ShortDate(r.ModTime)+" "+r.ModTime.In(a.Loc).Format("15:04"))
	}
	if r.AnalysedAs != "" {
		facts = append(facts, Tf("read as %s", r.AnalysedAs))
	}
	p.Facts = strings.Join(facts, " · ")
	for _, lc := range r.Clusters {
		row := LogRow{ID: lc.ID, Class: T(lc.Class), Count: lc.Count, Text: lc.First.Message, Cluster: lc.ID, Info: logs.Informational(lc.Class)}
		if i := strings.IndexByte(row.Text, '\n'); i >= 0 {
			row.Text = row.Text[:i]
		}
		if len(row.Text) > 240 {
			row.Text = row.Text[:237] + "…"
		}
		switch {
		case row.Info:
			row.Status = StatusNone
		case lc.Class == logs.ClassTexture || lc.Class == logs.ClassMaterial:
			row.Status = StatusWarn
		default:
			row.Status = StatusErr
		}
		if len(lc.Attr) > 0 {
			at := lc.Attr[0]
			name := at.Instance
			if m := st.Ix.ByID[at.Instance]; m != nil {
				name = m.DisplayName()
			}
			row.Mods = name + "（" + T(at.Role) + "）"
			if len(lc.Attr) > 1 {
				row.Mods += Tf(" +%d", len(lc.Attr)-1)
			}
		}
		p.Rows = append(p.Rows, row)
	}
	// errors first, then warnings, then what is only informational; each by count
	order := func(r LogRow) int {
		if r.Info {
			return 2
		}
		if r.Status == StatusWarn {
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
	return p, nil
}
