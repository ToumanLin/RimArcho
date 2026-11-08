package cmds_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// The archo commands in an MCP result become tool calls; a command with no tool, a flag the
// translation does not know and a placeholder stay as written (13 §1.1, MCP review P11).
func TestMCPCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ cli, want string }{
		{`archo log --cluster E9`, `analyze_log cluster=E9`},
		{`archo log --prev`, `analyze_log prev=true`},
		{`archo log --cluster E3 --game-version 1.6`, `analyze_log cluster=E3 game_version=1.6`},
		{`archo source fi7x --type Ns.T`, `read_source ref=fi7x type=Ns.T`},
		{`archo source gpto --type ModB.Pawn_Tick_B --all`, `read_source ref=gpto type=ModB.Pawn_Tick_B all=true`},
		{`archo source game --type Verse.Pawn`, `read_source game=true type=Verse.Pawn`},
		{`archo source search "AllComps.Remove(" --in fi7x --in gpto`, `read_source search="AllComps.Remove(" in=[fi7x,gpto]`},
		{`archo source search Foo --implicated`, `read_source search=Foo`},
		{`archo source search Foo --active --yes`, `read_source search=Foo scope=active`},
		{`archo harmony --target Verse.Pawn.Tick`, `get_harmony target=Verse.Pawn.Tick`},
		{`archo patches --mod iqmu --failed`, `get_patches mod=iqmu failed=true`},
		{`archo def ThingDef/Lib_Gun`, `get_def def=ThingDef/Lib_Gun`},
		{`archo mod iqmu`, `get_mod ref=iqmu`},
		{`archo why iqmu gpto`, `explain_relation a=iqmu b=gpto`},
		{`archo files search "Things/Gone"`, `search_files pattern=Things/Gone`},
		{`archo files search Lib_Gun --mod hnyg --def`, `search_files pattern=Lib_Gun mod=hnyg def=true`},
		{`archo problems`, `list_problems min_severity=info`},
		{`archo problems --code harmony.skip-prefix`, `list_problems code=harmony.skip-prefix min_severity=info`},
		{`archo problems --severity error`, `list_problems severity=error`},
		{`archo problems --ref p:a4972d`, `list_problems ref=p:a4972d`},
		{`archo problems show p:a4972d`, `list_problems ref=p:a4972d`},
		{`archo problems ignore p:a4972d`, `ignore_problem ref=p:a4972d`},
		{`archo changes iqmu`, `list_changes ref=iqmu`},
		{`archo enable z3bw`, `edit_loadout action=enable refs=[z3bw]`},
		{`archo enable a b --at 3`, `edit_loadout action=enable at=3 refs=[a,b]`},
		{`archo disable a`, `edit_loadout action=disable refs=[a]`},
		{`archo undo`, `edit_loadout action=undo`},
		{`archo sort --dry-run`, `edit_loadout action=sort dry_run=true`},
		{`archo save`, `save_loadout`},
		{`archo pin iqmu --dry-run`, `pin_mod ref=iqmu dry_run=true`},
		{`archo pattern pattern:g:rt.tick`, `analyze_log pattern=pattern:g:rt.tick`},
		{`archo workshop resubscribe 6olp --dry-run`, `manage_library action=resubscribe dry_run=true refs=[6olp]`},
		{`archo workshop subscribe nyh3`, `manage_library action=subscribe refs=[nyh3]`},
	} {
		got, ok := cmds.MCPCall(c.cli)
		if !ok || got != c.want {
			t.Errorf("%s\n got  %q (%v)\n want %q", c.cli, got, ok, c.want)
		}
	}
	// No tool, an unknown flag, a missing argument, a placeholder: kept as written.
	for _, cli := range []string{
		`archo plan use default`, `archo source clean`, `archo log --nonsense`, `archo source`,
		`archo source search`, `archo mod`, `archo enable`, `archo pattern --mod <packageId>`,
		`archo source search "AllComps.Remove(" --in <refs>`, `archo files search`, `plan use`,
	} {
		if got, ok := cmds.MCPCall(cli); ok {
			t.Errorf("%s translated to %q", cli, got)
		}
	}
}

func TestMCPHintsInText(t *testing.T) {
	t.Parallel()
	in := "log:E2 exception x1  → #iqmu (source)\n" +
		"  fixes: nothing archo can run here\n" +
		"next:\n" +
		"  archo source iqmu --type ModA.CompBroken\n" +
		"  archo source search \"AllComps.Remove(\" --in iqmu --in gpto\n" +
		"  archo plan use default\n" +
		"changes: 1 (archo changes iqmu)\n" +
		"hidden: 2 informational clusters (archo log --all)\n"
	want := "log:E2 exception x1  → #iqmu (source)\n" +
		"  fixes: nothing archo can run here\n" +
		"next:\n" +
		"  read_source ref=iqmu type=ModA.CompBroken\n" +
		"  read_source search=\"AllComps.Remove(\" in=[iqmu,gpto]\n" +
		"  archo plan use default\n" +
		"changes: 1 (list_changes ref=iqmu)\n" +
		"hidden: 2 informational clusters (analyze_log all=true)\n"
	if got := cmds.MCPHints(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := cmds.MCPHints("no commands here\n"); got != "no commands here\n" {
		t.Errorf("%q", got)
	}
}
