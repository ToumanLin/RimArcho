package view

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modsearch"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// These bounds keep retained results small relative to the 80 MB Go target (06 §10).
const FileSearchLineCap = 5000
const FileSearchReadCap = 2 << 20
const FileSearchViewCap = 1 << 20
const FileSearchDefaultFiles = "Defs patches About"

var errFileSearchCap = errors.New("file search result cap")

// ParseFileSearchKinds accepts the page's kinds, including exclusions from the panel.
// A blank field uses the default. A field containing only exclusions removes them
// from the default; CLI/MCP ParseKinds remains unchanged.
func ParseFileSearchKinds(text string) (modsearch.Kind, bool, error) {
	var kinds, exclude modsearch.Kind
	code, noCode, positive := false, false, false
	for _, word := range strings.Fields(text) {
		neg := strings.HasPrefix(word, "-")
		name := strings.TrimPrefix(strings.ToLower(word), "-")
		var kind modsearch.Kind
		switch name {
		case "c#":
			if neg {
				noCode = true
			} else {
				code, positive = true, true
			}
			continue
		case "defs":
			kind = modsearch.Defs
		case "patches":
			kind = modsearch.Patches
		case "about":
			kind = modsearch.About
		case "translations":
			kind = modsearch.Languages
		case "textures":
			kind = modsearch.Textures
		case "sounds":
			kind = modsearch.Sounds
		default:
			return 0, false, fmt.Errorf("%s", Tf("unknown file kind: %s", word))
		}
		if neg {
			exclude |= kind
		} else {
			kinds |= kind
			positive = true
		}
	}
	if !positive {
		kinds = modsearch.DefaultKinds
	}
	return kinds &^ exclude, code && !noCode, nil
}

type FileSearchInfo struct {
	Files    string     `json:"files"`
	Mods     string     `json:"mods"`
	Groups   []LibGroup `json:"groups"`
	Complete string     `json:"complete,omitempty"`
}

// BuildFileSearchInfo only reads the inventory database; it opens no Mod files.
func BuildFileSearchInfo(lib *app.Library, query string, defaults bool) (*FileSearchInfo, error) {
	if defaults {
		name := lib.St.Plan.Name
		if lib.Lists != nil && lib.Lists.Current != "" {
			name = lib.Lists.Current
		}
		query = `lists:"` + name + `"`
	}
	words, filters := LibSplitQuery(query)
	res, err := lib.Filter(LibQueryOf(words, filters, ""))
	if err != nil {
		return nil, err
	}
	return &FileSearchInfo{Files: FileSearchDefaultFiles, Mods: query, Groups: libraryGroups(lib, res), Complete: CompleteLibrary(query, lib)}, nil
}

// FileSearchDelta adds a matching file to a copy. Copies are appended on their first
// delta and never sorted again. Each file occurs only once in a run.
type FileSearchDelta struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Copy string         `json:"copy"`
	File FileSearchFile `json:"file"`
}

