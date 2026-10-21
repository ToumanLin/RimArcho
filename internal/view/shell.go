package view

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
)

// The window's shell (06 §2.1–§2.3, §2.15): the sidebar, the home page and the first-run
// guide's data. Until a player can keep several plans (04 §1) the environment's workspace
// is its one plan.

// ---- counts ----

// BreakCount is the number of items in 能不能跑's "会让游戏出错" group: the list's own
// problems, missing dependencies and conflicts (06 §2.4). The home page's state counts it.
func BreakCount(n *Needs) int {
	c := 0
	for _, it := range n.Items {
		if it.Counted && (it.Group == GroupList || it.Group == GroupDeps || it.Group == GroupConflict) {
			c++
		}
	}
	return c
}

// LastRunErrors is the number of the last run's errors in 能不能跑.
func LastRunErrors(n *Needs) int {
	c := 0
	for _, it := range n.Items {
		if it.Counted && it.Group == GroupLastRun {
			c++
		}
	}
	return c
}

// EnvChip is an environment as a chip: "steam · 1.6".
func EnvChip(name, gameVersion string) string {
	if name == "" {
		return T("no environment")
	}
	if mm := env.MajorMinor(gameVersion); mm != "" {
		return name + " · " + mm
	}
	return name
}

// ---- the sidebar (06 §2.1) ----

// Sidebar page IDs. The plan editor is "plan"; a pack's row is "pack:<name>".
const (
	PageHome     = "home"
	PageNeeds    = "needs"
	PagePlan     = "plan"
	PagePacks    = "packs"
	PageEnv      = "env"
	PageLibrary  = "library"
	PageChanges  = "changes"
	PageLog      = "log"
	PageSettings = "settings"
)

// NavItem is one sidebar entry. Kids show only under the open entry.
type NavItem struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Badge int       `json:"badge,omitempty"` // 能不能跑: the actionable items (06 §5)
	Key   string    `json:"key,omitempty"`   // shift+1–7
	Kids  []NavItem `json:"kids,omitempty"`
}

// NavGroup is one of the sidebar's three questions. A group with nothing in it yet is
// left out rather than shown empty.
type NavGroup struct {
	Title string    `json:"title"`
	Items []NavItem `json:"items"`
}

// Nav builds the sidebar: the 在玩 entries in the shift+1–7 order, with the current list as
// the one plan and every pack under 整合包. 没玩到 and 值得玩 have no pages yet (10) and
// are left out.
func Nav(needs int, packs []PackRow) []NavGroup {
	var packKids []NavItem
	for _, p := range packs {
		packKids = append(packKids, NavItem{ID: "pack:" + p.Name, Label: p.Name})
	}
	items := []NavItem{
		{ID: PageHome, Label: T("home")},
		{ID: PageNeeds, Label: T("can it run"), Badge: needs},
		{ID: PagePlan, Label: T("plans"), Kids: []NavItem{{ID: PagePlan, Label: T("current list")}}},
		{ID: PagePacks, Label: T("packs"), Kids: packKids},
		{ID: PageEnv, Label: T("environment")},
		{ID: PageLibrary, Label: T("mod library"), Kids: []NavItem{{ID: PageChanges, Label: T("changes")}}},
		{ID: PageLog, Label: T("game log")},
	}
	for i := range items {
		items[i].Key = fmt.Sprintf("shift+%d", i+1)
	}
	return []NavGroup{{Title: T("playing"), Items: items}}
}

// NavPath is where a page sits in the sidebar: the group, the entry and the sub-entry's
// labels, for the breadcrumb (06 §2.2), with the page each one opens.
func NavPath(groups []NavGroup, id string) []Crumb {
	if id == PageSettings {
		return []Crumb{{Label: T("settings"), ID: PageSettings}}
	}
	for _, g := range groups {
		for _, it := range g.Items {
			head := []Crumb{{Label: g.Title, ID: g.Items[0].ID}, {Label: it.Label, ID: it.ID}}
			for _, k := range it.Kids {
				if k.ID == id {
					return append(head, Crumb{Label: k.Label, ID: k.ID})
				}
			}
			if it.ID == id {
				return head
			}
		}
	}
	return nil
}

