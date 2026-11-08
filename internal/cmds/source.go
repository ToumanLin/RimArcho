package cmds

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/csharp"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func init() {
	register(&Command{Name: "source", Args: "<ref>", Summary: "decompile a mod's assemblies into the Source library (ilspycmd)", MinArgs: 1, MaxArgs: 1, Run: sourceCmd,
		Flags: []Flag{{Name: "type", Value: "<Namespace.Type>"}, {Name: "member", Value: "<name>", Help: "with --type: only this member, with its line numbers"}, {Name: "all"}}})
	register(&Command{Name: "source game", Summary: "read the current game's code from the Source library", Run: sourceGameCmd,
		Flags: []Flag{{Name: "type", Value: "<Namespace.Type>"}, {Name: "member", Value: "<name>", Help: "with --type: only this member, with its line numbers"}, {Name: "all"}}})
	register(&Command{Name: "source decompile", Summary: "decompile the current list's mods into the Source library", Run: sourceDecompileCmd,
		Flags: []Flag{{Name: "active", Help: "decompile every enabled mod"}}})
	register(&Command{Name: "source clean", Summary: "preview or remove unused Source library folders", Run: sourceCleanCmd,
		Flags: []Flag{{Name: "dry-run", Help: "preview folder count and size"}, {Name: "yes", Help: "remove the unused folders"}}})
	register(&Command{Name: "source search", Args: "<pattern>", Summary: "search decompiled C# (uncached assemblies are decompiled first)", MinArgs: 1, MaxArgs: 1, Run: sourceSearchCmd,
		Flags: []Flag{{Name: "in", Value: "<ref>", Multi: true}, {Name: "implicated", Help: "the mods the newest log and the Harmony warnings point at"},
			{Name: "active", Help: "every enabled mod (prints an estimate first; needs --yes)"}, {Name: "yes", Help: "confirm decompiling every enabled mod"},
			{Name: "game", Help: "include the current game's Assembly-CSharp.dll"}, {Name: "regex"}, {Name: "all"}, {Name: "budget", Value: "<seconds>", Help: "stop starting decompiles after this long; running work finishes"}}})
}

func noILSpy(err error) error {
	if errors.Is(err, app.ErrNoILSpy) {
		return fmt.Errorf("%v", err)
	}
	return err
}

func sourceCmd(c *Ctx, a *Args) (*render.Doc, error) {
	return sourceReadCmd(c, a, false)
}

func sourceGameCmd(c *Ctx, a *Args) (*render.Doc, error) { return sourceReadCmd(c, a, true) }

func sourceReadCmd(c *Ctx, a *Args, game bool) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	var ds []*app.SourceDLL
	label, ref := "RimWorld "+st.Ix.Env.GameVersion, "game"
	if game {
		var d *app.SourceDLL
		d, err = c.App.GameSourceDLL(st)
		if err == nil {
			ds = []*app.SourceDLL{d}
		}
	} else {
		var m *store.Mod
		m, err = st.Ix.ResolveOne(a.Pos[0])
		if err != nil {
			return nil, err
		}
		label, ref = modLabel(st.Ix, m.ID), st.Ix.Ref(m.ID)
		ds, err = c.App.SourceDLLs(st, m)
	}
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		d, root := render.New(label + "  no assemblies to decompile")
		root.Add("the effective content has no Assemblies/*.dll (or only known libraries such as 0Harmony.dll)")
		return d, nil
	}
	ctx := context.Background()
	if t := a.Str("type"); t != "" {
		src, path, err := c.App.DecompileType(ctx, st, ds, t)
		if err != nil {
			return nil, noILSpy(err)
		}
		return typeDoc(label, ref, t, src, path, a.Str("member"), a.Bool("all"))
	}
	if a.Str("member") != "" {
		return nil, &UsageError{Msg: "--member needs --type"}
	}
	d, root := render.New(fmt.Sprintf("%s  %d assemblies", label, len(ds)))
	sec := root.Section("assemblies")
	for _, x := range ds {
		was := x.Cached
		if err := c.App.DecompileDLL(ctx, st, x); err != nil {
			if errors.Is(err, app.ErrNoILSpy) {
				return nil, noILSpy(err)
			}
			sec.Addf("%s  error: %v", x.File, err)
			continue
		}
		n := 0
		filepath.WalkDir(x.Dir, func(p string, de os.DirEntry, err error) error {
			if err == nil && !de.IsDir() && strings.HasSuffix(p, ".cs") {
				n++
			}
			return nil
		})
		state := "decompiled now"
		if was {
			state = "cached"
		}
		sec.Addf("%s  %s, %d .cs files  %s", x.File, state, n, x.Dir)
	}
	if game {
		d.AddNext("archo source search \"<pattern>\" --game")
	} else {
		d.AddNext(fmt.Sprintf("archo source search \"<pattern>\" --in %s", ref))
	}
	return d, nil
}

