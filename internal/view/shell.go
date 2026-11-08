package view

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
)

// The window's shell (16 §2.1–§2.3, §2.15): the sidebar, the home page and the first-run
// guide's data. The plans are the environment's (plans.go).

// ---- counts ----

// BreakCount is the number of items in Can it run's "Will cause errors" group: the list's own
// problems, missing dependencies and conflicts (16 §2.4). The home page's state counts it.
func BreakCount(n *Needs) int {
	c := 0
	for _, it := range n.Items {
		if it.Counted && (it.Group == GroupList || it.Group == GroupDeps || it.Group == GroupConflict) {
			c++
		}
	}
	return c
}

// PackLinksText is the title of Can it run's item for a pack's links left in Mods/ ("" when
// there is none).
func PackLinksText(n *Needs) string {
	for _, it := range n.Items {
		for _, a := range it.Actions {
			if a.Kind == ActUnlinkPacks {
				return it.Title
			}
		}
	}
	return ""
}

// LastRunErrors is the number of the last run's errors in Can it run.
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
		return T("no game")
	}
	if mm := env.MajorMinor(gameVersion); mm != "" {
		return name + "  " + mm
	}
	return name
}

// ---- the sidebar (16 §2.1) ----

// Sidebar page IDs. "plan" is the current plan's editor; a plan's own page is
// PlanPage(id) and a pack's is "pack:<name>".
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
	PageTextures = "textures" // Texture optimisation (10 §2), under Library
	// PageSubscribe: Mod library › add a subscription (16 §2.9)
	PageSubscribe = "subscribe"
	// PageSearch: Mod library › Search files (16 §2.9)
	PageSearch = "search"
	// PageModSettings: Mod library › Mod settings (16 §2.9 "Mod settings")
	PageModSettings = "mod-settings"
)

// NavItem is one sidebar entry. Kids show only under the open entry.
type NavItem struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Badge int       `json:"badge,omitempty"` // Can it run: the actionable items (16 §5)
	Key   string    `json:"key,omitempty"`   // shift+1–8
	Kids  []NavItem `json:"kids,omitempty"`
	// Mark is a plan's "an outside version has not been loaded" (07 §4.3); Current the current plan (the one
	// launched, listed first); Open the plan being edited; InGame the plan the game's
	// ModsConfig.xml holds (a small mark when there are several).
	Mark    string `json:"mark,omitempty"`
	Current bool   `json:"current,omitempty"`
	Open    bool   `json:"open,omitempty"`
	InGame  bool   `json:"inGame,omitempty"`
	// Add: the entry opens a dialog ("+ new list"), not a page.
	Add bool `json:"add,omitempty"`
}

// NavLists is how many lists the sidebar shows under list; the rest are in all lists.
const NavLists = 4

// navPlans picks the sidebar's lists: the current one first, then the others in the order
// they were last enabled, then the never enabled ones by last use; NavLists at most.
// Opening a list in the editor does not move it (16 §2.1).
func navPlans(plans []PlanRow) []PlanRow {
	rank := func(p PlanRow) int {
		switch {
		case p.Current:
			return 0
		case p.Enabled > 0:
			return 1
		}
		return 2
	}
	out := slices.Clone(plans)
	slices.SortStableFunc(out, func(a, b PlanRow) int {
		if c := cmp.Compare(rank(a), rank(b)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Enabled, b.Enabled); c != 0 {
			return c
		}
		return b.UsedAt.Compare(a.UsedAt)
	})
	if len(out) > NavLists {
		out = out[:NavLists]
	}
	return out
}

// NavGroup is one of the sidebar's three questions. A group with nothing in it yet is
// left out rather than shown empty.
type NavGroup struct {
	Title string    `json:"title"`
	Items []NavItem `json:"items"`
}

