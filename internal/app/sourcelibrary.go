package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SourceDir is the managed, local-only library (05 §5.4, 17 §3).
func (a *App) SourceDir() string {
	if p := a.Store.Meta("cfg:source.dir"); p != "" {
		return p
	}
	return filepath.Join(a.Home, "source")
}

// Readers/decompilers share access; relocating and cleanup need exclusive access.
// The lock lives under Home so it remains stable while the library is moved.
func (a *App) sourceAccess(ctx context.Context, exclusive, wait bool) (func(), error) {
	for {
		unlock, err := lockSourceLibraryFile(filepath.Join(a.Home, "source-library.lock"), exclusive)
		if err == nil {
			return unlock, nil
		}
		if !wait || !sourceLockBusy(err) {
			return nil, fmt.Errorf("Source library is in use or unavailable; stop its tasks and searches before moving it: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func sourceComponent(s string) string {
	s = strings.Trim(safeName(s), ". ")
	if s == "" {
		return "unknown"
	}
	// Device names remain reserved with extensions on Windows.
	stem := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		s = "_" + s
	}
	return s
}

func (a *App) locateSource(d *SourceDLL) error {
	r, err := a.Store.SourceArtifact(d.Hash, d.GameBuild)
	if err != nil {
		return err
	}
	if r != nil {
		d.Dir = filepath.Join(a.SourceDir(), filepath.FromSlash(r.Folder))
	} else if d.Kind == "game" {
		d.Dir = filepath.Join(a.SourceDir(), "game", sourceComponent(d.GameBuild), "Assembly-CSharp")
	} else {
		stem := strings.TrimSuffix(filepath.Base(d.File), filepath.Ext(d.File))
		d.Dir = filepath.Join(a.SourceDir(), "mods", sourceComponent(strings.ToLower(d.PackageID)), sourceComponent(stem)+" "+d.Hash[:6])
	}
	if !source.Within(directoryReal(d.Dir), directoryReal(a.SourceDir())) || source.IsLink(d.Dir) {
		return fmt.Errorf("source folder is outside the library: %s", d.Dir)
	}
	b, _ := os.ReadFile(filepath.Join(d.Dir, ".done"))
	d.Cached = string(b) == d.Hash
	return nil
}

func (a *App) claimSource(d *SourceDLL) error {
	if err := a.locateSource(d); err != nil {
		return err
	}
	rel, err := filepath.Rel(a.SourceDir(), d.Dir)
	if err != nil {
		return err
	}
	r := store.SourceArtifact{Hash: d.Hash, GameBuild: d.GameBuild, Folder: filepath.ToSlash(rel), Kind: d.Kind, PackageID: d.PackageID, DLLName: filepath.Base(d.File), Created: a.Now()}
	claimed, err := a.Store.ClaimSourceArtifact(r)
	if err != nil {
		// A six-character prefix can collide. Keep the first path stable.
		r.Folder += " " + d.Hash[6:20]
		claimed, err = a.Store.ClaimSourceArtifact(r)
	}
	if err != nil {
		return err
	}
	d.Dir = filepath.Join(a.SourceDir(), filepath.FromSlash(claimed.Folder))
	return a.locateSource(d)
}

func (a *App) lockSourceArtifact(ctx context.Context, d *SourceDLL) (func(), error) {
	if err := a.managedRootReady("source", a.SourceDir()); err != nil {
		return nil, err
	}
	root := filepath.Join(a.SourceDir(), ".locks")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if err := a.Store.SetMeta(directoryKey("source"), a.SourceDir()); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(d.Hash+"|"+d.GameBuild)))
	p := filepath.Join(root, key)
	for {
		unlock, err := lockSourceLibraryFile(p, true)
		if err == nil {
			return unlock, nil
		}
		if !sourceLockBusy(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (a *App) recordSourceUse(d *SourceDLL, mode string, err error) error {
	status, msg := "ok", ""
	if err != nil {
		status, msg = "error", err.Error()
	}
	return a.Store.RecordSourceUse(d.Hash, d.Instance, d.File, mode, status, msg, a.Now())
}

func (a *App) sourceVersion(ctx context.Context) string {
	if a.Decompile != nil {
		return "custom"
	}
	exe := ILSpyPath()
	if exe == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return firstN(strings.TrimSpace(string(out)), 200)
}

func typeSourceDir(d *SourceDLL, typ string) string {
	h := sha256.Sum256([]byte(typ))
	return filepath.Join(d.Dir, ".types", fmt.Sprintf("%x", h[:8]))
}

func (a *App) publishSource(ctx context.Context, st *LoadoutState, d *SourceDLL, typ string) error {
	target := d.Dir
	if typ != "" {
		target = typeSourceDir(d, typ)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".decompile-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := a.decompiler()(ctx, d.Path, tmp, typ, a.referenceDirs(st, d.Path)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if typ == "" {
		if err := os.WriteFile(filepath.Join(tmp, ".done"), []byte(d.Hash), 0o644); err != nil {
			return err
		}
	} else {
		b, err := os.ReadFile(filepath.Join(tmp, safeName(typ)+".cs"))
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(b)) == 0 {
			return fmt.Errorf("type %s has no source", typ)
		}
	}
	// Preserve existing type-only output if publication fails.
	old := tmp + "-previous"
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Rename(old, target)
		return err
	}
	_ = os.RemoveAll(old)
	if typ == "" {
		d.Cached = true
	}
	return a.Store.UpdateSourceArtifact(d.Hash, d.GameBuild, a.sourceVersion(ctx), dirSize(d.Dir))
}

func (a *App) sourceType(ctx context.Context, st *LoadoutState, d *SourceDLL, typ string) (string, string, error) {
	unlock, err := a.lockSourceArtifact(ctx, d)
	if err != nil {
		return "", "", err
	}
	defer unlock()
	if err := a.claimSource(d); err != nil {
		return "", "", err
	}
	if d.Cached {
		p := findTypeFile(d.Dir, typ)
		if p == "" {
			return "", "", fmt.Errorf("type %s not found in %s", typ, d.Dir)
		}
		b, err := os.ReadFile(p)
		return string(b), p, errors.Join(err, a.recordSourceUse(d, "type-hit", err))
	}
	p := filepath.Join(typeSourceDir(d, typ), safeName(typ)+".cs")
	if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return string(b), p, a.recordSourceUse(d, "type-hit", nil)
	}
	err = a.publishSource(ctx, st, d, typ)
	if rec := a.recordSourceUse(d, "type", err); err != nil || rec != nil {
		return "", "", errors.Join(err, rec)
	}
	b, err := os.ReadFile(p)
	return string(b), p, err
}

