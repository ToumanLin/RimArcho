// Package modsearch searches mod content in game load order (03 §1).
package modsearch

import (
	"context"
	"errors"
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/textenc"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// FS supplies all directory listings and file reads. Implementations must support concurrent reads.
type FS interface {
	ReadDir(dir string) ([]fs.DirEntry, error)
	ReadFile(path string) ([]byte, error)
}

type osFS struct{}

func (osFS) ReadDir(dir string) ([]fs.DirEntry, error) { return os.ReadDir(dir) }
func (osFS) ReadFile(path string) ([]byte, error)      { return os.ReadFile(path) }

// OS is the default file system.
var OS FS = osFS{}

// Mod identifies one copy of a mod; copies with the same PackageID stay separate.
type Mod struct {
	Pos                        int
	Ref, PackageID, Name, Copy string
	Root                       string
	LoadFoldersXML             string
}

// Kind is a set of game content directories.
type Kind uint16

const (
	Defs Kind = 1 << iota
	Patches
	About
	Languages
	Textures
	Sounds
	Assemblies
	DefaultKinds = Defs | Patches | About
)
const allKinds = DefaultKinds | Languages | Textures | Sounds | Assemblies

// ParseKinds accepts comma-separated directory names, or all, ignoring case.
func ParseKinds(s string) (Kind, error) {
	var out Kind
	for _, name := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "all":
			out |= allKinds
		case "defs":
			out |= Defs
		case "patches":
			out |= Patches
		case "about":
			out |= About
		case "languages":
			out |= Languages
		case "textures":
			out |= Textures
		case "sounds":
			out |= Sounds
		case "assemblies":
			out |= Assemblies
		default:
			return 0, fmt.Errorf("unknown search kind %q; use defs, patches, about, languages, textures, sounds, assemblies, or all", name)
		}
	}
	return out, nil
}

// Options selects the content scope, matching rules, and result page.
type Options struct {
	Pattern                    string
	Regex, CaseSensitive, Word bool
	Def                        bool // Whole-word, case-sensitive literal defName search.
	MM                         string
	Active                     func(pkg string) bool
	AllVersions                bool     // Include every version and LoadFolders entry, ignoring conditions and shadowing.
	Kinds                      Kind     // Zero selects DefaultKinds, or Defs|Patches in Def mode.
	Langs                      []string // Case-insensitive language folder names; implies Languages.
	Offset, Limit              int      // Paging over kept matches; zero Limit means 50.
	PerFile                    int      // Kept matching lines per file, before paging; zero means 5.
	Workers                    int      // Zero clamps GOMAXPROCS to [4,16].
	FS                         FS
}

// Match is one matching line, or a binary asset path (Line=0). Col is a 1-based byte column.
type Match struct {
	Line, Col          int
	Text, Role, Within string
}

// FileResult describes a file represented on the current page. Count includes unkept hits.
type FileResult struct {
	Rel, Folder string
	Kind        Kind
	Count       int
	Matches     []Match
}

// FileCount describes every matched file, independent of the match page.
type FileCount struct {
	Rel   string
	Count int
}

// DefSummary counts roles and distinct enclosing defs/operations before caps or paging.
type DefSummary struct {
	Roles                        map[string]int
	Defined, Abstracts, Children []string
	PatchOperations              int
}

// ModResult preserves the input identity and resolved folder labels, even on an empty page.
type ModResult struct {
	Mod
	Folders                              []string
	Files                                []FileResult
	FileCounts                           []FileCount
	Summary                              DefSummary
	Matches, FilesMatched, FilesSearched int
}

// Result contains per-mod identities and totals across the whole search, before paging.
type Result struct {
	Mods                                                            []ModResult
	Matches, FilesMatched, FilesSearched, ModsSearched, ModsMatched int
	Shown, Offset                                                   int
	Kept                                                            int    // Matching lines eligible for paging after the per-file cap.
	Unreadable                                                      int    // Failed file reads and directory listings.
	FirstError                                                      string // First read error (with its path), in input/walk order.
	Truncated                                                       bool   // Some matching lines were omitted by PerFile or paging, including Offset.
	Elapsed                                                         time.Duration
}

type fileJob struct {
	mod                      int
	path, rel, folder, asset string
	kind                     Kind
}
type fileHits struct {
	count   int
	matches []Match
	err     error
	summary DefSummary
	offsets []int // byte offsets of matches, when their contexts are still to be computed
}

// pageContext is a shown file whose Within still has to be filled.
type pageContext struct {
	job       fileJob
	mod, file int
	offsets   []int
}

