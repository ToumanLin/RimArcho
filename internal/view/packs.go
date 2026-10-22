package view

import (
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// The packs screen (08 §5, 08 §5.3): like choosing a version in a game launcher — the packs,
// when each was last played, and a button to play it; building one from the current list.

// PackRow is one pack on the screen.
type PackRow struct {
	Name   string `json:"name"`
	Facts  string `json:"facts"` // "42 mods · 1.2 GB · 1.6 · built 10 月 20 日"
	Played string `json:"played"`
	Mods   int    `json:"mods"`
	Light  bool   `json:"light,omitempty"`
	// Versions are the game versions it runs on: "1.6", or "1.5–1.6" (PackVersions).
	Versions string `json:"versions,omitempty"`
	// PlayedAt is when it was last played (zero: never); the home page sorts by it.
	PlayedAt time.Time `json:"-"`
}

// PackRows lists the current environment's packs; others counts the other environments'.
func PackRows(a *app.App) (out []PackRow, others int, err error) {
	e, err := a.Env()
	if err != nil {
		return nil, 0, err
	}
	ps, others, err := a.PacksOf(e)
	if err != nil {
		return nil, 0, err
	}
	return packRows(a, ps), others, nil
}

func packRows(a *app.App, ps []*app.PackInfo) (out []PackRow) {
	for _, p := range ps {
		size := SizeText(p.Bytes)
		if p.Light {
			size = T("list only")
		}
		r := PackRow{Name: p.Name, Light: p.Light, Mods: len(p.Entries), Versions: PackVersions(a, p.Pack), Facts: Tf("%d mods · %s · %s · built %s", len(p.Entries), size, p.Game, ShortDate(p.Created)), Played: T("never played")}
		if !p.LastPlayed.IsZero() {
			r.Played = Tf("played %s", ShortDate(p.LastPlayed))
			r.PlayedAt = p.LastPlayed
		}
		out = append(out, r)
	}
	return out
}

// PackVersions are the game versions every carried mod of a pack supports (the
// intersection of their supportedVersions), as "1.6" or "1.5–1.6"; the version it was
// built for when that is unknown or empty.
func PackVersions(a *app.App, p *app.Pack) string {
	var common map[string]bool
	for _, e := range p.Entries {
		if e.Official || e.Instance == "" {
			continue
		}
		m, err := a.Store.Mod(e.Instance)
		if err != nil || m == nil || m.SupportedVersions == "" {
			continue
		}
		vs := map[string]bool{}
		for _, v := range strings.Fields(m.SupportedVersions) {
			if v = env.MajorMinor(v); v != "" {
				vs[v] = true
			}
		}
		if common == nil {
			common = vs
			continue
		}
		for v := range common {
			if !vs[v] {
				delete(common, v)
			}
		}
	}
	var list []string
	for v := range common {
		list = append(list, v)
	}
	if len(list) == 0 {
		return env.MajorMinor(p.Game)
	}
	sort.Slice(list, func(i, j int) bool { return env.VersionLess(list[i], list[j]) })
	if len(list) == 1 {
		return list[0]
	}
	return list[0] + "–" + list[len(list)-1]
}

// ---- building: the progress ----

// Levels of a step or a note: what the player must look at.
const (
	LevelOK   = ""
	LevelWarn = "warn" // amber: the build goes ahead
	LevelBad  = "bad"  // red: the build is refused
	LevelOff  = "off"  // a step that waits for an earlier one
)

// PackTextureOptions reads the window's texture choice: 0 off, 100 预转换 (full size), a
// smaller percent 精简.
func PackTextureOptions(tex int) *textures.Options {
	if tex <= 0 {
		return nil
	}
	o := textures.Options{Scale: tex}.Normal()
	return &o
}

// PlanTextures makes a plan convert textures as tex says (and finds the encoder for the
// preview). Call it with the app held, before Measure.
func PlanTextures(a *app.App, pl *app.PackPlan, tex int) {
	if o := PackTextureOptions(tex); o != nil && pl.R.Pack != nil {
		a.WithTextures(pl, o)
		pl.R.Tool = a.TextureTool()
	}
}

// driveOf names the drive of a folder ("D:"), or the folder.
func driveOf(dir string) string {
	if len(dir) >= 2 && dir[1] == ':' {
		return strings.ToUpper(dir[:2])
	}
	return dir
}

// PackProgress is a build as it goes: the step of the diagram it is at, and the copy.
type PackProgress struct {
	Name    string  `json:"name"`
	Step    int     `json:"step"` // index into the diagram's steps
	Text    string  `json:"text"`
	Frac    float64 `json:"frac"` // the copy, 0..1
	Running bool    `json:"running"`
	Done    bool    `json:"done,omitempty"`
	Ok      bool    `json:"ok,omitempty"`
	Error   string  `json:"error,omitempty"`
}

// ProgressOf turns a build's state into the screen's.
func ProgressOf(p *app.BuildProgress) PackProgress {
	out := PackProgress{Name: p.Name, Running: p.Running()}
	switch p.Step {
	case app.StepCheck:
		out.Step, out.Text = 0, T("checking the list…")
	case app.StepMeasure:
		out.Step, out.Text = 2, Tf("measuring %d mods…", p.ModsTotal)
	case app.StepCopy:
		out.Step = 2
		out.Text = Tf("%d of %d mods · %s of %s", p.Mods, p.ModsTotal, SizeText(p.Bytes), SizeText(p.BytesTotal))
		if p.Mod != "" {
			out.Text += " · " + p.Mod
		}
		if p.BytesTotal > 0 {
			out.Frac = float64(p.Bytes) / float64(p.BytesTotal)
		}
	case app.StepTextures:
		out.Step = 3
		out.Text = Tf("converting %d of %d textures", p.Files, p.FilesTotal)
		if p.Mod != "" {
			out.Text += " · " + p.Mod
		}
		if p.FilesTotal > 0 {
			out.Frac = float64(p.Files) / float64(p.FilesTotal)
		}
	case app.StepWrite:
		out.Step, out.Text, out.Frac = 4, T("writing pack.json…"), 1
	case app.StepDone:
		out.Step, out.Text, out.Frac, out.Done, out.Ok = 5, T("ready to play"), 1, true, true
	}
	return out
}

// UnfinishedRow is a build folder on the packs screen: running, or cut short.
type UnfinishedRow struct {
	Name     string        `json:"name"`
	Running  bool          `json:"running"`
	Facts    string        `json:"facts"`
	Copied   string        `json:"copied,omitempty"` // what it copied so far, "3.2 GB"
	Progress *PackProgress `json:"progress,omitempty"`
}

// UnfinishedRows lists the builds running or cut short (in any process).
func UnfinishedRows(a *app.App) ([]UnfinishedRow, error) {
	us, err := a.UnfinishedPacks()
	if err != nil {
		return nil, err
	}
	var out []UnfinishedRow
	for _, u := range us {
		r := UnfinishedRow{Name: u.Name, Running: u.Running}
		switch {
		case u.Running:
			p := ProgressOf(u.Progress)
			r.Progress, r.Facts = &p, T("building")+" · "+p.Text
		case u.Progress != nil:
			r.Copied = SizeText(u.Progress.Bytes)
			r.Facts = Tf("an unfinished build, stopped at %d of %d mods (%s copied) · %s", u.Progress.Mods, u.Progress.ModsTotal, SizeText(u.Progress.Bytes), ShortDate(u.Progress.Beat))
		default:
			r.Facts = T("an unfinished build")
		}
		out = append(out, r)
	}
	return out, nil
}
