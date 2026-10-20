package app

import (
	"archive/zip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Packs (10 §8, 09 §2): a plan compiled into a fixed product. Every mod is pinned to the
// instance and content it had, copied without the version folders this game does not load,
// and played from its own data folder through links in Mods/, so the environment's list and
// saves stay as they were.

// PackFormat is the pack.json format version.
const PackFormat = 1

// Pack is pack.json.
type Pack struct {
	Format  int         `json:"format"`
	Name    string      `json:"name"`
	Created time.Time   `json:"created"`
	Game    string      `json:"game"` // the game version it was built for
	Env     string      `json:"env"`
	Light   bool        `json:"light,omitempty"` // the list only, no files
	Entries []PackEntry `json:"entries"`
	Trimmed int64       `json:"trimmed_bytes,omitempty"`
}

// PackEntry is one mod of a pack, in load order.
type PackEntry struct {
	PackageID  string   `json:"package_id"`
	Name       string   `json:"name"`
	Official   bool     `json:"official,omitempty"` // Core or a DLC: from the game, not carried
	Lineage    string   `json:"lineage,omitempty"`
	Instance   string   `json:"instance,omitempty"`
	WorkshopID string   `json:"workshop_id,omitempty"`
	Hash       string   `json:"hash,omitempty"`   // semantic snapshot hash (02 §4)
	Folder     string   `json:"folder,omitempty"` // under mods/ (carried entries)
	Bytes      int64    `json:"bytes,omitempty"`
	Trimmed    int64    `json:"trimmed_bytes,omitempty"`
	TrimmedDir []string `json:"trimmed,omitempty"` // version folders left out
}

// PacksDir is where packs are kept: config pack.dir, else <data>/packs.
func (a *App) PacksDir() string {
	if v := a.Store.Meta("cfg:pack.dir"); v != "" {
		return v
	}
	return filepath.Join(a.Home, "packs")
}

var packNameRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} ._\-]{0,63}$`)

func checkPackName(name string) error {
	if !packNameRe.MatchString(name) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("pack name %q: letters, digits, space, . _ - (up to 64)", name)
	}
	return nil
}

func (a *App) packPath(name string) string { return filepath.Join(a.PacksDir(), name) }

// ReadPack reads a pack's pack.json.
func (a *App) ReadPack(name string) (*Pack, error) {
	b, err := os.ReadFile(filepath.Join(a.packPath(name), "pack.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no pack %q (archo pack list)", name)
		}
		return nil, err
	}
	var p Pack
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("pack %q: %v", name, err)
	}
	if p.Format > PackFormat {
		return nil, fmt.Errorf("pack %q has format %d; this RimArcho reads up to %d", name, p.Format, PackFormat)
	}
	return &p, nil
}

// ---- build ----

// PackBuild is the result of BuildPack.
type PackBuild struct {
	Pack     *Pack
	Dir      string
	DryRun   bool
	Blockers []string // why it cannot be built
	Moved    int      // entries the sort moved
}

// BuildPack compiles the workspace into a pack (10 §8.2): nothing must be left to fix,
// the list is sorted (for the pack only), and each mod is copied without the version
// folders this game does not load. light writes the list only.
func (a *App) BuildPack(name string, light, dryRun bool) (*PackBuild, error) {
	if err := checkPackName(name); err != nil {
		return nil, err
	}
	dir := a.packPath(name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("pack %q exists (%s); remove it first or pick another name", name, dir)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	r := &PackBuild{Dir: dir, DryRun: dryRun}
	for _, e := range st.List {
		if e.Instance == "" {
			r.Blockers = append(r.Blockers, "not installed: pkg:"+e.Missing)
		}
	}
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil, err
	}
	for _, p := range ps.List {
		// Order problems do not block: the pack is sorted on its own below.
		if p.Log == nil && p.Severity == finding.Error && !strings.HasPrefix(p.Code, "order.") && p.Code != "official.order" {
			r.Blockers = append(r.Blockers, p.Claim)
		}
	}
	if len(r.Blockers) > 0 {
		return r, nil
	}
	sr, err := a.Sort(true)
	if err != nil {
		return nil, err
	}
	r.Moved = len(sr.Changes)
	list := sr.Sorted
	var mods []*store.Mod
	for _, e := range list {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			mods = append(mods, m)
		}
	}
	if err := a.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, m := range mods {
		active[m.PackageID] = true
	}
	p := &Pack{Format: PackFormat, Name: name, Created: a.Now().UTC(), Game: st.Ix.Env.GameVersion, Env: st.Ix.Env.Name, Light: light}
	used := map[string]bool{}
	for _, m := range mods {
		pe := PackEntry{PackageID: m.PackageID, Name: m.DisplayName(), Lineage: m.LineageID, Instance: m.ID, WorkshopID: m.WorkshopID(), Hash: hex.EncodeToString(m.SnapHash)}
		if m.Kind == source.Official {
			pe = PackEntry{PackageID: m.PackageID, Name: m.DisplayName(), Official: true}
			p.Entries = append(p.Entries, pe)
			continue
		}
		if !light {
			pe.Folder = packFolder(m, used)
			root := st.Ix.ModDir(m)
			if real, err := source.RealPath(root); err == nil {
				root = real
			}
			res := content.Resolve(root, m.Decl().LoadFolders, st.GameMM(), func(pkg string) bool { return active[strings.TrimSuffix(strings.ToLower(pkg), "_steam")] })
			keep := map[string]bool{}
			for _, f := range res.Folders {
				if rel, err := filepath.Rel(root, f); err == nil && rel != "." {
					keep[strings.ToLower(strings.Split(filepath.ToSlash(rel), "/")[0])] = true
				}
			}
			ents, _ := os.ReadDir(root)
			for _, de := range ents {
				if de.IsDir() && content.IsVersionFolder(de.Name()) && !keep[strings.ToLower(de.Name())] {
					pe.TrimmedDir = append(pe.TrimmedDir, de.Name())
					pe.Trimmed += dirSize(filepath.Join(root, de.Name()))
				}
			}
			pe.Bytes = dirSize(root) - pe.Trimmed
			p.Trimmed += pe.Trimmed
		}
		p.Entries = append(p.Entries, pe)
	}
	r.Pack = p
	if dryRun {
		return r, nil
	}
	tmp := dir + ".archo-tmp"
	os.RemoveAll(tmp)
	if err := a.writePack(st, p, tmp); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return r, nil
}

func (a *App) writePack(st *LoadoutState, p *Pack, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, pe := range p.Entries {
		if pe.Official || pe.Folder == "" {
			continue
		}
		m := st.Ix.ByID[pe.Instance]
		root := st.Ix.ModDir(m)
		if real, err := source.RealPath(root); err == nil {
			root = real
		}
		dst := filepath.Join(dir, "mods", pe.Folder)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		skip := map[string]bool{}
		for _, t := range pe.TrimmedDir {
			skip[t] = true
		}
		res := &CopyResult{}
		ents, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, de := range ents {
			name := de.Name()
			if skip[name] || strings.EqualFold(name, ".git") || strings.EqualFold(name, ".rimarcho") {
				continue
			}
			src := filepath.Join(root, name)
			fi, err := os.Stat(src)
			if err != nil {
				continue
			}
			if fi.IsDir() {
				if err := copyTree(root, src, filepath.Join(dst, name), name, res); err != nil {
					return err
				}
			} else if err := copyFile(src, filepath.Join(dst, name), fi); err != nil {
				return err
			}
		}
		o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.WorkshopID(), SnapshotHash: pe.Hash,
			CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
		b, _ := json.MarshalIndent(o, "", "  ")
		if err := os.MkdirAll(filepath.Join(dst, ".rimarcho"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(filepath.Join(dir, "pack.json"), append(b, '\n'), 0o644)
}

var unsafeFolder = regexp.MustCompile(`[^A-Za-z0-9._\-]+`)

// packFolder names a mod's folder inside the pack: its folder name, made unique.
func packFolder(m *store.Mod, used map[string]bool) string {
	base := unsafeFolder.ReplaceAllString(filepath.Base(m.Rel), "_")
	if base == "" || base == "." || base == ".." {
		base = unsafeFolder.ReplaceAllString(m.PackageID, "_")
	}
	name := base
	for i := 2; used[strings.ToLower(name)]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[strings.ToLower(name)] = true
	return name
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// ---- list ----

// PacksOf lists the packs of an environment (10 §8): built in it, or — for a pack from
// another machine whose environment does not exist here — built for its game version.
// others counts the packs that belong to other environments.
func (a *App) PacksOf(e *env.Environment) (mine []*PackInfo, others int, err error) {
	all, err := a.Packs()
	if err != nil {
		return nil, 0, err
	}
	known := map[string]bool{}
	if envs, err := a.Store.Envs(); err == nil {
		for _, x := range envs {
			known[x.Name] = true
		}
	}
	for _, p := range all {
		ok := p.Env == e.Name
		if !ok && !known[p.Env] {
			ok = env.MajorMinor(p.Game) == env.MajorMinor(e.GameVersion)
		}
		if ok {
			mine = append(mine, p)
		} else {
			others++
		}
	}
	return mine, others, nil
}

// PackInfo is a pack in the list.
type PackInfo struct {
	*Pack
	Dir        string
	Bytes      int64
	LastPlayed time.Time
}

type packState struct {
	LastPlayed time.Time `json:"last_played"`
}

// Packs lists the packs.
func (a *App) Packs() ([]*PackInfo, error) {
	ents, err := os.ReadDir(a.PacksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*PackInfo
	for _, de := range ents {
		if !de.IsDir() || strings.HasSuffix(de.Name(), ".archo-tmp") {
			continue
		}
		p, err := a.ReadPack(de.Name())
		if err != nil {
			continue
		}
		pi := &PackInfo{Pack: p, Dir: a.packPath(de.Name()), Bytes: dirSize(filepath.Join(a.packPath(de.Name()), "mods"))}
		if b, err := os.ReadFile(filepath.Join(pi.Dir, "state.json")); err == nil {
			var s packState
			if json.Unmarshal(b, &s) == nil {
				pi.LastPlayed = s.LastPlayed
			}
		}
		out = append(out, pi)
	}
	return out, nil
}

// ---- launch ----

// PackLaunch is what LaunchPack does (or would do).
type PackLaunch struct {
	Pack       *Pack
	Links      []LinkChange // in Mods/: this pack's links added, other packs' removed
	ModsConfig string       // the pack's own ModsConfig.xml
	Tokens     []string
	UserData   string
	Plan       *LaunchPlan
	DryRun     bool
	Copied     []string // settings files copied from the environment on the first run
}

func packLink(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(source.PackLinkPrefix))
}

func packLinkName(pack, folder string) string {
	return source.PackLinkPrefix + unsafeFolder.ReplaceAllString(pack, "_") + "_" + folder
}

// LaunchPack plays a pack (10 §8.3): its mods are linked into Mods/, its list is written to
// its own data folder, and the game starts with -savedatafolder pointing there.
func (a *App) LaunchPack(name string, dryRun bool) (*PackLaunch, error) {
	p, err := a.ReadPack(name)
	if err != nil {
		return nil, err
	}
	if p.Light {
		return nil, fmt.Errorf("%q is a light pack (a list without files): archo pack import can load it into the workspace", name)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if pm, gm := env.MajorMinor(p.Game), st.GameMM(); pm != "" && gm != "" && pm != gm {
		return nil, fmt.Errorf("%w: pack %q was built for %s, the game is %s", ErrLaunchBlocked, name, pm, gm)
	}
	mods := modsDir(st.Ix)
	if mods == "" {
		return nil, fmt.Errorf("%w: the environment has no Mods/ folder", ErrLaunchBlocked)
	}
	dir := a.packPath(name)
	r := &PackLaunch{Pack: p, UserData: filepath.Join(dir, "userdata"), DryRun: dryRun}
	r.ModsConfig = env.ModsConfigPath(r.UserData)
	// How the game will register the pack's copies next to what is in Mods/ (04 §2.4).
	cands := Candidates(st.Ix, st.Aliases)
	want := map[string]string{} // link name (lower) → target
	for _, pe := range p.Entries {
		if pe.Official {
			continue
		}
		link := packLinkName(name, pe.Folder)
		want[strings.ToLower(link)] = filepath.Join(dir, "mods", pe.Folder)
		cands = append(cands, loadout.Candidate{ID: "pack:" + pe.Folder, PackageID: pe.PackageID, Kind: source.Local, Folder: link})
	}
	reg := loadout.NewRegistry(cands)
	var conflicts []string
	for _, pe := range p.Entries {
		if pe.Official {
			r.Tokens = append(r.Tokens, pe.PackageID)
			continue
		}
		id := "pack:" + pe.Folder
		if !reg.Loadable(id) {
			for _, o := range reg.SamePackage(id) {
				if o.Kind != source.Workshop && reg.Loadable(o.ID) {
					conflicts = append(conflicts, fmt.Sprintf("%s: Mods/%s has the same packageId and the game takes it first", pe.Name, o.Folder))
				}
			}
			continue
		}
		r.Tokens = append(r.Tokens, reg.Token(id))
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%w: the game would not load the pack's copy of %d mods:\n  %s", ErrLaunchBlocked, len(conflicts), strings.Join(conflicts, "\n  "))
	}
	// Links: other packs' go, this pack's are made (or fixed).
	ents, _ := os.ReadDir(mods)
	have := map[string]bool{}
	for _, de := range ents {
		n := de.Name()
		lp := filepath.Join(mods, n)
		if !packLink(n) || !source.IsLink(lp) {
			continue
		}
		if t, ok := want[strings.ToLower(n)]; ok && sameDir(lp, t) {
			have[strings.ToLower(n)] = true
			continue
		}
		tgt, _ := source.RealPath(lp)
		r.Links = append(r.Links, LinkChange{Op: "remove", Name: n, Target: tgt})
	}
	for _, pe := range p.Entries {
		if pe.Official {
			continue
		}
		link := packLinkName(name, pe.Folder)
		if !have[strings.ToLower(link)] {
			if _, err := os.Lstat(filepath.Join(mods, link)); err == nil && !source.IsLink(filepath.Join(mods, link)) {
				return nil, fmt.Errorf("%s already exists and is not a link; RimArcho does not touch it", filepath.Join(mods, link))
			}
			r.Links = append(r.Links, LinkChange{Op: "add", Name: link, Target: filepath.Join(dir, "mods", pe.Folder)})
		}
	}
	if r.Plan, err = a.gamePlan(e, r.UserData); err != nil {
		return nil, err
	}
	if dryRun {
		return r, nil
	}
	if err := a.checkRoots(mods); err != nil {
		return nil, err
	}
	if err := applyLinks(mods, r.Links); err != nil {
		return nil, err
	}
	// First run: the pack starts with the environment's settings (not its mod list).
	cfg := filepath.Join(r.UserData, "Config")
	if _, err := os.Stat(cfg); os.IsNotExist(err) && e.UserDataRoot != "" {
		if src, err := os.ReadDir(filepath.Join(e.UserDataRoot, "Config")); err == nil {
			os.MkdirAll(cfg, 0o755)
			for _, f := range src {
				if f.IsDir() || strings.EqualFold(f.Name(), "ModsConfig.xml") || !strings.EqualFold(filepath.Ext(f.Name()), ".xml") {
					continue
				}
				if fi, err := f.Info(); err == nil && copyFile(filepath.Join(e.UserDataRoot, "Config", f.Name()), filepath.Join(cfg, f.Name()), fi) == nil {
					r.Copied = append(r.Copied, f.Name())
				}
			}
		}
	}
	var known []string
	for _, m := range st.Ix.Mods {
		if m.Kind == source.Official && m.PackageID != "ludeon.rimworld" && m.Status == identity.Live {
			known = append(known, m.PackageID)
		}
	}
	version := e.GameVersion
	if version == "" {
		version = p.Game
	}
	var orig []byte
	if b, err := os.ReadFile(r.ModsConfig); err == nil {
		orig = b
	}
	data, err := loadout.Render(orig, version, r.Tokens, known)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(r.ModsConfig), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(r.ModsConfig, data, 0o644); err != nil {
		return nil, err
	}
	if err := a.startGame(r.Plan); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(packState{LastPlayed: a.Now().UTC()})
	os.WriteFile(filepath.Join(dir, "state.json"), b, 0o644)
	return r, a.Store.AddLaunch(e.ID, a.Now().UTC(), "pack:"+name, r.Plan.Command())
}

// removePackLinks removes the packs' links from Mods/ (all of them when keep is ""): playing
// the environment's own list must not show the game a pack's copies.
func (a *App) removePackLinks(ix *Index, keep string) ([]string, error) {
	mods := modsDir(ix)
	if mods == "" {
		return nil, nil
	}
	ents, err := os.ReadDir(mods)
	if err != nil {
		return nil, nil
	}
	var removed []string
	for _, de := range ents {
		n := de.Name()
		lp := filepath.Join(mods, n)
		if !packLink(n) || !source.IsLink(lp) {
			continue
		}
		if keep != "" && strings.HasPrefix(strings.ToLower(n), strings.ToLower(packLinkName(keep, ""))) {
			continue
		}
		if err := source.RemoveLink(lp); err != nil {
			return removed, err
		}
		removed = append(removed, n)
	}
	return removed, nil
}

// ---- export, import, remove ----

// ExportPack writes a pack to a file: a full pack as a zip (.archopack, without its saves),
// a light pack (or --light) as its pack.json (.archopack.json).
func (a *App) ExportPack(name, file string, light bool) (int64, error) {
	p, err := a.ReadPack(name)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(file); err == nil {
		return 0, fmt.Errorf("%s exists; nothing is overwritten", file)
	}
	if light || p.Light {
		q := *p
		q.Light = true
		for i := range q.Entries {
			q.Entries[i].Folder, q.Entries[i].Bytes, q.Entries[i].Trimmed, q.Entries[i].TrimmedDir = "", 0, 0, nil
		}
		q.Trimmed = 0
		b, _ := json.MarshalIndent(&q, "", "  ")
		return int64(len(b)), os.WriteFile(file, append(b, '\n'), 0o644)
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	zw := zip.NewWriter(f)
	root := a.packPath(name)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == "userdata" || rel == "state.json" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		in.Close()
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	f.Close()
	if err != nil {
		os.Remove(file)
		return 0, err
	}
	fi, _ := os.Stat(file)
	return fi.Size(), nil
}

// PackMatch compares a light pack with the installed mods (10 §8.4).
type PackMatch struct {
	Pack            *Pack
	Same, Different []PackEntry // installed with the same content / another version
	Missing         []PackEntry
	Imported        *EditResult // with asPlan: the workspace now holds the pack's list
	Dir             string      // a full pack: where it was unpacked
}

// ImportPack reads a pack file. A full pack (.archopack) is unpacked into the packs folder;
// a light pack is compared with the installed mods and, with asPlan, loaded into the
// workspace.
func (a *App) ImportPack(file string, asPlan bool) (*PackMatch, error) {
	zr, err := zip.OpenReader(file)
	if err == nil {
		defer zr.Close()
		return a.unpackPack(&zr.Reader)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var p Pack
	if err := json.Unmarshal(b, &p); err != nil || p.Format == 0 {
		return nil, fmt.Errorf("%s is not a pack (a .archopack zip or a pack.json)", file)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	r := &PackMatch{Pack: &p}
	var tokens []string
	for _, pe := range p.Entries {
		tokens = append(tokens, pe.PackageID)
		if pe.Official {
			continue
		}
		var same, other bool
		for _, m := range st.Ix.Mods {
			if m.Status != identity.Live || m.PackageID != pe.PackageID {
				continue
			}
			if hex.EncodeToString(m.SnapHash) == pe.Hash {
				same = true
			} else {
				other = true
			}
		}
		switch {
		case same:
			r.Same = append(r.Same, pe)
		case other:
			r.Different = append(r.Different, pe)
		default:
			r.Missing = append(r.Missing, pe)
		}
	}
	if asPlan {
		il := newList("pack")
		il.Title = p.Name
		for _, t := range tokens {
			il.add(t, "", "")
		}
		if r.Imported, err = a.importList(il, "pack "+p.Name); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (a *App) unpackPack(zr *zip.Reader) (*PackMatch, error) {
	var p Pack
	for _, f := range zr.File {
		if f.Name == "pack.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = json.NewDecoder(io.LimitReader(rc, 64<<20)).Decode(&p)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("pack.json: %v", err)
			}
		}
	}
	if p.Format == 0 {
		return nil, errors.New("the file has no pack.json")
	}
	if err := checkPackName(p.Name); err != nil {
		return nil, err
	}
	dir := a.packPath(p.Name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("pack %q exists; remove it first", p.Name)
	}
	tmp := dir + ".archo-tmp"
	os.RemoveAll(tmp)
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		dst := filepath.Join(tmp, name)
		if !within(tmp, dst) || strings.Contains(f.Name, "..") || filepath.IsAbs(name) {
			os.RemoveAll(tmp)
			return nil, fmt.Errorf("unsafe path in the pack: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			os.RemoveAll(tmp)
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			os.RemoveAll(tmp)
			return nil, err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(rc, 4<<30))
			out.Close()
		}
		rc.Close()
		if err != nil {
			os.RemoveAll(tmp)
			return nil, err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &PackMatch{Pack: &p, Dir: dir}, nil
}

// RemovePack moves a pack to the recycle bin, after removing its links.
func (a *App) RemovePack(name string, dryRun bool) (string, error) {
	if _, err := a.ReadPack(name); err != nil {
		return "", err
	}
	dir := a.packPath(name)
	if dryRun {
		return dir, nil
	}
	if ix, err := a.Index(); err == nil {
		mods := modsDir(ix)
		ents, _ := os.ReadDir(mods)
		for _, de := range ents {
			if strings.HasPrefix(strings.ToLower(de.Name()), strings.ToLower(packLinkName(name, ""))) && source.IsLink(filepath.Join(mods, de.Name())) {
				if err := source.RemoveLink(filepath.Join(mods, de.Name())); err != nil {
					return "", err
				}
			}
		}
	}
	trash := a.TrashFunc
	if trash == nil {
		trash = source.Trash
	}
	return dir, trash(dir)
}
