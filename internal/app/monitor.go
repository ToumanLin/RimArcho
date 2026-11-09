package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Archo Monitor (19 §6, 07 §1.5): RimArcho's in-game companion mod, supplied by any
// source. The bundled copy is installed as src:monitor; plans add Monitor after Harmony.

const (
	// MonitorPackage is Archo Monitor's packageId.
	MonitorPackage = "toumanlin.archomonitor"
	// MonitorLabel is the label of the monitor folder's source.
	MonitorLabel = "monitor"
	// MonitorNone in App.MonitorBundle turns the bundled monitor off (tests).
	MonitorNone   = "none"
	monitorFolder = "ArchoMonitor-"
	harmonyPkg    = "brrainz.harmony"
	// MonitorAddOp names the default insertion in the plan's undo stack (07 §1.5).
	MonitorAddOp = "add Archo Monitor after Harmony (default; archo config set monitor.default off)"
)

var monitorVersionRe = regexp.MustCompile(`^[0-9A-Za-z.+-]+$`)

// MonitorBundled is the monitor RimArcho ships: a mod folder and its version.
type MonitorBundled struct {
	Dir, Version string
}

// MonitorDir is where installed monitor versions live (19 §6): config monitor.dir, else
// %LOCALAPPDATA%\RimArcho\monitor, outside the data directory like the lock library.
func (a *App) MonitorDir() string {
	if v := a.Store.Meta("cfg:monitor.dir"); v != "" {
		return v
	}
	return filepath.Join(a.LocalDir(), "monitor")
}

// MonitorDefault reports whether the monitor is added to plans by default (07 §1.5).
func (a *App) MonitorDefault() bool { return a.Store.Meta("cfg:monitor.default") != "off" }

// FindMonitor finds the bundled monitor (19 §6): RIMARCHO_MONITOR, monitor/ArchoMonitor next
// to the executable, or, in a development build, monitor/out/ArchoMonitor above the
// executable or the working directory. tried lists the places looked at.
func (a *App) FindMonitor() (b *MonitorBundled, tried []string) {
	var cands []string
	switch {
	case a.MonitorBundle == MonitorNone:
		return nil, []string{"(turned off)"}
	case a.MonitorBundle != "":
		cands = []string{a.MonitorBundle}
	default:
		if v := os.Getenv("RIMARCHO_MONITOR"); v != "" {
			cands = append(cands, v)
		}
		exeDir := ""
		if exe, err := os.Executable(); err == nil {
			exeDir = filepath.Dir(exe)
			cands = append(cands, filepath.Join(exeDir, "monitor", "ArchoMonitor"))
		}
		if buildinfo.Version == "dev" {
			wd, _ := os.Getwd()
			for _, start := range []string{exeDir, wd} {
				for d, i := start, 0; d != "" && i < 8; d, i = filepath.Dir(d), i+1 {
					cands = append(cands, filepath.Join(d, "monitor", "out", "ArchoMonitor"))
					if filepath.Dir(d) == d {
						break
					}
				}
			}
		}
	}
	for _, c := range cands {
		tried = append(tried, c)
		if v, err := monitorVersion(c); err == nil {
			return &MonitorBundled{Dir: c, Version: v}, tried
		}
	}
	return nil, tried
}

// monitorVersion reads the monitor's About.xml: its packageId must be Archo Monitor's and
// its modVersion becomes the folder name's version.
func monitorVersion(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "About", "About.xml")); err != nil {
		return "", err
	}
	about := scan.ReadDecl(dir, false)
	if about.ParseError != "" {
		return "", errors.New(about.ParseError)
	}
	if !strings.EqualFold(about.PackageID, MonitorPackage) {
		return "", fmt.Errorf("%s is not Archo Monitor (packageId %s)", dir, about.PackageID)
	}
	v := strings.TrimSpace(about.ModVersion)
	if v == "" || !monitorVersionRe.MatchString(v) {
		return "", fmt.Errorf("%s: no usable modVersion (%q)", dir, v)
	}
	return v, nil
}

// MonitorInstance is a monitor instance of the current environment with its version.
type MonitorInstance struct {
	Mod     *store.Mod
	Version string
}

