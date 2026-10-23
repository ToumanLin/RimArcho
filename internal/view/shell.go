package view

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
)

// The window's shell (06 §2.1–§2.3, §2.15): the sidebar, the home page and the first-run
// guide's data. The plans are the environment's (plans.go).

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

// PackLinksText is the title of 能不能跑's item for a pack's links left in Mods/ ("" when
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
		return T("no game")
	}
	if mm := env.MajorMinor(gameVersion); mm != "" {
		return name + " · " + mm
	}
	return name
}

// ---- the sidebar (06 §2.1) ----

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
	PageTextures = "textures" // 贴图优化 (08 §5.2), under 库
)

// NavItem is one sidebar entry. Kids show only under the open entry.
type NavItem struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Badge int       `json:"badge,omitempty"` // 能不能跑: the actionable items (06 §5)
	Key   string    `json:"key,omitempty"`   // shift+1–8
	Kids  []NavItem `json:"kids,omitempty"`
	// Mark is a plan's "有未载入的外部版本" (04 §4.3); Current the plan being edited;
	// InGame the plan the game's ModsConfig.xml holds (a small mark when there are several).
	Mark    string `json:"mark,omitempty"`
	Current bool   `json:"current,omitempty"`
	InGame  bool   `json:"inGame,omitempty"`
	// Add: the entry opens a dialog ("+ 新方案"), not a page.
	Add bool `json:"add,omitempty"`
}

// NavGroup is one of the sidebar's three questions. A group with nothing in it yet is
// left out rather than shown empty.
type NavGroup struct {
	Title string    `json:"title"`
	Items []NavItem `json:"items"`
}

// Nav builds the sidebar: the 在玩 and 库 entries in the shift+1–8 order, every plan and "+ 新方案"
// under 方案, every pack under 整合包. 没玩到 and 值得玩 have no pages yet (10) and are
// left out.
func Nav(needs int, plans []PlanRow, packs []PackRow) []NavGroup {
	var planKids []NavItem
	for _, p := range plans {
		planKids = append(planKids, NavItem{ID: p.Page, Label: p.Name, Mark: p.ConflictText, Current: p.Current, InGame: p.InGame})
	}
	// 管理 heads the lists (06 §2.17); new lists are made there.
	planKids = append([]NavItem{{ID: PagePlans, Label: T("manage")}}, planKids...)
	var packKids []NavItem
	for _, p := range packs {
		packKids = append(packKids, NavItem{ID: "pack:" + p.Name, Label: p.Name})
	}
	// Two groups (06 §2.1): what you play now, and what you have.
	playing := []NavItem{
		{ID: PageHome, Label: T("start")},
		{ID: PageNeeds, Label: T("can it run"), Badge: needs},
		{ID: PageLog, Label: T("game log")},
	}
	library := []NavItem{
		// The entry opens 全部, its first sub-entry (06 §2.9).
		{ID: PageLibrary, Label: T("mod library"), Kids: []NavItem{{ID: PageLibrary, Label: T("all")}, {ID: PageChanges, Label: T("changes")}}},
		{ID: PagePlan, Label: T("lists"), Kids: planKids},
		{ID: PagePacks, Label: T("packs"), Kids: packKids},
		// what the game loads faster: next to the packs, which convert the same way (08 §5.2)
		{ID: PageTextures, Label: T("texture optimisation")},
		{ID: PageEnv, Label: T("game")},
	}
	groups := []NavGroup{{Title: T("playing"), Items: playing}, {Title: T("library"), Items: library}}
	n := 0
	for g := range groups {
		for i := range groups[g].Items {
			n++
			groups[g].Items[i].Key = fmt.Sprintf("shift+%d", n)
		}
	}
	return groups
}

// NavPath is where a page sits in the sidebar: the group, the entry and the sub-entry's
// labels, for the breadcrumb (06 §2.2), with the page each one opens.
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
	// Facts: "312 mods" (and what is not written to the game yet); Mode how its mods
	// change: a plan follows Workshop updates, a pack keeps its versions (08 §5).
	Facts string `json:"facts"`
	Mode  string `json:"mode,omitempty"`
	// Played is "played 10 月 20 日" or "never played".
	Played string `json:"played"`
	// A plan's row: its ID and page, whether it is the one being edited, its
	// "有未载入的外部版本" mark, and whether the game holds it now.
	Plan    int64  `json:"plan,omitempty"`
	Page    string `json:"page,omitempty"`
	Current bool   `json:"current,omitempty"`
	Mark    string `json:"mark,omitempty"`
	InGame  bool   `json:"inGame,omitempty"`
	// Links: the current plan's row when a pack's links are left in Mods/ (08 §5.3), the
	// sentence of its 能不能跑 item; the launch button's tab shows it.
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
	Rows []HomeRow `json:"rows"`
	// LastRun: "the list · 10 月 20 日 19:41" ("" when no run was read: the line is
	// hidden); LastRunDay its day; LastRunErrors its errors in 能不能跑.
	LastRun       string `json:"lastRun,omitempty"`
	LastRunDay    string `json:"lastRunDay,omitempty"`
	LastRunErrors int    `json:"lastRunErrors"`
	// Changes is the current plan's changes since it was last played (06 §2.3): one
	// sentence that opens 变化 with that scope and time.
	Changes *modchanges.Home `json:"changes,omitempty"`
	// Featured is the row of what is applied now (PickFeatured): the large card with
	// ▶ 启动游戏; -1 when there is none.
	Featured int `json:"featured"`
	// Launch is the current plan's most likely trouble (Needs.Launch): the tab under the
	// launch button says it with its reason and opens 能不能跑. It never blocks.
	Launch *LaunchHint `json:"launch,omitempty"`
}

