package app

import (
	"github.com/ToumanLin/RimArcho/internal/csproj"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// BuildInfo is what a Mod's folder says about building it (05 §5.6): the C# source it ships,
// read as data, and whether the dotnet SDK is on this machine. Nothing is run.
type BuildInfo struct {
	Source *csproj.Source
	Dotnet string // the dotnet executable; empty when there is none
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
	return &BuildInfo{Source: s, Dotnet: dotnet}
}
