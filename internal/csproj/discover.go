package csproj

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Limits that keep a read of one Mod's folder short, whatever the Mod ships.
const (
	maxDepth    = 6     // folders below the Mod's root that are searched for source
	maxEntries  = 20000 // directory entries looked at in one search
	maxProjects = 12    // projects read
	maxFiles    = 50000 // files looked at to count one project's .cs files
)

// assetDirs hold the Mod's content, not its source; they are never searched.
var assetDirs = map[string]bool{
	"about": true, "assemblies": true, "textures": true, "sounds": true, "defs": true,
	"patches": true, "languages": true, "news": true, "obj": true, "bin": true,
	"node_modules": true, "packages": true,
}

// LooseDir is a folder of .cs files that no .csproj covers.
type LooseDir struct {
	Dir   string // relative to the Mod's root, slash-separated
	Files int
}

// Source is what a Mod ships to build from.
type Source struct {
	Root      string
	Projects  []*Project
	Solutions []string   // .sln files, relative to Root
	Loose     []LooseDir // set only when there is no project
	Truncated bool       // the search stopped at a limit
}

// Find searches a Mod's folder for C# source: .csproj and .sln files, and .cs files. It returns
// nil when there is none. Asset folders (Textures, Defs, …) are not searched.
func Find(root string) *Source {
	s := &Source{Root: root}
	var projFiles []string
	loose := map[string]int{}
	entries := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		list, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range list {
			entries++
			if entries > maxEntries {
				s.Truncated = true
				return
			}
			name := e.Name()
			low := strings.ToLower(name)
			if e.IsDir() {
				if depth < maxDepth && !assetDirs[low] && !strings.HasPrefix(name, ".") {
					walk(filepath.Join(dir, name), depth+1)
				}
				continue
			}
			switch filepath.Ext(low) {
			case ".csproj":
				projFiles = append(projFiles, filepath.Join(dir, name))
			case ".sln":
				rel, _ := filepath.Rel(root, filepath.Join(dir, name))
				s.Solutions = append(s.Solutions, filepath.ToSlash(rel))
			case ".cs":
				if !strings.HasSuffix(low, ".g.cs") && !strings.HasSuffix(low, ".designer.cs") {
					rel, _ := filepath.Rel(root, dir)
					loose[filepath.ToSlash(rel)]++
				}
			}
		}
	}
	walk(root, 0)
	sort.Strings(projFiles)
	sort.Strings(s.Solutions)
	for i, f := range projFiles {
		if i >= maxProjects {
			s.Truncated = true
			break
		}
		s.Projects = append(s.Projects, Read(f, root))
	}
	if len(projFiles) == 0 {
		for d, n := range loose {
			s.Loose = append(s.Loose, LooseDir{Dir: d, Files: n})
		}
		sort.Slice(s.Loose, func(i, j int) bool {
			if s.Loose[i].Files != s.Loose[j].Files {
				return s.Loose[i].Files > s.Loose[j].Files
			}
			return s.Loose[i].Dir < s.Loose[j].Dir
		})
	}
	if len(s.Projects) == 0 && len(s.Loose) == 0 && len(s.Solutions) == 0 {
		return nil
	}
	return s
}

