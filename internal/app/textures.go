package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Texture optimisation (10 §2, docs/research/dds-texture-optimization.md): the DDS of a mod's PNGs, written next to
// them. The game (1.6 and 1.7) loads a .dds in place of the .png of the same name: no PNG to
// decode at start, and opaque textures take half the video memory (BC1, where the game makes
// BC7 or DXT5 of a PNG). The PNGs stay; RimArcho records every file it writes and only ever
// deletes those. Packs convert their own copies instead (pack.go) and drop the PNGs.

// ErrNoTodds: the encoder is not on this computer.
var ErrNoTodds = errors.New("todds (the DDS encoder) is not installed; archo textures tool --install downloads it")

// ToddsDir is where RimArcho keeps the todds it downloads: %LOCALAPPDATA%\RimArcho\todds.
func (a *App) ToddsDir() string { return filepath.Join(a.LocalDir(), "todds") }

// ToddsPath finds todds: config textures.todds, RimArcho's own copy, then PATH ("" none).
func (a *App) ToddsPath() string {
	return a.Deps().Status(deps.Todds).Path
}

// TextureTool is the encoder found, or why there is none.
type TextureTool struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Err     string `json:"err,omitempty"`
}

// Tool finds todds and checks it is the version RimArcho expects.
func (a *App) TextureTool() *TextureTool {
	if a.TextureEncoder != nil {
		return &TextureTool{Path: "(test)", Version: textures.ToddsVersion}
	}
	i := a.Deps().Status(deps.Todds)
	t := &TextureTool{Path: i.Path, Version: i.Version, Err: i.Err}
	if t.Path == "" {
		t.Err = ErrNoTodds.Error()
		return t
	}
	return t
}

func (a *App) encoder(log io.Writer) (textures.Encoder, error) {
	if a.TextureEncoder != nil {
		return a.TextureEncoder, nil
	}
	t := a.TextureTool()
	if t.Err != "" {
		if t.Path == "" {
			return nil, ErrNoTodds
		}
		return nil, errors.New(t.Err)
	}
	return &textures.Todds{Path: t.Path, Prepare: hideWindow, Log: log}, nil
}

// ToddsOffer is what downloading todds would do.
type ToddsOffer struct {
	URL     string `json:"url"`
	Dir     string `json:"dir"`
	Bytes   int64  `json:"bytes"`
	Version string `json:"version"`
	Home    string `json:"home"` // the project page (MPL-2.0)
}

func (a *App) toddsURL() string {
	if a.ToddsZip != "" {
		return a.ToddsZip
	}
	return textures.ToddsZipURL
}

// OfferTodds says where todds comes from and how big it is, so the player can be asked
// first (02 §1: RimArcho only goes online when the player asks).
func (a *App) OfferTodds() *ToddsOffer {
	return &ToddsOffer{URL: a.toddsURL(), Dir: a.ToddsDir(), Bytes: textures.ToddsZipSize, Version: textures.ToddsVersion, Home: textures.ToddsHome}
}

// InstallTodds downloads the todds release zip (its size must be the release's), unpacks
// todds.exe and its licence into ToddsDir and checks that it runs and is the right version.
// It runs only when the player said yes to OfferTodds.
func (a *App) InstallTodds() (string, error) {
	return a.InstallToddsProgress(nil)
}

