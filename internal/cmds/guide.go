package cmds

// `archo guide` and the MCP tool guide (05 §5.3): the detail behind the short MCP
// Instructions and tool descriptions, read when an agent needs it (progressive disclosure).
// Topics are the workflows below and every MCP tool by name (guide_tools.go).

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "guide", Args: "[<topic>]", Summary: "the agent guide behind the MCP tools: a workflow (diagnose, errors, plans, words) or one tool's full reference",
		MaxArgs: 1, Run: guideCmd})
}

// AllOnlyTools are the MCP tools only `archo mcp --tools all` serves (05 §5.2): the
// player's management tools. The topic list marks them the same way on every surface (I1);
// TestToolGroups (internal/surface/mcp) keeps this in step with the MCP tool table.
var AllOnlyTools = map[string]bool{
	"manage_subscriptions": true, "manage_packs": true, "manage_library": true, "exchange_list": true,
	"manage_textures": true, "estimate_memory": true, "maintenance": true, "resolve_identity": true,
	"manage_hidden": true, "check_links": true, "repair_links": true, "mute_version_warnings": true,
	"set_player_data": true, "preview_uninstall": true,
}

// guideTopics are the workflow topics, in the order the topic list shows them.
var guideTopics = []struct{ name, summary, text string }{
	{"diagnose", "the workflow for a game error or crash: classify it, which tool for which evidence, how to report", guideDiagnose},
	{"errors", "error kinds the workflow does not cover (save loading, config errors, missing assets, assembly loading, noise), most frequent first", patternChecklist},
	{"plans", "plans and their versions, the library, subscriptions and textures, and editing mod files safely", guidePlans},
	{"words", "what the interface calls things, for talking to the player", guideWords},
}

// Guide returns a topic's text: a workflow or an MCP tool's full reference.
func Guide(topic string) (string, bool) {
	for _, g := range guideTopics {
		if g.name == topic {
			return g.text, true
		}
	}
	for _, g := range toolGuides {
		if g.name == topic {
			return g.text, true
		}
	}
	return "", false
}

// GuideTopics lists every topic name: the workflows, then the tools.
func GuideTopics() []string {
	var out []string
	for _, g := range guideTopics {
		out = append(out, g.name)
	}
	for _, g := range toolGuides {
		out = append(out, g.name)
	}
	return out
}

func guideCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if len(a.Pos) == 0 {
		d, root := render.New("guide: topics (MCP: guide topic=<topic>; CLI: archo guide <topic>)")
		var rows [][]string
		for _, g := range guideTopics {
			rows = append(rows, []string{g.name, g.summary})
		}
		rows = append(rows, []string{"<tool>", "an MCP tool's full reference: every action, parameter details, an example call and output"})
		root.AddColumns(rows)
		var tools, more []string
		for _, g := range toolGuides {
			if AllOnlyTools[g.name] {
				more = append(more, g.name)
			} else {
				tools = append(tools, g.name)
			}
		}
		root.KV("tools", strings.Join(tools, ", "))
		root.KV("only with archo mcp --tools all", strings.Join(more, ", "))
		return d, nil
	}
	topic := strings.TrimSpace(a.Pos[0])
	text, ok := Guide(topic)
	if !ok {
		var names []string
		for _, g := range guideTopics {
			names = append(names, g.name)
		}
		return nil, &UsageError{Msg: fmt.Sprintf("unknown guide topic %q: use %s or an MCP tool name such as analyze_log (archo guide lists them)", topic, strings.Join(names, ", "))}
	}
	// Prose, returned as written: its MCP examples keep their #refs.
	return &render.Doc{Raw: "guide: " + topic + "\n" + strings.TrimRight(text, "\n") + "\n"}, nil
}