// Nav builds the sidebar: the Playing and Library entries in the shift+1–8 order, every plan and "+ new list"
// under Lists, every pack under pack. not played yet and Worth playing have no pages yet (10) and are
// left out.
func Nav(needs int, plans []PlanRow, packs []PackRow) []NavGroup {
	// all lists comes first, where every list is and new lists are made; then at most
	// NavLists lists: the current one, then the others by when they were last enabled.
	// list itself opens the current list (16 §2.1, §2.17).
	// When some are left out, all lists says how many there are.
	all := T("all lists")
	if len(plans) > NavLists {
		all = Tf("all lists (%d)", len(plans))
	}
	planKids := []NavItem{{ID: PagePlans, Label: all}}
	for _, p := range navPlans(plans) {
		planKids = append(planKids, NavItem{ID: p.Page, Label: p.Name, Mark: p.ConflictText, Current: p.Current, Open: p.Open, InGame: p.InGame})
	}
	// Packs the same way: at most NavLists, the most recently played; with more, Allpack
	// with their number comes first.
	var packKids []NavItem
	shown := slices.Clone(packs)
	if len(shown) > NavLists {
		slices.SortStableFunc(shown, func(a, b PackRow) int { return b.PlayedAt.Compare(a.PlayedAt) })
		shown = shown[:NavLists]
		packKids = append(packKids, NavItem{ID: PagePacks, Label: Tf("all packs (%d)", len(packs))})
	}
	for _, p := range shown {
		packKids = append(packKids, NavItem{ID: "pack:" + p.Name, Label: p.Name})
	}
	// Two groups (16 §2.1): what you play now, and what you have.
	playing := []NavItem{
		{ID: PageHome, Label: T("start")},
		{ID: PageNeeds, Label: T("can it run"), Badge: needs},
		{ID: PageLog, Label: T("game log")},
	}
	library := []NavItem{
		{ID: PagePlan, Label: T("lists"), Kids: planKids},
		{ID: PagePacks, Label: T("packs"), Kids: packKids},
		// Saves (16 §2.11), after the packs since 2026-11-04. Its own name: the plain
		// "saves" key is translated for settings rows; the sidebar's name is longer.
		{ID: PageSaves, Label: extraName("@nav.saves", "saves")},
		// Below the packs (2026-10-28). The entry opens All, its first sub-entry (16 §2.9).
		{ID: PageLibrary, Label: T("mod library"), Kids: []NavItem{{ID: PageLibrary, Label: T("all mods")}, {ID: PageChanges, Label: T("updates")}, {ID: PageSubscribe, Label: T("add a subscription")}, {ID: PageSearch, Label: T("search files")}, {ID: PageModSettings, Label: T("mod settings")}}},
		// what the game loads faster (10 §2)
		{ID: PageTextures, Label: T("texture optimisation")},
		{ID: PageEnv, Label: T("game")},
	}
	groups := []NavGroup{{Title: T("playing"), Items: playing}, {Title: T("library"), Items: library}}
	n := 0
	for g := range groups {
		for i := range groups[g].Items {
			if groups[g].Items[i].ID == PageSaves {
				continue // no shift key: shift+1–8 stay the eight pages the TUI also has (16 §3.5)
			}
			n++
			groups[g].Items[i].Key = fmt.Sprintf("shift+%d", n)
		}
	}
	return groups
}

// extraName is a name kept in the locale's supplemental table under key, en in English.
func extraName(key, en string) string {
	if lang := Lang(); lang != "en" {
		if name := localeExtra(lang, key); name != key {
			return name
		}
	}
	return en
}