// InstallToddsProgress reports the download to the task record (16 §2.16).
func (a *App) InstallToddsProgress(progress func(done, total int64)) (string, error) {
	unlock := accounts.Lock(a.ToddsDir())
	defer unlock()
	if err := toolIdle("todds", a.ToddsDir()); err != nil {
		return "", err
	}
	dir := a.ToddsDir()
	c := a.httpClient()
	if c.Timeout != 0 && c.Timeout < 2*time.Minute {
		cc := *c
		cc.Timeout = 2 * time.Minute
		c = &cc
	}
	resp, err := c.Get(a.toddsURL())
	if err != nil {
		return "", fmt.Errorf("downloading todds: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading todds: %s", resp.Status)
	}
	b, err := readToolDownload(resp, progress)
	if err != nil {
		return "", fmt.Errorf("downloading todds: %w", err)
	}
	if a.ToddsZip == "" && int64(len(b)) != textures.ToddsZipSize {
		return "", fmt.Errorf("the todds download is %d bytes, the release lists %d: not the file RimArcho expects", len(b), textures.ToddsZipSize)
	}
	if a.ToddsZip == "" && fmt.Sprintf("%x", sha256.Sum256(b)) != textures.ToddsZipSHA256 {
		return "", errors.New("the todds download does not match the 0.4.1 release: not the file RimArcho expects")
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("the todds download is not a zip: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	exe := ""
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.Contains(f.Name, "..") {
			continue
		}
		name := strings.ToLower(filepath.Base(filepath.FromSlash(f.Name)))
		var to string
		switch {
		case name == "todds.exe" || name == "todds":
			to = filepath.Join(dir, filepath.Base(f.Name))
		case strings.HasPrefix(name, "license") || strings.HasPrefix(name, "copying"):
			to = filepath.Join(dir, "LICENSE.txt")
		default:
			continue
		}
		if err := writeZipFile(f, to); err != nil {
			return "", err
		}
		if !strings.HasPrefix(name, "license") && !strings.HasPrefix(name, "copying") {
			exe = to
		}
	}
	if exe == "" {
		return "", errors.New("the todds download has no todds program")
	}
	if a.TextureEncoder == nil {
		td := &textures.Todds{Path: exe, Prepare: hideWindow}
		if v, err := td.Version(); err != nil {
			return "", err
		} else if v != textures.ToddsVersion {
			return "", fmt.Errorf("the download is todds %s, not %s", v, textures.ToddsVersion)
		}
	}
	return exe, nil
}

func writeZipFile(f *zip.File, to string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	w, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, io.LimitReader(rc, 256<<20))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}

// ---- which mods ----

// Texture scopes: the mods named, the current plan's, or every installed mod.
const (
	TexRefs = ""
	TexPlan = "plan"
	TexAll  = "all"
)

