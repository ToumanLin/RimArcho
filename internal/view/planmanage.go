package view

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The list management page (06 §2.17): every list of the game, and for the one picked its
// game versions (04 §1.6), the Steam collections it follows (04 §9.6) and how to share it.
// Reading the page never uses the network; CheckPlanFollows does, when the page asks.

// PagePlans is the sidebar's 列表 › 列表管理.
const PagePlans = "plans"

// PlanManage is the page.
type PlanManage struct {
	Lead string          `json:"lead"`
	Rows []PlanManageRow `json:"rows"`
	Sel  *PlanDetail     `json:"sel"`
}

// PlanManageRow is one list in the left column.
type PlanManageRow struct {
	ID      int64  `json:"id"`
	Page    string `json:"page"`
	Name    string `json:"name"`
	Current bool   `json:"current,omitempty"`
	InGame  bool   `json:"inGame,omitempty"`
	Mark    string `json:"mark,omitempty"` // an outside change not used yet
	Meta    string `json:"meta"`           // "1.5–1.7  412 mods  played 10 月 21 日"
	Follows int    `json:"follows,omitempty"`
	Changed bool   `json:"changed,omitempty"` // a followed collection changed
}

// VersionChip is one game version button.
type VersionChip struct {
	V    string `json:"v"`
	On   bool   `json:"on,omitempty"`
	Game bool   `json:"game,omitempty"` // the game now
	Tip  string `json:"tip"`
}

// FollowRow is one followed collection.
type FollowRow struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Items        int      `json:"items"`
	Meta         string   `json:"meta"` // "1.5–1.7  412 mods  played 10 月 21 日"
	NotInstalled int      `json:"notInstalled,omitempty"`
	Missing      []string `json:"missing,omitempty"` // their Workshop links
	Changes      string   `json:"changes,omitempty"` // "3 new, 1 dropped"; "" unchanged
	Added        []string `json:"added,omitempty"`
	Removed      []string `json:"removed,omitempty"`
	Mode         string   `json:"mode"`
	Link         string   `json:"link"`
	Checked      bool     `json:"checked,omitempty"`
}

// PlanDetail is the right column.
type PlanDetail struct {
	ID       int64         `json:"id"`
	Page     string        `json:"page"`
	Name     string        `json:"name"`
	Current  bool          `json:"current,omitempty"`
	InGame   string        `json:"inGame,omitempty"` // "in the game now: v18"
	Mark     string        `json:"mark,omitempty"`
	Facts    string        `json:"facts"`
	Versions []VersionChip `json:"versions"`
	Custom   bool          `json:"custom,omitempty"`
	Default  string        `json:"default"` // the default, for the reset button's tooltip
	Allowed  string        `json:"allowed"`
	Follows  []FollowRow   `json:"follows"`
	Pack     *PackRow      `json:"pack,omitempty"` // the newest pack created from it
	// Stale: a followed collection has not been read for CheckEvery (the page reads it).
	Stale bool `json:"stale,omitempty"`
}

func workshopLink(id string) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + id
}

// whenText is "today 14:02", "yesterday 14:02" or "10 月 21 日".
func whenText(t time.Time) string {
	t = t.Local()
	now := time.Now()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return Tf("today %s", t.Format("15:04"))
	case y1 == y2 && m1 == m2 && d1+1 == d2:
		return Tf("yesterday %s", t.Format("15:04"))
	}
	return ShortDate(t)
}

// BuildPlanManage builds the page with sel picked (0, or a list that is gone: the current
// one).
func BuildPlanManage(a *app.App, sel int64) (*PlanManage, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	rows, err := PlanRows(a)
	if err != nil {
		return nil, err
	}
	pg := &PlanManage{Lead: Tf("%s has %d lists", st.Ix.Env.Name, len(rows))}
	if len(rows) == 1 {
		pg.Lead = Tf("%s has 1 list", st.Ix.Env.Name)
	}
	var selRow *PlanRow
	for i := range rows {
		r := &rows[i]
		p, err := a.Store.PlanByID(r.ID)
		if err != nil || p == nil {
			continue
		}
		mr := PlanManageRow{ID: r.ID, Page: r.Page, Name: r.Name, Current: r.Current, InGame: r.InGame, Mark: r.ConflictText}
		vs := app.FormatVersions(a.PlanAllowedVersions(st, p))
		mr.Meta = vs + "  " + Tf("%d mods", r.Entries) + "  " + r.Played
		if fs, err := a.PlanFollowed(st, p); err == nil {
			mr.Follows = len(fs)
			for _, f := range fs {
				if f.Changed() {
					mr.Changed = true
				}
			}
		}
		pg.Rows = append(pg.Rows, mr)
		if r.ID == sel || (selRow == nil && r.Current) {
			selRow = r
		}
	}
	if selRow == nil && len(rows) > 0 {
		selRow = &rows[0]
	}
	if selRow != nil {
		pg.Sel, err = planDetail(a, st, selRow)
	}
	return pg, err
}

