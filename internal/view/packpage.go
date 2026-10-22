package view

import (
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// ---- 编译成整合包 (06 §2.6) ----

// BuildStep is one of the four things building does, as the page says it.
type BuildStep struct {
	Title string   `json:"title"`
	Text  string   `json:"text"`
	Level string   `json:"level,omitempty"` // LevelWarn, LevelBad, LevelOff
	More  []string `json:"more,omitempty"`  // the problems, the mods that cannot be copied, …
	Needs bool     `json:"needs,omitempty"` // the step's problems are fixed on 能不能跑
}

// BuildPage is the build page: on the left the four steps, on the right the summary and
// the 编译 button, which is disabled with Why when something stops the build (G14).
type BuildPage struct {
	Plan  string `json:"plan"`
	Name  string `json:"name"` // a free name to start from
	Light bool   `json:"light"`
	// The summary: environment, mods carried, size before → after, saves, location.
	Env    string      `json:"env"`
	Mods   int         `json:"mods"`
	Before string      `json:"before,omitempty"`
	After  string      `json:"after,omitempty"`
	Saves  string      `json:"saves"`
	Dir    string      `json:"dir"`
	Steps  []BuildStep `json:"steps"`
	Ok     bool        `json:"ok"`
	Why    string      `json:"why,omitempty"`     // why 编译 is disabled
	ToNeed bool        `json:"toNeeds,omitempty"` // Why is fixed on 能不能跑
}

// DefaultPackName is a free pack name for the plan: "<plan> <date>", then " 2", " 3".
func DefaultPackName(a *app.App, plan string) string {
	var b strings.Builder
	for _, r := range plan {
		if app.CheckPackName("a"+string(r)) == nil {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	base := strings.Join(strings.Fields(b.String()), " ")
	if base == "" || app.CheckPackName(base) != nil {
		base = "pack"
	}
	if r := []rune(base); len(r) > 50 {
		base = strings.TrimSpace(string(r[:50]))
	}
	base += " " + a.Now().In(a.Loc).Format("2006-01-02")
	name := base
	for i := 2; ; i++ {
		if _, err := a.ReadPack(name); err != nil && !exists(a, name) {
			return name
		}
		name = base + " " + strconv.Itoa(i)
	}
}

func exists(a *app.App, name string) bool {
	_, err := a.UnfinishedDir(name)
	return err == nil
}

// BuildPageOf turns a checked (and, for a full pack, measured) plan into the page. It reads
// only the plan, so the window builds it without holding the app.
func BuildPageOf(pl *app.PackPlan, plan, envChip, packsDir string, light bool) *BuildPage {
	r := pl.R
	st := pl.St
	mm := st.GameMM()
	p := &BuildPage{Plan: plan, Light: light, Env: envChip, Dir: packsDir, Saves: T("its own, empty the first time it starts")}
	if light {
		p.Saves = T("none: a light pack is a list for sharing, it does not start")
	}
	carried := 0
	if r.Pack != nil {
		for _, e := range r.Pack.Entries {
			if !e.Official {
				carried++
			}
		}
	} else {
		for _, e := range st.List {
			if m := st.Ix.ByID[e.Instance]; m == nil || m.Kind != source.Official {
				carried++
			}
		}
	}
	p.Mods = carried

	// 1 · check
	check := BuildStep{Title: T("1 · check that it runs")}
	if n := len(r.Blockers); n > 0 {
		check.Level, check.Needs = LevelBad, true
		check.Text = Tf("%d problems still break the game; fix them first. The load order is sorted when the pack is built.", n)
		for i, b := range r.Blockers {
			if i == 8 {
				check.More = append(check.More, Tf("… %d more", n-8))
				break
			}
			if pkg, ok := strings.CutPrefix(b, "not installed: pkg:"); ok {
				b = Tf("the list has %s, but it is not installed", pkg)
			} else {
				b = Humanize(st, b)
			}
			check.More = append(check.More, b)
		}
		p.Why, p.ToNeed = Tf("fix %d problems first", n), true
	} else {
		check.Text = T("nothing breaks the game.") + sp() + Tf("The load order is sorted for the pack (%d moved); your plan does not change.", r.Moved)
		if n := len(r.Mismatch); n > 0 {
			check.Level = LevelWarn
			check.Text += sp() + Tf("%d mods do not list game version %s.", n, mm)
			for i, m := range r.Mismatch {
				if i == 6 {
					check.More = append(check.More, Tf("… %d more", n-6))
					break
				}
				check.More = append(check.More, Humanize(st, m))
			}
		}
	}

	// 2 · trim
	trim := BuildStep{Title: T("2 · leave out what is not used")}
	switch {
	case light:
		trim.Level, trim.Text = LevelOff, T("a light pack is the list only: nothing is copied")
	case r.Pack == nil:
		trim.Level, trim.Text = LevelOff, T("worked out once the problems are fixed")
	default:
		withOld := 0
		for _, e := range r.Pack.Entries {
			if len(e.TrimmedDir) > 0 {
				withOld++
			}
		}
		if withOld > 0 {
			trim.Text = Tf("Only what %s loads is kept: %d mods carry folders for other game versions; leaving them out saves %s.", mm, withOld, SizeText(r.Pack.Trimmed))
		} else {
			trim.Text = Tf("Only what %s loads is kept; no mod carries folders for other game versions.", mm)
		}
		switch {
		case r.Free >= 0 && r.Free < r.Need+app.SpaceMargin:
			trim.Level = LevelBad
			trim.Text += sp() + Tf("Only %s free on %s: not enough.", SizeText(r.Free), driveOf(packsDir))
			p.Why = T("not enough free space")
		case r.Free >= 0 && r.Free-r.Need < 10<<30:
			trim.Level = LevelWarn
			trim.Text += sp() + Tf("%s free on %s; %s left after.", SizeText(r.Free), driveOf(packsDir), SizeText(r.Free-r.Need))
		}
		if len(r.CannotCopy) > 0 {
			trim.Level = LevelBad
			trim.Text += sp() + Tf("%d mods cannot be read.", len(r.CannotCopy))
			trim.More = append(trim.More, r.CannotCopy...)
			p.Why = Tf("%d mods cannot be read", len(r.CannotCopy))
		}
		p.Before, p.After = SizeText(r.Need+r.Pack.Trimmed), SizeText(r.Need)
	}

	// 3 · lock the versions
	lock := BuildStep{Title: T("3 · lock the versions")}
	if light {
		lock.Text = Tf("Records what %d mods are now; whoever imports it is told which match.", carried)
	} else {
		lock.Text = Tf("Records what %d mods are now and copies them: Workshop updates no longer change the pack.", carried)
	}

	// 4 · put it in place
	place := BuildStep{Title: T("4 · put it in place")}
	if light {
		place.Text = T("Writes the list (pack.json) to the packs folder; export it to share.")
	} else {
		place.Text = Tf("Goes into RimArcho's packs folder, never the Workshop folder. It starts with its own saves and settings; the saves of %s are not touched.", plan)
	}
	p.Steps = []BuildStep{check, trim, lock, place}
	p.Ok = p.Why == ""
	return p
}

// ---- one pack (06 §2.7) ----

// PackPageEntry is one mod of a pack's page.
type PackPageEntry struct {
	Name string `json:"name"`
	Size string `json:"size,omitempty"`
	Note string `json:"note,omitempty"` // "from the game", the version folders left out
}

// PackPage is a pack's own page: what it is, and its mods in load order.
type PackPage struct {
	Name     string          `json:"name"`
	Light    bool            `json:"light,omitempty"`
	Facts    []string        `json:"facts"` // "42 mods · 1.2 GB", "runs on 1.6", "built 10 月 20 日 in main", "played …"
	Dir      string          `json:"dir"`
	Mods     []PackPageEntry `json:"mods"`
	Exported string          `json:"exportDir"` // where 导出 offers to write
}

// BuildPackPage reads a pack for its page.
func BuildPackPage(a *app.App, name string) (*PackPage, error) {
	ps, _, err := packsOfCurrent(a)
	if err != nil {
		return nil, err
	}
	var p *app.PackInfo
	for _, x := range ps {
		if x.Name == name {
			p = x
		}
	}
	if p == nil {
		pk, err := a.ReadPack(name)
		if err != nil {
			return nil, err
		}
		p = &app.PackInfo{Pack: pk}
	}
	pg := &PackPage{Name: p.Name, Light: p.Light, Dir: p.Dir, Exported: DefaultExportDir()}
	carried := 0
	for _, e := range p.Entries {
		if !e.Official {
			carried++
		}
	}
	size := SizeText(p.Bytes)
	if p.Light {
		size = T("list only")
	}
	pg.Facts = append(pg.Facts, Tf("%d mods · %s", carried, size))
	if v := PackVersions(a, p.Pack); v != "" {
		pg.Facts = append(pg.Facts, Tf("runs on %s", v))
	}
	built := Tf("built %s", ShortDate(p.Created))
	if p.Env != "" {
		built += " · " + p.Env
	}
	pg.Facts = append(pg.Facts, built)
	if p.LastPlayed.IsZero() {
		pg.Facts = append(pg.Facts, T("never played"))
	} else {
		pg.Facts = append(pg.Facts, Tf("played %s", ShortDate(p.LastPlayed)))
	}
	for _, e := range p.Entries {
		r := PackPageEntry{Name: e.Name}
		switch {
		case e.Official:
			r.Note = T("from the game")
		case !p.Light:
			r.Size = SizeText(e.Bytes)
			if len(e.TrimmedDir) > 0 {
				r.Note = Tf("left out %s", strings.Join(e.TrimmedDir, ", "))
			}
		}
		pg.Mods = append(pg.Mods, r)
	}
	return pg, nil
}

func packsOfCurrent(a *app.App) ([]*app.PackInfo, int, error) {
	e, err := a.Env()
	if err != nil {
		return nil, 0, err
	}
	return a.PacksOf(e)
}

// sp joins two sentences: a space in English, nothing after a Chinese full stop.
func sp() string {
	if Lang() == "zh" {
		return ""
	}
	return " "
}
