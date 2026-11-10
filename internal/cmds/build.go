package cmds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/csproj"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Package names a build summary explains; anything else is listed as it is.
var packageNotes = map[string]string{
	"krafs.rimworld.ref": "the game's reference assemblies",
	"lib.harmony":        "Harmony",
	"lib.harmony.ref":    "Harmony",
	"microsoft.netframework.referenceassemblies": ".NET Framework reference assemblies",
}

const maxBuildProjects = 3

// buildBlock adds the "build:" block of a Mod that ships C# source (05 §5.6): where the source
// is, what the project needs and the commands that rebuild it. Read from the files; nothing is run.
func buildBlock(c *Ctx, root *render.Node, modRoot string, b *app.BuildInfo) {
	s := b.Source
	if len(s.Projects) == 0 {
		blk := root.KV("build", "C# source without a project file: nothing to build with dotnet as it is")
		for i, l := range s.Loose {
			if i == 3 {
				blk.Addf("… %d more folders", len(s.Loose)-3)
				break
			}
			blk.Addf("%d .cs files in %s", l.Files, relDir(l.Dir))
		}
		for _, sl := range s.Solutions {
			blk.Addf("solution without projects: %s", sl)
		}
		blk.Add("no .csproj in the Mod: a build needs one written first; for a small fix a separate Harmony patch Mod is simpler")
		helpLine(c, blk)
		return
	}
	total := 0
	for _, p := range s.Projects {
		total += p.Sources
	}
	head := fmt.Sprintf("the Mod ships source: %d project", len(s.Projects))
	if len(s.Projects) != 1 {
		head += "s"
	}
	head += fmt.Sprintf(", %d .cs files", total)
	if s.Truncated {
		head += " (the search stopped at its limit)"
	}
	blk := root.KV("build", head)
	for _, sl := range s.Solutions {
		blk.Addf("solution: %s", sl)
	}
	for i, p := range s.Projects {
		if i == maxBuildProjects {
			blk.Addf("… %d more projects (read their .csproj files)", len(s.Projects)-maxBuildProjects)
			break
		}
		projectBlock(blk, modRoot, p, b.Managed)
	}
	if b.Dotnet != "" {
		blk.KV("dotnet", b.Dotnet)
	} else {
		blk.KV("dotnet", "not found on PATH: building needs the .NET SDK (RimArcho installs nothing; ARCHO_DOTNET names a dotnet elsewhere)")
	}
	helpLine(c, blk)
}

func helpLine(c *Ctx, blk *render.Node) {
	if IsMCP(c.Context) {
		blk.KV("help", "guide topic=rebuild")
	} else {
		blk.KV("help", "archo guide rebuild")
	}
}

func relDir(d string) string {
	if d == "." {
		return "the Mod's folder"
	}
	return d + "/"
}

func projectBlock(blk *render.Node, modRoot string, p *csproj.Project, managed string) {
	rel, err := filepath.Rel(modRoot, p.File)
	if err != nil {
		rel = p.File
	}
	rel = filepath.ToSlash(rel)
	if p.Err != "" {
		blk.Addf("project: %s: %s", rel, p.Err)
		return
	}
	line := fmt.Sprintf("project: %s  %d .cs files", rel, p.Sources)
	if p.Legacy {
		line += "  old-style project (no Sdk attribute)"
	} else if p.SDK != "" {
		line += "  " + p.SDK
	}
	if len(p.Frameworks) > 0 {
		line += "  " + strings.Join(p.Frameworks, ";")
	}
	n := blk.Add(line)
	if len(p.Packages) > 0 {
		var ps []string
		for _, pk := range p.Packages {
			s := pk.Name
			if pk.Version != "" {
				s += " " + pk.Version
			}
			if note := packageNotes[strings.ToLower(pk.Name)]; note != "" {
				s += " (" + note + ")"
			}
			ps = append(ps, s)
		}
		n.KV("packages", strings.Join(ps, ", "))
	}
	if p.Publicizer {
		what := p.PublicizerPkg
		if what == "" {
			what = "a Reference marked Publicize"
		}
		var items string
		switch {
		case p.PublicizeAll:
			items = "every referenced assembly"
		case len(p.Publicize) > 0:
			items = strings.Join(firstN(p.Publicize, 4), ", ")
			if len(p.Publicize) > 4 {
				items += fmt.Sprintf(" and %d more", len(p.Publicize)-4)
			}
		default:
			items = "(no Publicize item found in the project)"
		}
		n.KV("publicizer", fmt.Sprintf("%s makes private game members reachable: %s. Build it with dotnet build; compiling it with csc fails at run time with FieldAccessException", what, items))
	}
	var missing []string
	for _, r := range p.References {
		if r.Missing {
			missing = append(missing, r.Name+" at "+r.HintPath)
		}
	}
	if len(missing) > 0 {
		n.KV("references", "not found: "+strings.Join(firstN(missing, 3), "; ")+" (HintPath is relative to the project; a copy outside the game folder needs it changed)")
	}
	if len(p.Unresolved) > 0 {
		var ps []string
		for _, u := range p.Unresolved {
			ps = append(ps, "$("+u+")")
		}
		n.KV("unresolved", strings.Join(ps, ", ")+" (set outside the project: an environment variable or dotnet build -p:Name=value)")
	}
	n.KV("output", outputLine(p))
	if l := builtAccessLine(p, managed); l != "" {
		n.KV("built", l)
	}
	if len(p.Inherited) > 0 {
		n.KV("also read", strings.Join(firstN(p.Inherited, 3), ", "))
	}
	if p.PackagesConfig {
		n.Add("packages.config: dotnet restore does not read it; restore with nuget.exe (nuget restore)")
	}
	if p.Legacy {
		n.Add("old-style project: dotnet build may refuse it; msbuild from Visual Studio Build Tools builds it")
	}
	n.KV("run in", p.Dir)
	n.Add("restore: dotnet restore")
	n.Add("build: dotnet build -c Release")
}