func planDetail(a *app.App, st *app.LoadoutState, r *PlanRow) (*PlanDetail, error) {
	p, err := a.Store.PlanByID(r.ID)
	if err != nil || p == nil {
		return nil, errors.New(T("that list is gone"))
	}
	d := &PlanDetail{ID: p.ID, Page: r.Page, Name: p.Name, Current: r.Current, Mark: r.ConflictText}
	if r.InGame {
		if g := InGame(a); g != nil && g.Version > 0 {
			d.InGame = Tf("in the game now: v%d", g.Version)
		} else {
			d.InGame = T("in the game")
		}
	}
	facts := []string{Tf("%d mods", r.Entries)}
	if !r.PlayedAt.IsZero() {
		facts = append(facts, Tf("last played %s", ShortDate(r.PlayedAt)))
	} else {
		facts = append(facts, T("never played"))
	}
	made := Tf("created %s", ShortDate(p.Created))
	if vs, err := a.Store.Versions(p.ID); err == nil && len(vs) > 0 {
		if n := versionNote(vs[len(vs)-1]); n != "" {
			made += "  " + n
		}
	}
	d.Facts = strings.Join(append(facts, made), "  ")

	pv, err := a.PlanVersionsOf(st, p)
	if err != nil {
		return nil, err
	}
	d.Custom, d.Default, d.Allowed = pv.Custom, app.FormatVersions(pv.Default), app.FormatVersions(pv.Allowed)
	for _, v := range pv.Shown {
		c := VersionChip{V: v, On: slices.Contains(pv.Allowed, v), Game: v == pv.Game}
		if n := pv.Missing[v]; n > 0 {
			c.Tip = Tf("%d mods do not list %s", n, v)
		} else if pv.Muted[v] > 0 {
			c.Tip = T("no version warnings")
		} else {
			c.Tip = Tf("every mod lists %s", v)
		}
		if n := pv.Muted[v]; n > 0 {
			c.Tip += "  " + Tf("%d mods muted", n)
		}
		if c.Game {
			c.Tip = T("the game now") + "  " + c.Tip
		}
		d.Versions = append(d.Versions, c)
	}

	fs, err := a.PlanFollowed(st, p)
	if err != nil {
		return nil, err
	}
	d.Follows = []FollowRow{}
	for _, f := range fs {
		fr := FollowRow{ID: f.WorkshopID, Title: f.Title, Items: len(f.Items()), Mode: f.Mode, Link: workshopLink(f.WorkshopID), NotInstalled: len(f.NotInstalled), Checked: !f.Checked.IsZero()}
		if f.Checked.IsZero() {
			fr.Meta = T("not checked yet")
		} else {
			fr.Meta = Tf("checked %s", whenText(f.Checked))
		}
		if f.Checked.IsZero() || time.Since(f.Checked) >= app.CheckEvery {
			d.Stale = true
		}
		for _, it := range f.NotInstalled {
			fr.Missing = append(fr.Missing, workshopLink(it.WorkshopID))
		}
		var parts []string
		if n := len(f.Added); n > 0 {
			parts = append(parts, Tf("%d new", n))
		}
		if n := len(f.Removed); n > 0 {
			parts = append(parts, Tf("%d dropped", n))
		}
		fr.Changes = strings.Join(parts, T(", "))
		for _, it := range f.Added {
			fr.Added = append(fr.Added, itemTitle(it))
		}
		for _, it := range f.Removed {
			fr.Removed = append(fr.Removed, itemTitle(it))
		}
		d.Follows = append(d.Follows, fr)
	}

	if ps, err := a.Packs(); err == nil {
		var newest *app.PackInfo
		for _, pk := range ps {
			if pk.PlanID == p.ID && pk.Env == st.Ix.Env.Name && (newest == nil || pk.Created.After(newest.Created)) {
				newest = pk
			}
		}
		if newest != nil {
			rs := packRows(a, []*app.PackInfo{newest})
			d.Pack = &rs[0]
			d.Pack.Facts = Tf("%s  created %s", SizeText(newest.Bytes), ShortDate(newest.Created))
		}
	}
	return d, nil
}

func itemTitle(it store.CollectionItem) string {
	if it.Title != "" {
		return it.Title
	}
	return "ws:" + it.WorkshopID
}

// ---- verbs ----

// SetPlanVersions sets a list's game versions; none goes back to the default.
func SetPlanVersions(a *app.App, id int64, versions []string) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		versions = []string{"default"}
	}
	r, err := a.SetPlanVersions(p.Name, versions)
	if err != nil {
		return "", err
	}
	if !r.After.Custom {
		return Tf("%s: game versions %s (default)", p.Name, app.FormatVersions(r.After.Allowed)), nil
	}
	return Tf("%s: game versions %s", p.Name, app.FormatVersions(r.After.Allowed)), nil
}

// FollowPlan links a collection to a list (reads it once over the network).
func FollowPlan(a *app.App, id int64, link, mode string) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	f, err := a.FollowCollection(p.Name, link, mode)
	if err != nil {
		return "", errors.New(Humanize(nil, err.Error()))
	}
	return Tf("%s follows %s; the list is unchanged", p.Name, f.Title), nil
}