// A type is shown whole when it is short; a longer one gets its head and a member index.
// Agents read the first lines of a long type, then asked again with --all every time
// (h4 P5, h8 G5): both limits are generous enough that most types fit.
const (
	wholeTypeLines = 250
	wholeTypeBytes = 12 << 10
	typeHeadLines  = 34
	memberIndexMax = 80
)

func numberedLine(n int, l string) string {
	return fmt.Sprintf("%4d  %s", n, strings.ReplaceAll(strings.TrimRight(l, "\r"), "\t", "    "))
}

// typeDoc renders one type of the Source library: whole, or head plus member index, or the
// members named by member (their line numbers are the file's).
func typeDoc(label, ref, typ, src, path, member string, all bool) (*render.Doc, error) {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	outline := csharp.Find(csharp.Parse(src), typ)
	if member != "" {
		if outline == nil {
			return nil, fmt.Errorf("no type declaration found in %s: read it whole with --all", path)
		}
		ms := outline.Match(member)
		if len(ms) == 0 {
			return nil, fmt.Errorf("%s has no member %q; members: %s", typ, member, outline.Index(memberIndexMax))
		}
		d, root := render.New(fmt.Sprintf("%s  %s.%s  (%s)", label, typ, ms[0].Name, memberLines(ms)))
		root.KV("file", path)
		for i, m := range ms {
			code := root.Section("source")
			if len(ms) > 1 {
				code = root.Addf("source %d of %d:", i+1, len(ms))
			}
			for n := m.Start; n <= m.End && n <= len(lines); n++ {
				code.Add(numberedLine(n, lines[n-1]))
			}
		}
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("%s  %s  (%d lines)", label, typ, len(lines)))
	root.KV("file", path)
	whole := all || len(lines) <= wholeTypeLines && len(src) <= wholeTypeBytes
	if !whole && outline != nil {
		root.KV("members", outline.Index(memberIndexMax))
	}
	code := root.Section("source")
	limit := len(lines)
	if !whole {
		limit = typeHeadLines
	}
	for i, l := range lines {
		if i >= limit {
			code.Addf("… %d more lines (archo source %s --type %s --all, or --member <name>)", len(lines)-limit, ref, typ)
			break
		}
		code.Add(numberedLine(i+1, l))
	}
	return d, nil
}

// memberLines names the lines of the matched members: "lines 141-175", "lines 3-5 and 8-10".
func memberLines(ms []csharp.Member) string {
	var parts []string
	for _, m := range ms {
		parts = append(parts, fmt.Sprintf("%d-%d", m.Start, m.End))
	}
	return "lines " + strings.Join(parts, " and ")
}

