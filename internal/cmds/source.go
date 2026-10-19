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

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/store"
)

func init() {
	register(&Command{Name: "source", Args: "<ref>", Summary: "decompile a mod's assemblies on demand (ilspycmd; cached per DLL hash)", MinArgs: 1, MaxArgs: 1, Run: sourceCmd,
		Flags: []Flag{{Name: "type", Value: "<Namespace.Type>"}, {Name: "all"}}})
	register(&Command{Name: "source search", Args: "<pattern>", Summary: "search decompiled C# (uncached assemblies are decompiled first)", MinArgs: 1, MaxArgs: 1, Run: sourceSearchCmd,
		Flags: []Flag{{Name: "in", Value: "<ref>", Multi: true}, {Name: "implicated", Help: "the mods the newest log and the Harmony warnings point at"},
			{Name: "active", Help: "every enabled mod (prints an estimate first; needs --yes)"}, {Name: "yes", Help: "confirm decompiling every enabled mod"},
			{Name: "regex"}, {Name: "all"}, {Name: "budget", Value: "<seconds>", Help: "stop decompiling after this long and search what is cached"}}})
}

func noILSpy(err error) error {
	if errors.Is(err, app.ErrNoILSpy) {
		return fmt.Errorf("%v", err)
	}
	return err
}

func sourceCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	ds, err := c.App.SourceDLLs(st, m)
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		d, root := render.New(modLabel(st.Ix, m.ID) + "  no assemblies to decompile")
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
		d, root := render.New(fmt.Sprintf("%s  %s  (%d lines)", modLabel(st.Ix, m.ID), t, len(lines)))
		root.KV("file", path)
		code := root.Section("source")
		limit := 34
		if a.Bool("all") {
			limit = len(lines)
		}
		for i, l := range lines {
			if i >= limit {
				code.Addf("… %d more lines (archo source %s --type %s --all)", len(lines)-limit, st.Ix.Ref(m.ID), t)
				break
			}
			code.Addf("%4d  %s", i+1, strings.ReplaceAll(strings.TrimRight(l, "\r"), "\t", "    "))
		}
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("%s  %d assemblies", modLabel(st.Ix, m.ID), len(ds)))
	sec := root.Section("assemblies")
	var errs []string
	for _, x := range ds {
		was := x.Cached
		if err := c.App.DecompileDLL(ctx, st, x); err != nil {
			if errors.Is(err, app.ErrNoILSpy) {
				return nil, noILSpy(err)
			}
			errs = append(errs, x.File+": "+err.Error())
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
	d.AddNext(fmt.Sprintf("archo source search \"<pattern>\" --in %s", st.Ix.Ref(m.ID)))
	return d, nil
}

// activeEstimate prints what searching every enabled mod would decompile (decisions-m4
// §7.4); nil when everything is cached already.
func activeEstimate(c *Ctx, st *app.LoadoutState, mods []*store.Mod, pattern string) (*render.Doc, error) {
	var total, todo int
	var bytes int64
	for _, m := range mods {
		ds, err := c.App.SourceDLLs(st, m)
		if err != nil {
			return nil, err
		}
		for _, x := range ds {
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
	root.KV("estimate", fmt.Sprintf("%.0f MB of DLLs → about %.0f MB of cache, roughly %d min with ilspycmd", float64(bytes)/(1<<20), 3*float64(bytes)/(1<<20), (todo*3/2+59)/60))
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
		if len(mods) == 0 {
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
	res, err := c.App.SourceSearch(st, mods, a.Pos[0], re, limit, budget, progress())
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
		lst.Addf("%s  %s:%d  %s", st.Ix.Ref(h.Instance), h.File, h.Line, t)
	}
	if len(res.Hits) == 0 {
		lst.Add("(none)")
	}
	if res.Total > len(res.Hits) {
		lst.Addf("… %d more (add --all)", res.Total-len(res.Hits))
	}
	return d, nil
}