// NavPath is where a page sits in the sidebar: the group, the entry and the sub-entry's
// labels, for the breadcrumb (16 §2.2), with the page each one opens.
func NavPath(groups []NavGroup, id string) []Crumb {
	if id == PageSettings {
		return []Crumb{{Label: T("settings"), ID: PageSettings}}
	}
	if id == PageTasks {
		return []Crumb{{Label: T("background tasks"), ID: PageTasks}}
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

// ---- the home page (16 §2.3) ----

// Home row kinds.
const (
	RowPlan = "plan"
	RowPack = "pack"
)

// HomeOK is a row that is ready to play (LevelOK is "", which reads as "not checked").
const HomeOK = "ok"

// HomeRow is one row of "playing now": a plan or a pack, its state, and what launches it.
type HomeRow struct {
	Memory      *MemoryLaunch `json:"memory,omitempty"`
	GameVersion string        `json:"gameVersion"`
	Kind        string        `json:"kind"`
	Name        string        `json:"name"`
	Env         string        `json:"env"`   // the environment chip
	State       string        `json:"state"` // "2 problems break the game", "ready to play", "not checked yet"
	Level       string        `json:"level"` // LevelBad, HomeOK, or "" for not checked
	// Facts: "312 mods"; Mode how its mods change: a plan follows Workshop updates, a pack
	// keeps its versions (10). Unsaved: the current plan's entries not written to the
	// game yet (the launch step says so).
	Facts   string `json:"facts"`
	Mode    string `json:"mode,omitempty"`
	Unsaved int    `json:"unsaved,omitempty"`
	// Played is "played Oct 20" or "never played".
	Played string `json:"played"`
	// A plan's row: its ID and page, whether it is the one being edited, its
	// "an outside version has not been loaded" mark, and whether the game holds it now.
	Plan    int64  `json:"plan,omitempty"`
	Page    string `json:"page,omitempty"`
	Current bool   `json:"current,omitempty"`
	Mark    string `json:"mark,omitempty"`
	InGame  bool   `json:"inGame,omitempty"`
	// Links: the current plan's row when a pack's links are left in Mods/ (10 §3), the
	// sentence of its Can it run item; the launch button's tab shows it.
	Links string `json:"links,omitempty"`
	// EnvName is its environment, Versions the game versions it runs on ("1.7"; a pack's
	// from its mods, "1.5–1.6"), Mods its number of mods; OtherEnv: not the current
	// environment (applying it switches environments first).
	EnvName  string `json:"envName"`
	Versions string `json:"versions"`
	Mods     int    `json:"mods"`
	OtherEnv bool   `json:"otherEnv,omitempty"`
	at       time.Time
}

// Home is the home page: the rows, then the activity card.
type Home struct {
	Error string    `json:"error,omitempty"`
	Rows  []HomeRow `json:"rows"`
	// LastRun: "the list · Oct 20 19:41" ("" when no run was read: the line is
	// hidden); LastRunDay its day; LastRunErrors its errors in Can it run.
	LastRun       string `json:"lastRun,omitempty"`
	LastRunDay    string `json:"lastRunDay,omitempty"`
	LastRunErrors int    `json:"lastRunErrors"`
	// Changes is the current plan's changes since it was last played (16 §2.3): one
	// sentence that opens Updates with that scope and time.
	Changes *modchanges.Home `json:"changes,omitempty"`
	// Featured is the row of what is applied now (PickFeatured): the large card with
	// ▶ launch game; -1 when there is none.
	Featured int `json:"featured"`
	// Launch is the current plan's most likely trouble (Needs.Launch): the tab under the
	// launch button says it with its reason and opens Can it run. It never blocks.
	Launch *LaunchHint `json:"launch,omitempty"`
	// Update is the line about a new RimArcho at the top right (17 §5.1); nil when there is none.
	Update *SelfUpdateLine `json:"update,omitempty"`
}

// BuildHome builds the home page: one row per plan of the environment and one per pack.
// The current plan's state counts its needs; another plan's comes from PlanProblems.
// Rows go by the last time they were played (launch records, 09 §2); never played ones
// follow by name.
func BuildHome(a *app.App, b *Board, n *Needs, plans []PlanRow, packs []PackRow) *Home {
	if b == nil || b.St == nil {
		return &Home{Featured: -1, Update: SelfUpdateHome(a)}
	}
	return BuildHomeFromState(a, b.St, n, plans, packs)
}

// BuildHomeFromState needs counts and list state, not the editor's thousands of
// inactive rows, copy captions and search fields.
func BuildHomeFromState(a *app.App, st *app.LoadoutState, n *Needs, plans []PlanRow, packs []PackRow) *Home {
	defer perftrace.Span("Home-rows")()
	h := &Home{Featured: -1}
	name, version := st.Ix.Env.Name, st.Ix.Env.GameVersion
	chip, ver := EnvChip(name, version), env.MajorMinor(version)
	active := 0
	for _, e := range st.List {
		if e.Instance == "" || st.Ix.ByID[e.Instance] != nil {
			active++ // the editor shows missing packages but omits unknown instances
		}
	}
	for _, p := range plans {
		r := HomeRow{Kind: RowPlan, Name: p.Name, Env: chip, Played: p.Played, at: p.PlayedAt, Plan: p.ID, Page: p.Page,
			Current: p.Current, Mark: p.ConflictText, InGame: p.InGame, Facts: Tf("%d mods", p.Entries), Mode: T("follows Workshop updates"),
			EnvName: name, GameVersion: version, Versions: ver, Mods: p.Entries}
		// a list says the game versions it allows (07 §1.6, 16 §2.3)
		if sp, err := a.Store.PlanByID(p.ID); err == nil && sp != nil {
			if vs := app.FormatVersions(a.PlanAllowedVersions(st, sp)); vs != "" {
				r.Versions = vs
			}
		}
		k, checked := 0, false
		if p.Current {
			r.Memory = BuildMemoryLaunch(a, st)
			r.Mods = active
			r.Facts = Tf("%d mods", active)
			r.Unsaved = len(st.Changes())
			k, checked = BreakCount(n), !n.Never
			r.Links = PackLinksText(n)
		} else {
			k, checked = PlanProblems(a, p.ID)
		}
		switch {
		case !checked:
			r.State = T("not checked yet")
		case k > 0:
			r.State, r.Level = Tf("%d problems break the game", k), LevelBad
		default:
			r.State, r.Level = T("ready to play"), HomeOK
		}
		h.Rows = append(h.Rows, r)
	}
	for _, p := range packs {
		h.Rows = append(h.Rows, packHomeRow(p, chip, name, version))
	}
	SortHome(h.Rows)
	h.LastRun, h.LastRunDay, h.LastRunErrors = n.LastRun, n.LastRunDay, LastRunErrors(n)
	h.Launch = n.Launch
	h.Changes = ChangeHome(a, st)
	h.Update = SelfUpdateHome(a)
	return h
}

func packHomeRow(p PackRow, chip, envName, gameVersion string) HomeRow {
	r := HomeRow{Kind: RowPack, Name: p.Name, Env: chip, Facts: p.Facts, Played: p.Played, at: p.PlayedAt,
		EnvName: envName, GameVersion: gameVersion, Versions: p.Versions, Mods: p.Mods}
	if p.Mods > 0 {
		r.Facts = Tf("%d mods", p.Mods)
	}
	r.Mode = T("versions locked  own saves")
	// A pack is checked when it is built (10 §2): error-level problems refuse it.
	r.State, r.Level = T("checked when created"), HomeOK
	return r
}

// AddOtherEnvs puts the other environments' plans and packs on the home page (16 §2.3):
// it lists what you play everywhere, not only in the current environment. Lists are
// shared by every game, so each game lists all of them. Their state comes from
// PlanProblems like any plan that is not current; nothing is loaded or scanned.
func AddOtherEnvs(a *app.App, h *Home, current string) {
	defer perftrace.Span("Home-other-envs")()
	envs, err := a.Store.Envs()
	if err != nil {
		return
	}
	for _, e := range envs {
		if e.ID == current {
			continue
		}
		chip, ver := EnvChip(e.Name, e.GameVersion), env.MajorMinor(e.GameVersion)
		ps, _ := a.PlansIn(e.ID)
		for _, p := range ps {
			r := HomeRow{Kind: RowPlan, Name: p.Name, Env: chip, Played: T("never played"), at: p.LastPlayed, Plan: p.ID,
				Facts: Tf("%d mods", p.Entries), Mode: T("follows Workshop updates"), EnvName: e.Name, GameVersion: e.GameVersion, Versions: ver, Mods: p.Entries, OtherEnv: true}
			if len(p.Plan.Versions) > 0 { // the player's choice; the default needs that game's mods loaded
				r.Versions = app.FormatVersions(p.Plan.Versions)
			}
			if !p.LastPlayed.IsZero() {
				r.Played = Tf("played %s", ShortDate(p.LastPlayed))
			}
			r.State = T("not checked yet")
			if k, checked := PlanProblems(a, p.ID); checked && k > 0 {
				r.State, r.Level = Tf("%d problems break the game", k), LevelBad
			} else if checked {
				r.State, r.Level = T("ready to play"), HomeOK
			}
			h.Rows = append(h.Rows, r)
		}
		if packs, _, err := a.PacksOf(e); err == nil {
			for _, p := range packRows(a, packs) {
				r := packHomeRow(p, chip, e.Name, e.GameVersion)
				r.OtherEnv = true
				h.Rows = append(h.Rows, r)
			}
		}
	}
	SortHome(h.Rows)
}

// FeaturedKey is the GUI setting that remembers a pack applied on the home page:
// "<environment>\x1f<pack>\x1f<the plan current then>". Applying a plan clears it, and it
// lapses when the current plan changes some other way.
const FeaturedKey = "ui.gui.home.pack"

// FeaturedValue is what FeaturedKey holds after applying pack in envName.
func FeaturedValue(envName, pack string, plan int64) string {
	return envName + "\x1f" + pack + "\x1f" + strconv.FormatInt(plan, 10)
}

// AppliedPack is the pack the Launch page applied in envName, from FeaturedKey's setting:
// "" when the setting names another game, or the current plan changed since (a plan is
// applied then).
func AppliedPack(setting, envName string, plan int64) string {
	f := strings.Split(setting, "\x1f")
	if len(f) == 3 && f[0] == envName && f[2] == strconv.FormatInt(plan, 10) {
		return f[1]
	}
	return ""
}

// PickFeatured sets h.Featured: the applied pack when it is still valid, else the
// current environment's current plan.
func PickFeatured(h *Home, setting, envName string, plan int64) {
	h.Featured = -1
	if f := strings.Split(setting, "\x1f"); len(f) == 3 && f[0] == envName && f[2] == strconv.FormatInt(plan, 10) {
		for i, r := range h.Rows {
			if r.Kind == RowPack && !r.OtherEnv && r.Name == f[1] {
				h.Featured = i
				return
			}
		}
	}
	for i, r := range h.Rows {
		if r.Kind == RowPlan && r.Current && !r.OtherEnv {
			h.Featured = i
			return
		}
	}
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

// ---- the first-run guide (16 §2.15) ----

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

// FindGames lists the installs detection finds (03 §3.2), each with a free name; the
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
			g.Name = freeName(env.MajorMinor(d.GameVersion), taken)
			taken[g.Name] = true
		}
		out = append(out, g)
	}
	return out
}

