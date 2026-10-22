package app

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Rules from the G7 agent evaluation (spikes/agenteval): who calls failing code is not its
// owner, HugsLib's log upload is not a mod problem, and Harmony 1.x type references name a
// library, not a dependency.

var (
	// The type whose initialisation or construction failed, as the game names it.
	failingTypeRe = regexp.MustCompile("static constructor of ([A-Za-z_][\\w.+`]*)|type initializer for '([^']+)'|instantiating a mod of type ([A-Za-z_][\\w.+]*)|Could not load type of field '([A-Za-z_][\\w.]*)")
	// The assembly the game could not list types of.
	asmTypesRe = regexp.MustCompile(`getting types in assembly ([A-Za-z_][\w.\-]*)|Unable To Get Types For: ([A-Za-z_][\w.\-]*)`)
	// A type reference into Harmony 1.x (0Harmony 1.x, namespace Harmony).
	harmony1Re = regexp.MustCompile(`in assembly '0Harmony, Version=1\.|expected class 'Harmony\.`)
	// Harmony's FileLog failing on harmony.log.txt.
	harmonyLogRe = regexp.MustCompile(`(?i)(?:UnauthorizedAccessException|IOException|denied|sharing violation)[^\n]{0,200}harmony\.log\.txt|harmony\.log\.txt[^\n]{0,80}(?:denied|in use)`)
)

// infraCallers are type prefixes of mods that start vanilla flows for the player: HugsLib's
// quickstarter and 1trickPwnyta's Defaults (G6: both named as wrong sources).
var infraCallers = []string{"HugsLib.Quickstart.", "Defaults."}

// dispatchFrames run code they did not write: a mod frame outside them (a profiler, an error
// checker, a loading-screen mod's static-constructor runner) calls into what failed.
var dispatchFrames = map[string]bool{
	"System.Reflection.Assembly.GetTypes":                                           true,
	"System.Reflection.Assembly.GetExportedTypes":                                   true,
	"System.Reflection.RuntimeAssembly.GetTypes":                                    true,
	"System.Runtime.CompilerServices.RuntimeHelpers.RunClassConstructor":            true,
	"System.Runtime.CompilerServices.RuntimeHelpers._RunClassConstructor":           true,
	"System.Activator.CreateInstance":                                               true,
	"Verse.StaticConstructorOnStartupUtility.CallAll":                               true,
	"Verse.ModAssemblyHandler.AssemblyIsUsable":                                     true,
	"Verse.ModAssemblyHandler.ReloadAll":                                            true,
	"Verse.LoadedModManager.CreateModClasses":                                       true,
	"System.Reflection.MonoCMethod.InternalInvoke":                                  true,
	"System.Reflection.RuntimeConstructorInfo.Invoke":                               true,
	"System.RuntimeType.CreateInstanceDefaultCtor":                                  true,
	"System.RuntimeTypeHandle.CreateInstance":                                       true,
	"System.Reflection.Emit.DynamicMethod.CreateDelegate":                           false,
	"HarmonyLib.PatchClassProcessor.Patch":                                          false,
	"HarmonyLib.Harmony.PatchAll":                                                   false,
	"System.Runtime.CompilerServices.RuntimeHelpers.RunModuleConstructor":           true,
	"System.Reflection.RuntimeMethodInfo.Invoke":                                    true,
	"System.Reflection.MethodBase.Invoke":                                           true,
	"System.Reflection.MonoMethod.Invoke":                                           true,
	"System.Reflection.MonoMethod.InternalInvoke":                                   true,
	"System.Reflection.RuntimeMethodInfo.InternalInvoke":                            true,
	"System.Reflection.RuntimeConstructorInfo.InternalInvoke":                       true,
	"System.Reflection.ConstructorInfo.Invoke":                                      true,
	"System.Reflection.Assembly.GetType":                                            false,
	"System.Type.GetType":                                                           false,
	"System.Reflection.Module.GetTypes":                                             true,
	"System.Reflection.RuntimeModule.GetTypes":                                      true,
	"System.Reflection.RuntimeAssembly.GetExportedTypes":                            true,
	"System.AppDomain.GetAssemblies":                                                false,
	"System.Runtime.CompilerServices.RuntimeHelpers.PrepareMethod":                  true,
	"System.Runtime.CompilerServices.RuntimeHelpers.PrepareDelegate":                true,
	"System.Runtime.CompilerServices.RuntimeHelpers.EnsureSufficientExecutionStack": false,
}

