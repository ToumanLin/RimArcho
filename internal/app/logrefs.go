package app

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/dotnet"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// refQuery is a name the log reports missing, looked up in the enabled mods' own files: the
// effective Defs and Patches XML (element text, and for defs a ThingDefCountClass element name)
// and, with dll, the string literals of their assemblies (code that asks for it by name).
type refQuery struct {
	lc    *LogCluster
	def   string   // short def type of a missing def; "" for a texture
	names []string // the name and its alternatives (a Graphic_Multi base path)
	dll   bool
	// code: the name was asked for at run time (DefDatabase.GetNamed), so only a string
	// literal in an assembly can be the source; XML that lists the name is only mentioned
	// (log corpus: vanilla biome names in animal and plant XML).
	code bool
	// field: an XML error names an element and value ("XML error: <F>V</F> doesn't correspond
	// to any field"); the files holding exactly <F>V</F> are looked for.
	field string
}

// referrerQuery is the lookup a cluster needs, if any.
func referrerQuery(lc *LogCluster) *refQuery {
	msg := lc.First.Message
	switch lc.Class {
	case logs.ClassXrefMissing:
		typ, name, ok := xrefName(msg)
		if !ok || strings.HasPrefix(msg, "Could not load reference") {
			// A save's reference: the save names the def, not a mod's files (log corpus: text
			// matches here were guesses).
			return nil
		}
		code := strings.HasPrefix(msg, "Failed to find ")
		return &refQuery{lc: lc, def: typ[strings.LastIndexByte(typ, '.')+1:], names: []string{name}, dll: code, code: code}
	case logs.ClassTexture:
		p := texturePath(msg)
		if p == "" {
			return nil
		}
		names := []string{p}
		if m := multiRe.FindStringSubmatch(p); m != nil {
			names = append(names, m[1]) // Graphic_Multi: the XML names the base path
		}
		return &refQuery{lc: lc, names: names, dll: true}
	case logs.ClassXMLLoad:
		if m := fieldErrRe.FindStringSubmatch(msg); m != nil && m[2] != "" {
			return &refQuery{lc: lc, field: m[1], names: []string{m[2]}}
		}
	}
	return nil
}

func (cx *attrCtx) enabledMods() []*store.Mod {
	var mods []*store.Mod
	for _, e := range cx.st.List {
		if m := cx.st.Ix.ByID[e.Instance]; m != nil && !cx.official(m.ID) {
			mods = append(mods, m)
		}
	}
	return mods
}

// assemblyPaths are the effective DLLs of an analysed instance, known libraries left out.
func (cx *attrCtx) assemblyPaths(m *store.Mod) []string {
	asms, _ := cx.a.Store.Assemblies(m.ID)
	var out []string
	for _, as := range asms {
		if !as.Skipped {
			out = append(out, filepath.Join(cx.st.Ix.ModDir(m), filepath.FromSlash(as.File)))
		}
	}
	return out
}

var defNameTag = []byte("defName")

// xmlRefs calls hit for each element's text (<a>TEXT</a>, trimmed) and each attribute-less
// start tag (<TEXT>1</TEXT>, elem true), with the byte offset. <defName> text is the def's own
// name, not a reference, and comments are skipped.
func xmlRefs(b []byte, hit func(tag, s []byte, elem bool, off int)) {
	for i := 0; i < len(b); i++ {
		if b[i] != '<' {
			continue
		}
		if bytes.HasPrefix(b[i:], []byte("<!--")) {
			k := bytes.Index(b[i:], []byte("-->"))
			if k < 0 {
				return
			}
			i += k + 2
			continue
		}
		j := i + 1
		if j < len(b) && (b[j] == '/' || b[j] == '!' || b[j] == '?') {
			continue
		}
		for j < len(b) && b[j] != '>' && b[j] != '/' && b[j] != ' ' && b[j] != '\t' && b[j] != '\r' && b[j] != '\n' {
			j++
		}
		name := b[i+1 : j]
		k := bytes.IndexByte(b[j:], '>')
		if k < 0 {
			return
		}
		k += j
		if b[k-1] == '/' { // <a/> or <a x="y"/>
			i = k
			continue
		}
		if j == k {
			hit(nil, name, true, i)
		}
		end := bytes.IndexByte(b[k+1:], '<')
		if end < 0 {
			return
		}
		if text := bytes.TrimSpace(b[k+1 : k+1+end]); len(text) > 0 && len(text) < 256 && !bytes.Equal(name, defNameTag) {
			hit(name, text, false, k+1)
		}
		i = k
	}
}

