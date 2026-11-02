// Package deps locates RimArcho's external tools without downloading or changing them.
// Finder uses explicit settings first and preserves each tool's discovery order.
package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/textures"
)

const (
	Todds    = "todds"
	SteamCMD = "steamcmd"
)

// Names is the display order used by All.
var Names = []string{Todds, SteamCMD}

// Info describes the selected copy. Own means it is in RimArcho's download directory.
// Dir is always the managed installation directory, even when another copy is selected.
type Info struct {
	// Source is config, own, path, or empty when missing.
	Name, Path, Version, Source string
	OK, Own                     bool
	Err, ConfigKey, Dir         string
	Installable                 bool
}

// Finder's paths and callbacks describe the invoking application. A nil LookPath
// disables PATH discovery. Version overrides version probes for tests.
// Do not change fields concurrently with calls to its methods.
type Finder struct {
	LocalDir string
	Setting  func(key string) string
	LookPath func(string) (string, error)
	Version  func(name, path string) (string, error)
	probes   probeCache
}

type probeCache struct {
	mu      sync.Mutex
	entries map[string]probe
}

// Real probes are shared by short-lived page readers. Test overrides stay local
// to the Finder so one fake executable cannot affect another test or application.
var realProbes probeCache

type probe struct {
	size, mtime int64
	version     string
	err         error
}

func isFile(path string) bool {
	s, err := os.Stat(path)
	return err == nil && !s.IsDir()
}

// SteamCMDIn finds the executable in a SteamCMD installation directory.
func SteamCMDIn(dir string) string {
	for _, n := range []string{"steamcmd.exe", "steamcmd.sh", "steamcmd"} {
		if p := filepath.Join(dir, n); isFile(p) {
			return p
		}
	}
	return ""
}

// Status selects a copy and checks it. A found but incompatible todds retains
// its Path and Version, with OK false and an explanation in Err.
func (f *Finder) Status(name string) Info {
	i := Info{Name: name}
	setting := func(key string) string {
		if f.Setting != nil {
			return f.Setting(key)
		}
		return ""
	}
	path := func(program string) string {
		if f.LookPath != nil {
			p, err := f.LookPath(program)
			if err == nil {
				return p
			}
		}
		return ""
	}
	selectPath := func(p, src string) bool {
		if p == "" {
			return false
		}
		i.Path, i.Source = p, src
		return true
	}
	switch name {
	case Todds:
		i.ConfigKey, i.Dir, i.Installable = "textures.todds", filepath.Join(f.LocalDir, Todds), true
		if !selectPath(textures.FindTodds(setting(i.ConfigKey)), "config") &&
			!selectPath(textures.FindTodds(i.Dir), "own") {
			selectPath(path("todds"), "path")
		}
	case SteamCMD:
		i.ConfigKey, i.Dir, i.Installable = "steamcmd.path", filepath.Join(f.LocalDir, SteamCMD), true
		p := setting(i.ConfigKey)
		if isFile(p) {
			selectPath(p, "config")
		} else {
			// Historically a configured program's directory is searched before PATH,
			// even when that particular executable has gone away.
			if p != "" {
				selectPath(SteamCMDIn(filepath.Dir(p)), "config")
			} else {
				selectPath(SteamCMDIn(i.Dir), "own")
			}
			if i.Path == "" {
				selectPath(path("steamcmd"), "path")
			}
		}
	default:
		i.Err = fmt.Sprintf("unknown dependency %q", name)
		return i
	}
	if i.Path == "" {
		i.Err = name + " is not installed"
		return i
	}
	i.Own = i.Source == "own" && owned(i.Dir, i.Path)
	i.OK = true
	if name == Todds {
		v, err := f.version(name, i.Path)
		i.Version = v
		if err != nil {
			i.OK, i.Err = false, err.Error()
		} else if name == Todds && v != textures.ToddsVersion {
			i.OK = false
			i.Err = fmt.Sprintf("todds %s found at %s; RimArcho works with %s", v, i.Path, textures.ToddsVersion)
		}
	}
	return i
}

// owned resolves links so a configured copy outside the managed directory is never removable.
func owned(dir, path string) bool {
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(dir))
	if err != nil {
		return false
	}
	rootRel, err := filepath.Rel(filepath.Join(parent, filepath.Base(dir)), d)
	if err != nil || rootRel != "." {
		return false
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	r, err := filepath.Rel(d, p)
	return err == nil && r != "." && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

func (f *Finder) version(name, path string) (string, error) {
	s, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	c := &realProbes
	if f.Version != nil {
		c = &f.probes
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]probe)
	}
	key := name + ":" + path
	p, ok := c.entries[key]
	if ok && p.size == s.Size() && p.mtime == s.ModTime().UnixNano() {
		return p.version, p.err
	}
	p = probe{size: s.Size(), mtime: s.ModTime().UnixNano()}
	if f.Version != nil {
		p.version, p.err = f.Version(name, path)
	} else {
		p.version, p.err = (&textures.Todds{Path: path, Prepare: hideWindow}).Version()
	}
	c.entries[key] = p
	return p.version, p.err
}

// Locate returns the selected path and version. ok means the copy is usable.
func (f *Finder) Locate(name string) (path, version string, ok bool) {
	i := f.Status(name)
	return i.Path, i.Version, i.OK
}

// All returns current statuses in Names order. It never downloads dependencies.
func (f *Finder) All() []Info {
	out := make([]Info, 0, len(Names))
	for _, n := range Names {
		out = append(out, f.Status(n))
	}
	return out
}