// failingTypes are the types the message says failed to initialise or construct.
func failingTypes(msg string) []string {
	var out []string
	for _, m := range failingTypeRe.FindAllStringSubmatch(msg, -1) {
		if t := strings.TrimSpace(m[1] + m[2] + m[3] + m[4]); t != "" {
			out = append(out, strings.TrimSuffix(t, "."))
		}
	}
	return uniqStr(out)
}

// typeOwners maps a type the game names to mods: its declaring type, else (when the type is
// not in the analysis facts) an enabled mod loading an assembly named after its namespace
// (WorldSearchDistance.Main → WorldSearchDistance.dll).
func (cx *attrCtx) typeOwners(t string) ([]string, string) {
	t = strings.ReplaceAll(t, "+", ".")
	if ids := cx.types(t); len(ids) > 0 {
		return ids, "type " + t
	}
	if i := strings.LastIndexByte(t, '.'); i > 0 {
		if ids := cx.types(t[:i]); len(ids) > 0 {
			return ids, "type " + t[:i]
		}
	}
	parts := strings.Split(t, ".")
	for n := len(parts) - 1; n >= 1; n-- {
		ns := strings.Join(parts[:n], ".")
		if ids := cx.assemblyOwners(ns); len(ids) > 0 {
			return ids, "its assembly " + ns + ".dll holds namespace " + ns
		}
	}
	return nil, ""
}

// assemblyOwners are the enabled mods whose loaded folders hold <name>.dll.
func (cx *attrCtx) assemblyOwners(name string) []string {
	if cx.asmMemo == nil {
		cx.asmMemo = map[string][]string{}
		for _, e := range cx.st.List {
			m := cx.st.Ix.ByID[e.Instance]
			if m == nil || cx.official(m.ID) {
				continue
			}
			asms, _ := cx.a.Store.Assemblies(m.ID)
			for _, as := range asms {
				if as.Skipped {
					continue
				}
				base := strings.ToLower(strings.TrimSuffix(path.Base(filepath.ToSlash(as.File)), path.Ext(as.File)))
				cx.asmMemo[base] = append(cx.asmMemo[base], m.ID)
			}
		}
	}
	return uniqStr(cx.asmMemo[strings.ToLower(name)])
}