// activeEstimate prints what searching every enabled mod would decompile (decisions-m4
// §7.4); nil when everything is cached already.
func activeEstimate(c *Ctx, st *app.LoadoutState, mods []*store.Mod, pattern string) (*render.Doc, error) {
	var total, todo int
	var bytes int64
	seen := map[string]bool{}
	for _, m := range mods {
		ds, err := c.App.SourceDLLs(st, m)
		if err != nil {
			return nil, err
		}
		for _, x := range ds {
			if seen[x.Hash] {
				continue
			}
			seen[x.Hash] = true
			total++
			if !x.Cached {
				todo++
				if fi, err := os.Stat(x.Path); err == nil {
					bytes += fi.Size()
				}
			}
		}
	}
	if todo == 0 {
		return nil, nil
	}
	d, root := render.New(fmt.Sprintf("source search --active: %d assemblies in %d enabled mods, %d not decompiled yet", total, len(mods), todo))
	root.KV("estimate", fmt.Sprintf("%.0f MB of DLLs → about %.0f MB in the Source library, roughly %d min with ilspycmd", float64(bytes)/(1<<20), 3*float64(bytes)/(1<<20), (todo*3/2+59)/60))
	root.Add("nothing was decompiled; confirm with --yes, or narrow the scope with --in / --implicated")
	d.AddNext(fmt.Sprintf("archo source search %q --active --yes", pattern), fmt.Sprintf("archo source search %q --implicated", pattern))
	return d, nil
}

func sourceSearchCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	var mods []*store.Mod
	for _, r := range a.List("in") {
		m, err := st.Ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		mods = append(mods, m)
	}
	scope := fmt.Sprintf("%d mods", len(mods))
	kind := "named" // how the scope was chosen: shown in the coverage line
	switch {
	case len(mods) > 0:
	case a.Bool("implicated"):
		kind = "implicated"
		for _, id := range c.App.ImplicatedMods(st) {
			mods = append(mods, st.Ix.ByID[id])
		}
		scope = fmt.Sprintf("%d implicated mods", len(mods))
		if len(mods) == 0 && !a.Bool("game") {
			d, root := render.New("source search: no implicated mods (no error clusters in the newest log, no Harmony warnings)")
			root.Add("name the mods with --in <ref>, or search every enabled mod with --active --yes")
			return d, nil
		}
	case a.Bool("active"):
		kind = "active"
		for _, e := range st.List {
			if m := st.Ix.ByID[e.Instance]; m != nil {
				mods = append(mods, m)
			}
		}
		scope = "the enabled mods"
		if !a.Bool("yes") {
			if d, err := activeEstimate(c, st, mods, a.Pos[0]); d != nil || err != nil {
				return d, err
			}
		}
	case a.Bool("game"):
		scope = "the game"
		kind = "game only"
	default:
		return nil, &UsageError{Msg: "source search needs a scope: --in <ref>, --implicated, or --active --yes"}
	}
	var re *regexp.Regexp
	if a.Bool("regex") {
		if re, err = regexp.Compile("(?i)" + a.Pos[0]); err != nil {
			return nil, &UsageError{Msg: "bad --regex pattern: " + err.Error()}
		}
	}
	var budget time.Duration
	if b := a.Str("budget"); b != "" {
		n, err := strconv.Atoi(b)
		if err != nil || n < 1 {
			return nil, &UsageError{Msg: "--budget takes whole seconds"}
		}
		budget = time.Duration(n) * time.Second
	}
	limit := boolLimit(a, render.DefaultLimit)
	if a.Bool("game") && scope != "the game" {
		scope += " and the game"
	}
	res, err := c.App.SourceSearchContext(context.Background(), st, mods, a.Pos[0], re, limit, budget, progress(), a.Bool("game"))
	if err != nil {
		return nil, noILSpy(err)
	}
	d, root := render.New(fmt.Sprintf("source search %q in %s: %d matches in %d assemblies", a.Pos[0], scope, res.Total, res.Assemblies))
	if res.Decompiled > 0 {
		// Why the call took long: the budget stops new decompiles but never one that runs.
		note := fmt.Sprintf("%s now, %.0f s", render.Plural(res.Decompiled, "assembly"), res.Took.Seconds())
		if res.Game {
			note += " (the game's code is decompiled once per game build)"
		}
		root.KV("decompiled", note)
	}
	if res.Pending > 0 {
		root.KV("pending", fmt.Sprintf("%d assemblies not decompiled yet (still computing; run again)", res.Pending))
	}
	for _, e := range res.Errors {
		root.KV("error", e)
	}
	lst := root.Section("matches")
	for _, h := range res.Hits {
		t := h.Text
		if len(t) > 140 {
			t = t[:137] + "..."
		}
		ref := st.Ix.Ref(h.Instance)
		if strings.HasPrefix(h.Instance, "game:") {
			ref = "game"
		}
		lst.Addf("%s  %s:%d  %s", ref, h.Path, h.Line, t)
		if len(h.Shared) > 1 {
			copies := make([]string, 0, len(h.Shared))
			for _, id := range h.Shared {
				copies = append(copies, modLabel(st.Ix, id))
			}
			lst.Addf("same code in: %s", strings.Join(copies, ", "))
		}
	}
	if len(res.Hits) == 0 {
		lst.Add("(none)")
	}
	if res.Total > len(res.Hits) {
		lst.Addf("… %d more (add --all)", res.Total-len(res.Hits))
	}
	for _, l := range sourceCoverage(c, st, res, kind, a.Bool("game"), scope) {
		root.Add(l)
	}
	return d, nil
}

