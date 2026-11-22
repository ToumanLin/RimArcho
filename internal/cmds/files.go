package cmds

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modsearch"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

var filesSearchFlags = []Flag{
	{Name: "mod", Value: "<ref>", Multi: true, Help: "search this copy; repeat for several mods; pkg: prefers its enabled copy"},
	{Name: "regex", Help: "use a Go regular expression"},
	{Name: "case", Help: "match case"}, {Name: "word", Help: "require identifier boundaries"},
	{Name: "def", Help: "find a defName: definitions, parents, children, patches, and references"},
	{Name: "kind", Value: "<kinds|all>", Help: "comma-separated defs,patches,about,languages,textures,sounds,assemblies"},
	{Name: "languages", Help: "also search translations in Languages/"},
	{Name: "lang", Value: "<Folder>", Multi: true, Help: "only these language folders; repeatable; implies languages"},
	{Name: "all-versions", Help: "search every version and LoadFolders branch without shadowing"},
	{Name: "game", Value: "<with|without|only>", Help: "include official Core/DLC content (default with)"},
	{Name: "mode", Value: "<matches|files|count>", Help: "show matching lines, files, or mod totals"},
	{Name: "page", Value: "<N>", Help: "1-based page (default 1)"},
	{Name: "limit", Value: "<N>", Help: "rows per page (default 40; 0 removes paging)"},
	{Name: "per-file", Value: "<N>", Help: "matching lines kept per file (default 5)"},
	{Name: "all", Help: "remove paging; per-file cap still applies"}, gameVersionFlag,
}

func searchNumber(a *Args, name string, fallback int) (int, error) {
	if !a.Bool(name) {
		return fallback, nil
	}
	n, err := strconv.Atoi(a.Str(name))
	if err != nil || n < 0 || name == "page" && n == 0 {
		return 0, &UsageError{Msg: "--" + name + " needs a non-negative integer (page starts at 1)"}
	}
	return n, nil
}

func filesSearchCmd(c *Ctx, a *Args) (*render.Doc, error) {
	f, err := parseFilesSearch(a)
	if err != nil {
		return nil, err
	}
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	mods, err := searchMods(st, a.List("mod"))
	if err != nil {
		return nil, err
	}
	mods = searchGameFilter(mods, f.game)
	if err := c.App.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	inputs := searchInputs(st, mods)
	active := searchActive(st)
	o := modsearch.Options{Pattern: a.Pos[0], Regex: a.Bool("regex"), CaseSensitive: a.Bool("case"), Word: a.Bool("word"), Def: a.Bool("def"), MM: st.GameMM(), Active: func(pkg string) bool { return active[pkg] }, AllVersions: a.Bool("all-versions"), Kinds: f.kinds, Langs: a.List("lang"), Offset: f.offset, Limit: f.limit, PerFile: f.perFile}
	if f.mode != "matches" {
		o.Offset, o.Limit = 0, 1
	}
	result, err := modsearch.Search(context.Background(), inputs, o)
	if err != nil {
		return nil, &UsageError{Msg: err.Error()}
	}
	scope := searchScope(a, st, o, f, len(inputs))
	d, root := render.New(fmt.Sprintf("files search %q (%s): %d matches in %d files of %d mods (searched %d files in %d mods, %d ms)", o.Pattern, scope, result.Matches, result.FilesMatched, result.ModsMatched, result.FilesSearched, result.ModsSearched, result.Elapsed.Milliseconds()))
	if o.Def {
		searchSummary(root, result)
	}
	rows := searchRows(root, result, f)
	if f.mode == "matches" {
		rows = result.Kept
	}
	if !f.unpaged && rows > f.offset && rows-f.offset > f.limit {
		d.AddNext(searchNext(c, a, f.page+1))
	}
	if result.Unreadable > 0 {
		root.Addf("unreadable: %d files or folders could not be read; first: %s", result.Unreadable, result.FirstError)
	}
	if result.Matches == 0 {
		root.Add("(none)")
		if len(a.List("mod")) == 0 && !a.Bool("kind") && f.game == "with" && !o.AllVersions && f.kinds&modsearch.Languages == 0 {
			root.Add("hint: translations, other versions, and inactive mods are excluded; widen with --languages, --all-versions, or --mod <ref>")
		}
	}
	return d, nil
}

// filesSearchFlagValues are the parsed paging, mode, game and kind flags of files search.
type filesSearchFlagValues struct {
	page, limit, perFile, offset int
	mode, game                   string
	unpaged                      bool
	kinds                        modsearch.Kind
}

