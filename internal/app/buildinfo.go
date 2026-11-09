package app

import (
	"github.com/ToumanLin/RimArcho/internal/csproj"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// BuildInfo is what a Mod's folder says about building it (05 §5.6): the C# source it ships,
// read as data, and whether the dotnet SDK is on this machine. Nothing is run.
type BuildInfo struct {
	Source *csproj.Source
	Dotnet string // the dotnet executable; empty when there is none
	// Managed is the game's folder of assemblies (Assembly-CSharp.dll), for checking a built
	// DLL's access to game members; empty when unknown.
	Managed string
}

// ModBuild reads the Mod's folder for source. It returns nil when the Mod ships none (a Mod
// with only DLLs), and never writes.
func (a *App) ModBuild(ix *Index, m *store.Mod) *BuildInfo {
	root := a.ModPath(ix, m)
	if root == "" {
		return nil
	}
	s := csproj.Find(root)
	if s == nil {
		return nil
	}
	dotnet := ""
	if a.Dotnet != nil {
		dotnet = a.Dotnet()
	} else {
		dotnet = csproj.Dotnet()
	}
	b := &BuildInfo{Source: s, Dotnet: dotnet}
	if ix.Env != nil {
		b.Managed = env.ManagedDir(ix.Env.GameRoot)
	}
	return b
}