// monitorInstances are the environment's usable Monitor instances, newest first.
// Workshop wins a version tie because it needs no Mods/ link (19 §6).
func monitorInstances(ix *Index) []MonitorInstance {
	var out []MonitorInstance
	for _, m := range ix.Mods {
		if !IsMonitor(m) || m.Status != identity.Live || m.Residue || m.Unloadable != "" {
			continue
		}
		out = append(out, MonitorInstance{Mod: m, Version: strings.TrimSpace(m.Decl().ModVersion)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if cmp := CompareVersions(out[i].Version, out[j].Version); cmp != 0 {
			return cmp > 0
		}
		return out[i].Mod.Kind == source.Workshop && out[j].Mod.Kind != source.Workshop
	})
	return out
}

func monitorSourceOf(ix *Index) *source.Source {
	for _, s := range ix.Sources {
		if s.Kind == source.Extra && s.Status != source.StatusRetired && s.Label == MonitorLabel {
			return s
		}
	}
	return nil
}

// IsMonitor reports whether m is an Archo Monitor instance (any source).
func IsMonitor(m *store.Mod) bool {
	return m != nil && strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam") == MonitorPackage
}

// CompareVersions orders semantic versions: numeric parts first, then a release above its
// pre-releases ("0.9.0" > "0.9.0-dev"), then pre-release text.
func CompareVersions(a, b string) int {
	split := func(v string) ([]int, string) {
		pre := ""
		if i := strings.IndexAny(v, "-+"); i >= 0 {
			v, pre = v[:i], v[i+1:]
		}
		var ns []int
		for _, p := range strings.Split(v, ".") {
			n, _ := strconv.Atoi(p)
			ns = append(ns, n)
		}
		return ns, pre
	}
	na, pa := split(a)
	nb, pb := split(b)
	for i := 0; i < max(len(na), len(nb)); i++ {
		x, y := 0, 0
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	return strings.Compare(pa, pb)
}

// MonitorStatus is what `archo monitor` and `archo status` show (19 §6).
type MonitorStatus struct {
	Bundled   *MonitorBundled
	Tried     []string
	Dir       string
	Installed []string // version folders in the monitor directory
	Source    *source.Source
	Newest    *MonitorInstance
	// In the current plan: the instance, its 1-based position, and whether it follows Harmony.
	InPlan       *store.Mod
	Position     int
	AfterHarmony bool
	Default      bool
}

// MonitorState reports the monitor of the current environment and plan.
func (a *App) MonitorState() (*MonitorStatus, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	s := &MonitorStatus{Dir: a.MonitorDir(), Default: a.MonitorDefault()}
	s.Bundled, s.Tried = a.FindMonitor()
	s.Installed = installedMonitors(s.Dir)
	s.Source = monitorSourceOf(st.Ix)
	if ms := monitorInstances(st.Ix); len(ms) > 0 {
		s.Newest = &ms[0]
	}
	for i, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; IsMonitor(m) {
			s.InPlan, s.Position = m, i+1
			if i > 0 {
				prev := st.Ix.ByID[st.List[i-1].Instance]
				s.AfterHarmony = prev != nil && strings.TrimSuffix(strings.ToLower(prev.PackageID), "_steam") == harmonyPkg
			}
			break
		}
	}
	return s, nil
}

func installedMonitors(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if e.IsDir() && strings.HasPrefix(e.Name(), monitorFolder) && !strings.Contains(e.Name(), ".archo-") {
			out = append(out, strings.TrimPrefix(e.Name(), monitorFolder))
		}
	}
	sort.Slice(out, func(i, j int) bool { return CompareVersions(out[i], out[j]) > 0 })
	return out
}

// MonitorPlanChange is one plan the check changed or left alone, and why.
type MonitorPlanChange struct {
	Plan   string
	Change string // "added", "updated <old> → <new>", or "" when skipped
	Reason string // why it was skipped
}

// MonitorResult is what one check did (19 §6 "when to check").
type MonitorResult struct {
	DryRun     bool
	Bundled    *MonitorBundled
	Tried      []string
	Target     string // the version folder
	Copied     bool   // a new version folder was assembled
	Replaced   bool   // a pre-release folder with other content was replaced
	Deferred   string // why a replacement waits
	Registered bool   // the environment's src:monitor was added
	Scanned    bool
	Merged     int // monitor instances folded into the newest one's lineage
	Plans      []MonitorPlanChange
	Notes      []string
	Err        error // ScanWithMonitor: the check failed, the scan did not

	stop bool // the monitor directory is unavailable: leave the plans alone
}

