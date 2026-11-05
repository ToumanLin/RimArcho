package app

import (
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// gameTypeFiles are the game's managed assemblies whose type names are indexed (05 §5.3).
// A save's Class="…" names a Verse.IExposable type, which only Assembly-CSharp and the Mods'
// assemblies can define, and the game looks it up only there and in mscorlib (17 §6). The
// Unity, Steamworks, audio and zip libraries beside them reference no game type.
var gameTypeFiles = []string{store.GameMainAssembly, "Assembly-CSharp-firstpass.dll"}

// indexGameTypes stores the type names of the environment's game assemblies for its game
// version, reading only files whose path, size or modification time changed since the
// last read (or that an older reader read). It runs inside the scan, off the GUI process.
func (a *App) indexGameTypes(e *env.Environment) error {
	mm := env.MajorMinor(e.GameVersion)
	if mm == "" {
		return nil // lookups are per game version; an unknown one keeps what was stored
	}
	have, err := a.Store.GameAssemblies(e.ID)
	if err != nil {
		return err
	}
	dir := env.ManagedDir(e.GameRoot)
	for _, file := range gameTypeFiles {
		var st os.FileInfo
		p := ""
		if dir != "" {
			p = filepath.Join(dir, file)
			st, err = os.Stat(p)
		}
		if dir == "" || err != nil || !st.Mode().IsRegular() {
			if have[file] != nil {
				if err := a.Store.DeleteGameAssembly(e.ID, file); err != nil {
					return err
				}
			}
			continue
		}
		g := &store.GameAssembly{EnvID: e.ID, File: file, MM: mm, Path: p, Size: st.Size(), MTime: st.ModTime().UnixNano(), Reader: analyze.Version}
		if old := have[file]; old != nil && old.Unchanged(g) {
			continue
		}
		var names []string
		if asm, err := dotnet.Open(p); err != nil {
			g.Err = err.Error()
		} else {
			names = asm.TypeNames()
		}
		if err := a.Store.SaveGameAssembly(g, names); err != nil {
			return err
		}
	}
	return nil
}
