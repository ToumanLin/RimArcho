package cmds

// archo source callers / source diff: IL facts read without decompiling (05 §5.4).

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func init() {
	register(&Command{Name: "source callers", Args: "<Type.Method>", Summary: "list the calls of a method in the game and the enabled mods, with the shape of each argument", MinArgs: 1, MaxArgs: 1, Run: sourceCallersCmd,
		Flags: []Flag{{Name: "in", Value: "<ref>", Multi: true, Help: "only these mods (and the game)"}, {Name: "all"}}})
	register(&Command{Name: "source diff", Args: "<old.dll> <new.dll>", Summary: "compare two builds of an assembly: changed methods, member and assembly references, attributes", MinArgs: 2, MaxArgs: 2, Run: sourceDiffCmd,
		Flags: []Flag{{Name: "all"}}})
}

func sourceCallersCmd(c *Ctx, a *Args) (*render.Doc, error) {
	t, ok := dotnet.ParseTarget(a.Pos[0])
	if !ok {
		return nil, &UsageError{Msg: "source callers needs Namespace.Type.Method or Namespace.Type..ctor, optionally with the parameter count: Verse.Lord.AddPawn(1)"}
	}
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
	if len(mods) == 0 {
		mods = c.App.EnabledCodeMods(st)
	}
	r, err := c.App.Callers(st, t, mods, true)
	if err != nil {
		return nil, err
	}
	target := a.Pos[0]
	if r.Signature != "" || len(r.Params) > 0 {
		var ps []string
		types := strings.Split(r.Signature, ", ")
		for i := 0; i < max(len(r.Params), r.NParams()); i++ {
			p := r.ParamName(i)
			if i < len(types) && types[i] != "" {
				p = shortCS(dotnet.CSName(types[i])) + " " + p
			}
			ps = append(ps, p)
		}
		target = strings.SplitN(target, "(", 2)[0] + "(" + strings.Join(ps, ", ") + ")"
	}
	d, root := render.New(fmt.Sprintf("callers of %s: %s in %d of %d assemblies", target, render.Plural(len(r.Sites), "site"), r.Assemblies, r.Scanned))
	for _, e := range r.Errors {
		root.KV("error", e)
	}
	if len(r.Sites) == 0 {
		root.Add("(no call, callvirt or newobj of it in the game's Assembly-CSharp.dll or the scanned mods' assemblies)")
		return d, nil
	}
	sum := root.Section("summary")
	for i := 0; i < r.NParams(); i++ {
		sum.Add(r.Summary(i))
	}
	sites := root.Section("sites")
	limit := boolLimit(a, render.DefaultLimit)
	for i, s := range r.Sites {
		if limit > 0 && i >= limit {
			sites.Addf("… %d more (add --all)", len(r.Sites)-i)
			break
		}
		who := "game"
		if !s.Game() {
			who = modLabel(st.Ix, s.Instance)
		}
		kind := dotnet.OpName(s.Op)
		if s.Base {
			kind = "base constructor call"
		}
		var args []string
		for j, v := range s.Args {
			args = append(args, r.ParamName(j)+"="+v.Describe(s.Names))
		}
		sites.Addf("%s  %s IL_%04x %s  %s", who, s.Caller, s.Off, kind, strings.Join(args, ", "))
	}
	return d, nil
}

