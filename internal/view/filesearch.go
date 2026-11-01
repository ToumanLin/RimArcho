package view

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modsearch"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Search files (06 §2.9): what `archo files search` (03 §5.5) and `archo source search`
// (03 §5.4) find, for the GUI. Nothing is indexed; every search reads the files again, and
// the page cancels a search it no longer wants.

// FileSearchReq is the page's question.
type FileSearchReq struct {
	Pattern string   `json:"pattern"`
	Code    bool     `json:"code"` // decompiled C# instead of the mods' files
	Regex   bool     `json:"regex"`
	Case    bool     `json:"case"`
	Word    bool     `json:"word"`
	Def     bool     `json:"def"`
	Kinds   []string `json:"kinds"` // modsearch kinds; empty: Defs, Patches, About
	Mods    []string `json:"mods"`  // instance IDs; empty: the open list's enabled mods
	Page    int      `json:"page"`  // 1-based
}

// FileSearchPerPage and FileSearchPerFile cap what one page carries (06 §10).
const (
	FileSearchPerPage = 60
	FileSearchPerFile = 5
	// CodeSearchBudget is how long a C# search decompiles before it answers with what is
	// ready; searching again goes on from the cache.
	CodeSearchBudget = 60 * time.Second
)

// FileSearchLine is a matching line (Line 0: a file matched by its path).
type FileSearchLine struct {
	Line   int    `json:"line"`
	Role   string `json:"role,omitempty"`
	Within string `json:"within,omitempty"`
	Text   string `json:"text"`
}

// FileSearchFile is a file with its matching lines on this page.
type FileSearchFile struct {
	Rel   string           `json:"rel"`
	Path  string           `json:"path"`
	Count int              `json:"count"`
	More  int              `json:"more,omitempty"` // matching lines in the file not shown
	Lines []FileSearchLine `json:"lines"`
}

// FileSearchMod is a mod with its files on this page.
type FileSearchMod struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Pos   int              `json:"pos,omitempty"`
	Count int              `json:"count"` // matching lines in the mod, all pages
	Files []FileSearchFile `json:"files"`
}

// FileSearchPage is one page of results.
type FileSearchPage struct {
	Summary string          `json:"summary"`
	Mods    []FileSearchMod `json:"mods"`
	Page    int             `json:"page"`
	More    bool            `json:"more"`
	Hint    string          `json:"hint,omitempty"`  // nothing found: what the search left out
	Notes   []string        `json:"notes,omitempty"` // unreadable files, assemblies not decompiled yet
}

// FileSearchKinds are the kinds the page offers, in its order.
var FileSearchKinds = []struct{ Kind, Label string }{
	{"defs", "Defs"}, {"patches", "patches"}, {"about", "About"}, {"languages", "translations"},
	{"textures", "textures"}, {"sounds", "sounds"}, {"assemblies", "assemblies"},
}

// FileSearchScope is the page's default scope: the open list's enabled mods.
func FileSearchScope(a *app.App) (string, error) {
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	n := 0
	for _, e := range st.List {
		if st.Ix.ByID[e.Instance] != nil {
			n++
		}
	}
	return Tf("the %d mods %s enables", n, st.Plan.Name), nil
}

// FileSearch runs the page's search; progress gets the decompiling's progress.
func FileSearch(ctx context.Context, a *app.App, q FileSearchReq, progress func(string)) (*FileSearchPage, error) {
	q.Pattern = strings.TrimSpace(q.Pattern)
	if q.Pattern == "" {
		return nil, errors.New(T("type what to look for"))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	var mods []*store.Mod
	if len(q.Mods) == 0 {
		for _, e := range st.List {
			if m := st.Ix.ByID[e.Instance]; m != nil {
				mods = append(mods, m)
			}
		}
	} else {
		for _, id := range q.Mods {
			if m := st.Ix.ByID[id]; m != nil {
				mods = append(mods, m)
			}
		}
	}
	if len(mods) == 0 {
		return nil, errors.New(T("no mods to search"))
	}
	if q.Code {
		return codeSearch(ctx, a, st, mods, q, progress)
	}
	if err := a.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	kinds := modsearch.Kind(0)
	if len(q.Kinds) > 0 {
		if kinds, err = modsearch.ParseKinds(strings.Join(q.Kinds, ",")); err != nil {
			return nil, err
		}
	}
	active := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
		}
	}
	inputs := make([]modsearch.Mod, 0, len(mods))
	byRef := map[string]*store.Mod{}
	for _, m := range mods {
		ref := st.Ix.Ref(m.ID)
		byRef[ref] = m
		inputs = append(inputs, modsearch.Mod{Pos: st.Position(m.ID), Ref: ref, PackageID: m.PackageID, Name: m.DisplayName(), Root: st.Ix.ModDir(m), LoadFoldersXML: m.Decl().LoadFolders})
	}
	o := modsearch.Options{Pattern: q.Pattern, Regex: q.Regex, CaseSensitive: q.Case, Word: q.Word, Def: q.Def, MM: st.GameMM(),
		Active: func(pkg string) bool { return active[pkg] }, Kinds: kinds,
		Offset: (q.Page - 1) * FileSearchPerPage, Limit: FileSearchPerPage, PerFile: FileSearchPerFile}
	r, err := modsearch.Search(ctx, inputs, o)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s", Humanize(st, err.Error()))
	}
	page := &FileSearchPage{Page: q.Page, Mods: []FileSearchMod{}, More: r.Kept > o.Offset+r.Shown}
	page.Summary = Tf("%d matches in %d files of %d mods", r.Matches, r.FilesMatched, r.ModsMatched) + T("; ") +
		Tf("searched %d files of %d mods in %s", r.FilesSearched, r.ModsSearched, r.Elapsed.Round(time.Millisecond))
	for _, m := range r.Mods {
		if len(m.Files) == 0 {
			continue
		}
		mod := byRef[m.Ref]
		row := FileSearchMod{ID: mod.ID, Name: m.Name, Pos: m.Pos, Count: m.Matches}
		for _, f := range m.Files {
			file := FileSearchFile{Rel: f.Rel, Path: filepath.Join(m.Root, filepath.FromSlash(f.Rel)), Count: f.Count, More: f.Count - len(f.Matches), Lines: []FileSearchLine{}}
			for _, h := range f.Matches {
				line := FileSearchLine{Line: h.Line, Role: roleText(h.Role), Text: h.Text}
				if h.Within != "" && !strings.Contains(h.Text, h.Within) {
					line.Within = h.Within
				}
				file.Lines = append(file.Lines, line)
			}
			row.Files = append(row.Files, file)
		}
		page.Mods = append(page.Mods, row)
	}
	if r.Unreadable > 0 {
		page.Notes = append(page.Notes, Tf("%d files or folders could not be read; the first: %s", r.Unreadable, r.FirstError))
	}
	if r.Matches == 0 && len(q.Kinds) == 0 && len(q.Mods) == 0 {
		page.Hint = T("translations, textures, other game versions and mods not enabled are not searched: choose them under kind or scope")
	}
	return page, nil
}