// GameAt is a game folder the player typed (when detection finds nothing). pending are the
// names already proposed on the page for games not added yet: the suggested name avoids
// them as well as the environments, so two games of one version get 1.6-2 and 1.6-3.
func GameAt(a *app.App, path string, pending ...string) (*GameFound, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return nil, errors.New(T("type the game folder"))
	}
	if !env.LooksLikeGameRoot(path) {
		return nil, errors.New(Tf("%s does not look like a RimWorld folder (no game executable or Data/Core)", path))
	}
	envs, _ := a.Store.Envs()
	taken := map[string]bool{}
	g := &GameFound{Game: path, Version: env.ReadGameVersion(path), Launch: "direct", UserData: a.DefaultUserData()}
	for _, n := range pending {
		taken[strings.TrimSpace(n)] = true
	}
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
		g.Name = freeName(env.MajorMinor(g.Version), taken)
	}
	return g, nil
}

// freeName is base (the game's version, "1.6"), or base-2, base-3… when it is taken.
func freeName(base string, taken map[string]bool) string {
	if base == "" {
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
	out, err := CreateGames(a, games)
	if err == nil && len(out) > 0 {
		a.EnvName = ""
		_, err = a.EnvUse(out[0])
	}
	return out, err
}

// CreateGames registers installs without scanning or selecting them (16 §8.4). The names
// are checked first, all of them, so a bad or clashing name adds none of the games.
func CreateGames(a *app.App, games []GameFound) ([]string, error) {
	if err := checkGameNames(a, games); err != nil {
		return nil, err
	}
	var out []string
	for _, g := range games {
		if g.Env != "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if _, _, err := a.EnvAdd(name, app.EnvSpec{GameRoot: g.Game, UserDataRoot: g.UserData, Launch: g.Launch}); err != nil {
			return out, err
		}
		out = append(out, name)
	}
	return out, nil
}