type FileSearchUpdate struct {
	Since    int               `json:"since"`
	Next     int               `json:"next"`
	Deltas   []FileSearchDelta `json:"deltas"`
	Searched int               `json:"searched"`
	Total    int               `json:"total"`
	Matches  int               `json:"matches"`
	Files    int               `json:"files"`
	Mods     int               `json:"mods"`
	Done     bool              `json:"done"`
	Stopped  bool              `json:"stopped"`
	Capped   bool              `json:"capped"`
	Summary  string            `json:"summary"`
	Hint     string            `json:"hint,omitempty"`
	Notes    []string          `json:"notes,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// FileSearchRun owns bounded history shared by a worker and GUI/TUI polling.
type FileSearchRun struct {
	mu       sync.Mutex
	deltas   []FileSearchDelta
	status   FileSearchUpdate
	seen     map[string]bool
	complete map[string]bool
	start    time.Time
	ended    time.Time
}

func NewFileSearchRun() *FileSearchRun {
	return &FileSearchRun{seen: map[string]bool{}, complete: map[string]bool{}, start: time.Now()}
}

func (r *FileSearchRun) Update(since int) FileSearchUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	since = max(0, min(since, len(r.deltas)))
	u := r.status
	u.Since, u.Next = since, len(r.deltas)
	u.Deltas = append([]FileSearchDelta{}, r.deltas[since:]...)
	u.Notes = append([]string(nil), u.Notes...)
	end := r.ended
	if end.IsZero() {
		end = time.Now()
	}
	switch {
	case !u.Done && u.Total == 0:
		u.Summary = T("searching…")
	case !u.Done:
		u.Summary = Tf("searching %d of %d mods · %d matches so far", u.Searched, u.Total, u.Matches)
	case u.Capped:
		u.Summary = Tf("%d matches collected; narrow the search", u.Matches)
	case u.Stopped:
		u.Summary = Tf("stopped: %d matches in %d of %d mods", u.Matches, u.Searched, u.Total)
	case u.Matches == 0 && u.Error == "":
		u.Summary = T("not found")
	default:
		u.Summary = Tf("%d matches in %d files of %d mods · %s s", u.Matches, u.Files, u.Mods, fmt.Sprintf("%.1f", end.Sub(r.start).Seconds()))
	}
	return u
}

func (r *FileSearchRun) note(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A damaged tree must not fill memory with read errors.
	if len(r.status.Notes) < 8 {
		r.status.Notes = append(r.status.Notes, text)
	}
}

func (r *FileSearchRun) finished(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.complete[id] {
		r.complete[id] = true
		r.status.Searched++
	}
}

func (r *FileSearchRun) add(id, name, copy string, f FileSearchFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	left := FileSearchLineCap - r.status.Matches
	if left <= 0 {
		r.status.Capped = true
		return errFileSearchCap
	}
	if len(f.Lines) > left {
		f.Lines = f.Lines[:left]
	}
	f.Count = len(f.Lines)
	if f.Count == 0 {
		return nil
	}
	if !r.seen[id] {
		r.seen[id] = true
		r.status.Mods++
	}
	r.status.Matches += f.Count
	r.status.Files++
	r.deltas = append(r.deltas, FileSearchDelta{ID: id, Name: name, Copy: copy, File: f})
	if r.status.Matches >= FileSearchLineCap {
		r.status.Capped = true
		return errFileSearchCap
	}
	return nil
}

// Search runs on a private app session. Cancellation preserves the deltas already
// emitted; a newer request owns a different FileSearchRun and cannot receive them.
func (r *FileSearchRun) Search(ctx context.Context, a *app.App, q FileSearchReq) {
	err := r.search(ctx, a, q)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Done = true
	r.ended = time.Now()
	r.status.Stopped = ctx.Err() != nil
	if err != nil && !errors.Is(err, errFileSearchCap) && ctx.Err() == nil {
		r.status.Error = err.Error()
	}
	if r.status.Matches == 0 && r.status.Error == "" && !r.status.Stopped {
		kinds, code, _ := ParseFileSearchKinds(q.Files)
		var excluded []string
		for _, kind := range []struct {
			bit   modsearch.Kind
			label string
		}{{modsearch.Defs, "Defs"}, {modsearch.Patches, "patches"}, {modsearch.About, "About"}, {modsearch.Languages, "translations"}, {modsearch.Textures, "textures"}, {modsearch.Sounds, "sounds"}} {
			if kinds&kind.bit == 0 {
				excluded = append(excluded, T(kind.label))
			}
		}
		if !code {
			excluded = append(excluded, "C#")
		}
		excluded = append(excluded, T("other game versions"))
		r.status.Hint = Tf("not searched: %s", strings.Join(excluded, ", "))
		if strings.TrimSpace(q.ModsQuery) != "" {
			r.status.Hint += T("; ") + T("the mods field limits copies")
		}
	}
}

func (r *FileSearchRun) Fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Done = true
	r.status.Error = err.Error()
}

func (r *FileSearchRun) search(ctx context.Context, a *app.App, q FileSearchReq) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if strings.TrimSpace(q.Pattern) == "" {
		return errors.New(T("type what to look for"))
	}
	kinds, code, err := ParseFileSearchKinds(q.Files)
	if err != nil {
		return err
	}
	lib, err := a.FileSearchRunLibrary()
	if err != nil {
		return err
	}
	words, filters := LibSplitQuery(q.ModsQuery)
	result, err := lib.Filter(LibQueryOf(words, filters, ""))
	if err != nil {
		return err
	}
	st := lib.St
	var mods []*store.Mod
	labels := map[string]string{}
	var core string
	for _, group := range result.Matches {
		for _, i := range group.Shown {
			if i.Mod == nil || i.Missing() {
				continue
			}
			mods = append(mods, i.Mod)
			labels[i.Mod.ID] = libLabel(i)
			if strings.EqualFold(i.Mod.PackageID, "ludeon.rimworld") {
				core = i.Mod.ID
			}
		}
	}
	if code && q.Game && core == "" {
		for _, group := range lib.Lineages {
			for _, i := range group.Instances {
				if i.Mod != nil && strings.EqualFold(i.Mod.PackageID, "ludeon.rimworld") {
					core = i.Mod.ID
					mods = append(mods, i.Mod)
					labels[core] = libLabel(i)
				}
			}
		}
	}
	r.mu.Lock()
	r.status.Total = len(mods)
	r.mu.Unlock()
	active := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
		}
	}
	o := modsearch.Options{Pattern: q.Pattern, Regex: q.Regex, CaseSensitive: q.Case, MM: st.GameMM(), Kinds: kinds, PerFile: FileSearchLineCap, LineMode: true, Active: func(pkg string) bool { return active[pkg] }}
	// Validate regex before reading a file, including an empty Mod scope.
	expression, err := SearchExpression(q)
	if err != nil {
		return err
	}
	for _, m := range mods {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if kinds == 0 {
			if !code {
				r.finished(m.ID)
			}
			continue
		}
		if err := a.Store.LoadDecls([]*store.Mod{m}); err != nil {
			return err
		}
		root := st.Ix.ModDir(m)
		o.FS = newSearchFS(root)
		input := modsearch.Mod{Ref: m.ID, PackageID: m.PackageID, Name: m.DisplayName(), Root: root, LoadFoldersXML: m.Decl().LoadFolders}
		err := modsearch.Stream(ctx, []modsearch.Mod{input}, o, func(_ modsearch.Mod, f modsearch.FileResult) error {
			file := FileSearchFile{Rel: f.Rel, Path: filepath.Join(root, filepath.FromSlash(f.Rel)), Lines: []FileSearchLine{}}
			for _, h := range f.Matches {
				file.Lines = append(file.Lines, FileSearchLine{Line: h.Line, Text: h.Text, Parts: searchLineParts(h.Text, expression)})
			}
			return r.add(m.ID, m.DisplayName(), labels[m.ID], file)
		}, nil, r.note)
		if err != nil {
			return err
		}
		if !code {
			r.finished(m.ID)
		}
	}
	if code {
		game := core != "" || q.Game
		// Core is only complete after Assembly-CSharp, not its empty Mod folder.
		finished := func(id string) {
			if id == core && game {
				return
			}
			if strings.HasPrefix(id, "game:") && core != "" {
				id = core
			}
			r.finished(id)
		}
		decompiled, err := a.SourceProjects(ctx, st, mods, game, func(d *app.SourceDLL) error {
			// First decompilation may have just created the library root.
			o.FS = newSearchFS(a.SourceDir())
			return filepath.WalkDir(d.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if walkErr != nil {
					r.note(walkErr.Error())
					return nil
				}
				if entry.IsDir() {
					return nil
				}
				if !strings.EqualFold(filepath.Ext(path), ".cs") {
					return nil
				}
				hits, err := modsearch.TextFile(ctx, path, o)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					r.note(err.Error())
					return nil
				}
				if len(hits) == 0 {
					return nil
				}
				rel, _ := filepath.Rel(d.Dir, path)
				file := FileSearchFile{Rel: filepath.ToSlash(filepath.Join(strings.TrimSuffix(filepath.Base(d.File), ".dll"), rel)), Path: path, Lines: []FileSearchLine{}}
				for _, h := range hits {
					file.Lines = append(file.Lines, FileSearchLine{Line: h.Line, Text: h.Text, Parts: searchLineParts(h.Text, expression)})
				}
				for _, id := range d.Shared {
					if strings.HasPrefix(id, "game:") && core != "" {
						id = core
					}
					name, copy := T("Core"), labels[id]
					if m := st.Ix.ByID[id]; m != nil {
						name = m.DisplayName()
					}
					if err := r.add(id, name, copy, file); err != nil {
						return err
					}
				}
				return nil
			})
		}, finished, r.note)
		if decompiled > 0 {
			r.mu.Lock()
			if len(r.status.Notes) == 8 {
				r.status.Notes = r.status.Notes[:7]
			}
			r.status.Notes = append(r.status.Notes, Tf("assemblies decompiled now: %d; they stay in the source library", decompiled))
			r.mu.Unlock()
		}
		if err != nil {
			return err
		}
		for _, m := range mods {
			r.finished(m.ID)
		}
	}
	return ctx.Err()
}
