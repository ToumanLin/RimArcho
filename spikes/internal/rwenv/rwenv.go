// Package rwenv locates a RimWorld install and enumerates mods the way the game does.
// Spike-quality code: shared helpers for the M0 spikes only.
package rwenv

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Env struct {
	SteamLibs   []string
	GameDir     string // .../steamapps/common/RimWorld
	WorkshopDir string // .../steamapps/workshop/content/294100
	UserDir     string // %USERPROFILE%/AppData/LocalLow/Ludeon Studios/RimWorld by Ludeon Studios
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

func Detect() (*Env, error) {
	e := &Env{}
	for _, steam := range []string{`C:\Program Files (x86)\Steam`, `C:\Program Files\Steam`} {
		b, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, m := range vdfPath.FindAllSubmatch(b, -1) {
			e.SteamLibs = append(e.SteamLibs, strings.ReplaceAll(string(m[1]), `\`, `\`))
		}
		break
	}
	for _, lib := range e.SteamLibs {
		g := filepath.Join(lib, "steamapps", "common", "RimWorld")
		if st, err := os.Stat(g); err == nil && st.IsDir() {
			e.GameDir = g
			e.WorkshopDir = filepath.Join(lib, "steamapps", "workshop", "content", "294100")
			break
		}
	}
	if e.GameDir == "" {
		return nil, errors.New("RimWorld not found in any Steam library")
	}
	home, _ := os.UserHomeDir()
	e.UserDir = filepath.Join(home, "AppData", "LocalLow", "Ludeon Studios", "RimWorld by Ludeon Studios")
	return e, nil
}

func (e *Env) ModsConfigPath() string { return filepath.Join(e.UserDir, "Config", "ModsConfig.xml") }
func (e *Env) PlayerLogPath() string  { return filepath.Join(e.UserDir, "Player.log") }

type ModsConfig struct {
	Version         string   `xml:"version"`
	ActiveMods      []string `xml:"activeMods>li"`
	KnownExpansions []string `xml:"knownExpansions>li"`
}

func ReadModsConfig(path string) (*ModsConfig, error) {
	var mc ModsConfig
	if err := DecodeFile(path, &mc); err != nil {
		return nil, err
	}
	return &mc, nil
}

// DecodeFile decodes an XML file, tolerating BOM and non-UTF-8 declarations.
func DecodeFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d.Decode(v)
}

type Source int

const (
	Official Source = iota
	Local
	Workshop
)

func (s Source) String() string { return [...]string{"official", "local", "workshop"}[s] }

type Mod struct {
	Dir          string
	Source       Source
	PackageID    string // lower-case, as in About.xml (no postfix)
	Name         string
	SteamPostfix bool // game appends "_steam" (workshop copy shadowed by a local copy)
}

// GameID is ModMetaData.PackageId: lower-case id plus "_steam" when the postfix applies.
func (m *Mod) GameID() string {
	if m.SteamPostfix {
		return m.PackageID + "_steam"
	}
	return m.PackageID
}

type about struct {
	PackageID string `xml:"packageId"`
	Name      string `xml:"name"`
}

func readAbout(dir string) (about, bool) {
	var a about
	if err := DecodeFile(filepath.Join(dir, "About", "About.xml"), &a); err != nil {
		return a, false
	}
	a.PackageID = strings.ToLower(strings.TrimSpace(a.PackageID))
	a.Name = strings.TrimSpace(a.Name)
	return a, true
}

func subdirs(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		// Junctions and symlinks report !IsDir(); the game's DirectoryInfo.GetDirectories follows them.
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

type Registry struct {
	All  []*Mod
	ByID map[string]*Mod // keyed by GameID()
	// Rejected lists mods the game would refuse ("same packageId multiple times").
	Rejected []*Mod
}

// Scan mirrors ModLister.RebuildModList + TryAddMod: official (Data/), local (Mods/),
// then workshop. A later mod with a duplicate id is rejected unless exactly one of the two
// is a workshop item, in which case the workshop copy gets the "_steam" postfix.
// Approximation: every folder in the workshop dir is treated as subscribed.
func (e *Env) Scan() *Registry {
	r := &Registry{ByID: map[string]*Mod{}}
	add := func(dir string, src Source) {
		a, ok := readAbout(dir)
		if !ok {
			return
		}
		id := a.PackageID
		if id == "" && src == Official {
			id = "ludeon.rimworld." + strings.ToLower(filepath.Base(dir))
		}
		name := a.Name
		if name == "" { // ModMetaData.Init fallback; official DLCs ship without <name>
			if src == Workshop {
				name = "Workshop mod " + filepath.Base(dir)
			} else {
				name = filepath.Base(dir)
			}
		}
		m := &Mod{Dir: dir, Source: src, PackageID: id, Name: name}
		if prev, dup := r.ByID[m.GameID()]; dup {
			// Scan order (official, local, workshop) means only the new mod can be the
			// workshop side, so it is the one that gets the postfix.
			if m.Source == Workshop && prev.Source != Workshop {
				m.SteamPostfix = true
			}
			if _, dup2 := r.ByID[m.GameID()]; dup2 {
				r.Rejected = append(r.Rejected, m)
				return
			}
		}
		r.All = append(r.All, m)
		r.ByID[m.GameID()] = m
	}
	for _, d := range subdirs(filepath.Join(e.GameDir, "Data")) {
		add(d, Official)
	}
	for _, d := range subdirs(filepath.Join(e.GameDir, "Mods")) {
		add(d, Local)
	}
	for _, d := range subdirs(e.WorkshopDir) {
		add(d, Workshop)
	}
	return r
}

// Resolve maps a ModsConfig entry to a mod the way ModsConfig load does: exact GameID,
// then folder name, then "<id>_steam" → "<id>".
func (r *Registry) Resolve(entry string) *Mod {
	if m := r.ByID[entry]; m != nil {
		return m
	}
	for _, m := range r.All {
		if filepath.Base(m.Dir) == entry {
			return m
		}
	}
	if strings.HasSuffix(entry, "_steam") {
		return r.ByID[strings.TrimSuffix(entry, "_steam")]
	}
	return nil
}