// callerRules runs after the frame and message rules of attributeCluster.
//
//   - The message names the failing type ("Error in static constructor of X", "type initializer
//     for 'X'", "instantiating a mod of type X"): X's owner is the source; a mod that is only on
//     the stack below it (Loading Progress running every static constructor, Mod Error Checker
//     listing types) calls into it and is only mentioned. When X maps to no mod, the callers
//     are still not the source (G6: 7 of 11 wrong sources were such callers).
//   - A mod frame outside a dispatch frame (Assembly.GetTypes, RunClassConstructor,
//     Activator.CreateInstance …) called into other code: it is mentioned, not the source.
//   - HugsLib's log upload (HugsLib.Logs.*) failing is a network or paste-service problem.
func (cx *attrCtx) callerRules(lc *LogCluster) {
	e := lc.First
	text := e.Message + "\n" + strings.Join(e.Extra, "\n")
	owners := map[string]bool{}
	named := failingTypes(text)
	for _, t := range named {
		ids, via := cx.typeOwners(t)
		for _, id := range ids {
			if cx.official(id) {
				continue
			}
			owners[id] = true
			lc.add(id, RoleSource, fmt.Sprintf("log message: %s failed to initialise (%s)", t, via))
			lc.next(fmt.Sprintf("archo source %s --type %s", cx.st.Ix.Ref(id), outerType(t)))
		}
		if len(ids) == 0 {
			lc.Details = append(lc.Details, fmt.Sprintf("failing type %s: no enabled mod's analysed assemblies define it (search_files / find_mods for its namespace)", t))
		}
	}
	// Mod frames outside a dispatch frame are callers.
	callers := map[string]string{}
	inner := true
	for i, fr := range e.Frames {
		target := fr.Type + "." + fr.Method
		if dispatchFrames[target] {
			inner = false
			continue
		}
		if inner || logs.GameNamespace(fr.Type) {
			continue
		}
		for _, id := range cx.types(fr.Type) {
			if _, ok := callers[id]; !ok {
				callers[id] = fmt.Sprintf("log frame %d %s calls into the failing code (below %s on the stack): a caller, not the owner", i+1, target, "a reflection/initialisation frame")
			}
		}
	}
	if len(named) > 0 {
		// Every frame mod that does not own the failing type is a caller.
		for _, fr := range e.Frames {
			if logs.GameNamespace(fr.Type) {
				continue
			}
			for _, id := range cx.types(fr.Type) {
				if !owners[id] {
					if _, ok := callers[id]; !ok {
						callers[id] = fmt.Sprintf("log frame %s.%s: on the stack of %s's initialisation, not its owner", fr.Type, fr.Method, strings.Join(named, ", "))
					}
				}
			}
		}
	}
	for id, why := range callers {
		if owners[id] {
			continue
		}
		cx.demote(lc, id, why, false)
	}
	// Infrastructure that runs vanilla flows for the player (HugsLib's quickstarter, Defaults
	// applying default policies): the vanilla code it starts fails on someone else's data (G6).
	for _, fr := range e.Frames {
		if logs.GameNamespace(fr.Type) {
			continue
		}
		for _, p := range infraCallers {
			if strings.HasPrefix(fr.Type, p) {
				for _, id := range cx.types(fr.Type) {
					cx.demote(lc, id, fmt.Sprintf("log frame %s.%s runs a vanilla flow (quickstart / default settings); the vanilla code it starts failed on data from elsewhere", fr.Type, fr.Method), false)
				}
			}
		}
		break // only the topmost mod frame
	}
	// Harmony's debug log file cannot be written: the machine, not a mod.
	if harmonyLogRe.MatchString(text) {
		for _, at := range lc.Attr {
			if at.Role == RoleSource {
				cx.demote(lc, at.Instance, "Harmony could not write its debug log (harmony.log.txt): a file-permission problem on this machine with Harmony debug logging on, not this mod's defect", true)
			}
		}
		lc.Details = append(lc.Details, "harmony.log.txt cannot be written (access denied / in use): Harmony debug logging is on and the Desktop (or the log folder) is protected; every mod that patches then fails the same way. Fix the folder permission (e.g. Windows Controlled Folder Access) or turn Harmony debug logging off")
	}
	// HugsLib's log upload.
	upload := false
	for _, fr := range e.Frames {
		if strings.HasPrefix(fr.Type, "HugsLib.Logs.") {
			upload = true
		}
	}
	if upload {
		for _, at := range lc.Attr {
			if at.Role == RoleSource {
				cx.demote(lc, at.Instance, "HugsLib's log upload failed (HugsLib.Logs on the stack): a network or paste-service problem, not a mod's", true)
			}
		}
		lc.Details = append(lc.Details, "HugsLib's log upload failed (the request to the paste service): no mod is at fault; ignore it or upload again")
	}
}

// demote makes a source candidate a mentioned one, with the reason. Without force only a
// source that rests on stack frames alone is demoted: a message prefix, a file path or a
// def it defines is the mod's own evidence.
func (cx *attrCtx) demote(lc *LogCluster, id, why string, force bool) {
	for _, at := range lc.Attr {
		if at.Instance != id {
			continue
		}
		if at.Role == RoleSource && !force {
			for _, ev := range at.Evidence {
				if !strings.HasPrefix(ev, "log frame ") && !strings.HasPrefix(ev, "log harmony:") {
					return
				}
			}
		}
		if at.Role == RoleSource {
			at.Role = RoleMentioned
		}
		at.Evidence = append(at.Evidence, why)
	}
}

