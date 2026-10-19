package app

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/store"
)

// modDir is where an instance's files are (links resolved).
func (ix *Index) ModDir(m *store.Mod) string {
	if m.RealPath != "" {
		return m.RealPath
	}
	if s := ix.Source(m.ID); s != nil {
		return filepath.Join(s.Root, m.Rel)
	}
	return m.Rel
}

// AnalyzeInput builds the analysis input from the workspace: the enabled list in order,
// plus every other live, loadable instance (as inactive definers for no-match reasons).
// mm overrides the game version ("" = the environment's).
func (a *App) AnalyzeInput(st *LoadoutState, mm string) (analyze.Input, string, error) {
	if mm == "" {
		mm = st.GameMM()
	}
	in := analyze.Input{MM: mm}
	var all []*store.Mod
	pos := map[string]int{}
	for i, e := range st.List {
		if e.Instance != "" {
			if _, dup := pos[e.Instance]; !dup {
				pos[e.Instance] = i + 1
			}
		}
	}
	for _, m := range st.Ix.Mods {
		if !m.TombstoneAt.IsZero() || m.SnapID == 0 {
			continue
		}
		if pos[m.ID] == 0 && (m.Residue || m.Unloadable != "" || !st.Reg.Loadable(m.ID)) {
			continue
		}
		all = append(all, m)
	}
	if err := a.Store.LoadDecls(all); err != nil {
		return in, "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "v%d|%s\n", analyze.Version, mm)
	for _, m := range all {
		in.Mods = append(in.Mods, &analyze.Mod{
			Instance: m.ID, Lineage: m.LineageID, SnapID: m.SnapID, SnapHash: m.SnapHash,
			Dir: st.Ix.ModDir(m), Pkg: strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"),
			Name: m.DisplayName(), Decl: m.Decl(), Pos: pos[m.ID],
		})
	}
	// The run key: everything the merged document, the simulation and the reasons depend on.
	for _, m := range in.Mods {
		fmt.Fprintf(h, "%s %x %d\n", m.Instance, m.SnapHash, m.Pos)
	}
	return in, fmt.Sprintf("%x", h.Sum(nil))[:24], nil
}