func containsSourceInstance(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// GameSourceDLL selects the actual configured installation, independent of a
// historical --game-version used to inspect a Mod's effective content.
func (a *App) GameSourceDLL(st *LoadoutState) (*SourceDLL, error) {
	build := sourceGameBuild(st.Ix.Env.GameVersion)
	if build == "" {
		return nil, fmt.Errorf("the current environment has no game build; configure its game version first")
	}
	dir := env.ManagedDir(st.Ix.Env.GameRoot)
	if dir == "" {
		return nil, fmt.Errorf("Assembly-CSharp.dll not found in game folder %s", st.Ix.Env.GameRoot)
	}
	p := filepath.Join(dir, "Assembly-CSharp.dll")
	h, err := fileHash(p)
	if err != nil {
		return nil, err
	}
	d := &SourceDLL{Instance: "game:" + st.Ix.Env.ID, File: "Assembly-CSharp.dll", Path: p, Hash: h, Kind: "game", GameBuild: build}
	if err := a.locateSource(d); err != nil {
		return nil, err
	}
	return d, nil
}

func sourceGameBuild(s string) string {
	if parts := strings.Fields(s); len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// DecompileActive is explicitly requested work; callers attach their task's stop
// context and persist progress using the existing background task infrastructure.
func (a *App) DecompileActive(ctx context.Context, progress func(done, total int, file string)) error {
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	var ds []*SourceDLL
	seen := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			xs, err := a.SourceDLLs(st, m)
			if err != nil {
				return err
			}
			ds = append(ds, xs...)
		}
	}
	var errs []error
	for i, d := range ds {
		if err := ctx.Err(); err != nil {
			return err
		}
		if progress != nil {
			progress(i, len(ds), d.File)
		}
		if seen[d.Hash] {
			if err := a.locateSource(d); err != nil {
				return err
			}
		}
		if err := a.DecompileDLL(ctx, st, d); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrNoILSpy) {
				return err
			}
			errs = append(errs, err)
		}
		seen[d.Hash] = true
		if progress != nil {
			progress(i+1, len(ds), d.File)
		}
	}
	return errors.Join(errs...)
}

