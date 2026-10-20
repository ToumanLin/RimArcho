package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// The packs screen (10 §8, 09 §2.2): like choosing a version in a game launcher — the packs,
// when each was last played, and a button to play it; building one from the current list.

// PackRow is one pack on the screen.
type PackRow struct {
	Name   string `json:"name"`
	Facts  string `json:"facts"` // "42 mods · 1.2 GB · 1.6 · built 10 月 20 日"
	Played string `json:"played"`
	Light  bool   `json:"light,omitempty"`
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
	for _, p := range ps {
		size := SizeText(p.Bytes)
		if p.Light {
			size = T("list only")
		}
		r := PackRow{Name: p.Name, Light: p.Light, Facts: Tf("%d mods · %s · %s · built %s", len(p.Entries), size, p.Game, ShortDate(p.Created)), Played: T("never played")}
		if !p.LastPlayed.IsZero() {
			r.Played = Tf("played %s", ShortDate(p.LastPlayed))
		}
		out = append(out, r)
	}
	return out, others, nil
}

// ---- building: the steps (preview and progress) ----

// The steps of a build as the screen draws them, in order.
var packSteps = []string{"check", "sort", "copy", "write", "ready"}

// Levels of a step or a note: what the player must look at.
const (
	LevelOK   = ""
	LevelWarn = "warn" // amber: the build goes ahead
	LevelBad  = "bad"  // red: the build is refused
	LevelOff  = "off"  // a step that waits for an earlier one
)

// PackStep is one box of the build diagram.
type PackStep struct {
	Key   string   `json:"key"`
	Title string   `json:"title"`
	Facts []string `json:"facts"`
	Level string   `json:"level,omitempty"`
	More  []string `json:"more,omitempty"` // the blockers, the mods that cannot be copied, …
}

// PackPreview is what building a pack from the current list would do.
type PackPreview struct {
	Name  string     `json:"name"`
	Steps []PackStep `json:"steps"`
	Ok    bool       `json:"ok"`
	Dir   string     `json:"dir"`
}

// PreviewPack runs a dry-run build and says what it found.
func PreviewPack(a *app.App, name string) (*PackPreview, error) {
	pl, err := a.PlanPack(name, false)
	if err != nil {
		return nil, err
	}
	pl.Measure(a.PacksDir())
	return PreviewOf(pl, a.PacksDir()), nil
}

// PreviewOf turns a measured plan into the diagram. It reads only the plan, so the window
// builds it without holding the app.
func PreviewOf(pl *app.PackPlan, packsDir string) *PackPreview {
	r := pl.R
	pv := &PackPreview{Name: name(r), Dir: r.Dir}
	check := PackStep{Key: "check", Title: T("check the list")}
	if len(r.Blockers) > 0 {
		check.Level = LevelBad
		check.Facts = []string{Tf("%d things need fixing first (需要处理)", len(r.Blockers))}
		for i, b := range r.Blockers {
			if i == 8 {
				check.More = append(check.More, Tf("… %d more", len(r.Blockers)-8))
				break
			}
			if pkg, ok := strings.CutPrefix(b, "not installed: pkg:"); ok {
				b = Tf("the list has %s, but it is not installed", pkg)
			} else {
				b = Humanize(pl.St, b)
			}
			check.More = append(check.More, b)
		}
		pv.Steps = append(pv.Steps, check)
		for _, k := range packSteps[1:] {
			pv.Steps = append(pv.Steps, PackStep{Key: k, Title: stepTitle(k), Level: LevelOff, Facts: []string{T("waits for the list")}})
		}
		return pv
	}
	check.Facts = []string{T("nothing to fix")}
	if len(r.Mismatch) > 0 {
		check.Level = LevelWarn
		check.Facts = append(check.Facts, Tf("%d mods do not list game version %s", len(r.Mismatch), env.MajorMinor(r.Pack.Game)))
		for i, m := range r.Mismatch {
			if i == 6 {
				check.More = append(check.More, Tf("… %d more", len(r.Mismatch)-6))
				break
			}
			check.More = append(check.More, Humanize(pl.St, m))
		}
	}
	if r.Pack.Game == "" {
		check.Level = LevelWarn
		check.Facts = append(check.Facts, T("the game's version is unknown"))
	}
	sorted := PackStep{Key: "sort", Title: T("sort for the pack"), Facts: []string{Tf("%d moved", r.Moved), T("your list does not change")}}

	carried, official := 0, 0
	for _, e := range r.Pack.Entries {
		if e.Official {
			official++
		} else {
			carried++
		}
	}
	cp := PackStep{Key: "copy", Title: Tf("copy %d mods", carried),
		Facts: []string{SizeText(r.Need), Tf("%s of other game versions left out", SizeText(r.Pack.Trimmed))}}
	switch {
	case r.Free < 0:
		cp.Facts = append(cp.Facts, T("free space unknown"))
	case r.Free < r.Need+app.SpaceMargin:
		cp.Level = LevelBad
		cp.Facts = append(cp.Facts, Tf("only %s free on %s: not enough", SizeText(r.Free), driveOf(packsDir)))
	case r.Free-r.Need < 10<<30:
		cp.Level = LevelWarn
		cp.Facts = append(cp.Facts, Tf("%s free on %s; %s left after", SizeText(r.Free), driveOf(packsDir), SizeText(r.Free-r.Need)))
	default:
		cp.Facts = append(cp.Facts, Tf("%s free on %s", SizeText(r.Free), driveOf(packsDir)))
	}
	if len(r.CannotCopy) > 0 {
		cp.Level = LevelBad
		cp.Facts = append(cp.Facts, Tf("%d mods cannot be read", len(r.CannotCopy)))
		cp.More = append(cp.More, r.CannotCopy...)
	}
	write := PackStep{Key: "write", Title: T("write pack.json"), Facts: []string{Tf("%d entries, in the pack's order", len(r.Pack.Entries)), Tf("Core and %d DLCs from the game", official-1), T("its own ModsConfig and saves")}}
	if official == 0 {
		write.Facts[1] = T("Core and the DLCs come from the game")
	}
	ready := PackStep{Key: "ready", Title: T("ready to play"), Facts: []string{Tf("for %s", r.Pack.Game), T("played with its own saves")}}
	pv.Steps = append(pv.Steps, check, sorted, cp, write, ready)
	pv.Ok = cp.Level != LevelBad
	return pv
}

func name(r *app.PackBuild) string {
	if r.Pack != nil {
		return r.Pack.Name
	}
	return ""
}

func stepTitle(k string) string {
	switch k {
	case "check":
		return T("check the list")
	case "sort":
		return T("sort for the pack")
	case "copy":
		return T("copy the mods")
	case "write":
		return T("write pack.json")
	}
	return T("ready to play")
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
	case app.StepWrite:
		out.Step, out.Text, out.Frac = 3, T("writing pack.json…"), 1
	case app.StepDone:
		out.Step, out.Text, out.Frac, out.Done, out.Ok = 4, T("ready to play"), 1, true, true
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