// Dotnet returns the dotnet executable: ARCHO_DOTNET, PATH, then the standard install folder.
// Empty when there is none; nothing is run.
func Dotnet() string {
	if p := os.Getenv("ARCHO_DOTNET"); p != "" {
		return p
	}
	if p, err := exec.LookPath("dotnet"); err == nil {
		return p
	}
	var dirs []string
	if r := os.Getenv("DOTNET_ROOT"); r != "" {
		dirs = append(dirs, r)
	}
	dirs = append(dirs, `C:\Program Files\dotnet`, "/usr/share/dotnet", "/usr/local/share/dotnet", "/usr/lib/dotnet")
	for _, d := range dirs {
		for _, n := range []string{"dotnet.exe", "dotnet"} {
			p := filepath.Join(d, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// glob matches MSBuild item patterns (*, ?, **) against absolute, slash-separated paths.
type glob struct {
	re   *regexp.Regexp
	base string // the folder to walk: the pattern up to its first wildcard
	lit  string // the file itself when the pattern has no wildcard
}

func newGlob(dir, pat string) *glob {
	pat = strings.ReplaceAll(pat, `\`, "/")
	abs := filepath.ToSlash(filepath.Clean(filepath.Join(dir, filepath.FromSlash(pat))))
	i := strings.IndexAny(abs, "*?")
	if i < 0 {
		return &glob{lit: abs}
	}
	base := abs[:strings.LastIndex(abs[:i], "/")+1]
	var sb strings.Builder
	sb.WriteString("(?i)^")
	for j := 0; j < len(abs); j++ {
		switch c := abs[j]; {
		case c == '*' && j+1 < len(abs) && abs[j+1] == '*':
			j++
			if j+1 < len(abs) && abs[j+1] == '/' {
				j++
				sb.WriteString("(?:.*/)?")
			} else {
				sb.WriteString(".*")
			}
		case c == '*':
			sb.WriteString("[^/]*")
		case c == '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return &glob{re: regexp.MustCompile(sb.String()), base: filepath.FromSlash(base)}
}

func (g *glob) match(slash string) bool {
	if g.re == nil {
		return strings.EqualFold(g.lit, slash)
	}
	return g.re.MatchString(slash)
}

// countSources counts the .cs files the project compiles: every .cs under its folder unless
// the project lists them (old style, or EnableDefaultCompileItems=false), plus listed ones,
// minus Compile Remove.
func (p *Project) countSources(e *eval) int {
	set := map[string]bool{}
	visited := 0
	collect := func(g *glob) {
		if g.re == nil {
			if st, err := os.Stat(filepath.FromSlash(g.lit)); err == nil && !st.IsDir() && strings.EqualFold(filepath.Ext(g.lit), ".cs") {
				set[strings.ToLower(g.lit)] = true
			}
			return
		}
		walkCS(g.base, &visited, func(path string) {
			if s := filepath.ToSlash(path); g.match(s) {
				set[strings.ToLower(s)] = true
			}
		})
	}
	explicit := p.Legacy || strings.EqualFold(e.get("EnableDefaultCompileItems"), "false")
	if !explicit {
		self := strings.ToLower(p.File)
		walkCSIn(p.Dir, self, &visited, func(path string) {
			set[strings.ToLower(filepath.ToSlash(path))] = true
		})
	}
	for _, inc := range e.compInc {
		for _, one := range strings.Split(inc, ";") {
			if one = strings.TrimSpace(one); one != "" {
				collect(newGlob(p.Dir, one))
			}
		}
	}
	for _, rm := range e.compRem {
		for _, one := range strings.Split(rm, ";") {
			if one = strings.TrimSpace(one); one == "" {
				continue
			}
			g := newGlob(p.Dir, one)
			for k := range set {
				if g.match(k) {
					delete(set, k)
				}
			}
		}
	}
	return len(set)
}

// walkCS visits the .cs files under dir (bin, obj and dot-folders skipped).
func walkCS(dir string, visited *int, fn func(string)) {
	walkCSIn(dir, "", visited, fn)
}

// walkCSIn is walkCS that does not enter a folder holding a .csproj other than self (a
// project of its own).
func walkCSIn(dir, self string, visited *int, fn func(string)) {
	list, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range list {
		*visited++
		if *visited > maxFiles {
			return
		}
		name := e.Name()
		if e.IsDir() {
			low := strings.ToLower(name)
			if low == "bin" || low == "obj" || strings.HasPrefix(name, ".") {
				continue
			}
			sub := filepath.Join(dir, name)
			if self != "" && hasOtherProject(sub, self) {
				continue
			}
			walkCSIn(sub, self, visited, fn)
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".cs") {
			fn(filepath.Join(dir, name))
		}
	}
}

func hasOtherProject(dir, self string) bool {
	list, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range list {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".csproj") &&
			strings.ToLower(filepath.Join(dir, e.Name())) != self {
			return true
		}
	}
	return false
}