// Search counts one hit per matching line. Unreadable files and folders are skipped; only context errors abort.
// PerFile caps each file before Offset/Limit select the page; totals always count all hits.
func Search(ctx context.Context, mods []Mod, o Options) (*Result, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Offset < 0 || o.Limit < 0 || o.PerFile < 0 || o.Workers < 0 {
		return nil, fmt.Errorf("search offset, limit, per-file cap, and workers must be non-negative")
	}
	if o.Kinds & ^allKinds != 0 {
		return nil, fmt.Errorf("search kinds contain unsupported bits: %#x", o.Kinds & ^allKinds)
	}
	if o.Limit == 0 {
		o.Limit = 50
	}
	if o.PerFile == 0 {
		o.PerFile = 5
	}
	if o.Kinds == 0 {
		o.Kinds = DefaultKinds
		if o.Def {
			o.Kinds = Defs | Patches
		}
	}
	if len(o.Langs) > 0 {
		o.Kinds |= Languages
	}
	if o.FS == nil {
		o.FS = OS
	}
	if o.Workers == 0 {
		o.Workers = min(16, max(4, runtime.GOMAXPROCS(0)))
	}
	matcher, err := newMatcher(o)
	if err != nil {
		return nil, err
	}
	r := &Result{Mods: make([]ModResult, len(mods)), Offset: o.Offset, ModsSearched: len(mods)}
	listings := make([]listing, len(mods))
	runWorkers(ctx, len(mods), o.Workers, func(next <-chan int) {
		for i := range next {
			listings[i] = listFiles(ctx, mods[i], o)
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var jobs []fileJob
	for i, listed := range listings {
		if listed.err != nil {
			return nil, listed.err
		}
		r.Unreadable += listed.unreadable
		if r.FirstError == "" {
			r.FirstError = listed.firstError
		}
		r.Mods[i] = ModResult{Mod: mods[i], Folders: listed.folders, FilesSearched: len(listed.files)}
		for _, f := range listed.files {
			f.mod = i
			jobs = append(jobs, f)
		}
	}
	hits := make([]fileHits, len(jobs))
	runWorkers(ctx, len(jobs), o.Workers, func(next <-chan int) {
		var scratch []byte
		for i := range next {
			hits[i] = matcher.search(ctx, jobs[i], o, &scratch)
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kept := 0
	var pending []pageContext
	for i, hit := range hits {
		r.FilesSearched++
		if hit.err != nil {
			if errors.Is(hit.err, context.Canceled) || errors.Is(hit.err, context.DeadlineExceeded) {
				return nil, hit.err
			}
			r.Unreadable++
			if r.FirstError == "" {
				r.FirstError = errText(jobs[i].path, hit.err)
			}
			continue
		}
		j := jobs[i]
		mr := &r.Mods[j.mod]
		if hit.count == 0 {
			continue
		}
		mr.Matches += hit.count
		mr.FilesMatched++
		mr.FileCounts = append(mr.FileCounts, FileCount{Rel: j.rel, Count: hit.count})
		mr.Summary.add(hit.summary)
		r.Matches += hit.count
		r.FilesMatched++
		file := FileResult{Rel: j.rel, Folder: j.folder, Kind: j.kind, Count: hit.count}
		var offs []int
		for mi, m := range hit.matches {
			if kept >= o.Offset && r.Shown < o.Limit {
				file.Matches = append(file.Matches, m)
				if hit.offsets != nil {
					offs = append(offs, hit.offsets[mi])
				}
				r.Shown++
			}
			kept++
		}
		if len(file.Matches) > 0 {
			mr.Files = append(mr.Files, file)
			if offs != nil {
				pending = append(pending, pageContext{job: j, mod: j.mod, file: len(mr.Files) - 1, offsets: offs})
			}
		}
	}
	// The enclosing def / xpath costs an XML pass, so it is computed only for shown files.
	for _, p := range pending {
		b, err := o.FS.ReadFile(p.job.path)
		if err != nil {
			continue
		}
		contexts(ctx, textenc.UTF8(b), p.job.kind, o, r.Mods[p.mod].Files[p.file].Matches, p.offsets)
	}
	for _, m := range r.Mods {
		if m.Matches > 0 {
			r.ModsMatched++
		}
	}
	r.Kept = kept
	r.Truncated = r.Matches > r.Shown
	r.Elapsed = time.Since(start)
	return r, nil
}

// runWorkers bounds both listing and matching without changing their indexed result order.
func runWorkers(ctx context.Context, n, workers int, work func(<-chan int)) {
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(workers, n); w++ {
		wg.Add(1)
		go func() { defer wg.Done(); work(next) }()
	}
send:
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			break send
		case next <- i:
		}
	}
	close(next)
	wg.Wait()
}

func (s *DefSummary) add(other DefSummary) {
	if len(other.Roles) > 0 && s.Roles == nil {
		s.Roles = map[string]int{}
	}
	for role, count := range other.Roles {
		s.Roles[role] += count
	}
	s.Defined = append(s.Defined, other.Defined...)
	s.Abstracts = append(s.Abstracts, other.Abstracts...)
	s.Children = append(s.Children, other.Children...)
	s.PatchOperations += other.PatchOperations
}