// textureMods are the mods a command acts on: never Core or a DLC (the game's own folder),
// never a mod that is gone.
func (a *App) textureMods(refs []string, scope string) (*LoadoutState, []*store.Mod, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, nil, err
	}
	var out []*store.Mod
	seen := map[string]bool{}
	add := func(m *store.Mod) {
		if m == nil || seen[m.ID] || m.Kind == source.Official || !m.TombstoneAt.IsZero() {
			return
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	switch scope {
	case TexPlan:
		for _, e := range st.List {
			add(st.Ix.ByID[e.Instance])
		}
	case TexAll:
		for _, m := range st.Ix.Mods {
			add(m)
		}
	default:
		if len(refs) == 0 {
			return nil, nil, errors.New("name the mods, or use --plan or --all")
		}
		for _, r := range refs {
			ms, err := st.Ix.Resolve(r)
			if err != nil {
				return nil, nil, err
			}
			for _, m := range ms {
				if m.Kind == source.Official {
					return nil, nil, fmt.Errorf("%s is part of the game; RimArcho does not change the game's files", m.DisplayName())
				}
				add(m)
			}
		}
	}
	return st, out, nil
}

// modDir is the mod's real folder.
func modDir(ix *Index, m *store.Mod) string {
	d := ix.ModDir(m)
	if real, err := source.RealPath(d); err == nil {
		return real
	}
	return d
}

// TextureTarget is the small, immutable input of a page read. Capture it while the UI
// App is locked; a separate reader then needs no second copy of the full Mod index.
type TextureTarget struct {
	Mod      *store.Mod
	Ref, Dir string
}

func (a *App) TextureTargets(refs []string, scope string) ([]TextureTarget, error) {
	st, mods, err := a.textureMods(refs, scope)
	if err != nil {
		return nil, err
	}
	out := make([]TextureTarget, 0, len(mods))
	for _, m := range mods {
		// Only these fields are read by the summary/view, with no shared mutable Mod.
		copy := &store.Mod{ID: m.ID, Name: m.Name, Alias: m.Alias, Rel: m.Rel}
		out = append(out, TextureTarget{Mod: copy, Ref: st.Ix.Ref(m.ID), Dir: st.Ix.ModDir(m)})
	}
	return out, nil
}

// ---- the state of one mod ----

// TextureMod is one mod's textures: what is there, what RimArcho made, what a run would do.
type TextureMod struct {
	Mod  *store.Mod
	Ref  string
	Dir  string
	Plan *textures.Plan // what an optimisation would encode now
	PNG  int            // the PNG textures
	// PNGBytes is their size on disk.
	PNGBytes int64
	Ours     int   // DDS files RimArcho made and still has
	OursSize int64 // their size
	Stale    int   // of Ours: the PNG changed since (the game shows the old texture)
	Orphans  int   // of Ours: the PNG is gone (the DDS adds a texture the mod no longer has)
	Foreign  int   // DDS files RimArcho did not make (the author's, or another tool's)
	Last     *store.TextureMod
	Err      string

	set  *textures.Set
	rows map[string]store.TextureFile // PathKey → row
}

// textureMod reads one mod's textures and decides what a run with o would do; the records
// of DDS files that no longer exist are dropped.
func (a *App) textureMod(ix *Index, m *store.Mod, o textures.Options, force bool, last map[string]store.TextureMod) *TextureMod {
	return a.textureModContext(context.Background(), ix, m, o, force, last)
}

func (a *App) textureModContext(ctx context.Context, ix *Index, m *store.Mod, o textures.Options, force bool, last map[string]store.TextureMod) *TextureMod {
	return a.readTextureMod(ctx, m, ix.Ref(m.ID), modDir(ix, m), o, force, last)
}

func (a *App) readTextureMod(ctx context.Context, m *store.Mod, ref, dir string, o textures.Options, force bool, last map[string]store.TextureMod) *TextureMod {
	t := &TextureMod{Mod: m, Ref: ref, Dir: dir, rows: map[string]store.TextureFile{}}
	if l, ok := last[m.ID]; ok {
		t.Last = &l
	}
	rows, err := a.Store.TextureFilesIn(t.Dir)
	if err != nil {
		t.Err = err.Error()
		return t
	}
	var gone []string
	for _, r := range rows {
		fi, err := os.Stat(r.Path)
		if err != nil {
			gone = append(gone, r.Path)
			continue
		}
		if !r.Pending && (fi.Size() != r.DDSSize || fi.ModTime().UnixNano() != r.DDSMTime) {
			gone = append(gone, r.Path) // rewritten by someone else: no longer ours
			continue
		}
		t.rows[store.PathKey(r.Path)] = r
	}
	a.Store.DeleteTextureFiles(gone)
	cache := filepath.Join(a.Home, "cache", "textures", fmt.Sprintf("%x.json", sha256.Sum256([]byte(store.PathKey(t.Dir)))))
	set, err := textures.CollectCached(ctx, t.Dir, textures.AllFolders(t.Dir), cache)
	if err != nil {
		t.Err = err.Error()
		return t
	}
	t.set = set
	o = o.Normal()
	own := textures.Owned{
		Ours: func(p string) bool { _, ok := t.rows[store.PathKey(p)]; return ok },
		Fresh: func(s *textures.Source) bool {
			r, ok := t.rows[store.PathKey(s.DDS())]
			return ok && !r.Pending && r.SrcSize == s.Size && r.SrcMTime == s.MTime && r.Scale == o.ScaleFor(s) && r.Mips == !o.NoMips
		},
	}
	t.Plan = textures.MakePlan(set, o, own, force)
	srcOf := map[string]*textures.Source{}
	for _, s := range set.Sources {
		t.PNG++
		t.PNGBytes += s.Size
		srcOf[store.PathKey(s.DDS())] = s
	}
	for k, r := range t.rows {
		t.Ours++
		t.OursSize += r.DDSSize
		s := srcOf[k]
		switch {
		case s == nil:
			t.Orphans++
		case s.Size != r.SrcSize || s.MTime != r.SrcMTime:
			t.Stale++
		}
	}
	for _, x := range set.Others {
		if x.Ext == ".dds" && !own.Ours(x.Path) {
			t.Foreign++
		}
	}
	return t
}

// TextureReport is the state of some mods' textures, or what a run did.
type TextureReport struct {
	Options textures.Options
	Mods    []*TextureMod
	Est     textures.Estimate // what encoding the planned textures costs and saves
	Tool    *TextureTool
	DryRun  bool
	// Set by a run.
	Converted  int
	Failed     []TextureFailure
	Removed    int   // DDS deleted: orphans, and the keys a failure left half done
	Written    int64 // DDS bytes written
	Stopped    bool
	Incomplete int // files not done when the run stopped
}

// TextureFailure is a texture that could not be converted.
type TextureFailure struct {
	Mod, Rel, Err string
}

// Totals adds up the mods: PNG files and bytes, RimArcho's DDS files and bytes, stale ones.
func (r *TextureReport) Totals() (png int, pngBytes int64, ours int, oursBytes int64, stale int) {
	for _, m := range r.Mods {
		png += m.PNG
		pngBytes += m.PNGBytes
		ours += m.Ours
		oursBytes += m.OursSize
		stale += m.Stale + m.Orphans
	}
	return
}

// TextureStatus reads the textures of mods (refs, or scope TexPlan / TexAll) and what an
// optimisation with o would do. It updates the PNG header cache and forgets records of
// DDS files that are gone; no texture file is changed.
func (a *App) TextureStatus(refs []string, scope string, o textures.Options) (*TextureReport, error) {
	return a.textureStatus(context.Background(), refs, scope, o)
}

// TextureSummary reads one mod at a time and retains its totals, not all the PNGs and
// conversion jobs. Page reads can be cancelled between files when the player leaves.
func (a *App) TextureSummary(ctx context.Context, refs []string, scope string, o textures.Options) (*TextureReport, error) {
	targets, err := a.TextureTargets(refs, scope)
	if err != nil {
		return nil, err
	}
	return a.TextureSummaryTargets(ctx, targets, o)
}

// TextureSummaryTargets uses only captured names/paths and this reader's own connection.
func (a *App) TextureSummaryTargets(ctx context.Context, targets []TextureTarget, o textures.Options) (*TextureReport, error) {
	return a.TextureSummaryProgress(ctx, targets, o, nil)
}

// TextureSummaryProgress publishes each measured mod so pages can fill rows as they finish (16 §2.13).
func (a *App) TextureSummaryProgress(ctx context.Context, targets []TextureTarget, o textures.Options, progress func(*TextureMod)) (*TextureReport, error) {
	return a.textureSummaries(ctx, targets, o, progress, textureSummaryWorkers)
}

func (a *App) textureStatus(ctx context.Context, refs []string, scope string, o textures.Options) (*TextureReport, error) {
	st, mods, err := a.textureMods(refs, scope)
	if err != nil {
		return nil, err
	}
	last, err := a.Store.TextureMods()
	if err != nil {
		return nil, err
	}
	r := &TextureReport{Options: o.Normal(), Tool: a.TextureTool(), DryRun: true}
	for _, m := range mods {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t := a.textureModContext(ctx, st.Ix, m, o, false, last)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.Mods = append(r.Mods, t)
		if t.Plan != nil {
			r.Est.Plus(t.Plan.Est)
		}
	}
	return r, nil
}

// ---- optimising ----

// TextureRun is an optimisation.
type TextureRun struct {
	Refs    []string
	Scope   string
	Options textures.Options
	Force   bool // re-encode the textures that are up to date too
	DryRun  bool
	Ctx     context.Context // a stop; nil never stops
	// Progress is called as files are done.
	Progress func(TextureProgress)
	Log      io.Writer // todds' output
}

// TextureProgress is how far a run is.
type TextureProgress struct {
	Mod        string `json:"mod"`
	Mods       int    `json:"mods"`
	ModsTotal  int    `json:"modsTotal"`
	Files      int    `json:"files"`
	FilesTotal int    `json:"filesTotal"`
	Bytes      int64  `json:"bytes"`
}

// ErrGameRuns: textures are not written while a game runs (it may be reading them).
var ErrGameRuns = errors.New("a game is running")

// OptimizeTextures writes the DDS of the mods' PNGs next to them (only those a run would
// change: new or changed PNGs, other options). Each file is recorded before todds runs and
// checked after; a key whose conversion failed in any folder loses all of RimArcho's DDS
// (a lone DDS would shadow a newer PNG elsewhere), and RimArcho's DDS whose PNG is gone are
// deleted. The mod's snapshot is kept to tell when an update makes its DDS stale.
func (a *App) OptimizeTextures(run TextureRun) (*TextureReport, error) {
	if !run.DryRun {
		unlock := accounts.Lock(a.ToddsDir())
		defer unlock()
	}
	if g, _ := a.RunningGame(); g != nil && !run.DryRun {
		return nil, fmt.Errorf("%w: %s; close it first (it may be reading the textures)", ErrGameRuns, g.Describe(nil))
	}
	st, mods, err := a.textureMods(run.Refs, run.Scope)
	if err != nil {
		return nil, err
	}
	last, err := a.Store.TextureMods()
	if err != nil {
		return nil, err
	}
	r := &TextureReport{Options: run.Options.Normal(), DryRun: run.DryRun}
	for _, m := range mods {
		t := a.textureMod(st.Ix, m, run.Options, run.Force, last)
		r.Mods = append(r.Mods, t)
		if t.Plan != nil {
			r.Est.Plus(t.Plan.Est)
		}
	}
	if run.DryRun {
		r.Tool = a.TextureTool()
		return r, nil
	}
	enc, err := a.encoder(run.Log)
	if err != nil {
		return nil, err
	}
	ctx := run.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// A stop asked from another process (the window runs this as a child): the stop file.
	stop := a.TextureStopFile()
	os.Remove(stop)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := os.Stat(stop); err == nil {
					os.Remove(stop)
					cancel()
					return
				}
			}
		}
	}()
	p := TextureProgress{ModsTotal: len(r.Mods), FilesTotal: r.Est.Files}
	for _, t := range r.Mods {
		if ctx.Err() != nil {
			break
		}
		p.Mod = t.Mod.DisplayName()
		if run.Progress != nil {
			run.Progress(p)
		}
		if t.Err == "" {
			if err := a.optimizeMod(ctx, enc, st.Ix, t, r, &p, run.Progress); err != nil && !errors.Is(err, context.Canceled) {
				return r, err
			}
		}
		p.Mods++
	}
	if ctx.Err() != nil {
		r.Stopped = true
		r.Incomplete = max(0, p.FilesTotal-r.Converted-len(r.Failed)) // the batch a stop cut short is not done
	}
	if run.Progress != nil {
		p.Mod = ""
		run.Progress(p)
	}
	return r, nil
}

