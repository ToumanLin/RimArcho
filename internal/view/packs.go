package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
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
		size := mbText(p.Bytes)
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

// PackPreview is what building a pack from the current list would do.
type PackPreview struct {
	Lines []string `json:"lines"`
	Ok    bool     `json:"ok"`
}

// PreviewPack runs a dry-run build and says what it found.
func PreviewPack(a *app.App, name string) (*PackPreview, error) {
	r, err := a.BuildPack(name, false, true)
	if err != nil {
		return nil, err
	}
	pv := &PackPreview{}
	if len(r.Blockers) > 0 {
		pv.Lines = append(pv.Lines, Tf("%d things need fixing first (需要处理):", len(r.Blockers)))
		for i, b := range r.Blockers {
			if i == 8 {
				pv.Lines = append(pv.Lines, Tf("… %d more", len(r.Blockers)-8))
				break
			}
			if pkg, ok := strings.CutPrefix(b, "not installed: pkg:"); ok {
				b = Tf("the list has %s, but it is not installed", pkg)
			} else if st, err := a.Loadout(); err == nil {
				b = Humanize(st, b)
			}
			pv.Lines = append(pv.Lines, "· "+b)
		}
		return pv, nil
	}
	carried := 0
	var bytes int64
	for _, e := range r.Pack.Entries {
		if !e.Official {
			carried++
			bytes += e.Bytes
		}
	}
	pv.Ok = true
	pv.Lines = append(pv.Lines,
		Tf("%d mods are copied (%s); Core and the DLCs come from the game", carried, mbText(bytes)),
		Tf("%s of folders for other game versions are left out", mbText(r.Pack.Trimmed)),
		Tf("the pack is sorted on its own (%d moved); your list does not change", r.Moved),
		fmt.Sprintf("%s: %s", T("folder"), r.Dir))
	return pv, nil
}