// SetFollowMode changes how catching up treats a collection.
func SetFollowMode(a *app.App, id int64, ws, mode string) error {
	p, err := planByID(a, id)
	if err != nil {
		return err
	}
	_, err = a.FollowCollection(p.Name, ws, mode)
	return err
}

// UnfollowPlan unlinks a collection; the list stays as it is.
func UnfollowPlan(a *app.App, id int64, ws string) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	c, err := a.UnfollowCollection(p.Name, ws)
	if err != nil {
		return "", err
	}
	return Tf("%s no longer follows %s; the list is unchanged", p.Name, c.Title), nil
}

// SyncPlan catches a list up with a collection: one undoable edit of that list.
func SyncPlan(a *app.App, id int64, ws string) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	r, err := a.SyncCollection(p.Name, ws, false)
	if err != nil {
		return "", errors.New(Humanize(nil, err.Error()))
	}
	t := Tf("%s caught up with %s: %d enabled", p.Name, r.Collection.Title, len(r.Enabled))
	if len(r.Disabled) > 0 {
		t += ", " + Tf("%d disabled", len(r.Disabled))
	}
	if len(r.NotInstalled) > 0 {
		t += "  " + Tf("%d not installed", len(r.NotInstalled))
	}
	return t, nil
}

// CheckPlanFollows reads a list's followed collections again when they are older than
// CheckEvery (the network); it says whether anything was read.
func CheckPlanFollows(a *app.App, id int64) (bool, error) {
	p, err := planByID(a, id)
	if err != nil {
		return false, err
	}
	st, err := a.Loadout()
	if err != nil {
		return false, err
	}
	fs, err := a.PlanFollowed(st, p)
	if err != nil {
		return false, err
	}
	stale := false
	for _, f := range fs {
		if f.Checked.IsZero() || time.Since(f.Checked) >= app.CheckEvery {
			stale = true
		}
	}
	if !stale {
		return false, nil
	}
	_, err = a.CheckFollowed(p.Name, "", false)
	if err != nil {
		return true, errors.New(Humanize(nil, err.Error()))
	}
	return true, nil
}

// ---- the export sub-page ----

// PlanExport is the export sub-page of a list.
type PlanExport struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Page    string         `json:"page"`
	Lead    string         `json:"lead"`
	Facts   string         `json:"facts"` // "412 mods  1.5–1.7"
	Formats []ExportFormat `json:"formats"`
	Format  string         `json:"format"`
	Text    string         `json:"text"`
}

// BuildPlanExport renders a list in a format for the export sub-page.
func BuildPlanExport(a *app.App, id int64, format string) (*PlanExport, error) {
	p, err := planByID(a, id)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	fs := ExportFormats(a)
	ok := false
	for _, f := range fs {
		ok = ok || f.ID == format
	}
	if !ok {
		format = fs[0].ID
	}
	b, err := a.ExportPlan(id, format)
	if err != nil {
		return nil, err
	}
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	return &PlanExport{ID: id, Name: p.Name, Page: PlanPage(id), Format: format, Formats: fs, Text: string(b),
		Facts: Tf("%d mods", len(ps.List)) + "  " + app.FormatVersions(a.PlanAllowedVersions(st, p))}, nil
}

// ExportPlanTo writes a list to a file (as ExportTo does for the current list).
func ExportPlanTo(a *app.App, id int64, format, path string, overwrite bool) (string, error) {
	p, err := planByID(a, id)
	if err != nil {
		return "", err
	}
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return "", errors.New(T("no file chosen"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if st, err := a.Loadout(); err == nil && st.FilePath != "" {
		if game, err := filepath.Abs(st.FilePath); err == nil && strings.EqualFold(game, abs) {
			return "", errors.New(T("that is the game's own ModsConfig.xml: save instead (with a preview and a version)"))
		}
	}
	if fi, err := os.Stat(abs); err == nil {
		if fi.IsDir() {
			return "", fmt.Errorf("%s: %s", abs, T("is a folder"))
		}
		if !overwrite {
			return "", fmt.Errorf("%s: %s", abs, T("exists; choose another name (nothing is overwritten)"))
		}
	}
	b, err := a.ExportPlan(id, format)
	if err != nil {
		return "", err
	}
	if !dirExists(filepath.Dir(abs)) {
		return "", fmt.Errorf("%s: %s", filepath.Dir(abs), T("no such folder"))
	}
	if err := os.WriteFile(abs, b, 0o644); err != nil {
		return "", err
	}
	name := format
	for _, f := range ExportFormats(a) {
		if f.ID == format {
			name = f.Name
		}
	}
	return Tf("wrote %s (%s, list %s) to %s", filepath.Base(abs), name, p.Name, abs), nil
}

// ExportPlanText renders a list for copying.
func ExportPlanText(a *app.App, id int64, format string) (string, error) {
	b, err := a.ExportPlan(id, format)
	return string(b), err
}