// EnsureMonitor installs or updates the bundled monitor for the current environment and
// applies the default to its plans (19 §6, 07 §1.5). Without a bundled monitor it does
// nothing to disk and only adds already installed monitors to plans.
func (a *App) EnsureMonitor(dryRun bool) (*MonitorResult, error) {
	r := &MonitorResult{DryRun: dryRun}
	read, err := a.prepareMonitor(r, dryRun)
	if err != nil || dryRun || r.stop {
		return r, err
	}
	if read != "" {
		// Only the monitor's own folder changed; the other sources keep their instances.
		if _, err := a.ScanWith(ScanOptions{Sources: []string{read}}); err != nil {
			return r, err
		}
		r.Scanned = true
	}
	return r, a.finishMonitor(r)
}

// ScanWithMonitor is `archo scan` (04 §1.1, 19 §6): it installs or updates the bundled
// monitor and registers its source first, so the one scan reads the monitor's folder with
// the others, then applies the monitor to the plans. A monitor failure does not fail the
// scan; it is in the result's Err.
func (a *App) ScanWithMonitor(o ScanOptions) (*ScanReport, *MonitorResult, error) {
	m := &MonitorResult{}
	read, merr := a.prepareMonitor(m, false)
	if merr == nil && read != "" {
		m.Scanned = true
		if o.GameOnly || len(o.Sources) > 0 { // a scan that leaves shared folders out
			o.Sources = append(slices.Clone(o.Sources), read)
		}
	}
	rep, err := a.ScanWith(o)
	if err != nil {
		return nil, nil, err
	}
	if merr == nil && !m.stop {
		merr = a.finishMonitor(m)
	}
	m.Err = merr
	return rep, m, nil
}

// prepareMonitor installs the bundled monitor and registers src:monitor (19 §6 steps 1–3).
// read is the monitor source's ID when its folder must be read before finishMonitor.
func (a *App) prepareMonitor(r *MonitorResult, dryRun bool) (read string, err error) {
	r.Bundled, r.Tried = a.FindMonitor()
	b := r.Bundled
	if b == nil {
		return "", nil
	}
	ix, err := a.Index()
	if err != nil {
		return "", err
	}
	dir := a.MonitorDir()
	r.Target = filepath.Join(dir, monitorFolder+b.Version)
	if err := a.managedRootReady("monitor", dir); err != nil {
		r.Notes = append(r.Notes, err.Error())
		r.Deferred = "The monitor directory is unavailable. Choose its current location before installing an update."
		r.stop = true
		return "", nil
	}
	changed, err := a.installMonitor(r, b, dryRun)
	if err != nil {
		return "", err
	}
	src := monitorSourceOf(ix)
	if src == nil {
		r.Registered = true
		if dryRun {
			return "", nil
		}
		if src, err = a.registerMonitorSource(dir); err != nil {
			return "", err
		}
		return src.ID, nil
	}
	if env.PathKey(src.Root) != env.PathKey(dir) {
		r.Notes = append(r.Notes, fmt.Sprintf("src:%s is at %s, not %s (monitor.dir changed?); RimArcho keeps using it", MonitorLabel, src.Root, dir))
	}
	if changed && !dryRun {
		return src.ID, nil
	}
	return "", nil
}

// finishMonitor folds the monitor instances into one lineage and applies the newest to
// every plan (19 §6 step 5), after the monitor's folder was read.
func (a *App) finishMonitor(r *MonitorResult) error {
	ix, err := a.Index()
	if err != nil {
		return err
	}
	ms := monitorInstances(ix)
	if len(ms) == 0 {
		return nil
	}
	newest := ms[0]
	for _, o := range ms[1:] {
		if o.Mod.LineageID != newest.Mod.LineageID {
			if err := a.MergeLineages(ix, newest.Mod.LineageID, o.Mod.LineageID); err != nil {
				return err
			}
			r.Merged++
		}
	}
	if r.Merged > 0 {
		if ix, err = a.Index(); err != nil {
			return err
		}
		ms = monitorInstances(ix)
		newest = ms[0]
	}
	plans, err := a.Store.Plans()
	if err != nil {
		return err
	}
	for _, p := range plans {
		c, err := a.monitorPlan(ix, p, newest)
		if err != nil {
			return err
		}
		if c.Change != "" || c.Reason != "" {
			r.Plans = append(r.Plans, c)
		}
	}
	return nil
}

