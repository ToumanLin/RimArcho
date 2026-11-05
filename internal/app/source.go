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

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// On-demand decompilation with ilspycmd (03 §5.4). Generated projects live in the
// managed Source library; identities and uses survive removed output.

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
	Instance  string
	File      string // relative to the mod root
	Path      string
	Hash      string
	Dir       string // whole-project directory in the Source library
	Cached    bool
	PackageID string
	GameBuild string
	Kind      string
	Shared    []string // requested instances sharing this assembly, in scope order
}

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
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
	for _, as := range content.CodeFiles(dir, r.Folders) {
		if harmony.SkippedFor(filepath.Base(as.Path), m.PackageID, harmony.DefaultSkip) {
			continue
		}
		h, err := fileHash(as.Path)
		if err != nil {
			continue
		}
		d := &SourceDLL{Instance: m.ID, File: as.Rel, Path: as.Path, Hash: h, PackageID: m.PackageID, Kind: "mod"}
		if err := a.locateSource(d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// referenceDirs: the game's managed assemblies and every enabled mod's Assemblies folder
// that holds 0Harmony.dll, plus the DLL's own folder.
func (a *App) referenceDirs(st *LoadoutState, dll string) []string {
	var out []string
	if d := env.ManagedDir(st.Ix.Env.GameRoot); d != "" {
		out = append(out, d)
	}
	out = append(out, filepath.Dir(dll))
	harmonyDir := ""
	{
		for _, e := range st.List {
			m := st.Ix.ByID[e.Instance]
			if m == nil || !strings.Contains(strings.ToLower(m.PackageID), "harmony") {
				continue
			}
			filepath.WalkDir(st.Ix.ModDir(m), func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.EqualFold(d.Name(), "0Harmony.dll") && harmonyDir == "" {
					harmonyDir = filepath.Dir(p)
				}
				return nil
			})
		}
	}
	if harmonyDir != "" {
		out = append(out, harmonyDir)
	}
	return out
}

func (a *App) decompiler() Decompiler {
	if a.Decompile != nil {
		return a.Decompile
	}
	return ILSpyDecompiler
}

// DecompileDLL publishes a whole project atomically and records reuse too.
func (a *App) DecompileDLL(ctx context.Context, st *LoadoutState, d *SourceDLL) error {
	release, err := a.sourceAccess(ctx, false, true)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := a.lockSourceArtifact(ctx, d)
	if err != nil {
		return err
	}
	defer unlock()
	if err := a.claimSource(d); err != nil {
		return err
	}
	if d.Cached {
		return a.recordSourceUse(d, "project-hit", nil)
	}
	err = a.publishSource(ctx, st, d, "")
	return errors.Join(err, a.recordSourceUse(d, "project", err))
}

// DecompileType returns the source of one type, decompiling only that type unless the
// whole project is cached.
func (a *App) DecompileType(ctx context.Context, st *LoadoutState, ds []*SourceDLL, typ string) (string, string, error) {
	release, err := a.sourceAccess(ctx, false, true)
	if err != nil {
		return "", "", err
	}
	defer release()
	for _, d := range ds {
		if err := a.locateSource(d); err != nil {
			return "", "", err
		}
	}
	// 1. A cached project file named after the type.
	for _, d := range ds {
		if !d.Cached {
			continue
		}
		if p := findTypeFile(d.Dir, typ); p != "" {
			b, err := os.ReadFile(p)
			return string(b), p, errors.Join(err, a.recordSourceUse(d, "type-hit", err))
		}
	}
	// 2. Decompile the type alone, from the DLL that defines it.
	owners, _ := a.Store.TypeOwners(typ)
	for _, d := range ds {
		match := d.Kind == "game" || len(owners) == 0
		for _, o := range owners {
			if o[0] == d.Instance && o[1] == d.File {
				match = true
			}
		}
		if !match {
			continue
		}
		src, p, err := a.sourceType(ctx, st, d, typ)
		if err != nil {
			if errors.Is(err, ErrNoILSpy) {
				return "", "", err
			}
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			continue
		}
		return src, p, nil
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
	File     string // display path: <dll>/<path in project>
	Path     string // the decompiled file on disk
	Line     int
	Text     string
	Shared   []string
}

// SourceSearchResult is the outcome of SourceSearch.
type SourceSearchResult struct {
	Hits       []SourceHit
	Total      int
	Assemblies int
	Pending    int // not decompiled within the budget
	Errors     []string
}

// SourceSearch searches the library for the given instances, decompiling missing
// projects first. The budget stops new work; in-flight work finishes (0 = no limit).
func (a *App) SourceSearch(st *LoadoutState, mods []*store.Mod, pattern string, re *regexp.Regexp, limit int, budget time.Duration, progress func(string)) (*SourceSearchResult, error) {
	return a.SourceSearchContext(context.Background(), st, mods, pattern, re, limit, budget, progress)
}

// SourceSearchContext is SourceSearch that stops when ctx ends (the GUI's stop, a newer
// search): decompiling stops between assemblies and the search between files.
func (a *App) SourceSearchContext(ctx context.Context, st *LoadoutState, mods []*store.Mod, pattern string, re *regexp.Regexp, limit int, budget time.Duration, progress func(string), game ...bool) (*SourceSearchResult, error) {
	release, err := a.sourceAccess(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer release()
	res := &SourceSearchResult{}
	var all []*SourceDLL
	for _, m := range mods {
		ds, err := a.SourceDLLs(st, m)
		if err != nil {
			return nil, err
		}
		all = append(all, ds...)
	}
	if len(game) > 0 && game[0] {
		d, err := a.GameSourceDLL(st)
		if err != nil {
			return nil, err
		}
		all = append(all, d)
	}
	// Group before dispatch: identical assemblies share one job and one result.
	uses := all
	all = nil
	byHash := map[string]*SourceDLL{}
	for _, d := range uses {
		key := d.Hash + "|" + d.GameBuild
		if first := byHash[key]; first != nil {
			if !containsSourceInstance(first.Shared, d.Instance) {
				first.Shared = append(first.Shared, d.Instance)
			}
			continue
		}
		d.Shared = []string{d.Instance}
		byHash[key], all = d, append(all, d)
	}
	res.Assemblies = len(all)
	var todo []*SourceDLL
	for _, d := range all {
		if !d.Cached {
			todo = append(todo, d)
		}
	}
	if len(todo) > 0 {
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
		var budgetC <-chan time.Time
		if !deadline.IsZero() {
			timer := time.NewTimer(time.Until(deadline))
			defer timer.Stop()
			budgetC = timer.C
		}
		for _, d := range todo {
			if ctx.Err() != nil || !deadline.IsZero() && time.Now().After(deadline) {
				res.Pending++
				continue
			}
			mu.Lock()
			noILSpy = len(res.Errors) > 0 && strings.Contains(res.Errors[0], "ilspycmd not found")
			mu.Unlock()
			if noILSpy {
				break
			}
			select {
			case next <- d:
			case <-ctx.Done():
				res.Pending++
			case <-budgetC:
				res.Pending++
			}
		}
		close(next)
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, e := range res.Errors {
			if strings.Contains(e, "ilspycmd not found") {
				return nil, ErrNoILSpy
			}
		}
	}
	for _, d := range uses {
		first := byHash[d.Hash+"|"+d.GameBuild]
		if first.Cached {
			d.Dir, d.Cached = first.Dir, true
			if err := a.recordSourceUse(d, "search-hit", nil); err != nil {
				return nil, err
			}
		}
	}
	lower := strings.ToLower(pattern)
	for _, d := range all {
		if !d.Cached {
			continue
		}
		filepath.WalkDir(d.Dir, func(p string, de os.DirEntry, err error) error {
			if ctx.Err() != nil {
				return filepath.SkipAll
			}
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
					res.Hits = append(res.Hits, SourceHit{Instance: d.Instance, File: filepath.ToSlash(filepath.Join(filepath.Base(d.File), rel)), Path: p, Line: n, Text: strings.TrimSpace(l), Shared: d.Shared})
				}
			}
			return nil
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(res.Hits, func(i, j int) bool { return st.Position(res.Hits[i].Instance) < st.Position(res.Hits[j].Instance) })
	return res, nil
}