// sourceCoverage ends a source search with what it did not search (05 §5.4): a search of
// the implicated pair that finds nothing says nothing about the other enabled mods, and
// agents took it for an answer (h7 G2). A restricted search with no match also says where
// to look next.
func sourceCoverage(c *Ctx, st *app.LoadoutState, res *app.SourceSearchResult, kind string, game bool, scope string) []string {
	enabled := map[string]bool{}
	for _, e := range st.List {
		if st.Ix.ByID[e.Instance] != nil {
			enabled[e.Instance] = true
		}
	}
	in, other := 0, 0
	for _, id := range res.Searched {
		if enabled[id] {
			in++
		} else {
			other++
		}
	}
	rest := len(enabled) - in
	hint := "--active --yes"
	if IsMCP(c.Context) {
		hint = `scope="active": repeat while pending`
	}
	var sb strings.Builder
	switch {
	case kind == "game only":
		sb.WriteString("searched the game only")
	default:
		fmt.Fprintf(&sb, "searched %d of %d enabled mods", in, len(enabled))
		if other > 0 {
			fmt.Fprintf(&sb, " and %d not enabled", other)
		}
		if game {
			sb.WriteString(" and the game")
		}
		fmt.Fprintf(&sb, " (%s)", kind)
	}
	if res.Pending > 0 {
		fmt.Fprintf(&sb, "; %d of %d assemblies not decompiled yet: repeat the call", res.Pending, res.Assemblies)
	}
	if rest > 0 {
		fmt.Fprintf(&sb, "; not searched: %d (%s)", rest, hint)
	}
	out := []string{sb.String()}
	if res.Total == 0 && res.Pending == 0 && rest > 0 {
		out = append(out, fmt.Sprintf("no matches in %s: the code you look for may be in one of the %d enabled mods not searched; search them with %s", scope, rest, hint))
	}
	return out
}

func sourceDecompileCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if !a.Bool("active") {
		return nil, &UsageError{Msg: "source decompile requires --active"}
	}
	err := c.App.DecompileActive(context.Background(), func(done, total int, file string) {
		if p := progress(); p != nil {
			p(fmt.Sprintf("decompiled %d/%d assemblies: %s", done, total, file))
		}
	})
	if err != nil {
		return nil, noILSpy(err)
	}
	d, _ := render.New("the current list's mods are in the source library")
	return d, nil
}

func sourceCleanCmd(c *Ctx, a *Args) (*render.Doc, error) {
	remove := a.Bool("yes") && !a.Bool("dry-run")
	p, err := c.App.CleanSourceLibrary(context.Background(), remove)
	if err != nil {
		return nil, err
	}
	verb := "unused"
	if remove {
		verb = "removed"
	}
	d, root := render.New(fmt.Sprintf("source library: %d folders %s, %s", p.Folders, verb, render.Size(p.Bytes)))
	for _, path := range p.Paths {
		root.Add(path)
	}
	if !remove && p.Folders > 0 {
		d.AddNext("archo source clean --yes")
	}
	return d, nil
}
