package app

// Call-site tables (05 §5.4, read_source mode=callers): every call, callvirt and newobj of a
// method in the game's Assembly-CSharp.dll and the enabled Mods' assemblies, with the shape
// of each argument. The null trace's "game contract" line is the same summary (06 §2).

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/harmony"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// CallerSite is one call site with the assembly it is in.
type CallerSite struct {
	Instance string // "" for the game
	File     string // the DLL's file name
	dotnet.CallSite
}

// Game reports a site in the game's assembly.
func (s CallerSite) Game() bool { return s.Instance == "" }

// CallersResult is the call-site table of one target.
type CallersResult struct {
	Target     dotnet.Target
	Params     []string // the target's parameter names (from its definition), when known
	Signature  string   // the target's parameter types, when one overload is called
	Sites      []CallerSite
	Assemblies int // assemblies that reference the target
	Scanned    int // assemblies read
	Errors     []string
}

// callCache keeps scan results per DLL hash and target, for the life of the process (an
// MCP session asks the same table again; the null trace and read_source share it).
var callCache sync.Map // hash|target → []dotnet.CallSite

// hashMemo: path|size|mtime → SHA-256, so an unchanged DLL is hashed once.
var hashMemo sync.Map

// fileVersion is the memo key of a file as it is now.
func fileVersion(p string) (string, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s|%d|%d", p, fi.Size(), fi.ModTime().UnixNano()), nil
}

func targetKey(t dotnet.Target) string {
	return fmt.Sprintf("%s|%s|%d", strings.ReplaceAll(t.Owner, "+", "."), t.Name, t.Params)
}

// mentionsTarget: the file holds the target type's and method's names (#Strings), so it can
// reference it; most Mod assemblies are skipped without reading their metadata.
func mentionsTarget(b []byte, t dotnet.Target) bool {
	typ := t.Owner
	if i := strings.LastIndexAny(typ, ".+"); i >= 0 {
		typ = typ[i+1:]
	}
	if !bytes.Contains(b, []byte(typ+"\x00")) {
		return false
	}
	return t.Name == ".ctor" || bytes.Contains(b, []byte(t.Name+"\x00"))
}

// scanCallSites returns the target's call sites in one DLL, from the cache when its hash was
// scanned for this target before.
func scanCallSites(path string, t dotnet.Target) ([]dotnet.CallSite, error) {
	ver, err := fileVersion(path)
	if err != nil {
		return nil, err
	}
	if h, ok := hashMemo.Load(ver); ok {
		if v, ok := callCache.Load(h.(string) + "|" + targetKey(t)); ok {
			return v.([]dotnet.CallSite), nil
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	h := fmt.Sprintf("%x", sha256.Sum256(b))
	hashMemo.Store(ver, h)
	key := h + "|" + targetKey(t)
	if v, ok := callCache.Load(key); ok {
		return v.([]dotnet.CallSite), nil
	}
	var sites []dotnet.CallSite
	if mentionsTarget(b, t) {
		a, err := dotnet.Open(path)
		if err != nil {
			return nil, err
		}
		sites = a.CallSites(t)
	}
	callCache.Store(key, sites)
	return sites, nil
}

// GameAssemblyPath is the environment's Assembly-CSharp.dll ("" when not found).
func GameAssemblyPath(st *LoadoutState) string {
	if d := env.ManagedDir(st.Ix.Env.GameRoot); d != "" {
		return filepath.Join(d, "Assembly-CSharp.dll")
	}
	return ""
}

// Callers builds the call-site table of t in the game's assembly (game) and the given Mods'
// effective assemblies.
func (a *App) Callers(st *LoadoutState, t dotnet.Target, mods []*store.Mod, game bool) (*CallersResult, error) {
	r := &CallersResult{Target: t}
	if game {
		p := GameAssemblyPath(st)
		if p == "" {
			r.Errors = append(r.Errors, "the game's Assembly-CSharp.dll was not found")
		} else {
			r.add("", p, t)
			if ga, err := dotnet.Open(p); err == nil {
				if rows := ga.FindMethods(t.Owner, t.Name, t.Params); len(rows) == 1 {
					r.Params = ga.ParamNames(rows[0])
				}
			}
		}
	}
	for _, m := range mods {
		ps, err := a.modDLLPaths(st, m)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			r.add(m.ID, p, t)
		}
	}
	sigs := map[string]bool{}
	for _, s := range r.Sites {
		sigs[strings.Join(s.Callee.Types, ", ")] = true
	}
	if len(sigs) == 1 {
		for k := range sigs {
			r.Signature = k
		}
	}
	return r, nil
}

func (r *CallersResult) add(inst, path string, t dotnet.Target) {
	r.Scanned++
	sites, err := scanCallSites(path, t)
	if err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", filepath.Base(path), err))
		return
	}
	if len(sites) > 0 {
		r.Assemblies++
		// A Mod's own method: the parameter names of its definition.
		if len(r.Params) == 0 && sites[0].Callee.Def != 0 {
			if a, err := dotnet.Open(path); err == nil {
				r.Params = a.ParamNames(sites[0].Callee.Def)
			}
		}
	}
	for _, s := range sites {
		r.Sites = append(r.Sites, CallerSite{Instance: inst, File: filepath.Base(path), CallSite: s})
	}
}