// referrers finds which enabled mods' files ask for each missing name (B2), reading every file
// once. Core and the DLCs are not searched: their files are full of names that are tags, enum
// values or defs of another type (log corpus), and a text match never makes them a source.
func (cx *attrCtx) referrers(qs []*refQuery) {
	mods := cx.enabledMods()
	if err := cx.a.Store.LoadDecls(mods); err != nil {
		return
	}
	active := map[string]bool{}
	for _, e := range cx.st.List {
		if m := cx.st.Ix.ByID[e.Instance]; m != nil {
			active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
		}
	}
	text := map[string][]int{}  // element text → queries
	elem := map[string][]int{}  // element name → queries (defs only)
	lit := map[string][]int{}   // assembly string literal → queries
	field := map[string][]int{} // element name + "\x00" + text → queries
	for i, q := range qs {
		for _, n := range q.names {
			if q.field != "" {
				field[q.field+"\x00"+n] = append(field[q.field+"\x00"+n], i)
				continue
			}
			text[n] = append(text[n], i)
			if q.def != "" {
				elem[n] = append(elem[n], i)
			}
			if q.dll {
				lit[n] = append(lit[n], i)
			}
		}
	}
	type job struct {
		m    *store.Mod
		path string
		rel  string
		dll  bool
	}
	var jobs []job
	for _, m := range mods {
		dir := cx.st.Ix.ModDir(m)
		r := content.Resolve(dir, m.Decl().LoadFolders, cx.st.GameMM(), func(p string) bool { return active[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
		for _, sub := range []string{"Defs", "Patches"} {
			for _, as := range content.Assets(dir, r.Folders, sub) {
				jobs = append(jobs, job{m, as.Path, as.Rel, false})
			}
		}
		if len(lit) > 0 {
			for _, p := range cx.assemblyPaths(m) {
				rel, _ := filepath.Rel(dir, p)
				jobs = append(jobs, job{m, p, filepath.ToSlash(rel), true})
			}
		}
	}
	type hit struct {
		q    int
		inst string
		file string
		line int    // 0: an assembly's string literal
		name string // the name or alternative that matched
	}
	var mu sync.Mutex
	var hits []hit
	var wg sync.WaitGroup
	next := make(chan job)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range next {
				first := map[int]hit{}
				if j.dll {
					a, err := dotnet.Open(j.path)
					if err != nil {
						continue
					}
					for _, s := range a.UserStrings() {
						for _, q := range lit[s] {
							first[q] = hit{q, j.m.ID, j.rel, 0, s}
						}
					}
				} else {
					b, err := os.ReadFile(j.path)
					if err != nil {
						continue
					}
					xmlRefs(b, func(tag, s []byte, isElem bool, off int) {
						idx := text[string(s)]
						if isElem {
							idx = elem[string(s)]
						} else if len(field) > 0 {
							idx = append(idx[:len(idx):len(idx)], field[string(tag)+"\x00"+string(s)]...)
						}
						for _, q := range idx {
							if _, ok := first[q]; !ok {
								first[q] = hit{q, j.m.ID, j.rel, bytes.Count(b[:off], []byte{'\n'}) + 1, string(s)}
							}
						}
					})
				}
				if len(first) > 0 {
					mu.Lock()
					for _, h := range first {
						hits = append(hits, h)
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, j := range jobs {
		next <- j
	}
	close(next)
	wg.Wait()
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.q != b.q {
			return a.q < b.q
		}
		if pa, pb := cx.st.Position(a.inst), cx.st.Position(b.inst); pa != pb {
			return pa < pb
		}
		if a.file != b.file {
			return a.file < b.file
		}
		return a.line < b.line
	})
	// A mod that defines the missing def itself and refers to it is not an ordinary referrer:
	// the game did not have its definition (log corpus: the current version of the mod often
	// defines what the player's version lacked).
	defines := make([]map[string]string, len(qs))
	for i, q := range qs {
		if q.def == "" {
			continue
		}
		defines[i] = map[string]string{}
		ds, _ := cx.a.Store.DefsNamed(q.names[0], false)
		for _, d := range ds {
			if d.Type == q.def || strings.HasSuffix(d.Type, "."+q.def) {
				defines[i][d.Instance] = fmt.Sprintf("%s:%d", d.File, d.Line)
			}
		}
	}
	refs := make([]map[string]bool, len(qs))
	selfs := make([]map[string]bool, len(qs))
	for _, h := range hits {
		if qs[h.q].code && h.line > 0 {
			continue
		}
		set := &refs[h.q]
		if _, ok := defines[h.q][h.inst]; ok {
			set = &selfs[h.q]
		}
		if *set == nil {
			*set = map[string]bool{}
		}
		(*set)[h.inst] = true
	}
	hadSource := make([]bool, len(qs))
	for i, q := range qs {
		hadSource[i] = hasSource(q.lc)
	}
	// A text match is weak evidence: when many mods match, or another rule already found the
	// source, the matches are only mentioned.
	for _, h := range hits {
		q := qs[h.q]
		where := fmt.Sprintf("at %s:%d", h.file, h.line)
		if h.line == 0 {
			where = "in its code (" + h.file + " string literal)"
		}
		name := h.name
		if q.code && h.line > 0 {
			q.lc.add(h.inst, RoleMentioned, fmt.Sprintf("references %s %s (XML; the failed lookup was made by code)", name, where))
			continue
		}
		if at, self := defines[h.q][h.inst]; self {
			if len(refs[h.q]) > 0 {
				continue
			}
			role := RoleSource
			if len(selfs[h.q]) > 3 || hadSource[h.q] {
				role = RoleMentioned
			}
			q.lc.add(h.inst, role, fmt.Sprintf("references %s %s and defines it itself (%s), but the game did not have that definition", name, where, at))
			continue
		}
		role := RoleSource
		if len(refs[h.q]) > 3 || hadSource[h.q] {
			role = RoleMentioned
		}
		if q.field != "" {
			q.lc.add(h.inst, role, fmt.Sprintf("has <%s>%s</%s> %s", q.field, name, q.field, where))
			continue
		}
		q.lc.add(h.inst, role, fmt.Sprintf("references %s %s", name, where))
	}
	for i, q := range qs {
		if len(refs[i]) == 0 && len(selfs[i]) > 0 {
			q.lc.Details = append(q.lc.Details, "the mod that refers to it also defines it now: the player's version likely lacked the definition (update the mod), or a MayRequire or load-folder condition skipped it")
		}
		cx.rank(q.lc)
	}
}

// rank orders a cluster's candidates by role and sets its confidence after a late rule.
func (cx *attrCtx) rank(lc *LogCluster) {
	sort.SliceStable(lc.Attr, func(i, j int) bool { return roleRank[lc.Attr[i].Role] < roleRank[lc.Attr[j].Role] })
	if len(lc.Attr) > 0 && lc.Attr[0].Role == RoleSource && lc.Confidence == finding.Possible {
		lc.Confidence, lc.Why = finding.Likely, ""
	}
}

// textureProviders: an installed mod that ships a missing texture but is not enabled is a
// missing dependency (only for textures no enabled mod's files asked for).
func (cx *attrCtx) textureProviders(qs []*refQuery) {
	var need []*refQuery
	for _, q := range qs {
		if q.def == "" && q.field == "" && !hasSource(q.lc) {
			need = append(need, q)
		}
	}
	if len(need) == 0 {
		return
	}
	files, err := cx.a.Store.InstanceFiles(cx.st.Ix.Env.ID, "%Textures/%")
	if err != nil {
		return
	}
	// Texture path (lower case, no extension) → instance → file.
	by := map[string]map[string]string{}
	for inst, rels := range files {
		for _, rel := range rels {
			r := filepath.ToSlash(rel)
			k := strings.LastIndex(strings.ToLower(r), "textures/")
			if k < 0 {
				continue
			}
			key := strings.ToLower(strings.TrimSuffix(r[k+len("textures/"):], path.Ext(r)))
			if by[key] == nil {
				by[key] = map[string]string{}
			}
			if _, ok := by[key][inst]; !ok {
				by[key][inst] = r
			}
		}
	}
	for _, q := range need {
		var ids []string
		owners := map[string]string{}
		for _, n := range q.names {
			for inst, rel := range by[strings.ToLower(n)] {
				if _, ok := owners[inst]; !ok {
					owners[inst] = rel
					ids = append(ids, inst)
				}
			}
		}
		sort.Strings(ids)
		for _, inst := range ids {
			if cx.st.Ix.ByID[inst] == nil || cx.official(inst) {
				continue
			}
			if cx.st.Position(inst) == 0 {
				q.lc.add(inst, RoleDependency, fmt.Sprintf("ships %s but is not enabled", owners[inst]))
				q.lc.next("archo enable " + cx.st.Ix.Ref(inst))
			} else {
				q.lc.add(inst, RoleMentioned, fmt.Sprintf("ships %s and is enabled (a load folder the game does not use, or a newer version than the player's)", owners[inst]))
			}
		}
		cx.rank(q.lc)
	}
}

// roUse is how a mod's code uses the ReflectionOnly APIs.
type roUse struct {
	resolver string // the DLL hooking AppDomain.ReflectionOnlyAssemblyResolve
	loader   string // the DLL calling Assembly.ReflectionOnlyLoad(From)
}

// reflectionOnly reads, once per log, which enabled mods' assemblies use the ReflectionOnly APIs.
func (cx *attrCtx) reflectionOnly() map[string]roUse {
	if cx.roMemo != nil {
		return cx.roMemo
	}
	cx.roMemo = map[string]roUse{}
	for _, m := range cx.enabledMods() {
		var u roUse
		for _, p := range cx.assemblyPaths(m) {
			a, err := dotnet.Open(p)
			if err != nil {
				continue
			}
			if u.resolver == "" && a.HasMemberRef("System.AppDomain", "add_ReflectionOnlyAssemblyResolve") {
				u.resolver = filepath.Base(p)
			}
			if u.loader == "" && (a.HasMemberRef("System.Reflection.Assembly", "ReflectionOnlyLoad") || a.HasMemberRef("System.Reflection.Assembly", "ReflectionOnlyLoadFrom")) {
				u.loader = filepath.Base(p)
			}
		}
		if u != (roUse{}) {
			cx.roMemo[m.ID] = u
		}
	}
	return cx.roMemo
}

// missingAssembly: "Cannot resolve dependency to assembly 'X' because it has not been preloaded.
// When using the ReflectionOnly APIs, …". No stack trace names the caller.
func (cx *attrCtx) missingAssembly(lc *LogCluster, name string) {
	lc.Details = append(lc.Details, "missing: assembly "+name)
	if gameAssembly(name) {
		// Log corpus: always UnityEngine.InputLegacyModule or System; 718 of 773 logs have a mod
		// that hooks reflection-only resolution enabled.
		lc.Details = append(lc.Details, name+" ships with the game (Unity/.NET), not with a mod: the line comes from code that resolves the dependencies of assemblies loaded reflection-only; usually harmless")
		ro := cx.reflectionOnly()
		var ids []string
		for id := range ro {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return cx.st.Position(ids[i]) < cx.st.Position(ids[j]) })
		for _, id := range ids {
			u := ro[id]
			if u.resolver != "" {
				lc.add(id, RoleSource, fmt.Sprintf("its code handles reflection-only assembly resolution (%s: AppDomain.ReflectionOnlyAssemblyResolve)", u.resolver))
			}
			if u.loader != "" {
				lc.add(id, RoleMentioned, fmt.Sprintf("its code loads assemblies reflection-only (%s: Assembly.ReflectionOnlyLoad)", u.loader))
			}
		}
		return
	}
	files, _ := cx.a.Store.InstanceFiles(cx.st.Ix.Env.ID, "%"+name+".dll")
	var ids []string
	for id := range files {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var prov []string
	for _, id := range ids {
		for _, rel := range files[id] {
			if !strings.EqualFold(path.Base(filepath.ToSlash(rel)), name+".dll") || cx.st.Ix.ByID[id] == nil {
				continue
			}
			if cx.st.Position(id) == 0 {
				lc.add(id, RoleDependency, fmt.Sprintf("ships %s but is not enabled", rel))
				lc.next("archo enable " + cx.st.Ix.Ref(id))
				prov = append(prov, cx.st.Ix.Ref(id)+" (not enabled)")
			} else {
				lc.add(id, RoleMentioned, fmt.Sprintf("ships %s and is enabled (a load folder the game does not use?)", rel))
				prov = append(prov, cx.st.Ix.Ref(id)+" (active)")
			}
			break
		}
	}
	if len(prov) == 0 {
		lc.Details = append(lc.Details, "providers: none among installed mods")
	} else {
		lc.Details = append(lc.Details, "providers: "+strings.Join(prov, ", "))
	}
}

// saveLoad explains save-load references. Only the holder of a dangling reference is
// attributable: the save names objects and think-tree keys, not the mods that made them.
func (cx *attrCtx) saveLoad(lc *LogCluster) {
	msg := lc.First.Message
	switch {
	case strings.HasPrefix(msg, "Could not resolve reference to object with loadID"):
		what := "an object"
		if m := loadIDRe.FindStringSubmatch(msg); m != nil {
			what = m[2] + " " + m[1]
		}
		lc.Details = append(lc.Details, "raised while loading a save: a saved object refers to "+what+", which the save no longer has (destroyed, or from a removed mod)")
		m := curParentRe.FindStringSubmatch(msg)
		if m == nil {
			return
		}
		t := m[1]
		if logs.GameNamespace(t) {
			lc.Details = append(lc.Details, "the reference is held by the game's own "+t+"; the game drops it (usually harmless)")
			return
		}
		found := false
		for _, id := range cx.types(t) {
			if cx.st.Position(id) > 0 && !cx.official(id) {
				lc.add(id, RoleSource, fmt.Sprintf("its type %s holds the reference (curParent): it saved a reference to %s and did not clear it", t, what))
				lc.next(fmt.Sprintf("archo source %s --type %s", cx.st.Ix.Ref(id), outerType(t)))
				found = true
			}
		}
		if !found {
			lc.Details = append(lc.Details, "the holder "+t+" is in no enabled mod's code")
		}
	case strings.HasPrefix(msg, "Could not find think node with key"):
		lc.Details = append(lc.Details, "raised while loading a save: the save's think-tree node keys no longer match the think trees, because a mod that patches think trees (ThinkTreeDef) was added, removed or updated since the save; usually harmless, pawns start a new job")
	}
}
