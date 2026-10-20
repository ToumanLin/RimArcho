package view

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Details (06 §6): who the mod is, its problems, the facts a player uses (package id,
// version, Workshop page, tags, alias, note, dependencies), the description, its history,
// and the full detail's sub-sections (06 §6.3). Each interface lays these out itself.

// DetailData is what the detail shows for one mod beyond its row, loaded off the input path.
type DetailData struct {
	ID       string
	Decl     scan.Decl
	Desc     *app.Description
	About    []bbcode.Block
	Workshop []bbcode.Block
	Events   []*store.Event // the mod's change events, oldest first
	Fetching bool           // the Workshop page is being fetched
}

// LoadDetail reads the declaration, About.xml and the cached Workshop text; fetch allows
// the network (06 §6.4 item 3), refresh ignores the cache.
func LoadDetail(a *app.App, ix *app.Index, mod *store.Mod, fetch, refresh bool) *DetailData {
	d := &DetailData{ID: mod.ID, Decl: mod.Decl()}
	d.Desc = a.Describe(ix, mod, fetch, refresh)
	d.About, d.Workshop = bbcode.Parse(d.Desc.About), bbcode.Parse(d.Desc.Workshop)
	d.Events, _ = a.Store.Events(store.EventFilter{EnvID: ix.Env.ID, InstanceIDs: []string{mod.ID}})
	return d
}

// HeaderFacts: the source, the supported versions and where the mod is (number N, staged,
// not enabled), for the line under the name.
func HeaderFacts(mod *store.Mod, pos int, staged bool) []string {
	facts := []string{SourceText(mod)}
	if mod.Kind != source.Official {
		if v := strings.Fields(mod.SupportedVersions); len(v) > 0 {
			facts = append(facts, T("supports")+" "+strings.Join(v, " "))
		}
	}
	switch {
	case pos > 0:
		facts = append(facts, Tf("number %d", pos))
	case staged:
		facts = append(facts, T("staged"))
	default:
		facts = append(facts, T("not enabled"))
	}
	return facts
}

// UnloadableWhy says why the game cannot load a mod.
func UnloadableWhy(mod *store.Mod) string {
	switch {
	case mod.Unloadable != "":
		return T("About.xml is renamed: the game cannot load it")
	case mod.Residue:
		return T("no longer subscribed: the game ignores this folder")
	}
	return T("the game cannot load it")
}

// Fact kinds the interfaces decorate (a key hint, a button).
const (
	FactPackage  = "package"
	FactVersion  = "version"
	FactWorkshop = "workshop"
	FactTags     = "tags" // Value empty: no tags
	FactAlias    = "alias"
	FactNote     = "note"
)

// Fact is one labelled fact of the detail.
type Fact struct {
	Kind  string   `json:"kind"`
	Label string   `json:"label"`
	Value string   `json:"value"`
	Extra string   `json:"extra,omitempty"` // the alias's original name
	List  []string `json:"list,omitempty"`  // tags
}

// Facts: package, version, Workshop id, tags, alias, note (06 §6.1).
func Facts(mod *store.Mod, d *DetailData) []Fact {
	fs := []Fact{{Kind: FactPackage, Label: T("package"), Value: mod.PackageID}}
	if d != nil && d.Decl.ModVersion != "" {
		fs = append(fs, Fact{Kind: FactVersion, Label: T("version"), Value: d.Decl.ModVersion})
	}
	if ws := mod.WorkshopID(); IsNumber(ws) {
		fs = append(fs, Fact{Kind: FactWorkshop, Label: T("workshop"), Value: ws})
	}
	fs = append(fs, Fact{Kind: FactTags, Label: T("tags"), Value: strings.Join(mod.Tags, " · "), List: mod.Tags})
	if mod.Alias != "" {
		fs = append(fs, Fact{Kind: FactAlias, Label: T("alias"), Value: mod.Alias, Extra: mod.Name})
	}
	if mod.Note != "" {
		fs = append(fs, Fact{Kind: FactNote, Label: T("note"), Value: mod.Note})
	}
	return fs
}

// WorkshopURL is the Workshop page of an item id.
func WorkshopURL(id string) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + id
}