// assemblyRules: the game could not list the types of an assembly ("getting types in
// assembly X", "Unable To Get Types For: X") or a type reference points into Harmony 1.x.
// It returns true when the missing-type providers must not be listed as dependencies.
func (cx *attrCtx) assemblyRules(lc *LogCluster) (skipProviders bool) {
	e := lc.First
	text := e.Message + "\n" + strings.Join(e.Extra, "\n")
	if harmony1Re.MatchString(text) {
		skipProviders = true
		lc.Details = append(lc.Details, "the type belongs to Harmony 1.x (0Harmony 1.x, namespace Harmony): an assembly built against Harmony 1 cannot be loaded under RimWorld 1.6's Harmony 2, and the game skips it. Mods that bundle an old 0Harmony.dll are not a missing dependency. The failing assembly is the one named in the ReflectionTypeLoadException line next to this one")
	}
	ms := asmTypesRe.FindAllStringSubmatch(text, -1)
	if len(ms) == 0 && (skipProviders || strings.Contains(text, "from typeref")) && cx.lf != nil {
		// The type-load error names the type; the next lines name the assembly
		// ("ReflectionTypeLoadException getting types in assembly AchtungMod").
		for _, x := range cx.lf.Entries {
			if x.Line > e.Line && x.Line <= e.Line+40 {
				if m := asmTypesRe.FindStringSubmatch(x.Message); m != nil {
					ms = append(ms, m)
					break
				}
			}
		}
	}
	for _, m := range ms {
		name := m[1] + m[2]
		if name == "" || gameAssembly(name) {
			continue
		}
		ids := cx.assemblyOwners(name)
		for _, id := range ids {
			lc.add(id, RoleSource, fmt.Sprintf("loads %s.dll, the assembly whose types could not be listed", name))
			lc.next(fmt.Sprintf("archo harmony --mod %s", cx.st.Ix.Ref(id)))
		}
		if len(ids) == 0 {
			// Who ships it anywhere (inactive version folders, disabled mods)?
			files, _ := cx.a.Store.InstanceFiles(cx.st.Ix.Env.ID, "%"+name+".dll")
			var where []string
			var keys []string
			for id := range files {
				keys = append(keys, id)
			}
			sort.Strings(keys)
			for _, id := range keys {
				for _, rel := range files[id] {
					if strings.EqualFold(path.Base(filepath.ToSlash(rel)), name+".dll") && cx.st.Ix.ByID[id] != nil {
						where = append(where, fmt.Sprintf("%s %s (%s)", cx.st.Ix.Ref(id), rel, map[bool]string{true: "enabled, folder not loaded for this game version", false: "not enabled"}[cx.st.Position(id) > 0]))
						break
					}
				}
			}
			d := fmt.Sprintf("assembly %s: no enabled mod loads %s.dll from the folders the game reads for this version", name, name)
			if len(where) > 0 {
				d += "; installed copies: " + strings.Join(where, ", ")
			}
			lc.Details = append(lc.Details, d+". The player's copy of some mod differs (an old version folder or a stray DLL): no listed mod can be named from this line")
		}
	}
	return skipProviders
}

// nearMessages are up to 4 log messages before and 3 after e (other entries, clipped).
func nearMessages(lf *logs.File, e *logs.Entry) []string {
	if lf == nil || e == nil {
		return nil
	}
	idx := -1
	for i, x := range lf.Entries {
		if x == e {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	var out []string
	add := func(x *logs.Entry, mark string) {
		if x.Line < e.Line-30 || x.Line > e.Line+30 || x.Message == e.Message {
			return
		}
		m := x.Message
		if len(m) > 200 {
			m = m[:197] + "..."
		}
		out = append(out, fmt.Sprintf("%s:%d %s", mark, x.Line, m))
	}
	for i := max(0, idx-4); i < idx; i++ {
		add(lf.Entries[i], "before")
	}
	for i := idx + 1; i < len(lf.Entries) && i <= idx+3; i++ {
		add(lf.Entries[i], "after")
	}
	return out
}