// Crumb is one level of the breadcrumb and the page a click on it opens.
type Crumb struct {
	Label string `json:"label"`
	ID    string `json:"id"`
}

// ---- the home page (06 §2.3) ----

// Home row kinds.
const (
	RowPlan = "plan"
	RowPack = "pack"
)

// HomeOK is a row that is ready to play (LevelOK is "", which reads as "not checked").
const HomeOK = "ok"

// HomeRow is one row of "正在玩": a plan or a pack, its state, and what launches it.
type HomeRow struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Env   string `json:"env"`   // the environment chip
	State string `json:"state"` // "2 problems break the game", "ready to play", "not checked yet"
	Level string `json:"level"` // LevelBad, HomeOK, or "" for not checked
	Facts string `json:"facts"`
	// Played is "played 10 月 20 日" or "never played"; Light packs are a list only and do
	// not launch.
	Played string `json:"played"`
	Light  bool   `json:"light,omitempty"`
	at     time.Time
}

// Home is the home page: the rows, then the activity card.
type Home struct {
	Rows []HomeRow `json:"rows"`
	// LastRun: "the list · 10 月 20 日 19:41"; LastRunErrors its errors in 能不能跑.
	LastRun       string `json:"lastRun,omitempty"`
	LastRunErrors int    `json:"lastRunErrors"`
	// Changes: the last scan's changes in words (ChangeSummary), ScanWhen when it ran.
	Changes  string `json:"changes,omitempty"`
	ScanWhen string `json:"scanWhen,omitempty"`
}

// BuildHome builds the home page from the board, the needs and the packs. Rows go by the
// last time they were played (launch records, 08 §2); never played ones follow by name.
func BuildHome(a *app.App, b *Board, n *Needs, packs []PackRow) *Home {
	h := &Home{}
	if b == nil || b.St == nil {
		return h
	}
	chip := EnvChip(b.EnvName, b.GameVersion)
	plan := HomeRow{Kind: RowPlan, Name: T("current list"), Env: chip, Played: T("never played"),
		Facts: Tf("%d mods enabled", len(b.Cols[Active]))}
	if b.Unsaved > 0 {
		plan.Facts += " · " + Tf("%d not written", b.Unsaved)
	}
	switch k := BreakCount(n); {
	case n.Never:
		plan.State = T("not checked yet")
	case k > 0:
		plan.State, plan.Level = Tf("%d problems break the game", k), LevelBad
	default:
		plan.State, plan.Level = T("ready to play"), HomeOK
	}
	if t := a.Store.LastListLaunch(b.St.Ix.Env.ID); !t.IsZero() {
		plan.at, plan.Played = t, Tf("played %s", ShortDate(t))
	}
	h.Rows = append(h.Rows, plan)
	for _, p := range packs {
		r := HomeRow{Kind: RowPack, Name: p.Name, Env: chip, Facts: p.Facts, Played: p.Played, Light: p.Light, at: p.PlayedAt}
		if p.Light {
			r.State = T("list only")
		} else {
			// A pack is checked when it is built (08 §5.2): error-level problems refuse it.
			r.State, r.Level = T("checked when built"), HomeOK
		}
		h.Rows = append(h.Rows, r)
	}
	SortHome(h.Rows)
	h.LastRun, h.LastRunErrors = n.LastRun, LastRunErrors(n)
	if len(b.LastEvents) > 0 {
		h.Changes = ChangeSummary(b.LastEvents)
	}
	if sc, _, _ := a.LastScan(b.St.Ix.Env.ID); sc != nil {
		h.ScanWhen = ShortDate(sc.Started) + " " + sc.Started.In(a.Loc).Format("15:04")
	}
	return h
}