// IsNumber: s is a non-empty string of digits.
func IsNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Dependency states.
const (
	DepEnabled  = "enabled"
	DepDisabled = "disabled" // installed, not enabled
	DepMissing  = "missing"  // not installed
)

// Dep is one declared dependency with its state.
type Dep struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// Deps: the declared dependencies (plus the current version's), each once, with their
// state (06 §6.1).
func Deps(st *app.LoadoutState, d *DetailData) []Dep {
	if d == nil || st == nil {
		return nil
	}
	mm := "v" + st.GameMM()
	seen := map[string]bool{}
	var pkgs []scan.Dependency
	for _, x := range append(append([]scan.Dependency{}, d.Decl.Dependencies...), d.Decl.DependenciesByVersion[mm]...) {
		p := strings.ToLower(x.PackageID)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		pkgs = append(pkgs, x)
	}
	var out []Dep
	for _, x := range pkgs {
		p := strings.ToLower(x.PackageID)
		var inst []*store.Mod
		for _, mod := range st.Ix.Mods {
			if mod.PackageID == p && mod.Status == identity.Live {
				inst = append(inst, mod)
			}
		}
		name := x.DisplayName
		if name == "" {
			name = p
		}
		enabled := false
		for _, mod := range inst {
			if st.List.Index(mod.ID) >= 0 {
				enabled, name = true, mod.DisplayName()
			}
		}
		switch {
		case enabled:
			out = append(out, Dep{name, DepEnabled})
		case len(inst) > 0:
			out = append(out, Dep{inst[0].DisplayName(), DepDisabled})
		default:
			out = append(out, Dep{name, DepMissing})
		}
	}
	return out
}

// DescNote is the faint note after the description heading: the Workshop page's state.
func DescNote(d *DetailData) string {
	switch st := d.Desc.WorkshopState; {
	case d.Fetching && st == "not cached":
		return T("fetching the Workshop page…")
	case st == "not cached":
		return T("the Workshop page is fetched in a moment")
	case st == "offline":
		return T("offline (workshop.fetch)")
	case st != "" && st != "fetched" && st != "cached":
		return T("Workshop page: ") + st
	}
	return ""
}

// EventText is a change event in words.
func EventText(kind string) string {
	switch kind {
	case identity.EvDiscovered:
		return T("first seen")
	case identity.EvUpdated:
		return T("updated")
	case identity.EvMoved:
		return T("moved")
	case identity.EvCopied:
		return T("copied")
	case identity.EvDisappeared:
		return T("gone")
	case identity.EvReappeared:
		return T("reappeared")
	case identity.EvPackageIDChanged:
		return T("package id changed")
	case identity.EvRenamed:
		return T("renamed")
	case identity.EvAuthorChanged:
		return T("author changed")
	case identity.EvVersionSupport:
		return T("supported versions changed")
	case identity.EvDependencies:
		return T("dependencies changed")
	case identity.EvLoadRules:
		return T("load order rules changed")
	}
	return kind
}

// HistoryLine: the one-line history, the three newest events (06 §6.1).
func HistoryLine(evs []*store.Event) string {
	var parts []string
	for i := len(evs) - 1; i >= 0 && len(parts) < 3; i-- {
		parts = append(parts, ShortDate(evs[i].ScanTime)+" "+EventText(evs[i].Kind))
	}
	return strings.Join(parts, " · ")
}

// ---- the full detail's sub-sections (06 §6.3) ----

// Section is one sub-section of the full detail.
type Section struct {
	Title string   `json:"title"`
	Count int      `json:"count"` // -1: no count
	Lines []string `json:"lines"` // indented text lines
}

var countRe = regexp.MustCompile(`(?:defs|harmony|patches): (\d+)`)