// installMonitor copies the bundled monitor into its version folder; changed reports a copy.
func (a *App) installMonitor(r *MonitorResult, b *MonitorBundled, dryRun bool) (bool, error) {
	want, err := treeDigest(b.Dir)
	if err != nil {
		return false, fmt.Errorf("reading the bundled monitor %s: %w", b.Dir, err)
	}
	if _, err := os.Stat(r.Target); err == nil {
		have, err := treeDigest(r.Target)
		if err == nil && have == want {
			return false, nil
		}
		if !strings.ContainsAny(b.Version, "-+") {
			r.Notes = append(r.Notes, fmt.Sprintf("%s differs from the bundled Archo Monitor %s, but a release folder is never replaced", r.Target, b.Version))
			return false, nil
		}
		if g, _ := a.RunningGame(); g != nil {
			r.Deferred = "a game is running; the monitor folder is replaced the next time"
			return false, nil
		}
		r.Replaced = true
	} else {
		r.Copied = true
	}
	if dryRun {
		return true, nil
	}
	if err := a.checkRoots(filepath.Dir(r.Target)); err != nil {
		return false, err
	}
	tmp := r.Target + ".archo-tmp"
	os.RemoveAll(tmp)
	if err := copyPlainTree(b.Dir, tmp); err != nil {
		os.RemoveAll(tmp)
		return false, err
	}
	if r.Replaced {
		old := r.Target + ".archo-old"
		os.RemoveAll(old)
		if err := os.Rename(r.Target, old); err != nil {
			os.RemoveAll(tmp)
			return false, fmt.Errorf("replacing %s: %w", r.Target, err)
		}
		if err := os.Rename(tmp, r.Target); err != nil {
			os.Rename(old, r.Target)
			return false, err
		}
		os.RemoveAll(old) // RimArcho's own product (19 §6)
		return true, nil
	}
	return true, os.Rename(tmp, r.Target)
}

// registerMonitorSource adds src:monitor, a shared extra source (03 §2 item 10).
func (a *App) registerMonitorSource(dir string) (*source.Source, error) {
	if err := a.checkRoots(dir); err != nil {
		return nil, err
	}
	if o, err := a.overlapping(dir, nil); err != nil {
		return nil, err
	} else if o != nil {
		return nil, fmt.Errorf("the monitor folder %s overlaps src:%s (%s); choose another folder with archo config set monitor.dir <dir>", dir, o.Label, o.Root)
	}
	return a.sharedSource(MonitorLabel, dir)
}

func monitorAddedKey(planID int64) string { return store.MonitorAddedKey(planID) }

// monitorPlan updates a plan's older monitor to the newest, or adds the monitor right after
// Harmony once (07 §1.5). Each change is one undoable edit of that plan.
func (a *App) monitorPlan(ix *Index, p *store.Plan, newest MonitorInstance) (MonitorPlanChange, error) {
	c := MonitorPlanChange{Plan: p.Name}
	unlock, err := a.lockLaunch()
	if err != nil {
		return c, err
	}
	defer unlock()
	ws, err := a.planWorkspace(p)
	if err != nil {
		return c, err
	}
	list := loadout.Decode(ws.Entries)
	after := make(loadout.List, 0, len(list))
	old, found := "", false
	for _, e := range list {
		m := ix.ByID[e.Instance]
		if !IsMonitor(m) && packageBase(strings.ToLower(e.Missing)) != MonitorPackage {
			after = append(after, e)
			continue
		}
		if !found {
			if m != nil {
				old = strings.TrimSpace(m.Decl().ModVersion)
			}
			after = append(after, loadout.Entry{Instance: newest.Mod.ID})
			found = true
		}
	}
	if found {
		a.Store.SetMeta(monitorAddedKey(p.ID), "1")
		if after.Equal(list) {
			return c, nil
		}
		op := fmt.Sprintf("update Archo Monitor %s → %s", old, newest.Version)
		c.Change = fmt.Sprintf("updated %s → %s", old, newest.Version)
		if old == newest.Version {
			// Same version from another source, such as the subscribed Workshop copy.
			op = fmt.Sprintf("use the %s copy of Archo Monitor %s", newest.Mod.Kind, old)
			c.Change = fmt.Sprintf("now uses the %s copy", newest.Mod.Kind)
		}
		if err := a.editPlan(p, ws, list, after, op); err != nil {
			return c, err
		}
		return c, nil
	}
	if a.Store.Meta(monitorAddedKey(p.ID)) != "" {
		return c, nil
	}
	if !a.MonitorDefault() {
		c.Reason = "monitor.default is off"
		return c, nil
	}
	at, why := a.monitorSlot(ix, list, newest.Mod)
	if why != "" {
		c.Reason = why
		return c, nil
	}
	after = insert(list.Clone(), at, loadout.Entry{Instance: newest.Mod.ID})
	if err := a.editPlan(p, ws, list, after, MonitorAddOp); err != nil {
		return c, err
	}
	a.Store.SetMeta(monitorAddedKey(p.ID), "1")
	c.Change = "added after Harmony"
	return c, nil
}

