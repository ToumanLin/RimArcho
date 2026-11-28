package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ScanModHarmony reads the Harmony layer of one Mod's effective assemblies now, for a Mod
// the analysis did not cover (outside the current list, never analysed for this game
// version): `harmony --mod` answers "what does it patch" without adding it (05 §5.3). Its
// load folders are resolved as if it were enabled in the current list. Nothing is stored.
// It stops reading once budget has passed; pending counts the assemblies left.
func (a *App) ScanModHarmony(st *LoadoutState, m *store.Mod, budget time.Duration) (asms []*store.AssemblyRow, rows []*store.HarmonyRow, pending int, err error) {
	if err := a.Store.LoadDecls([]*store.Mod{m}); err != nil {
		return nil, nil, 0, err
	}
	active := map[string]bool{strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam"): true}
	for _, e := range st.List {
		if x := st.Ix.ByID[e.Instance]; x != nil {
			active[strings.TrimSuffix(strings.ToLower(x.PackageID), "_steam")] = true
		}
	}
	dir := st.Ix.ModDir(m)
	r := content.Resolve(dir, m.Decl().LoadFolders, st.GameMM(), func(p string) bool { return active[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
	deadline := time.Now().Add(budget)
	files := content.CodeFiles(dir, r.Folders)
	for i, as := range files {
		if budget > 0 && time.Now().After(deadline) {
			return asms, rows, len(files) - i, nil
		}
		skipped := harmony.SkippedFor(filepath.Base(as.Path), m.PackageID, harmony.DefaultSkip)
		row := &store.AssemblyRow{Instance: m.ID, File: as.Rel, Skipped: skipped}
		h, err := harmony.Read(as.Path, skipped)
		if err != nil {
			row.Err = err.Error()
			asms = append(asms, row)
			continue
		}
		row.Manual, row.UsesHm, row.Types, row.Patches = h.Manual, h.UsesHarm, len(h.Types), len(h.Patches)
		asms = append(asms, row)
		for _, p := range h.Patches {
			rows = append(rows, &store.HarmonyRow{Instance: m.ID, File: as.Rel, PatchType: p.PatchType, PatchMethod: p.PatchMethod,
				Kind: p.Kind, Target: p.Target(), Skips: p.Skips, Dynamic: p.Dynamic, Manual: p.Manual, FromTargetMethod: p.FromTargetMethod})
		}
	}
	return asms, rows, 0, nil
}