// parseFilesSearch reads and checks the flags of files search.
func parseFilesSearch(a *Args) (filesSearchFlagValues, error) {
	var f filesSearchFlagValues
	var err error
	if f.page, err = searchNumber(a, "page", 1); err != nil {
		return f, err
	}
	if f.limit, err = searchNumber(a, "limit", 40); err != nil {
		return f, err
	}
	if f.perFile, err = searchNumber(a, "per-file", 5); err != nil {
		return f, err
	}
	f.mode = a.Str("mode")
	if f.mode == "" {
		f.mode = "matches"
	}
	if f.mode != "matches" && f.mode != "files" && f.mode != "count" {
		return f, &UsageError{Msg: "--mode must be matches, files, or count"}
	}
	f.game = a.Str("game")
	if f.game == "" {
		f.game = "with"
	}
	if f.game != "with" && f.game != "without" && f.game != "only" {
		return f, &UsageError{Msg: "--game must be with, without, or only"}
	}
	f.unpaged = a.Bool("all") || f.limit == 0
	if f.unpaged {
		f.page, f.limit = 1, 100000
	}
	if f.page-1 > int(^uint(0)>>1)/f.limit {
		return f, &UsageError{Msg: "--page is too large for --limit"}
	}
	f.offset = (f.page - 1) * f.limit
	f.kinds = modsearch.DefaultKinds
	if a.Bool("def") {
		f.kinds = modsearch.Defs | modsearch.Patches
	}
	if a.Bool("kind") {
		f.kinds, err = modsearch.ParseKinds(a.Str("kind"))
		if err != nil {
			return f, &UsageError{Msg: err.Error()}
		}
	}
	if a.Bool("languages") || len(a.List("lang")) > 0 {
		f.kinds |= modsearch.Languages
	}
	return f, nil
}