// SortHome puts the rows played most recently first; the never played ones follow by name.
func SortHome(rows []HomeRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.at.IsZero() != b.at.IsZero() {
			return !a.at.IsZero()
		}
		if !a.at.Equal(b.at) {
			return a.at.After(b.at)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// ---- the first-run guide (06 §2.15) ----

// GameFound is a RimWorld install the guide offers as an environment.
type GameFound struct {
	Name     string `json:"name"` // a suggested environment name (the player may change it)
	Game     string `json:"game"` // the game folder
	Version  string `json:"version"`
	Launch   string `json:"launch"`
	Workshop string `json:"workshop,omitempty"`
	UserData string `json:"userData,omitempty"`
	// Env is the environment that already uses this folder ("" when none).
	Env string `json:"env,omitempty"`
}

// FindGames lists the installs detection finds (02 §2.2), each with a free name; the
// ones an environment already uses say which.
func FindGames(a *app.App) []GameFound {
	envs, _ := a.Store.Envs()
	used := map[string]string{}
	taken := map[string]bool{}
	for _, e := range envs {
		used[env.PathKey(e.GameRoot)] = e.Name
		taken[e.Name] = true
	}
	var out []GameFound
	for _, d := range env.DetectAll(a.Detect) {
		g := GameFound{Game: d.GameRoot, Version: d.GameVersion, Launch: d.Launch, Workshop: d.WorkshopRoot, UserData: d.UserDataRoot, Env: used[env.PathKey(d.GameRoot)]}
		if g.Env == "" {
			g.Name = freeName(d.Launch, taken)
			taken[g.Name] = true
		}
		out = append(out, g)
	}
	return out
}

// GameAt is a game folder the player typed (when detection finds nothing).
func GameAt(a *app.App, path string) (*GameFound, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return nil, errors.New(T("type the game folder"))
	}
	if !env.LooksLikeGameRoot(path) {
		return nil, errors.New(Tf("%s does not look like a RimWorld folder (no game executable or Data/Core)", path))
	}
	envs, _ := a.Store.Envs()
	taken := map[string]bool{}
	g := &GameFound{Game: path, Version: env.ReadGameVersion(path), Launch: "direct", UserData: env.DefaultUserDataRoot()}
	for _, e := range envs {
		taken[e.Name] = true
		if env.PathKey(e.GameRoot) == env.PathKey(path) {
			g.Env = e.Name
		}
	}
	if ws := env.WorkshopRootFor(path); ws != "" && dirExists(ws) {
		g.Workshop, g.Launch = ws, "steam"
	}
	if g.Env == "" {
		g.Name = freeName(g.Launch, taken)
	}
	return g, nil
}

// freeName is base, or base-2, base-3… when it is taken.
func freeName(base string, taken map[string]bool) string {
	if base == "direct" || base == "" {
		base = "rimworld"
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s-%d", base, i); !taken[n] {
			return n
		}
	}
}

// AddGames creates an environment for each chosen install (env add) and makes the first
// one current: the guide's next steps scan it and choose its starting list. It returns
// the environments created.
func AddGames(a *app.App, games []GameFound) ([]string, error) {
	var out []string
	for _, g := range games {
		if g.Env != "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if _, _, err := a.EnvAdd(name, app.EnvSpec{GameRoot: g.Game, UserDataRoot: g.UserData, WorkshopRoot: g.Workshop, Launch: g.Launch}); err != nil {
			return out, err
		}
		out = append(out, name)
	}
	if len(out) > 0 {
		a.EnvName = ""
		if _, err := a.EnvUse(out[0]); err != nil {
			return out, err
		}
	}
	return out, nil
}

// PackImportText says what importing a pack did (pack import, 08 §5.4).
func PackImportText(m *app.PackMatch) string {
	if m == nil || m.Pack == nil {
		return ""
	}
	if m.Dir != "" {
		return Tf("pack %s is in the packs folder; play it from the home page", m.Pack.Name)
	}
	s := Tf("pack %s: %d installed, %d another version, %d missing", m.Pack.Name, len(m.Same), len(m.Different), len(m.Missing))
	if m.Imported != nil {
		s += " · " + T("its list is now the current list; u undoes")
	}
	return s
}