// Sections: Defs, patches, Harmony (the read-only command text), rules and the full
// history of a mod.
func Sections(a *app.App, ix *app.Index, mod *store.Mod) []Section {
	ref := ix.Ref(mod.ID)
	var out []Section
	run := func(title string, argv ...string) {
		text, _ := cmds.Execute(a, argv)
		ls := strings.Split(strings.TrimRight(text, "\n"), "\n")
		sec := Section{Title: title, Count: -1}
		if len(ls) > 0 {
			if mt := countRe.FindStringSubmatch(ls[0]); mt != nil {
				fmt.Sscan(mt[1], &sec.Count)
			}
			if strings.HasPrefix(ls[0], "#") { // "#ref Name  defs: N": the title says it
				ls = ls[1:]
			}
		}
		for i, l := range ls {
			t := strings.TrimSpace(l)
			if t == "list:" || t == "next:" || strings.HasPrefix(t, "archo ") {
				continue
			}
			if strings.HasSuffix(t, ":") && (i+1 == len(ls) || indentOf(ls[i+1]) <= indentOf(l)) {
				continue // a heading with nothing under it
			}
			sec.Lines = append(sec.Lines, l)
		}
		out = append(out, sec)
	}
	run(T("Defs"), "def", "--mod", ref)
	run(T("patches"), "patches", "--mod", ref)
	run("Harmony", "harmony", "--mod", ref)
	out = append(out, rulesSection(a, ix, mod), historySection(a, ix, mod.ID))
	return out
}

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// rulesSection: the order the mod declares in About.xml and the player's rules that name it.
func rulesSection(a *app.App, ix *app.Index, mod *store.Mod) Section {
	sec := Section{Title: T("rules"), Count: -1}
	d := mod.Decl()
	name := func(pkg string) string {
		for _, x := range ix.Mods {
			if x.PackageID == strings.ToLower(pkg) && x.Status == identity.Live {
				return x.DisplayName()
			}
		}
		return pkg
	}
	for _, g := range []struct {
		f    string
		pkgs []string
	}{
		{"loads after %s (About.xml)", append(append([]string{}, d.LoadAfter...), d.ForceLoadAfter...)},
		{"loads before %s (About.xml)", append(append([]string{}, d.LoadBefore...), d.ForceLoadBefore...)},
		{"incompatible with %s (About.xml)", d.IncompatibleWith},
	} {
		for _, p := range g.pkgs {
			sec.Lines = append(sec.Lines, "  "+Tf(g.f, name(p)))
		}
	}
	lin := func(l string) string {
		if p := ix.Lineages[l]; p != nil && ix.ByID[p.PrimaryID] != nil {
			return ix.ByID[p.PrimaryID].DisplayName()
		}
		if ms := ix.ByLineage[l]; len(ms) > 0 {
			return ms[0].DisplayName()
		}
		return "?"
	}
	rs, _ := a.Store.Rules(ix.Env.ID)
	for _, r := range rs {
		if r.A != mod.LineageID && r.B != mod.LineageID {
			continue
		}
		var l string
		switch r.Kind {
		case order.RuleAfter:
			l = Tf("%s loads after %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleBefore:
			l = Tf("%s loads before %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleIncompatible:
			l = Tf("%s is incompatible with %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleAttach:
			l = Tf("%s follows %s (your rule)", lin(r.A), lin(r.B))
		default:
			continue
		}
		if r.Note != "" {
			l += "  ·  " + r.Note
		}
		sec.Lines = append(sec.Lines, "  "+l)
	}
	if len(sec.Lines) == 0 {
		sec.Lines = []string{"  " + T("none")}
	}
	return sec
}

// historySection: every change event of the mod, newest first.
func historySection(a *app.App, ix *app.Index, id string) Section {
	sec := Section{Title: T("full history"), Count: -1}
	evs, _ := a.Store.Events(store.EventFilter{EnvID: ix.Env.ID, InstanceIDs: []string{id}})
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		l := "  " + e.ScanTime.Local().Format("2006-01-02 15:04") + "  " + EventText(e.Kind)
		if d := strings.TrimSpace(e.Detail); d != "" && e.Kind != identity.EvDiscovered {
			l += "  " + d
		}
		sec.Lines = append(sec.Lines, l)
	}
	sec.Count = len(evs)
	if len(evs) == 0 {
		sec.Lines = []string{"  " + T("none")}
	}
	return sec
}