const guideDiagnose = `Diagnosis workflow (use the exact evidence, not paraphrases):
1. Start from the error. If the user pasted log text, call analyze_log with text=…; otherwise analyze_log (current Player.log), then analyze_log cluster=E<n> for the cluster in question. Each cluster lists candidate mods with a role: source (its code/data produced the error), patcher (only has a Harmony patch on a frame), dependency (provides something that is missing or disabled), mentioned (appears in the message or a lower frame). Never call a patcher or a mentioned mod the cause without further evidence.
   Evidence that is the mod's own (log message prefix [Mod], a patch file or log file in its folder, "defines … which is the def the XML error is in", "loads X.dll") keeps it the source unless a tool result contradicts it: a patch-failed cluster's owner stays the source even when get_patches failed=true lists nothing for it (the simulation runs on the installed files; the player's copy, list or order may differ) — only the reason stays open. A framework that only provides the class, field or type an error mentions (Vanilla Expanded Framework, HugsLib, Harmony) is not the source of an error in another mod's def, patch or code; a mod that only calls into failing code (it runs every static constructor, lists every assembly's types, uploads the log) is not its owner.
   When the cluster names no source and no tool result shows a mod's own file, def or code failing, say the cause is undecided: many errors (vanilla code, save data of removed mods, a texture no installed mod ships) cannot be pinned on a mod, and naming one there is wrong.
2. Confirm identity and state with get_mod: the exact instance (#id), whether it is enabled and at which position, its path, other copies in its lineage.
3. Classify the failure:
   - XML (PatchOperation failure, missing cross-reference, duplicate/missing parent def, malformed XML): use get_patches failed=true (reasons: target-inactive, target-missing, target-later, wrong-version-route, stale-name, unguarded-optional, node-absent), get_def, search_files. What a reason means for the player: target-inactive / target-missing / unguarded-optional = a compatibility patch for a mod that is not active (harmless, the mod should guard it); target-later = load order; stale-name / node-absent with the target mod active = the patch is outdated for that mod's version; wrong-version-route = the mod's own folder layout.
   - Material/icon (_MainTex, ResolveIcon): XML-side first; search_files for uiIconColor without uiIconPath/texPath in the mods named.
   - Assembly (C# exception, Pawn.Tick, ThingWithComps.Tick, world tick): get_harmony target=<method> for every patched frame, then read_source type=<patch class> or search=<pattern>. For tick crashes check AllComps mutations in the cluster's candidates: read_source search="AllComps.Remove(" in=[…] (also "AllComps.Add(", "InitializeComps", "PostExposeData").
   - Assembly loading (ReflectionTypeLoadException, "Could not resolve type … expected class X in assembly A", TypeLoadException, "Error in static constructor of X"): the failing assembly or type is named in the message or the lines next to it; analyze_log says which enabled mod loads that DLL, or that none does (then no listed mod can be named from that line). A type in namespace Harmony (0Harmony 1.x) means an assembly built for Harmony 1; mods that bundle an old 0Harmony.dll are not missing dependencies.
   - Mixed: confirm the XML side loaded (get_patches failed=true) before blaming code.
   - Anything else (save loading, config errors, missing textures, noise): guide topic=errors.
4. list_problems gives the static picture (dependencies, incompatibilities, load order, def overrides, patch failures, Harmony conflicts); explain_relation explains two specific mods.
5. Report: failure stage, XML/Assembly/Mixed, the exact mod instance and path, the file+line or class+method, why it fails, whether a named mod is cause, dependency or observer, and the smallest fix.
6. If it worked before: list_changes (mods that changed) and plan_history action=diff (what changed in the plan since a version).`

const guidePlans = `Plans: every editing tool acts on the current plan. edit_loadout changes it (stored at once, every step undoable) and never touches the game; edit_loadout action=checkout goes back to a version from plan_history. Nothing reaches the game until it is saved: save_loadout saves the current plan without starting the game, launch_game saves it and starts the game (refused while any game runs). manage_plans lists, creates, switches (writes no game file), renames and removes plans.
Inventory: list_library lists every mod on this machine with its copies, sizes, states and the plans using it; manage_library moves a spare Mods/ copy into the lock library or takes mods out of every plan. manage_subscriptions adds, checks, updates and removes the subscriptions RimArcho keeps itself (Git repositories, download pages, Workshop items downloaded with SteamCMD). manage_textures converts mods' PNG textures to DDS next to them (lighter loading and video memory; the PNGs stay) and takes them back.
Editing mod files: pin_mod first creates a locked version (a copy the current plan uses instead; never edit Workshop files in place — Steam overwrites them), then edit the copy.`

const guideWords = `Talking to the player: the interface calls a plan a list, an environment a game, the library the mod library, save_loadout "save", pin_mod "lock this version", swap "switch version" and a pack a pack (06 §7.2); use those words with the player, and the tool names only in calls.`