func (a *App) optimizeMod(ctx context.Context, enc textures.Encoder, ix *Index, t *TextureMod, r *TextureReport, p *TextureProgress, progress func(TextureProgress)) error {
	o := r.Options
	now := a.Now()
	row := func(j textures.Job) store.TextureFile {
		rel, _ := filepath.Rel(t.Dir, j.DDS())
		return store.TextureFile{Path: j.DDS(), InstanceID: t.Mod.ID, ModDir: t.Dir, Rel: filepath.ToSlash(rel),
			SrcSize: j.Size, SrcMTime: j.MTime, Scale: j.Scale, Mips: !o.NoMips, Pending: true, MadeAt: now}
	}
	failedKeys := map[string]bool{}
	before := func(js []textures.Job) error {
		rows := make([]store.TextureFile, 0, len(js))
		for _, j := range js {
			if _, err := os.Stat(j.DDS()); err == nil {
				if _, ours := t.rows[store.PathKey(j.DDS())]; !ours {
					// appeared since the plan (another tool?): never overwrite a DDS that is not ours
					return fmt.Errorf("%s appeared while optimising; stopped so as not to overwrite it", j.DDS())
				}
			}
			rows = append(rows, row(j))
		}
		return a.Store.PutTextureFiles(rows)
	}
	after := func(rs []textures.Result) error {
		var done []store.TextureFile
		var drop []string
		for _, x := range rs {
			if x.OK {
				f := row(x.Job)
				f.Pending, f.DDSSize, f.DDSMTime = false, x.DDSSize, x.DDSTime
				done = append(done, f)
				t.rows[store.PathKey(f.Path)] = f
				r.Converted++
				r.Written += x.DDSSize
				continue
			}
			failedKeys[x.Key] = true
			os.Remove(x.DDS) // ours: recorded before the run (a half-written file goes too)
			drop = append(drop, x.DDS)
			delete(t.rows, store.PathKey(x.DDS))
			// a stop interrupts the batch being written: those are not done, not failed
			if ctx.Err() == nil && len(r.Failed) < 200 {
				r.Failed = append(r.Failed, TextureFailure{Mod: t.Mod.DisplayName(), Rel: x.Rel, Err: x.Err})
			}
		}
		if err := a.Store.PutTextureFiles(done); err != nil {
			return err
		}
		return a.Store.DeleteTextureFiles(drop)
	}
	base := p.Files
	_, err := textures.Run(ctx, enc, t.Plan.Jobs, o, before, after, func(x textures.Progress) {
		p.Files, p.Bytes = base+x.Done, r.Written
		if progress != nil {
			progress(*p)
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	// A key converted in one folder and not in another: the DDS would shadow the PNG left
	// in the other, so the key goes back to its PNGs everywhere. And RimArcho's DDS whose PNG
	// is gone add a texture the mod no longer has.
	srcs := map[string]bool{}
	for _, s := range t.set.Sources {
		srcs[store.PathKey(s.DDS())] = true
		if failedKeys[s.Key] {
			if rw, ok := t.rows[store.PathKey(s.DDS())]; ok {
				if removeOurs(rw) {
					r.Removed++
				}
				a.Store.DeleteTextureFiles([]string{rw.Path})
				delete(t.rows, store.PathKey(s.DDS()))
			}
		}
	}
	for k, rw := range t.rows {
		if !srcs[k] {
			if removeOurs(rw) {
				r.Removed++
			}
			a.Store.DeleteTextureFiles([]string{rw.Path})
			delete(t.rows, k)
		}
	}
	if err != nil { // stopped: the files done are recorded; the mod is not marked done
		return err
	}
	var files int
	var bytes int64
	for _, rw := range t.rows {
		files++
		bytes += rw.DDSSize
	}
	if files == 0 {
		return a.Store.DeleteTextureMod(t.Mod.ID)
	}
	return a.Store.PutTextureMod(store.TextureMod{InstanceID: t.Mod.ID, SnapHash: t.Mod.SnapHash, Scale: o.Scale, Mips: !o.NoMips,
		Files: files, Bytes: bytes, MadeAt: now})
}

// TextureStopFile asks a running optimisation (in any process) to stop between files.
func (a *App) TextureStopFile() string { return filepath.Join(a.Home, "textures.stop") }

// StopTextures asks a running optimisation to stop: what it converted stays recorded.
func (a *App) StopTextures() error { return os.WriteFile(a.TextureStopFile(), nil, 0o644) }

// ChildArcho runs `<exe> <args>` (exe "": this executable) with the same data directory and
// passes each output line to progress; it returns the output lines that are not progress.
func ChildArcho(exe, home, envName string, args []string, progress func(string)) ([]string, error) {
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
		if b := strings.ToLower(filepath.Base(exe)); strings.HasSuffix(b, ".test") || strings.HasSuffix(b, ".test.exe") {
			return nil, errors.New("a child archo from a test binary: set the runner")
		}
	}
	if envName != "" {
		args = append(append([]string{}, args...), "--env", envName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var lines []string
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "progress:") {
			if progress != nil {
				progress(l)
			}
			continue
		}
		if l != "" {
			lines = append(lines, l)
		}
	}
	if err := cmd.Wait(); err != nil {
		return lines, fmt.Errorf("%s", strings.TrimPrefix(strings.Join(lines, "\n"), "error: "))
	}
	return lines, nil
}

// removeOurs deletes a DDS RimArcho wrote, unless something else has rewritten it since.
func removeOurs(r store.TextureFile) bool {
	fi, err := os.Stat(r.Path)
	if err != nil {
		return false
	}
	if !r.Pending && fi.Size() != r.DDSSize {
		return false
	}
	return os.Remove(r.Path) == nil
}

// ---- cleaning ----

// TextureClean is what CleanTextures did (or would do).
type TextureClean struct {
	DryRun bool
	Mods   []TextureCleanMod
	Files  int
	Bytes  int64
	Left   int // recorded files that changed since: forgotten, not deleted
}

// TextureCleanMod is one mod's part.
type TextureCleanMod struct {
	Name  string
	Ref   string
	Dir   string
	Files int
	Bytes int64
}

// CleanTextures deletes the DDS RimArcho wrote into mods (refs, or scope TexPlan / TexAll;
// TexAll also takes the files of mods that are gone). The PNGs were never touched, so the
// game goes back to them.
func (a *App) CleanTextures(refs []string, scope string, dryRun bool) (*TextureClean, error) {
	if g, _ := a.RunningGame(); g != nil && !dryRun {
		return nil, fmt.Errorf("%w: %s; close it first", ErrGameRuns, g.Describe(nil))
	}
	c := &TextureClean{DryRun: dryRun}
	var rows []store.TextureFile
	names := map[string]string{}
	refOf := map[string]string{}
	if scope == TexAll {
		st, err := a.Loadout()
		if err != nil {
			return nil, err
		}
		if rows, err = a.Store.TextureFilesOf(nil); err != nil {
			return nil, err
		}
		for _, m := range st.Ix.Mods {
			names[m.ID], refOf[m.ID] = m.DisplayName(), st.Ix.Ref(m.ID)
		}
	} else {
		st, mods, err := a.textureMods(refs, scope)
		if err != nil {
			return nil, err
		}
		for _, m := range mods {
			fs, err := a.Store.TextureFilesIn(modDir(st.Ix, m))
			if err != nil {
				return nil, err
			}
			rows = append(rows, fs...)
			names[m.ID], refOf[m.ID] = m.DisplayName(), st.Ix.Ref(m.ID)
		}
	}
	byMod := map[string]*TextureCleanMod{}
	var order []string
	var forget []string
	for _, r := range rows {
		cm := byMod[r.InstanceID]
		if cm == nil {
			n := names[r.InstanceID]
			if n == "" {
				n = filepath.Base(r.ModDir)
			}
			cm = &TextureCleanMod{Name: n, Ref: refOf[r.InstanceID], Dir: r.ModDir}
			byMod[r.InstanceID] = cm
			order = append(order, r.InstanceID)
		}
		fi, err := os.Stat(r.Path)
		if err != nil {
			forget = append(forget, r.Path)
			continue
		}
		if !r.Pending && fi.Size() != r.DDSSize {
			c.Left++
			forget = append(forget, r.Path)
			continue
		}
		cm.Files++
		cm.Bytes += fi.Size()
		c.Files++
		c.Bytes += fi.Size()
		if !dryRun {
			if err := os.Remove(r.Path); err != nil {
				return c, fmt.Errorf("deleting %s: %w", r.Path, err)
			}
			forget = append(forget, r.Path)
		}
	}
	for _, id := range order {
		c.Mods = append(c.Mods, *byMod[id])
		if !dryRun {
			a.Store.DeleteTextureMod(id)
		}
	}
	if dryRun {
		return c, nil
	}
	return c, a.Store.DeleteTextureFiles(forget)
}

// uninstallTextures is uninstall's textures step (02 §2): one item per mod folder with
// DDS RimArcho wrote; only those files go, the folder and the PNGs stay.
func (a *App) uninstallTextures() ([]*UninstallItem, error) {
	rows, err := a.Store.TextureFilesOf(nil)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if ix, err := a.Index(); err == nil {
		for _, m := range ix.Mods {
			names[m.ID] = m.DisplayName()
		}
	}
	byDir := map[string]*UninstallItem{}
	count := map[string]int{}
	var out []*UninstallItem
	for _, r := range rows {
		it := byDir[r.ModDir]
		if it == nil {
			dir := filepath.Dir(r.Path)
			if rel := filepath.FromSlash(r.Rel); strings.HasSuffix(strings.ToLower(r.Path), strings.ToLower(rel)) {
				dir = filepath.Clean(r.Path[:len(r.Path)-len(rel)]) // the mod folder as written
			}
			it = &UninstallItem{Path: dir, Name: names[r.InstanceID], Do: DoDDS}
			byDir[r.ModDir] = it
			out = append(out, it)
		}
		if fi, err := os.Stat(r.Path); err == nil {
			it.Bytes += fi.Size()
			count[r.ModDir]++
		}
	}
	for k, it := range byDir {
		it.Note = fmt.Sprintf("%d DDS files RimArcho wrote next to the PNGs; the PNGs stay", count[k])
	}
	return out, nil
}

// deleteTexturesIn deletes the DDS RimArcho recorded in a mod folder.
func (a *App) deleteTexturesIn(dir string) error {
	rows, err := a.Store.TextureFilesIn(dir)
	if err != nil {
		return err
	}
	var done []string
	var errs []string
	for _, r := range rows {
		if _, err := os.Stat(r.Path); err == nil && !removeOurs(r) {
			if !r.Pending {
				if fi, _ := os.Stat(r.Path); fi != nil && fi.Size() != r.DDSSize {
					done = append(done, r.Path) // rewritten by something else: not ours to delete
					continue
				}
			}
			errs = append(errs, r.Path)
			continue
		}
		done = append(done, r.Path)
	}
	a.Store.DeleteTextureFiles(done)
	if len(errs) > 0 {
		return fmt.Errorf("could not delete %d DDS files, e.g. %s", len(errs), errs[0])
	}
	return nil
}

// ---- stale DDS ----

// StaleTextures are the mods whose DDS may show old textures: the mod changed since its
// textures were optimised. A mod whose files all still match is marked current again.
func (a *App) StaleTextures(ix *Index) []*store.Mod {
	last, err := a.Store.TextureMods()
	if err != nil || len(last) == 0 {
		return nil
	}
	var out []*store.Mod
	for id, l := range last {
		m := ix.ByID[id]
		if m == nil || !m.TombstoneAt.IsZero() || bytes.Equal(m.SnapHash, l.SnapHash) {
			continue
		}
		rows, err := a.Store.TextureFilesIn(modDir(ix, m))
		if err != nil {
			continue
		}
		stale := false
		for _, r := range rows {
			png := strings.TrimSuffix(r.Path, filepath.Ext(r.Path)) + ".png"
			fi, err := os.Stat(png)
			if err != nil || fi.Size() != r.SrcSize || fi.ModTime().UnixNano() != r.SrcMTime || r.Pending {
				stale = true
				break
			}
		}
		if stale {
			out = append(out, m)
		} else {
			l.SnapHash = m.SnapHash
			a.Store.PutTextureMod(l)
		}
	}
	SortMods(out)
	return out
}

// TextureSummary is what Mod library shows for a copy: RimArcho's DDS files and bytes, the
// scale they were made at, and whether the mod changed since.
type TextureSummary struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	Scale int   `json:"scale"`
	Stale bool  `json:"stale,omitempty"`
}

// TextureSummaries maps instance IDs to their optimised textures (only those with any).
func (a *App) TextureSummaries() map[string]TextureSummary {
	return a.textureSummariesAtIndex(nil)
}

// A stored-inventory reader supplies its index so this metadata lookup cannot
// trigger normal index loading and filesystem alias repair.
func (a *App) textureSummariesAtIndex(ix *Index) map[string]TextureSummary {
	// from the files recorded: a run that was stopped wrote some and no mod record
	counts, err := a.Store.TextureCounts()
	if err != nil || len(counts) == 0 {
		return nil
	}
	last, _ := a.Store.TextureMods()
	out := map[string]TextureSummary{}
	for id, n := range counts {
		s := TextureSummary{Files: int(n[0]), Bytes: n[1], Scale: last[id].Scale}
		if s.Scale == 0 {
			s.Scale = 100
		}
		out[id] = s
	}
	if ix == nil {
		ix, _ = a.Index()
	}
	if ix != nil {
		for id, s := range out {
			if l, ok := last[id]; ok && ix.ByID[id] != nil && !bytes.Equal(ix.ByID[id].SnapHash, l.SnapHash) {
				s.Stale = true
				out[id] = s
			}
		}
	}
	return out
}

// HasPNGTextures reports whether a copy had PNGs under a Textures/ folder at its last scan
// (from the stored file lists, cached until a scan changes the snapshots): Mod library offers
// optimise textures for those copies only.
func (a *App) HasPNGTextures(m *store.Mod) bool {
	if m == nil || m.SnapID == 0 {
		return false
	}
	return a.pngSnapshots()[m.SnapID]
}

// pngSnapshots is the set of snapshots with PNG textures, read again only when a scan
// changed the snapshots. Checking that (SnapshotsKey counts the snapshots) costs a table
// scan, so a caller going through many mods takes the set once (Mod library: once per mod made
// opening the page take 0.4 s on 3,500 instances).
func (a *App) pngSnapshots() map[int64]bool {
	if k := a.Store.SnapshotsKey(); a.pngSnaps == nil || a.pngSnapsKey != k {
		s, err := a.Store.SnapshotsWithPNG()
		if err != nil {
			return nil
		}
		a.pngSnaps, a.pngSnapsKey = s, k
	}
	return a.pngSnaps
}