// checkGameNames refuses a batch whose names are invalid, repeat each other or belong to
// an environment already.
func checkGameNames(a *app.App, games []GameFound) error {
	seen := map[string]bool{}
	for _, g := range games {
		if g.Env != "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if err := app.ValidEnvName(name); err != nil {
			return err
		}
		if seen[name] {
			return errors.New(Tf("two games are named %s; give each its own name", name))
		}
		seen[name] = true
		if e, err := a.Store.EnvByName(name); err != nil {
			return err
		} else if e != nil {
			return errors.New(Tf("a game named %s is already added; give this one another name", name))
		}
	}
	return nil
}

// PackImportText says what unpacking a pack did (pack import, 10 §4).
func PackImportText(m *app.PackMatch) string {
	if m == nil || m.Pack == nil {
		return ""
	}
	parts := []string{Tf("pack %s is in the packs folder; play it from the home page", m.Pack.Name)}
	if m.Verified {
		parts = append(parts, T("every mod's files match the pack"))
	} else {
		parts = append(parts, T("files not checked (a pack from an older RimArcho)"))
	}
	// what differs from this game (10 §5); info stays on the pack's page
	if c := PackCheckOf(m.Check); c != nil {
		for _, it := range c.Items {
			if it.Level == finding.Warning || it.Level == finding.Error {
				parts = append(parts, it.Text)
			}
		}
	}
	return strings.Join(parts, "  ")
}