// searchGameFilter keeps the mods that --game allows, in place.
func searchGameFilter(mods []*store.Mod, game string) []*store.Mod {
	filtered := mods[:0]
	for _, m := range mods {
		official := m.Kind == source.Official
		if game == "without" && official || game == "only" && !official {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// searchActive is the set of package ids in the active list.
func searchActive(st *app.LoadoutState) map[string]bool {
	active := map[string]bool{}
	for _, entry := range st.List {
		if m := st.Ix.ByID[entry.Instance]; m != nil {
			active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
		}
	}
	return active
}

// searchInputs describes the mods to search for modsearch.
func searchInputs(st *app.LoadoutState, mods []*store.Mod) []modsearch.Mod {
	inputs := make([]modsearch.Mod, 0, len(mods))
	for _, m := range mods {
		inputs = append(inputs, modsearch.Mod{Pos: st.Position(m.ID), Ref: st.Ix.Ref(m.ID), PackageID: m.PackageID, Name: m.DisplayName(), Copy: location(m), Root: st.Ix.ModDir(m), LoadFoldersXML: m.Decl().LoadFolders})
	}
	return inputs
}

// searchScope describes what files search covered, for its title.
func searchScope(a *Args, st *app.LoadoutState, o modsearch.Options, f filesSearchFlagValues, nInputs int) string {
	scope := "active list"
	if len(a.List("mod")) > 0 {
		scope = fmt.Sprintf("%d mods", nInputs)
	}
	version := st.GameMM()
	if o.AllVersions {
		version = "all versions"
	}
	scope += ", " + version + ", " + searchKinds(f.kinds)
	if len(o.Langs) > 0 {
		scope += ", languages " + strings.Join(o.Langs, ",")
	}
	if f.game != "with" {
		scope += ", game " + f.game
	}
	return scope
}

// searchRows adds each mod's results in the chosen mode and returns the rows counted
// for paging (files and count modes).
func searchRows(root *render.Node, result *modsearch.Result, f filesSearchFlagValues) int {
	rows := 0
	for _, m := range result.Mods {
		pos := searchPosition(m.Pos)
		switch f.mode {
		case "matches":
			if len(m.Files) == 0 {
				continue
			}
			searchMatches(root, pos, m)
		case "files":
			for _, fc := range m.FileCounts {
				if rows >= f.offset && rows-f.offset < f.limit {
					root.Addf("%s %s  %s  %d", pos, m.Ref, fc.Rel, fc.Count)
				}
				rows++
			}
		case "count":
			if m.Matches == 0 {
				continue
			}
			if rows >= f.offset && rows-f.offset < f.limit {
				root.Addf("%s %s %s  %d matches in %d files", pos, m.Ref, m.Name, m.Matches, m.FilesMatched)
			}
			rows++
		}
	}
	return rows
}

// searchMatches adds one mod's matching files and lines.
func searchMatches(root *render.Node, pos string, m modsearch.ModResult) {
	node := root.Addf("%s %s %s  pkg:%s  %s  load: %s", pos, m.Ref, m.Name, m.PackageID, m.Copy, strings.Join(m.Folders, ", "))
	for _, f := range m.Files {
		line := f.Rel
		if more := f.Count - len(f.Matches); more > 0 {
			line += fmt.Sprintf("  (+%d more in this file)", more)
		}
		file := node.Add(line)
		for _, hit := range f.Matches {
			if hit.Line == 0 {
				file.Add(hit.Text)
				continue
			}
			parts := []string{strconv.Itoa(hit.Line)}
			if hit.Role != "" {
				parts = append(parts, hit.Role)
			}
			if hit.Within != "" && !strings.Contains(hit.Text, hit.Within) { // an xpath line already shows it
				parts = append(parts, "in "+hit.Within)
			}
			parts = append(parts, hit.Text)
			file.Add(strings.Join(parts, "  "))
		}
	}
}

// Package references prefer exactly one active copy; explicit instance references stay exact.
func searchMods(st *app.LoadoutState, refs []string) ([]*store.Mod, error) {
	var mods []*store.Mod
	if len(refs) == 0 {
		for _, entry := range st.List {
			if m := st.Ix.ByID[entry.Instance]; m != nil {
				mods = append(mods, m)
			}
		}
		return mods, nil
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		found, err := st.Ix.Resolve(ref)
		if err != nil {
			return nil, err
		}
		if _, isPkg := app.PackageRef(ref); len(found) > 1 && isPkg {
			var active []*store.Mod
			for _, m := range found {
				if st.Position(m.ID) > 0 {
					active = append(active, m)
				}
			}
			if len(active) == 1 {
				found = active
			}
		}
		if len(found) != 1 {
			return nil, &app.Ambiguous{Ref: ref, Mods: found}
		}
		if !seen[found[0].ID] {
			mods = append(mods, found[0])
			seen[found[0].ID] = true
		}
	}
	sort.SliceStable(mods, func(i, j int) bool {
		a, b := st.Position(mods[i].ID), st.Position(mods[j].ID)
		if a == 0 {
			return false
		}
		return b == 0 || a < b
	})
	return mods, nil
}

func searchKinds(k modsearch.Kind) string {
	var names []string
	for _, entry := range []struct {
		kind modsearch.Kind
		name string
	}{
		{modsearch.Defs, "defs"}, {modsearch.Patches, "patches"}, {modsearch.About, "about"}, {modsearch.Languages, "languages"}, {modsearch.Textures, "textures"}, {modsearch.Sounds, "sounds"}, {modsearch.Assemblies, "assemblies"},
	} {
		if k&entry.kind != 0 {
			names = append(names, entry.name)
		}
	}
	return strings.Join(names, ",")
}
func searchPosition(pos int) string {
	if pos == 0 {
		return "-"
	}
	return strconv.Itoa(pos)
}

func searchSummary(root *render.Node, r *modsearch.Result) {
	var defined, abstracts, children, patchers []string
	operations, references := 0, 0
	for _, m := range r.Mods {
		label := searchPosition(m.Pos) + " " + m.Ref + " " + m.Name
		for _, within := range m.Summary.Defined {
			defined = append(defined, label+" ("+within+")")
		}
		for _, within := range m.Summary.Abstracts {
			abstracts = append(abstracts, label+" ("+within+")")
		}
		for _, within := range m.Summary.Children {
			children = append(children, label+" ("+within+")")
		}
		if m.Summary.PatchOperations > 0 {
			patchers = append(patchers, m.Ref)
			operations += m.Summary.PatchOperations
		}
		if m.Summary.Roles["ref"] > 0 || m.Summary.Roles["translation"] > 0 || m.Summary.Roles["patch-value"] > 0 {
			references++
		}
	}
	section := root.Section("summary")
	show := func(name string, values []string) {
		text := "(none)"
		if len(values) > 0 {
			text = strings.Join(values, ", ")
		}
		if name == "defined" && len(values) > 1 {
			text += " (last wins)"
		}
		section.KV(name, text)
	}
	show("defined", defined)
	show("abstract/parents", abstracts)
	show("children", children)
	text := fmt.Sprintf("%d operations in %d mods", operations, len(patchers))
	if len(patchers) > 0 {
		text += " (" + strings.Join(patchers, ", ") + ")"
	}
	section.KV("patched by", text)
	section.Addf("referenced by: %d mods", references)
}

func searchNext(c *Ctx, a *Args, page int) string {
	words := []string{"archo"}
	if c.App.EnvName != "" {
		words = append(words, "--env", searchQuote(c.App.EnvName))
	}
	words = append(words, "files", "search")
	for _, flag := range filesSearchFlags {
		if flag.Name == "page" || flag.Name == "all" {
			continue
		}
		for _, value := range a.List(flag.Name) {
			words = append(words, "--"+flag.Name)
			if flag.Value != "" {
				words = append(words, searchQuote(value))
			}
		}
	}
	words = append(words, "--page", strconv.Itoa(page))
	if strings.HasPrefix(a.Pos[0], "--") {
		words = append(words, "--")
	}
	words = append(words, searchQuote(a.Pos[0]))
	return strings.Join(words, " ")
}
func searchQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\r\n'\"$`;&|<>()[{}]*?!#\\") {
		return s
	}
	escape := "'\\''"
	if runtime.GOOS == "windows" {
		escape = "''"
	}
	return "'" + strings.ReplaceAll(s, "'", escape) + "'"
}
