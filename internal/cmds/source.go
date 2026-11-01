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
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func init() {
	register(&Command{Name: "source", Args: "<ref>", Summary: "decompile a mod's assemblies into the Source library (ilspycmd)", MinArgs: 1, MaxArgs: 1, Run: sourceCmd,
		Flags: []Flag{{Name: "type", Value: "<Namespace.Type>"}, {Name: "all"}}})
	register(&Command{Name: "source game", Summary: "read the current game's code from the Source library", Run: sourceGameCmd,
		Flags: []Flag{{Name: "type", Value: "<Namespace.Type>"}, {Name: "all"}}})
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
		lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
		d, root := render.New(fmt.Sprintf("%s  %s  (%d lines)", label, t, len(lines)))
		root.KV("file", path)
		code := root.Section("source")
		limit := 34
		if a.Bool("all") {
			limit = len(lines)
		}
		for i, l := range lines {
			if i >= limit {
				code.Addf("… %d more lines (archo source %s --type %s --all)", len(lines)-limit, ref, t)
				break
			}
			code.Addf("%4d  %s", i+1, strings.ReplaceAll(strings.TrimRight(l, "\r"), "\t", "    "))
		}
		return d, nil
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
	switch {
	case len(mods) > 0:
	case a.Bool("implicated"):
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
	return d, nil
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
	d, _ := render.New("the current list's mods are in the Source library")
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
	d, root := render.New(fmt.Sprintf("Source library: %d folders %s, %d bytes", p.Folders, verb, p.Bytes))
	for _, path := range p.Paths {
		root.Add(path)
	}
	if !remove && p.Folders > 0 {
		d.AddNext("archo source clean --yes")
	}
	return d, nil
}
