package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/harmony"
	"github.com/rimarcho/rimarcho/internal/store"
)

// On-demand decompilation with ilspycmd (03 §5.3). Output is cached per DLL hash under
// <home>/cache/source/<hash>/; the source_cache table records every decompile, so the
// history survives a deleted cache and a missing directory is re-decompiled on demand.

// Decompiler turns dll into C# under outDir (a whole project, or one type when typ != "").
type Decompiler func(ctx context.Context, dll, outDir, typ string, refs []string) error

// ErrNoILSpy is returned when ilspycmd cannot be found.
var ErrNoILSpy = errors.New("ilspycmd not found. Install it with: dotnet tool install --global ilspycmd " +
	"(requires the .NET SDK; then reopen the terminal). RimArcho never installs it itself")

// ILSpyPath locates ilspycmd: ARCHO_ILSPYCMD, PATH, then ~/.dotnet/tools.
func ILSpyPath() string {
	if p := os.Getenv("ARCHO_ILSPYCMD"); p != "" {
		return p
	}
	if p, err := exec.LookPath("ilspycmd"); err == nil {
		return p
	}
	if h, err := os.UserHomeDir(); err == nil {
		for _, n := range []string{"ilspycmd.exe", "ilspycmd"} {
			p := filepath.Join(h, ".dotnet", "tools", n)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// ILSpyDecompiler runs ilspycmd.
func ILSpyDecompiler(ctx context.Context, dll, outDir, typ string, refs []string) error {
	exe := ILSpyPath()
	if exe == "" {
		return ErrNoILSpy
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	args := []string{dll}
	for _, r := range refs {
		args = append(args, "-r", r)
	}
	if typ != "" {
		args = append(args, "-t", typ)
		out, err := exec.CommandContext(ctx, exe, args...).Output()
		if err != nil && len(out) == 0 {
			return fmt.Errorf("ilspycmd -t %s: %v", typ, err)
		}
		return os.WriteFile(filepath.Join(outDir, safeName(typ)+".cs"), out, 0o644)
	}
	args = append(args, "-p", "-o", outDir)
	if out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ilspycmd: %v: %s", err, firstN(string(out), 300))
	}
	return nil
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var unsafeRe = regexp.MustCompile(`[^\w.+-]`)

func safeName(s string) string { return unsafeRe.ReplaceAllString(s, "_") }

// SourceDLL is one decompilable assembly of an instance.
type SourceDLL struct {
	Instance string
	File     string // relative to the mod root
	Path     string
	Hash     string
	Dir      string // cache directory of the whole project ("" until decompiled)
	Cached   bool
}

func (a *App) sourceRoot() string { return filepath.Join(a.Home, "cache", "source") }

func fileHash(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:20], nil
}

// SourceDLLs lists the effective, non-library assemblies of an instance.
func (a *App) SourceDLLs(st *LoadoutState, m *store.Mod) ([]*SourceDLL, error) {
	if err := a.Store.LoadDecls([]*store.Mod{m}); err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, e := range st.List {
		if x := st.Ix.ByID[e.Instance]; x != nil {
			active[strings.TrimSuffix(strings.ToLower(x.PackageID), "_steam")] = true
		}
	}
	dir := st.Ix.ModDir(m)
	r := content.Resolve(dir, m.Decl().LoadFolders, st.GameMM(), func(p string) bool { return active[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
	var out []*SourceDLL
	for _, as := range content.Files(dir, r.Folders, "Assemblies", ".dll") {
		if harmony.Skipped(filepath.Base(as.Path), harmony.DefaultSkip) {
			continue
		}
		h, err := fileHash(as.Path)
		if err != nil {
			continue
		}
		d := &SourceDLL{Instance: m.ID, File: as.Rel, Path: as.Path, Hash: h, Dir: filepath.Join(a.sourceRoot(), h)}
		if fi, err := os.Stat(filepath.Join(d.Dir, ".done")); err == nil && !fi.IsDir() {
			d.Cached = true
		}
		out = append(out, d)
	}
	return out, nil
}

// referenceDirs: the game's managed assemblies and every enabled mod's Assemblies folder
// that holds 0Harmony.dll, plus the DLL's own folder.
func (a *App) referenceDirs(st *LoadoutState, dll string) []string {
	var out []string
	root := st.Ix.Env.GameRoot
	for _, d := range []string{filepath.Join(root, "RimWorldWin64_Data", "Managed"), root} {
		if _, err := os.Stat(filepath.Join(d, "Assembly-CSharp.dll")); err == nil {
			out = append(out, d)
			break
		}
	}
	out = append(out, filepath.Dir(dll))
	if a.harmonyDir == "" {
		for _, e := range st.List {
			m := st.Ix.ByID[e.Instance]
			if m == nil || !strings.Contains(strings.ToLower(m.PackageID), "harmony") {
				continue
			}
			filepath.WalkDir(st.Ix.ModDir(m), func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.EqualFold(d.Name(), "0Harmony.dll") && a.harmonyDir == "" {
					a.harmonyDir = filepath.Dir(p)
				}
				return nil
			})
		}
	}
	if a.harmonyDir != "" {
		out = append(out, a.harmonyDir)
	}
	return out
}

func (a *App) decompiler() Decompiler {
	if a.Decompile != nil {
		return a.Decompile
	}
	return ILSpyDecompiler
}

// Decompile makes sure the DLL's whole project is cached.
func (a *App) DecompileDLL(ctx context.Context, st *LoadoutState, d *SourceDLL) error {
	if d.Cached {
		return nil
	}
	tmp := d.Dir + ".tmp"
	os.RemoveAll(tmp)
	err := a.decompiler()(ctx, d.Path, tmp, "", a.referenceDirs(st, d.Path))
	status, msg := "ok", ""
	if err != nil {
		status, msg = "error", err.Error()
		os.RemoveAll(tmp)
	} else {
		os.RemoveAll(d.Dir)
		if err = os.Rename(tmp, d.Dir); err == nil {
			os.WriteFile(filepath.Join(d.Dir, ".done"), []byte(d.Path), 0o644)
			d.Cached = true
		}
	}
	if !errors.Is(err, ErrNoILSpy) {
		a.Store.RecordDecompile(d.Hash, d.Instance, d.File, status, msg, a.Now())
	}
	return err
}

// DecompileType returns the source of one type, decompiling only that type unless the
// whole project is cached.
func (a *App) DecompileType(ctx context.Context, st *LoadoutState, ds []*SourceDLL, typ string) (string, string, error) {
	// 1. A cached project file named after the type.
	for _, d := range ds {
		if !d.Cached {
			continue
		}
		if p := findTypeFile(d.Dir, typ); p != "" {
			b, err := os.ReadFile(p)
			return string(b), p, err
		}
	}
	// 2. Decompile the type alone, from the DLL that defines it.
	owners, _ := a.Store.TypeOwners(typ)
	for _, d := range ds {
		match := len(owners) == 0
		for _, o := range owners {
			if o[0] == d.Instance && o[1] == d.File {
				match = true
			}
		}
		if !match {
			continue
		}
		dir := filepath.Join(d.Dir + ".types")
		p := filepath.Join(dir, safeName(typ)+".cs")
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return string(b), p, nil
		}
		if err := a.decompiler()(ctx, d.Path, dir, typ, a.referenceDirs(st, d.Path)); err != nil {
			if errors.Is(err, ErrNoILSpy) {
				return "", "", err
			}
			continue
		}
		if b, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(b)) > 0 {
			return string(b), p, nil
		}
	}
	return "", "", fmt.Errorf("type %s not found in %d assemblies", typ, len(ds))
}

func findTypeFile(dir, typ string) string {
	name := typ
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ReplaceAll(name, "+", ".")
	var hit string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(d.Name(), name+".cs") && hit == "" {
			hit = p
		}
		return nil
	})
	return hit
}

// SourceHit is one search match.
type SourceHit struct {
	Instance string
	File     string // relative to the cache dir: <dll>/<path in project>
	Line     int
	Text     string
}

// SourceSearchResult is the outcome of SourceSearch.
type SourceSearchResult struct {
	Hits       []SourceHit
	Total      int
	Assemblies int
	Pending    int // not decompiled within the budget
	Errors     []string
}

// SourceSearch searches the cached sources of the given instances, decompiling uncached
// assemblies first (at most for budget; 0 = no limit).
func (a *App) SourceSearch(st *LoadoutState, mods []*store.Mod, pattern string, re *regexp.Regexp, limit int, budget time.Duration, progress func(string)) (*SourceSearchResult, error) {
	res := &SourceSearchResult{}
	var all []*SourceDLL
	for _, m := range mods {
		ds, err := a.SourceDLLs(st, m)
		if err != nil {
			return nil, err
		}
		all = append(all, ds...)
	}
	res.Assemblies = len(all)
	var todo []*SourceDLL
	for _, d := range all {
		if !d.Cached {
			todo = append(todo, d)
		}
	}
	if len(todo) > 0 {
		ctx := context.Background()
		var cancel context.CancelFunc = func() {}
		deadline := time.Time{}
		if budget > 0 {
			deadline = time.Now().Add(budget)
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		next := make(chan *SourceDLL)
		done := 0
		for w := 0; w < 6; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for d := range next {
					err := a.DecompileDLL(ctx, st, d)
					mu.Lock()
					done++
					if err != nil {
						res.Errors = append(res.Errors, fmt.Sprintf("%s %s: %v", st.Ix.Ref(d.Instance), d.File, err))
					}
					if progress != nil {
						progress(fmt.Sprintf("decompiled %d/%d assemblies", done, len(todo)))
					}
					mu.Unlock()
				}
			}()
		}
		var noILSpy bool
		for _, d := range todo {
			if !deadline.IsZero() && time.Now().After(deadline) {
				res.Pending++
				continue
			}
			mu.Lock()
			noILSpy = len(res.Errors) > 0 && strings.Contains(res.Errors[0], "ilspycmd not found")
			mu.Unlock()
			if noILSpy {
				break
			}
			next <- d
		}
		close(next)
		wg.Wait()
		cancel()
		for _, e := range res.Errors {
			if strings.Contains(e, "ilspycmd not found") {
				return nil, ErrNoILSpy
			}
		}
	}
	lower := strings.ToLower(pattern)
	for _, d := range all {
		if !d.Cached {
			continue
		}
		filepath.WalkDir(d.Dir, func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.EqualFold(filepath.Ext(p), ".cs") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			if re == nil && !bytes.Contains(bytes.ToLower(b), []byte(lower)) {
				return nil
			}
			rel, _ := filepath.Rel(d.Dir, p)
			sc := bufio.NewScanner(bytes.NewReader(b))
			sc.Buffer(make([]byte, 64<<10), 8<<20)
			n := 0
			for sc.Scan() {
				n++
				l := sc.Text()
				ok := false
				if re != nil {
					ok = re.MatchString(l)
				} else {
					ok = strings.Contains(strings.ToLower(l), lower)
				}
				if !ok {
					continue
				}
				res.Total++
				if limit == 0 || len(res.Hits) < limit {
					res.Hits = append(res.Hits, SourceHit{Instance: d.Instance, File: filepath.ToSlash(filepath.Join(filepath.Base(d.File), rel)), Line: n, Text: strings.TrimSpace(l)})
				}
			}
			return nil
		})
	}
	sort.SliceStable(res.Hits, func(i, j int) bool { return st.Position(res.Hits[i].Instance) < st.Position(res.Hits[j].Instance) })
	return res, nil
}