type SourceCleanPreview struct {
	Folders int      `json:"folders"`
	Bytes   int64    `json:"bytes"`
	Paths   []string `json:"paths"`
}

// unusedSourceArtifacts checks current bytes in every installed/locked copy,
// including inactive copies and version folders. Past use alone cannot keep old
// DLL revisions alive. An unreadable source aborts removal rather than guessing.
func (a *App) unusedSourceArtifacts(ctx context.Context) ([]*store.SourceArtifact, error) {
	artifacts, err := a.Store.SourceArtifacts()
	if err != nil {
		return nil, err
	}
	es, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	ss, err := a.Store.AllSources()
	if err != nil {
		return nil, err
	}
	roots := map[string]string{}
	for _, s := range ss {
		roots[s.ID] = s.Root
	}
	builds, hashes, visited := map[string]bool{}, map[string]bool{}, map[string]bool{}
	ids := []string{store.Shared}
	for _, e := range es {
		builds[sourceGameBuild(e.GameVersion)] = true
		ids = append(ids, e.ID)
	}
	for _, id := range ids {
		mods, err := a.Store.Mods(id)
		if err != nil {
			return nil, err
		}
		for _, m := range mods {
			if m.Status != identity.Live || visited[m.ID] {
				continue
			}
			visited[m.ID] = true
			root := m.RealPath
			if root == "" {
				root = filepath.Join(roots[m.SourceID], m.Rel)
			}
			if _, err := os.Stat(roots[m.SourceID]); err != nil {
				return nil, fmt.Errorf("cannot check installed copies in %s: %w", roots[m.SourceID], err)
			}
			if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
				continue
			}
			err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".dll") {
					return nil
				}
				h, err := fileHash(p)
				if err == nil {
					hashes[h] = true
				}
				return err
			})
			if err != nil {
				return nil, err
			}
		}
	}
	var out []*store.SourceArtifact
	for _, r := range artifacts {
		if r.Kind == "game" && !builds[r.GameBuild] || r.Kind == "mod" && !hashes[r.Hash] {
			out = append(out, r)
		}
	}
	return out, nil
}

// CleanSourceLibrary always recomputes liveness; a preview never authorizes deletion.
func (a *App) CleanSourceLibrary(ctx context.Context, remove bool) (*SourceCleanPreview, error) {
	unlock, err := a.sourceAccess(ctx, true, true)
	if err != nil {
		return nil, err
	}
	defer unlock()
	unused, err := a.unusedSourceArtifacts(ctx)
	if err != nil {
		return nil, err
	}
	p := &SourceCleanPreview{Paths: []string{}}
	for _, r := range unused {
		path := filepath.Join(a.SourceDir(), filepath.FromSlash(r.Folder))
		if !source.Within(directoryReal(path), directoryReal(a.SourceDir())) || path == a.SourceDir() || source.IsLink(path) {
			return nil, fmt.Errorf("refusing source removal outside library: %s", path)
		}
		p.Folders++
		p.Bytes += dirSize(path)
		p.Paths = append(p.Paths, path)
		if remove {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := os.RemoveAll(path); err != nil {
				return nil, err
			}
			if err := a.Store.ForgetSourceArtifact(r.Hash, r.GameBuild); err != nil {
				return nil, err
			}
		}
	}
	return p, nil
}