// roleText is a Def search role (03 §5.5) in the player's words.
func roleText(role string) string {
	switch role {
	case "def":
		return T("defines")
	case "abstract":
		return T("abstract")
	case "child":
		return T("inherits")
	case "patch", "patch-value":
		return T("patch")
	case "translation":
		return T("translation")
	case "ref":
		return T("reference")
	}
	return role
}

// codeSearch searches the mods' decompiled C#: assemblies not in the cache are decompiled
// first, for at most CodeSearchBudget.
func codeSearch(ctx context.Context, a *app.App, st *app.LoadoutState, mods []*store.Mod, q FileSearchReq, progress func(string)) (*FileSearchPage, error) {
	var re *regexp.Regexp
	if q.Regex {
		var err error
		if re, err = regexp.Compile("(?i)" + q.Pattern); err != nil {
			return nil, fmt.Errorf("%s: %v", T("not a regular expression"), err)
		}
	}
	limit := q.Page * FileSearchPerPage
	res, err := a.SourceSearchContext(ctx, st, mods, q.Pattern, re, limit+1, CodeSearchBudget, progress)
	if err != nil {
		if errors.Is(err, app.ErrNoILSpy) {
			return nil, errors.New(T("searching C# needs ilspycmd: dotnet tool install --global ilspycmd (it needs the .NET SDK)"))
		}
		return nil, err
	}
	page := &FileSearchPage{Page: q.Page, Mods: []FileSearchMod{}, More: len(res.Hits) > limit}
	page.Summary = Tf("%d matches in the C# of %d assemblies", res.Total, res.Assemblies)
	if res.Assemblies == 0 && res.Pending == 0 {
		page.Summary, page.Hint = "", T("the mods you named have no assemblies")
	}
	from := min((q.Page-1)*FileSearchPerPage, len(res.Hits))
	hits := res.Hits[from:min(limit, len(res.Hits))]
	counts := map[string]int{}
	for _, h := range res.Hits {
		counts[h.Instance]++
	}
	for _, h := range hits {
		if n := len(page.Mods); n == 0 || page.Mods[n-1].ID != h.Instance {
			name := h.Instance
			if m := st.Ix.ByID[h.Instance]; m != nil {
				name = m.DisplayName()
			}
			page.Mods = append(page.Mods, FileSearchMod{ID: h.Instance, Name: name, Pos: st.Position(h.Instance), Count: counts[h.Instance]})
		}
		mod := &page.Mods[len(page.Mods)-1]
		if n := len(mod.Files); n == 0 || mod.Files[n-1].Rel != h.File {
			mod.Files = append(mod.Files, FileSearchFile{Rel: h.File, Path: h.Path, Lines: []FileSearchLine{}})
		}
		f := &mod.Files[len(mod.Files)-1]
		f.Count++
		f.Lines = append(f.Lines, FileSearchLine{Line: h.Line, Text: snippetText(h.Text, 160)})
	}
	if res.Pending > 0 {
		page.Notes = append(page.Notes, Tf("%d assemblies are not decompiled yet: search again to go on", res.Pending))
	}
	page.Notes = append(page.Notes, res.Errors...)
	return page, nil
}

// SearchFolder is the folder a result's file is in, for opening it (ctrl+o on a result):
// only a file inside mod id's folder, or a decompiled file in RimArcho's C# cache.
func SearchFolder(ix *app.Index, home, id, path string) (string, error) {
	inside := func(root string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
	}
	m := ix.ByID[id]
	if m == nil || !inside(ix.ModDir(m)) && !inside(filepath.Join(home, "cache", "source")) {
		return "", errors.New(T("the folder is not there"))
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", errors.New(T("the folder is not there"))
	}
	return dir, nil
}

func snippetText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}