func sourceDiffCmd(c *Ctx, a *Args) (*render.Doc, error) {
	paths := []string{a.Pos[0], a.Pos[1]}
	var asms [2]*dotnet.Assembly
	for i, p := range paths {
		x, err := dotnet.Open(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p, err)
		}
		asms[i] = x
	}
	// Referenced assemblies: beside either DLL, then the current game's Managed folder.
	dirs := []string{filepath.Dir(paths[1]), filepath.Dir(paths[0])}
	if st, err := c.App.Loadout(); err == nil {
		if g := app.GameAssemblyPath(st); g != "" {
			dirs = append(dirs, filepath.Dir(g))
		}
	}
	open := asmOpener(dirs...)
	r := dotnet.Diff(asms[0], asms[1], open)
	d, root := render.New(fmt.Sprintf("assembly diff %s %s → %s %s: %d changed, %d added, %d removed methods (%d unchanged)",
		r.Old.Name, r.Old.Version, r.New.Name, r.New.Version, len(r.Changed), len(r.Added), len(r.Removed), r.Unchanged))
	limit := boolLimit(a, render.DefaultLimit)
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		s := root.Section(title)
		for i, x := range items {
			if limit > 0 && i >= limit {
				s.Addf("… %d more (add --all)", len(items)-i)
				break
			}
			s.Add(x)
		}
	}
	if n := len(r.Inaccessible); n > 0 {
		var names []string
		for _, x := range firstN2(r.Inaccessible, 3) {
			names = append(names, x.Text)
		}
		more := ""
		if n > 3 {
			more = fmt.Sprintf(" and %d more", n-3)
		}
		was := ""
		if r.OldSkipsVerification {
			was = " (the old build does)"
		}
		root.KV("access", fmt.Sprintf("!! the new build uses %d non-public %s (%s%s) and does not skip verification%s: the game throws FieldAccessException / MethodAccessException when it runs that code. %s",
			n, plural(n, "member", "members"), strings.Join(names, ", "), more, was, rebuildHint))
	}
	list("methods changed", r.Changed)
	list("methods added", r.Added)
	list("methods removed", r.Removed)
	list("types added", r.TypesAdded)
	list("types removed", r.TypesRemoved)
	ignores := map[string]bool{}
	for _, x := range r.IgnoresAccessChecksTo {
		ignores[x] = true
	}
	refLine := func(x dotnet.RefDiff) string {
		l := x.Text
		var notes []string
		if x.Scope != "" {
			notes = append(notes, x.Scope)
		}
		switch {
		case x.Missing:
			notes = append(notes, "not found in that assembly")
		case x.Access != "":
			acc := x.Access
			if x.TypeAccess != "" && x.TypeAccess != dotnet.AccessPublic {
				acc += ", type " + x.TypeAccess
			}
			notes = append(notes, acc)
		case x.Scope != "":
			notes = append(notes, "accessibility unknown: assembly not found")
		}
		if len(notes) > 0 {
			l += "  (" + strings.Join(notes, "; ") + ")"
		}
		switch {
		case !x.NonPublic():
		case r.SkipsVerification:
			l += "  non-public; the build skips verification (unsafe, as Krafs.Publicizer builds it), so the game allows it"
		case !x.Blocked:
			l += "  protected, used from a type that derives from " + x.Owner
		case ignores[x.Scope]:
			l += "  !! non-public: IgnoresAccessChecksTo(" + x.Scope + ") does not help in the game's Mono; it throws FieldAccessException / MethodAccessException. " + rebuildHint
		default:
			l += "  !! non-public and the build does not skip verification: the game throws FieldAccessException / MethodAccessException. " + rebuildHint
		}
		return l
	}
	var added, removed []string
	for _, x := range r.RefsAdded {
		added = append(added, refLine(x))
	}
	for _, x := range r.RefsRemoved {
		removed = append(removed, x.Text)
	}
	list("member references added", added)
	list("member references removed", removed)
	list("assembly references", r.AsmRefs)
	list("attributes added", r.AttrsAdded)
	list("attributes removed", r.AttrsRemoved)
	if len(r.Changed)+len(r.Added)+len(r.Removed)+len(added)+len(removed)+len(r.AsmRefs)+len(r.AttrsAdded)+len(r.AttrsRemoved) == 0 {
		root.Add("no differences in method bodies, references or attributes")
	}
	return d, nil
}

// shortCS drops the namespace: "System.Collections.Generic.List<>" → "List<>".
func shortCS(t string) string {
	head, rest := t, ""
	if i := strings.IndexByte(t, '<'); i >= 0 {
		head, rest = t[:i], t[i:]
	}
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		head = head[i+1:]
	}
	return head + rest
}