func outputLine(p *csproj.Project) string {
	out := p.OutputAbs
	if out == "" {
		out = p.Output + " (not fully known)"
	}
	s := out
	switch {
	case p.OutputDefault:
		s += " (the default; set OutputPath to build into the Mod's Assemblies folder)"
	case p.TFMAppended:
		s += " (the SDK adds the " + strings.ToLower(p.Frameworks[0]) + " folder to OutputPath)"
	}
	if p.OutputAbs != "" {
		if st, err := os.Stat(filepath.Join(p.OutputAbs, p.DLL())); err == nil {
			s += fmt.Sprintf("; %s there now: %s", p.DLL(), render.Size(st.Size()))
		} else {
			s += fmt.Sprintf("; no %s there yet", p.DLL())
		}
	}
	return s
}

func firstN(l []string, n int) []string {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// rebuildHint ends a line that finds a DLL the game will refuse (05 §5.4, §5.6).
const rebuildHint = "Build it with dotnet build and the project's publicizer (guide topic=rebuild), not csc"

// builtAccessLine checks the DLL at the project's output: one that uses non-public game members
// without skipping verification loads, then throws FieldAccessException or MethodAccessException
// in the game (05 §5.6). Empty when there is no DLL or nothing to report.
func builtAccessLine(p *csproj.Project, managed string) string {
	if p.OutputAbs == "" {
		return ""
	}
	path := filepath.Join(p.OutputAbs, p.DLL())
	a, err := dotnet.Open(path)
	if err != nil || a.SkipsVerification() {
		return ""
	}
	refs := a.InaccessibleRefs(asmOpener(p.OutputAbs, managed))
	if len(refs) == 0 {
		return ""
	}
	var names []string
	for _, r := range firstN2(refs, 3) {
		names = append(names, r.Text)
	}
	more := ""
	if len(refs) > 3 {
		more = fmt.Sprintf(" and %d more", len(refs)-3)
	}
	return fmt.Sprintf("!! %s uses %d non-public game %s (%s%s) and does not skip verification: the game throws FieldAccessException / MethodAccessException when it runs that code. %s",
		p.DLL(), len(refs), plural(len(refs), "member", "members"), strings.Join(names, ", "), more, rebuildHint)
}

func firstN2(l []dotnet.RefDiff, n int) []dotnet.RefDiff {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// asmOpener opens a referenced assembly by name from the first folder that has it; nil when
// none does. Results are kept.
func asmOpener(dirs ...string) func(string) *dotnet.Assembly {
	opened := map[string]*dotnet.Assembly{}
	return func(name string) *dotnet.Assembly {
		if x, ok := opened[name]; ok {
			return x
		}
		var x *dotnet.Assembly
		for _, dir := range dirs {
			if dir == "" {
				continue
			}
			if y, err := dotnet.Open(filepath.Join(dir, name+".dll")); err == nil {
				x = y
				break
			}
		}
		opened[name] = x
		return x
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