// monitorSlot is where the monitor goes in a list: right after Harmony. why says why it
// cannot go in (no Harmony, Mods/ cannot hold links).
func (a *App) monitorSlot(ix *Index, list loadout.List, monitor *store.Mod) (at int, why string) {
	at = -1
	for i, e := range list {
		if m := ix.ByID[e.Instance]; m != nil && strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam") == harmonyPkg {
			at = i + 1
			break
		}
	}
	if at < 0 {
		return -1, "no Harmony in the plan; Archo Monitor needs it"
	}
	if !gameVisible(monitor.Kind) {
		if fs := a.modsVolumeFS(ix); noLinks(fs) {
			return -1, fmt.Sprintf("Mods/ is on %s, which cannot hold the link Archo Monitor needs (19 §6)", fs)
		}
	}
	return at, ""
}

// MonitorAddUserOp names AddMonitor's step in the undo stack.
const MonitorAddUserOp = "add Archo Monitor after Harmony"

// AddMonitor puts the newest installed Archo Monitor right after Harmony in the current
// plan, as one undoable edit: the default insertion (07 §1.5) on the player's request, also
// after they removed it. It returns the instance added.
func (a *App) AddMonitor() (*store.Mod, error) {
	var added *store.Mod
	var planID int64
	_, err := a.edit(MonitorAddUserOp, func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		planID = st.Plan.ID
		for _, e := range l {
			if IsMonitor(st.Ix.ByID[e.Instance]) {
				return nil, nil, errors.New("Archo Monitor is already in the plan")
			}
		}
		ms := monitorInstances(st.Ix)
		if len(ms) == 0 {
			return nil, nil, errors.New("Archo Monitor is not installed (archo monitor install)")
		}
		at, why := a.monitorSlot(st.Ix, l, ms[0].Mod)
		if why != "" {
			return nil, nil, errors.New(why)
		}
		added = ms[0].Mod
		return insert(l, at, loadout.Entry{Instance: added.ID}), nil, nil
	})
	if err != nil {
		return nil, err
	}
	a.Store.SetMeta(monitorAddedKey(planID), "1")
	return added, nil
}

// editPlan replaces a plan's list as one undoable edit of that plan (current or not).
func (a *App) editPlan(p *store.Plan, ws *store.Workspace, before, after loadout.List, op string) error {
	unlock, err := a.lockLaunch()
	if err != nil {
		return err
	}
	defer unlock()

	fresh, err := a.Store.Workspace(p.ID)
	if err != nil {
		return err
	}
	if fresh == nil || fresh.Entries != ws.Entries {
		return fmt.Errorf("list changed while editing; reload and try again")
	}

	if after.Equal(before) {
		return nil
	}
	ws.Entries, ws.Updated = after.Encode(), a.Now().UTC()
	if ws.OriginKind == "checkout" {
		ws.OriginKind = "edited"
	}
	return a.Store.EditWorkspace(ws, before.Encode(), op, nil)
}

// MonitorForNewPlan applies the default to a plan just created (07 §1.5): no install, no
// scan; only an installed monitor is added.
func (a *App) MonitorForNewPlan(p *store.Plan) (string, error) {
	ix, err := a.Index()
	if err != nil {
		return "", err
	}
	ms := monitorInstances(ix)
	if len(ms) == 0 {
		return "", nil
	}
	c, err := a.monitorPlan(ix, p, ms[0])
	return c.Change, err
}

// modsVolumeFS is the file system of the volume holding Mods/ ("" if unknown).
func (a *App) modsVolumeFS(ix *Index) string {
	dir := modsDir(ix)
	if dir == "" {
		return ""
	}
	f := a.VolumeFS
	if f == nil {
		f = source.VolumeFS
	}
	return f(dir)
}

// noLinks reports file systems without directory junctions (09 §3).
func noLinks(fs string) bool {
	switch strings.ToUpper(fs) {
	case "FAT", "FAT12", "FAT16", "FAT32", "EXFAT":
		return true
	}
	return false
}

// treeDigest fingerprints a folder: every file's relative path, size and SHA-256.
func treeDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		fh := sha256.New()
		n, err := io.Copy(fh, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%x\n", filepath.ToSlash(strings.ToLower(rel)), n, fh.Sum(nil))
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// copyPlainTree copies regular files and folders; links are refused (the bundle has none).
func copyPlainTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !d.Type().IsRegular() {
			return errors.New(p + " is not a regular file")
		}
		return copyPlain(p, dst)
	})
}
