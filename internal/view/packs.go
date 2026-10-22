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

// ---- building: the steps (preview and progress) ----

// The steps of a build as the screen draws them, in order.
var packSteps = []string{"check", "sort", "copy", "textures", "write", "ready"}

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
func PreviewPack(a *app.App, name string, tex int) (*PackPreview, error) {
	pl, err := a.PlanPack(name, false)
	if err != nil {
		return nil, err
	}
	PlanTextures(a, pl, tex)
	pl.Measure(a.PacksDir())
	return PreviewOf(pl, a.PacksDir()), nil
}

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
	case r.Free < max(r.Need, r.Peak)+app.SpaceMargin:
		cp.Level = LevelBad
		cp.Facts = append(cp.Facts, Tf("only %s free on %s: not enough", SizeText(r.Free), driveOf(packsDir)))
	case r.Free-max(r.Need, r.Peak) < 10<<30:
		cp.Level = LevelWarn
		cp.Facts = append(cp.Facts, Tf("%s free on %s; %s left after", SizeText(r.Free), driveOf(packsDir), SizeText(r.Free-r.Final)))
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
	tex := textureStep(r)
	pv.Steps = append(pv.Steps, check, sorted, cp, tex, write, ready)
	pv.Ok = cp.Level != LevelBad && tex.Level != LevelBad
	return pv
}

// textureStep is the build's 贴图 box: off (the PNGs are copied as they are), 预转换 at full
// size or 精简 when scaled, with what it costs and saves (08 §5.2).
func textureStep(r *app.PackBuild) PackStep {
	o := r.TexOptions
	if o == nil {
		return PackStep{Key: "textures", Title: T("textures"), Level: LevelOff, Facts: []string{T("PNGs stay as they are")}}
	}
	title := T("pre-convert textures")
	if o.Scale < 100 {
		title = T("slim textures") + " · " + Tf("%d%%", o.Scale)
	}
	s := PackStep{Key: "textures", Title: title}
	e := r.TexEst
	if e.Files == 0 {
		s.Facts = []string{T("nothing to convert")}
		return s
	}
	s.Facts = append([]string{Tf("%d PNG become DDS; the PNGs are not copied", e.Files)}, EstimateFacts(e)[1:]...)
	s.Facts = append(s.Facts, Tf("the pack is about %s after (up to %s while converting)", SizeText(r.Final), SizeText(r.Peak)))
	if r.TexSkipped > 0 {
		s.More = append(s.More, Tf("%d keep their PNG (a DDS or another format of the same name)", r.TexSkipped))
	}
	if r.Final > r.Need {
		s.Level = LevelWarn
		s.More = append(s.More, T("the pack grows: DDS take more disk than PNG"))
	}
	if t := r.Tool; t != nil && t.Err != "" {
		s.Level = LevelBad
		s.More = append([]string{T("the DDS encoder (todds) is not installed")}, s.More...)
		if t.Path != "" {
			s.More[0] = t.Err
		}
	}
	return s
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
	case "textures":
		return T("textures")
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
