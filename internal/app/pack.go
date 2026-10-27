package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Packs (08 §5): a plan compiled into a fixed product. Every mod is pinned to the
// instance and content it had, copied without the version folders this game does not load,
// and played from its own data folder through links in Mods/, so the environment's list and
// saves stay as they were.

// PackFormat is the pack.json format version.
const PackFormat = 2

// Pack is pack.json.
type Pack struct {
	Format  int       `json:"format"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Game    string    `json:"game"` // the game version it was built for
	Env     string    `json:"env"`
	// Light: the list only, no files — a RimArcho list (04 §9.3) or a pack's exported list.
	Light bool `json:"light,omitempty"`
	// PlanID is the plan the pack was created from, in the database that created it (the
	// list management page shows a plan's newest pack, 06 §2.17).
	PlanID int64 `json:"plan_id,omitempty"`
	// Versions are the game versions the list is meant for (a RimArcho list, 04 §1.6, §9.3).
	Versions []string    `json:"versions,omitempty"`
	Entries  []PackEntry `json:"entries"`
	Trimmed  int64       `json:"trimmed_bytes,omitempty"`
	// Textures: the copies' PNGs were converted to DDS and deleted (08 §5.2).
	Textures *PackTextures `json:"textures,omitempty"`
}

// PackTextures is what a build did to the copies' textures: 预转换 at full size, 精简 when
// scaled. The PNGs of the textures converted are gone from the pack; those that could not
// be converted (or share a name with a DDS or another format) keep their PNG.
type PackTextures struct {
	Tool      string `json:"tool"` // "todds 0.4.1"
	Scale     int    `json:"scale"`
	Mipmaps   bool   `json:"mipmaps"`
	Converted int    `json:"converted"`
	Kept      int    `json:"kept,omitempty"`   // PNGs left (skipped or failed)
	Failed    int    `json:"failed,omitempty"` // of Kept: the encoder could not convert them
	PNGBytes  int64  `json:"png_bytes"`        // the PNGs deleted
	DDSBytes  int64  `json:"dds_bytes"`        // the DDS written
}

// TextureLabel names a texture option the way the interface does: 预转换 at full size,
// 精简 when scaled (2026-10-22).
func TextureLabel(o textures.Options) string {
	if o = o.Normal(); o.Scale == 100 {
		return "pre-convert"
	}
	return fmt.Sprintf("slim to %d%%", o.Scale)
}

// PackEntry is one mod of a pack, in load order.
type PackEntry struct {
	PackageID       string   `json:"package_id"`
	Name            string   `json:"name"`
	Official        bool     `json:"official,omitempty"` // Core or a DLC: from the game, not carried
	Lineage         string   `json:"lineage,omitempty"`
	Instance        string   `json:"instance,omitempty"`
	WorkshopID      string   `json:"workshop_id,omitempty"`
	Hash            string   `json:"hash,omitempty"`   // semantic snapshot hash (02 §4)
	Folder          string   `json:"folder,omitempty"` // under mods/ (carried entries)
	Bytes           int64    `json:"bytes,omitempty"`
	Trimmed         int64    `json:"trimmed_bytes,omitempty"`
	TrimmedDir      []string `json:"trimmed,omitempty"` // version folders left out
	Files           int      `json:"files,omitempty"`
	SHA256          string   `json:"sha256,omitempty"`
	WorkshopUpdated int64    `json:"workshop_updated,omitempty"`
	GitHead         string   `json:"git_head,omitempty"` // observed fingerprint, not Git subscription provenance
}

// PacksDir is where packs are kept: config pack.dir, else <data>/packs.
func (a *App) PacksDir() string {
	if v := a.Store.Meta("cfg:pack.dir"); v != "" {
		return v
	}
	return filepath.Join(a.Home, "packs")
}

// packNameOK: a letter or digit, then up to 63 letters, digits, spaces and . _ -. Checked
// rune by rune: the same rule as a regexp, whose {0,63} copies of two Unicode classes cost
// about 1.4 MB of heap for the life of the process.
func packNameOK(name string) bool {
	n := 0
	for _, r := range name {
		n++
		if n > 64 {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && (n == 1 || !strings.ContainsRune(" ._-", r)) {
			return false
		}
	}
	return n > 0
}

// CheckPackName says whether name can name a pack (a folder in the packs folder).
func CheckPackName(name string) error { return checkPackName(name) }

func checkPackName(name string) error {
	if !packNameOK(name) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
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
	if p.Format < 1 || p.Format > PackFormat {
		return nil, fmt.Errorf("pack %q has format %d; this RimArcho reads up to %d", name, p.Format, PackFormat)
	}
	a.aliasPack(&p)
	return &p, nil
}

// aliasPack maps the IDs of a pack made before migration v14 to the IDs kept (02 §0.1): the
// file on disk is not rewritten.
func (a *App) aliasPack(p *Pack) {
	al, err := a.Store.IDAliases()
	if err != nil || len(al) == 0 {
		return
	}
	for i := range p.Entries {
		pe := &p.Entries[i]
		if n := al[pe.Instance]; n != "" {
			pe.Instance = n
		}
		if n := al[pe.Lineage]; n != "" {
			pe.Lineage = n
		}
	}
}

// ---- build ----

// PackBuild is the result of BuildPack.
type PackBuild struct {
	Pack     *Pack
	Dir      string
	DryRun   bool
	Blockers []string // why it cannot be built (需要处理)
	Moved    int      // entries the sort moved
	// Measured: what the copy needs, and what the packs drive has (-1: unknown). A build is
	// refused when Free < Need + SpaceMargin.
	Need, Free int64
	CannotCopy []string // mods whose folder cannot be read: the build is refused
	Mismatch   []string // mods that do not list the game's version (warnings)
	Skipped    int      // links leaving a mod's folder, not copied
	// Textures (when the build converts them): the estimate before, then what was done;
	// Peak is the most the packs drive needs while converting (the copy, the DDS of the mods
	// converted, the PNGs not yet deleted), Final the pack's size after.
	TexOptions *textures.Options
	TexEst     textures.Estimate
	TexSkipped int // PNGs that keep their PNG: a DDS or another format of the same name
	Peak       int64
	Final      int64
	Tool       *TextureTool
}

// BuildOptions for BuildPackWith.
type BuildOptions struct {
	DryRun   bool
	Progress func(BuildProgress) // each step, and while copying
	// Textures converts the copies' PNGs to DDS and deletes them (nil: they are copied as
	// they are). Full size is 预转换, a scale 精简.
	Textures *textures.Options
}

// ErrBuildRunning: a build of this pack is going on (in this or another process).
var ErrBuildRunning = errors.New("a build of this pack is running")

// ErrNoSpace: the packs drive has less free space than the copy needs.
var ErrNoSpace = errors.New("not enough free space for the pack")

// SpaceMargin is kept free on the packs drive beyond the copy itself.
const SpaceMargin = 256 << 20

// BuildPack compiles the workspace into a pack (08 §5.2): nothing must be left to fix,
// the list is sorted (for the pack only), and each mod is copied without the version
// folders this game does not load. (Sharing the list alone is an export: 04 §9.3.)
func (a *App) BuildPack(name string, dryRun bool) (*PackBuild, error) {
	return a.BuildPackWith(name, BuildOptions{DryRun: dryRun})
}

// PackPlan is a build before anything is measured or copied: the checked, sorted list
// with the version folders each mod leaves out.
type PackPlan struct {
	St    *LoadoutState
	R     *PackBuild
	roots []string // per entry: the folder copied ("" for Core and the DLCs)
	free  func(string) int64
	tex   *textures.Options
	gen   []map[string]bool // per entry: RimArcho's DDS in the mod (not copied, not the mod's)
}

// WithTextures makes the plan convert textures (Measure then estimates them).
func (a *App) WithTextures(pl *PackPlan, o *textures.Options) {
	if o != nil {
		n := o.Normal()
		o = &n
	}
	pl.tex = o
	pl.R.TexOptions = o
}

// PlanPack checks the list and sorts it for the pack (the first two steps of a build).
func (a *App) PlanPack(name string) (*PackPlan, error) {
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
	r := &PackBuild{Dir: dir, Free: -1}
	pl := &PackPlan{St: st, R: r, free: a.FreeSpace}
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
		if p.Log != nil {
			continue
		}
		// Order problems do not block: the pack is sorted on its own below. Nor does what
		// Mods/ holds now (leftover links, two folders of one mod): launching the pack deals
		// with it, and the build only writes to the packs folder.
		if p.Severity == finding.Error && !strings.HasPrefix(p.Code, "order.") && p.Code != "official.order" && p.Code != CodePackLinks && p.Code != CodeModsDuplicate {
			r.Blockers = append(r.Blockers, p.Claim)
		} else if p.Code == "version.unsupported" && len(p.Subjects) > 0 {
			line := p.Claim
			if m := st.Ix.ByID[finding.InstID(p.Subjects[0])]; m != nil {
				line = fmt.Sprintf("%s (%s)", m.DisplayName(), strings.TrimSpace(m.SupportedVersions))
			}
			r.Mismatch = append(r.Mismatch, line)
		}
	}
	if len(r.Blockers) > 0 {
		return pl, nil
	}
	sr, err := a.Sort(true)
	if err != nil {
		return nil, err
	}
	r.Moved = len(sr.Changes)
	var mods []*store.Mod
	for _, e := range sr.Sorted {
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
	p := &Pack{Format: PackFormat, Name: name, Created: a.Now().UTC(), Game: st.Ix.Env.GameVersion, Env: st.Ix.Env.Name, PlanID: st.Plan.ID}
	used := map[string]bool{}
	for _, m := range mods {
		pe := packEntryOf(m)
		if pe.Official {
			p.Entries = append(p.Entries, pe)
			pl.roots = append(pl.roots, "")
			pl.gen = append(pl.gen, nil)
			continue
		}
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
			}
		}
		p.Entries = append(p.Entries, pe)
		pl.roots = append(pl.roots, root)
		pl.gen = append(pl.gen, a.generatedIn(root))
	}
	r.Pack = p
	return pl, nil
}

// packEntryOf is a mod as a pack or a RimArcho list records it: Core and the DLCs by
// packageId (they come from the game), the others with their lineage, instance, workshop ID
// and content hash.
func packEntryOf(m *store.Mod) PackEntry {
	if m.Kind == source.Official {
		return PackEntry{PackageID: m.PackageID, Name: m.DisplayName(), Official: true}
	}
	return PackEntry{PackageID: m.PackageID, Name: m.DisplayName(), Lineage: m.LineageID, Instance: m.ID, WorkshopID: m.WorkshopID(), Hash: hex.EncodeToString(m.SnapHash), WorkshopUpdated: m.ACFTimeUpdated, GitHead: m.GitHead}
}

// Measure sizes each mod's copy, finds the folders that cannot be read and the free
// space of the packs drive. It only reads the file system, so the window runs it without
// holding the app.
func (pl *PackPlan) Measure(packsDir string) {
	r := pl.R
	if r.Pack == nil {
		return
	}
	r.Need, r.Pack.Trimmed, r.CannotCopy = 0, 0, nil
	for i := range r.Pack.Entries {
		pe := &r.Pack.Entries[i]
		root := pl.roots[i]
		if root == "" {
			continue
		}
		if _, err := os.ReadDir(root); err != nil {
			r.CannotCopy = append(r.CannotCopy, fmt.Sprintf("%s: %s", pe.Name, root))
			continue
		}
		pe.Trimmed = 0
		for _, t := range pe.TrimmedDir {
			pe.Trimmed += dirSize(filepath.Join(root, t))
		}
		pe.Bytes = dirSize(root) - pe.Trimmed - genSize(root, pl.gen[i])
		r.Pack.Trimmed += pe.Trimmed
		r.Need += pe.Bytes
	}
	r.TexEst, r.TexSkipped, r.Peak, r.Final = textures.Estimate{}, 0, r.Need, r.Need
	if pl.tex != nil {
		var maxPNG int64
		for i := range r.Pack.Entries {
			root := pl.roots[i]
			if root == "" {
				continue
			}
			est, skipped := packTextureEstimate(root, r.Pack.Entries[i].TrimmedDir, *pl.tex, pl.gen[i])
			r.TexEst.Plus(est)
			r.TexSkipped += skipped
			maxPNG = max(maxPNG, est.PNGBytes)
		}
		// The mods are copied first, then converted one by one, each losing its PNGs.
		r.Final = r.Need - r.TexEst.PNGBytes + r.TexEst.DDSBytes
		r.Peak = max(r.Need, r.Final+maxPNG)
	}
	if pl.free != nil {
		r.Free = pl.free(packsDir)
	} else {
		r.Free = DiskFree(packsDir)
	}
}

// BuildPackWith is BuildPack with progress. While it copies, the pack is assembled in
// <name>.archo-tmp next to the packs, whose build.json says how far it got; a build cut
// short (its process ended) leaves that folder, which UnfinishedPacks reports.
func (a *App) BuildPackWith(name string, o BuildOptions) (*PackBuild, error) {
	if err := checkPackName(name); err != nil {
		return nil, err
	}
	tmp := a.packPath(name) + TmpSuffix
	if b := ReadBuildProgress(tmp); b != nil && b.Running() {
		return nil, fmt.Errorf("%w (%s)", ErrBuildRunning, name)
	}
	bs := &buildState{report: o.Progress}
	if !o.DryRun {
		if err := a.managedRootReady("packs", a.PacksDir()); err != nil {
			return nil, err
		}
		if _, err := os.Stat(a.packPath(name)); err == nil {
			return nil, fmt.Errorf("pack %q exists (%s); remove it first or pick another name", name, a.packPath(name))
		}
		// What is left of an earlier, unfinished build of this name is replaced.
		if err := os.RemoveAll(tmp); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return nil, err
		}
		if err := a.Store.SetMeta(directoryKey("packs"), a.PacksDir()); err != nil {
			return nil, err
		}
		bs.start(tmp, BuildProgress{Name: name, Step: StepCheck})
	}
	cleanup := func() {
		bs.stop()
		if !o.DryRun {
			os.RemoveAll(tmp)
		}
	}
	bs.update(func(p *BuildProgress) { p.Step = StepCheck })
	pl, err := a.PlanPack(name)
	if err != nil {
		cleanup()
		return nil, err
	}
	r := pl.R
	r.DryRun = o.DryRun
	if len(r.Blockers) > 0 {
		cleanup()
		return r, nil
	}
	if o.Textures != nil {
		a.WithTextures(pl, o.Textures)
		r.Tool = a.TextureTool()
	}
	bs.update(func(p *BuildProgress) { p.Step, p.Env, p.ModsTotal = StepMeasure, r.Pack.Env, carriedCount(r.Pack) })
	pl.Measure(a.PacksDir())
	if o.DryRun {
		return r, nil
	}
	var enc textures.Encoder
	if r.TexOptions != nil {
		// before copying gigabytes: the encoder must be there
		if enc, err = a.encoder(nil); err != nil {
			cleanup()
			return nil, err
		}
	}
	if len(r.CannotCopy) > 0 {
		cleanup()
		return nil, fmt.Errorf("%d mods cannot be read, so they cannot be copied:\n  %s", len(r.CannotCopy), strings.Join(r.CannotCopy, "\n  "))
	}
	if need := max(r.Need, r.Peak); r.Free >= 0 && r.Free < need+SpaceMargin {
		cleanup()
		return nil, fmt.Errorf("%w: it needs %s, the drive of %s has %s free", ErrNoSpace, render.Size(need+SpaceMargin), a.PacksDir(), render.Size(r.Free))
	}
	if r.TexOptions != nil {
		r.Pack.Textures = &PackTextures{Tool: "todds " + textures.ToddsVersion, Scale: r.TexOptions.Scale, Mipmaps: !r.TexOptions.NoMips}
	}
	bs.update(func(p *BuildProgress) { p.Step, p.BytesTotal = StepCopy, r.Need })
	if err := a.writePack(pl.St, r.Pack, tmp, bs, &r.Skipped); err != nil {
		cleanup()
		return nil, err
	}
	if r.TexOptions != nil {
		bs.update(func(p *BuildProgress) { p.Step, p.Mod, p.Mods, p.FilesTotal = StepTextures, "", 0, r.TexEst.Files })
		if err := a.packTextures(r, tmp, enc, bs); err != nil {
			cleanup()
			return nil, err
		}
	}
	bs.update(func(p *BuildProgress) { p.Step, p.Mod = StepWrite, "" })
	for i := range r.Pack.Entries {
		pe := &r.Pack.Entries[i]
		if pe.Official || pe.Folder == "" {
			continue
		}
		pe.Files, pe.SHA256, err = copyDigest(filepath.Join(tmp, "mods", pe.Folder))
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%s: %w", pe.Name, err)
		}
	}
	b, _ := json.MarshalIndent(r.Pack, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, "pack.json"), append(b, '\n'), 0o644); err != nil {
		cleanup()
		return nil, err
	}
	bs.stop()
	os.Remove(filepath.Join(tmp, BuildFile))
	if err := os.Rename(tmp, r.Dir); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	bs.update(func(p *BuildProgress) { p.Step = StepDone })
	return r, nil
}

func carriedCount(p *Pack) int {
	n := 0
	for _, pe := range p.Entries {
		if !pe.Official && pe.Folder != "" {
			n++
		}
	}
	return n
}

func (a *App) writePack(st *LoadoutState, p *Pack, dir string, bs *buildState, skipped *int) error {
	for _, pe := range p.Entries {
		if pe.Official || pe.Folder == "" {
			continue
		}
		m := st.Ix.ByID[pe.Instance]
		root := st.Ix.ModDir(m)
		if real, err := source.RealPath(root); err == nil {
			root = real
		}
		bs.update(func(p *BuildProgress) { p.Mod = pe.Name })
		dst := filepath.Join(dir, "mods", pe.Folder)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		skip := map[string]bool{}
		for _, t := range pe.TrimmedDir {
			skip[t] = true
		}
		res := &CopyResult{onFile: func(n int64) error {
			bs.update(func(p *BuildProgress) { p.Bytes += n })
			return bs.stopRequested()
		}, generated: a.generatedIn(root)} // RimArcho's DDS next to the PNGs are not the mod's
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
			} else if err := res.onFile(fi.Size()); err != nil {
				return err
			}
		}
		*skipped += len(res.Skipped)
		o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.WorkshopID(), SnapshotHash: pe.Hash,
			CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
		if p.Textures != nil {
			o.Textures = "dds" // its PNGs become DDS in the pack: not a change of the mod
			if p.Textures.Scale < 100 {
				o.Textures = fmt.Sprintf("dds %d%%", p.Textures.Scale)
			}
		}
		b, _ := json.MarshalIndent(o, "", "  ")
		if err := os.MkdirAll(filepath.Join(dst, ".rimarcho"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
			return err
		}
		bs.update(func(p *BuildProgress) { p.Mods++ })
	}
	return nil
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
	// Unix links have nonzero sizes; retain WalkDir's link accounting there.
	// Windows links have size zero, so skipping them preserves the total.
	if runtime.GOOS != "windows" {
		return dirSizeWithLinks(dir)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return 0
	}
	// WalkDir counted a file or link root itself, without following its target.
	if !fi.IsDir() || source.IsLink(dir) {
		return fi.Size()
	}
	// Pack data contains copies; launch junctions live outside it, in game Mods/.
	_, n, _ := fsscan.Size(context.Background(), dir, 0)
	return n
}

func dirSizeWithLinks(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
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

// PacksOf lists the packs of an environment (08 §5): built in it, or — for a pack from
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
		if !de.IsDir() || strings.HasSuffix(de.Name(), TmpSuffix) {
			continue
		}
		p, err := a.ReadPack(de.Name())
		if err != nil || p.Light {
			continue // a list without files (light packs before 2026-10-22) is not a pack
		}
		pi := &PackInfo{Pack: p, Dir: a.packPath(de.Name())}
		for _, pe := range p.Entries {
			pi.Bytes += pe.Bytes // measured when it was built: listing does not walk the copies
		}
		if pi.Bytes == 0 {
			pi.Bytes = dirSize(filepath.Join(pi.Dir, "mods"))
		}
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
	Check      *PackCheck
	Links      []LinkChange // in Mods/: this pack's links added, other packs' removed
	ModsConfig string       // the pack's own ModsConfig.xml
	Tokens     []string
	UserData   string
	Plan       *LaunchPlan
	DryRun     bool
	Copied     []string        // settings files copied from the environment on the first run
	InMods     []string        // mods whose copy already in Mods/ (the same content) stands in for the pack's
	Moved      []DuplicateMove // folders moved into the lock library first (or, in a dry run, to move)
}

func packLink(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(source.PackLinkPrefix))
}

func packLinkName(pack, folder string) string {
	return source.PackLinkPrefix + unsafeFolder.ReplaceAllString(pack, "_") + "_" + folder
}

// LaunchPack plays a pack (08 §5.3): its mods are linked into Mods/, its list is written to
// its own data folder, and the game starts with -savedatafolder pointing there.
func (a *App) LaunchPack(name string, dryRun bool) (*PackLaunch, error) {
	return a.LaunchPackWith(name, dryRun, false)
}

// LaunchPackWith is LaunchPack that, with move, first moves plain Mods/ folders holding
// another version of a pack copy into the lock library (08 §5.3; --move-duplicates).
func (a *App) LaunchPackWith(name string, dryRun, move bool) (*PackLaunch, error) {
	unlock, err := a.waitLaunch()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return a.launchPackWith(name, dryRun, move)
}

func (a *App) launchPackWith(name string, dryRun, move bool) (*PackLaunch, error) {
	if err := a.checkNoGame(); err != nil { // one game at a time (08 §1)
		return nil, err
	}
	p, err := a.ReadPack(name)
	if err != nil {
		return nil, err
	}
	if p.Light {
		return nil, fmt.Errorf("%q is a list without files, not a pack: archo loadout import %s loads it", name, filepath.Join(a.packPath(name), "pack.json"))
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	check := a.CheckPack(p, e, nil)
	if err := a.refuseUnhealthyLinks(modsDir(st.Ix)); err != nil {
		return nil, err
	}
	if check.Blocked() {
		return nil, fmt.Errorf("%w: pack %q: %s", ErrLaunchBlocked, name, strings.Join(check.Errors(), "; "))
	}
	mods := modsDir(st.Ix)
	if mods == "" {
		return nil, fmt.Errorf("%w: the environment has no Mods/ folder", ErrLaunchBlocked)
	}
	dir := a.packPath(name)
	r := &PackLaunch{Pack: p, Check: check, UserData: filepath.Join(dir, "userdata"), DryRun: dryRun}
	r.ModsConfig = env.ModsConfigPath(r.UserData)
	// What Mods/ holds besides this pack (08 §5.3 step 2): a second folder with a pack copy's
	// packageId makes the game report an error and ignore one of them, so a pack copy is
	// never linked next to one. A copy of the player's (a folder, or another tool's link)
	// with the same content stands in for the pack's; with other content the launch is
	// refused. A plan's _Archo_ link of the same packageId is removed for the run (writing
	// the plan to the game makes it again).
	byPkg := map[string][]ModsCopy{}
	copies := a.ModsCopies(st.Ix)
	for _, c := range copies {
		byPkg[c.PackageID] = append(byPkg[c.PackageID], c)
	}
	cands := Candidates(st.Ix, st.Aliases)
	want := map[string]string{}         // link name (lower) → target
	useLocal := map[string]*store.Mod{} // pack folder → the player's copy that stands in
	dropDeploy := map[string]bool{}     // _Archo_ link names (lower) that would duplicate
	var dups []DuplicateMove
	for _, pe := range p.Entries {
		if pe.Official {
			continue
		}
		var player []ModsCopy
		for _, c := range byPkg[strings.ToLower(pe.PackageID)] {
			switch c.By {
			case CopyDeploy:
				dropDeploy[strings.ToLower(c.Name)] = true
			case CopyPlayer:
				player = append(player, c)
			}
		}
		switch {
		case len(player) == 0:
			link := packLinkName(name, pe.Folder)
			want[strings.ToLower(link)] = filepath.Join(dir, "mods", pe.Folder)
			cands = append(cands, loadout.Candidate{ID: "pack:" + pe.Folder, PackageID: pe.PackageID, Kind: source.Local, Folder: link})
		case len(player) == 1 && packSameContent(pe, player[0].Mod):
			useLocal[pe.Folder] = player[0].Mod
		default:
			// Another version in the way: it may move into the lock library, after which
			// the pack's copy is linked as usual (the dry run shows it so).
			link := packLinkName(name, pe.Folder)
			for _, c := range player {
				dups = append(dups, DuplicateMove{Copy: c, For: fmt.Sprintf("pack %s's copy of %s", name, pe.Name), Pack: name, Mod: pe.Name, Link: link})
			}
			want[strings.ToLower(link)] = filepath.Join(dir, "mods", pe.Folder)
			cands = append(cands, loadout.Candidate{ID: "pack:" + pe.Folder, PackageID: pe.PackageID, Kind: source.Local, Folder: link})
		}
	}
	if len(dups) > 0 {
		de := &DuplicatesError{Moves: a.planMoves(st, copies, dups), Fix: "archo pack launch " + quoteRef(name) + " --move-duplicates"}
		if !move || !de.Movable() {
			if !de.Movable() {
				de.Fix = ""
			}
			return nil, de
		}
		if !dryRun {
			// Move them, then launch from the state after the move.
			if _, err := a.doMoves(de.Moves); err != nil {
				return nil, err
			}
			r2, err := a.launchPackWith(name, false, false)
			if r2 != nil {
				r2.Moved = de.Moves
			}
			return r2, err
		}
		r.Moved = de.Moves
	}
	reg := loadout.NewRegistry(cands)
	for _, pe := range p.Entries {
		if pe.Official {
			r.Tokens = append(r.Tokens, pe.PackageID)
			continue
		}
		if m := useLocal[pe.Folder]; m != nil {
			r.Tokens = append(r.Tokens, reg.Token(m.ID))
			r.InMods = append(r.InMods, pe.Name)
			continue
		}
		id := "pack:" + pe.Folder
		if !reg.Loadable(id) {
			return nil, fmt.Errorf("%w: the game would not load the pack's copy of %s", ErrLaunchBlocked, pe.Name)
		}
		r.Tokens = append(r.Tokens, reg.Token(id))
	}
	// Links: other packs' and the duplicating _Archo_ ones go, this pack's are made (or fixed).
	ents, _ := os.ReadDir(mods)
	have := map[string]bool{}
	for _, de := range ents {
		n := de.Name()
		lp := filepath.Join(mods, n)
		if dropDeploy[strings.ToLower(n)] && source.IsLink(lp) {
			tgt, _ := source.RealPath(lp)
			r.Links = append(r.Links, LinkChange{Op: "remove", Name: n, Target: tgt})
			continue
		}
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
		if pe.Official || useLocal[pe.Folder] != nil {
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
	if err := a.applyLinks(mods, r.Links); err != nil {
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
	for _, m := range installedOfficial(st.Ix.Mods) {
		if m.PackageID != "ludeon.rimworld" {
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
	// A pack's launch has no plan; its fingerprint is the pack's list (08 §1).
	return r, a.Store.AddLaunchRecord(&store.LaunchRecord{EnvID: e.ID, Started: a.Now().UTC(), Method: "pack:" + name,
		Command: r.Plan.Command(), Fingerprint: TokenFingerprint(r.Tokens)})
}

// ---- export, import, remove ----

// ExportPack writes a pack to a file (.archopack, without its saves), or
// with listOnly its list as a RimArcho list (.archolist.json, 04 §9.3).
func (a *App) ExportPack(name, file string, listOnly bool) (int64, error) {
	p, err := a.ReadPack(name)
	if err != nil {
		return 0, err
	}
	if _, err := os.Lstat(file); err == nil {
		return 0, fmt.Errorf("%s exists; nothing is overwritten", file)
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	if listOnly || p.Light {
		q := *p
		q.Light = true
		q.Entries = append([]PackEntry(nil), p.Entries...)
		for i := range q.Entries {
			q.Entries[i].Folder, q.Entries[i].Bytes, q.Entries[i].Trimmed, q.Entries[i].TrimmedDir = "", 0, 0, nil
			q.Entries[i].Files, q.Entries[i].SHA256 = 0, ""
		}
		q.Trimmed, q.Textures = 0, nil // a list carries no files
		b, _ := json.MarshalIndent(&q, "", "  ")
		f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		n, err := f.Write(append(b, '\n'))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return int64(n), err
	}
	return a.exportPackArchive(name, file, p)
}

// PackMatch compares a RimArcho list (a pack.json without files) with the installed mods
// (08 §5.4), or says where a pack was unpacked.
type PackMatch struct {
	Pack            *Pack
	Same, Different []PackEntry // installed with the same content / another version
	Missing         []PackEntry
	Imported        *EditResult // with asPlan: the workspace now holds the list
	Dir             string      // a pack: where it was unpacked
	Verified        bool        // false for format 1, whose copies have no content digests
	Check           *PackCheck
}

// IsPackZip recognizes both ZIP and zstd packs by magic, for existing GUI callers.
func IsPackZip(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [4]byte
	_, err = f.Read(magic[:])
	return err == nil && (string(magic[:]) == "PK\x03\x04" || string(magic[:]) == "\x28\xb5\x2f\xfd")
}

// ImportPack reads a pack file. A pack (.archopack) is unpacked into the packs folder; a
// RimArcho list (.archolist.json, or a pack.json) is compared with the installed mods and,
// with asPlan, loaded into the workspace (loadout import reads it too, 04 §9.2).
func (a *App) ImportPack(file string, asPlan bool) (*PackMatch, error) {
	if IsPackZip(file) {
		return a.importPackArchive(file)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var p Pack
	if err := json.Unmarshal(b, &p); err != nil || p.Format == 0 {
		return nil, fmt.Errorf("%s is neither a pack (.archopack) nor a RimArcho list (.archolist.json)", file)
	}
	if p.Format < 1 || p.Format > PackFormat {
		return nil, fmt.Errorf("%s has format %d; this RimArcho reads up to %d", file, p.Format, PackFormat)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	r := &PackMatch{Pack: &p}
	for _, pe := range p.Entries {
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
		if r.Imported, err = a.importList(archoList(&p), p.Name); err != nil {
			return nil, err
		}
	}
	r.Check = a.CheckPack(&p, st.Ix.Env, nil)
	r.Check.missingMods(r.Missing)
	return r, nil
}

// RemovePack moves a pack to the recycle bin, after removing its links.
func (a *App) RemovePack(name string, dryRun bool) (string, error) {
	if _, err := a.ReadPack(name); err != nil {
		return "", err
	}
	dir := a.packPath(name)
	if _, err := a.PlanIncomingLinks(dir, ""); err != nil {
		return "", err
	}
	if dryRun {
		return dir, nil
	}
	return dir, a.ToTrash(dir)
}

// quoteRef quotes a name for a command line when it has a space.
func quoteRef(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