// BuildHome builds the home page: one row per plan of the environment and one per pack.
// The current plan's state counts its needs; another plan's comes from PlanProblems.
// Rows go by the last time they were played (launch records, 08 §2); never played ones
// follow by name.
func BuildHome(a *app.App, b *Board, n *Needs, plans []PlanRow, packs []PackRow) *Home {
	h := &Home{Featured: -1}
	if b == nil || b.St == nil {
		return h
	}
	chip, ver := EnvChip(b.EnvName, b.GameVersion), env.MajorMinor(b.GameVersion)
	for _, p := range plans {
		r := HomeRow{Kind: RowPlan, Name: p.Name, Env: chip, Played: p.Played, at: p.PlayedAt, Plan: p.ID, Page: p.Page,
			Current: p.Current, Mark: p.ConflictText, InGame: p.InGame, Facts: Tf("%d mods", p.Entries), Mode: T("follows Workshop updates"),
			EnvName: b.EnvName, Versions: ver, Mods: p.Entries}
		k, checked := 0, false
		if p.Current {
			r.Mods = len(b.Cols[Active])
			r.Facts = Tf("%d mods", len(b.Cols[Active]))
			if b.Unsaved > 0 {
				r.Facts += " · " + Tf("%d not written", b.Unsaved)
			}
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
		h.Rows = append(h.Rows, packHomeRow(p, chip, b.EnvName))
	}
	SortHome(h.Rows)
	h.LastRun, h.LastRunDay, h.LastRunErrors = n.LastRun, n.LastRunDay, LastRunErrors(n)
	h.Launch = n.Launch
	h.Changes = ChangeHome(a, b.St)
	return h
}

func packHomeRow(p PackRow, chip, envName string) HomeRow {
	r := HomeRow{Kind: RowPack, Name: p.Name, Env: chip, Facts: p.Facts, Played: p.Played, at: p.PlayedAt,
		EnvName: envName, Versions: p.Versions, Mods: p.Mods}
	if p.Mods > 0 {
		r.Facts = Tf("%d mods", p.Mods)
	}
	r.Mode = T("versions locked · own saves")
	// A pack is checked when it is built (08 §5.2): error-level problems refuse it.
	r.State, r.Level = T("checked when created"), HomeOK
	return r
}

// AddOtherEnvs puts the other environments' plans and packs on the home page (06 §2.3):
// it lists what you play everywhere, not only in the current environment. Their state
// comes from PlanProblems like any plan that is not current; nothing is loaded or scanned.
func AddOtherEnvs(a *app.App, h *Home, current string) {
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
				Facts: Tf("%d mods", p.Entries), Mode: T("follows Workshop updates"), EnvName: e.Name, Versions: ver, Mods: p.Entries, OtherEnv: true}
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
				r := packHomeRow(p, chip, e.Name)
				r.OtherEnv = true
				h.Rows = append(h.Rows, r)
			}
		}
	}
	SortHome(h.Rows)
}

// FeaturedKey is the GUI setting that remembers a pack applied on the home page:
// "<environment><pack><the plan current then>". Applying a plan clears it, and it
// lapses when the current plan changes some other way.
const FeaturedKey = "ui.gui.home.pack"

// FeaturedValue is what FeaturedKey holds after applying pack in envName.
func FeaturedValue(envName, pack string, plan int64) string {
	return envName + "" + pack + "" + strconv.FormatInt(plan, 10)
}

// PickFeatured sets h.Featured: the applied pack when it is still valid, else the
// current environment's current plan.
func PickFeatured(h *Home, setting, envName string, plan int64) {
	h.Featured = -1
	if f := strings.Split(setting, ""); len(f) == 3 && f[0] == envName && f[2] == strconv.FormatInt(plan, 10) {
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
			g.Name = freeName(env.MajorMinor(d.GameVersion), taken)
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
	g := &GameFound{Game: path, Version: env.ReadGameVersion(path), Launch: "direct", UserData: a.DefaultUserData()}
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
	var out []string
	for _, g := range games {
		if g.Env != "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		// The Workshop folder of the game's Steam library is added by env add when RimArcho
		// has none yet (02 §0 item 3); naming it here would move an existing one.
		if _, _, err := a.EnvAdd(name, app.EnvSpec{GameRoot: g.Game, UserDataRoot: g.UserData, Launch: g.Launch}); err != nil {
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

// PackImportText says what unpacking a pack did (pack import, 08 §5.4).
func PackImportText(m *app.PackMatch) string {
	if m == nil || m.Pack == nil {
		return ""
	}
	return Tf("pack %s is in the packs folder; play it from the home page", m.Pack.Name)
}