// ParamName names parameter i of the target.
func (r *CallersResult) ParamName(i int) string {
	if i < len(r.Params) && r.Params[i] != "" {
		return r.Params[i]
	}
	return fmt.Sprintf("param %d", i+1)
}

// NParams is the largest argument count among the sites.
func (r *CallersResult) NParams() int {
	n := 0
	for _, s := range r.Sites {
		n = max(n, len(s.Args))
	}
	return n
}

// Summary sums up parameter i over the sites: "recipient: game sites 7, null at 0 (param 5,
// field 2); Mod sites 1, null at 1 (TraceA.PlayLogEntry_Mutter..ctor)".
func (r *CallersResult) Summary(i int) string {
	part := func(game bool) string {
		n, nulls := 0, 0
		kinds := map[string]int{}
		var where []string
		for _, s := range r.Sites {
			if s.Game() != game || i >= len(s.Args) {
				continue
			}
			n++
			l := s.Args[i].Label()
			kinds[l]++
			if l == "null" {
				nulls++
				where = append(where, s.Caller)
			}
		}
		label := "Mod sites"
		if game {
			label = "game sites"
		}
		out := fmt.Sprintf("%s %d, null at %d", label, n, nulls)
		if n == 0 {
			return fmt.Sprintf("%s 0", label)
		}
		var ks []string
		for k, c := range kinds {
			if k != "null" {
				ks = append(ks, fmt.Sprintf("%s %d", k, c))
			}
		}
		sort.Strings(ks)
		where = uniqStr(where)
		if len(where) > 3 {
			where = append(where[:3], fmt.Sprintf("… %d more", len(where)-3))
		}
		var paren []string
		if len(ks) > 0 && nulls < n {
			paren = append(paren, strings.Join(ks, ", "))
		}
		if len(where) > 0 {
			paren = append(paren, strings.Join(where, ", "))
		}
		if len(paren) > 0 {
			out += " (" + strings.Join(paren, "; ") + ")"
		}
		return out
	}
	return r.ParamName(i) + ": " + part(true) + "; " + part(false)
}

// modDLLPaths are a Mod's effective assemblies without the known libraries (as SourceDLLs,
// without hashing: scanCallSites hashes once per file version).
func (a *App) modDLLPaths(st *LoadoutState, m *store.Mod) ([]string, error) {
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
	var out []string
	for _, as := range content.CodeFiles(dir, r.Folders) {
		if !harmony.SkippedFor(filepath.Base(as.Path), m.PackageID, harmony.DefaultSkip) {
			out = append(out, as.Path)
		}
	}
	return out, nil
}

// enabledCodeMods are the enabled Mods other than Core and the DLCs.
func (a *App) enabledCodeMods(st *LoadoutState) []*store.Mod {
	var out []*store.Mod
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || strings.HasPrefix(strings.ToLower(m.PackageID), "ludeon.rimworld") {
			continue
		}
		out = append(out, m)
	}
	return out
}

// EnabledCodeMods are the enabled Mods other than Core and the DLCs (the default scope of
// a call-site table).
func (a *App) EnabledCodeMods(st *LoadoutState) []*store.Mod { return a.enabledCodeMods(st) }

// gameFrame reports a frame in a game namespace that the game's assembly can hold
// (Verse, RimWorld, LudeonTK), not System or Unity code.
func gameFrame(typ string) bool {
	return logs.GameNamespace(typ) && (strings.HasPrefix(typ, "Verse.") || strings.HasPrefix(typ, "RimWorld.") || strings.HasPrefix(typ, "LudeonTK."))
}
