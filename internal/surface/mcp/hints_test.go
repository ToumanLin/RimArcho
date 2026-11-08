package mcp_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// words splits a printed command or tool call into words; a word in double quotes keeps its
// spaces and may hold \".
func words(s string) []string {
	var out []string
	var cur strings.Builder
	quoted, has := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && quoted && i+1 < len(s) && s[i+1] == '"':
			cur.WriteByte('"')
			i++
		case c == '"':
			quoted, has = !quoted, true
		case c == ' ' && !quoted:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

// callJSON parses "tool key=value key=[a,b] key=true" into the tool name and its JSON arguments.
func callJSON(t *testing.T, call string) (string, []byte) {
	t.Helper()
	ws := words(call)
	args := map[string]any{}
	for _, w := range ws[1:] {
		k, v, ok := strings.Cut(w, "=")
		if !ok {
			t.Fatalf("%q: %q is not key=value", call, w)
		}
		switch {
		case v == "true":
			args[k] = true
		case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
			args[k] = strings.Split(v[1:len(v)-1], ",")
		default:
			if n, err := strconv.Atoi(v); err == nil && slices.Contains([]string{"at", "budget", "limit", "page", "per_file"}, k) {
				args[k] = n
			} else {
				args[k] = v
			}
		}
	}
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return ws[0], b
}

// dropPair removes "flag value" from argv unless want has it.
func dropPair(argv, want []string, flag, value string) []string {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value && !slices.Contains(want, flag) {
			return slices.Delete(slices.Clone(argv), i, i+2)
		}
	}
	return argv
}

// 13 §1.1: every hint MCPCall writes is a call the tool accepts and runs as the command it
// replaced (the tool's defaults aside: warning-and-above problems, a 10 s source budget).
func TestHintsAreValidToolCalls(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		cli  string
		argv string // what the tool runs, when it is not the printed command
	}{
		{cli: `archo log --cluster E9`},
		{cli: `archo log --prev`},
		{cli: `archo log --cluster E3 --game-version 1.6`},
		{cli: `archo source fi7x --type Ns.T`},
		{cli: `archo source gpto --type ModB.Pawn_Tick_B --all`},
		{cli: `archo source game --type Verse.Pawn`},
		{cli: `archo source search "AllComps.Remove(" --in fi7x --in gpto`},
		{cli: `archo source search Foo --implicated`},
		{cli: `archo source search Foo --active --yes`},
		{cli: `archo harmony --target Verse.Pawn.Tick`},
		{cli: `archo patches --mod iqmu --failed`},
		{cli: `archo def ThingDef/Lib_Gun`},
		{cli: `archo mod iqmu`},
		{cli: `archo why iqmu gpto`},
		{cli: `archo files search "Things/Gone"`},
		{cli: `archo files search Lib_Gun --mod hnyg --def`},
		{cli: `archo problems`},
		{cli: `archo problems --code harmony.skip-prefix`},
		{cli: `archo problems --severity error`},
		{cli: `archo problems --ref p:a4972d`},
		{cli: `archo problems show p:a4972d`, argv: `problems --ref p:a4972d`},
		{cli: `archo problems ignore p:a4972d`},
		{cli: `archo changes iqmu`},
		{cli: `archo enable z3bw`},
		{cli: `archo enable a b --at 3`},
		{cli: `archo disable a`},
		{cli: `archo undo`},
		{cli: `archo sort --dry-run`},
		{cli: `archo save`},
		{cli: `archo pin iqmu --dry-run`},
		{cli: `archo pattern pattern:g:rt.tick`},
		{cli: `archo workshop resubscribe 6olp --dry-run`},
		{cli: `archo workshop subscribe nyh3`},
	} {
		call, ok := cmds.MCPCall(c.cli)
		if !ok {
			t.Errorf("%s: no translation", c.cli)
			continue
		}
		tool, args := callJSON(t, call)
		got, err := mcp.ArgvFor(tool, args)
		if err != nil {
			t.Errorf("%s → %s: %v", c.cli, call, err)
			continue
		}
		want := words(c.cli)[1:]
		if c.argv != "" {
			want = words(c.argv)
		}
		got = dropPair(got, want, "--budget", "10")
		got = dropPair(got, want, "--min-severity", "info")
		if !slices.Contains(want, "--implicated") {
			got = slices.DeleteFunc(slices.Clone(got), func(s string) bool { return s == "--implicated" })
		}
		a, b := slices.Clone(got), slices.Clone(want)
		slices.Sort(a)
		slices.Sort(b)
		if !slices.Equal(a, b) || got[0] != want[0] {
			t.Errorf("%s → %s runs %v, want %v", c.cli, call, got, want)
		}
	}
}

// The suggestions of an MCP result are tool calls, not `archo` commands (13 §1.1, h8 G8).
func TestMCPLogHintsAreToolCalls(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	cs := connect(t, e)
	for _, in := range []map[string]any{{}, {"cluster": "E3"}, {"cluster": "E4"}, {"cluster": "E1"}} {
		out, isErr := call(t, cs, "analyze_log", in)
		if isErr {
			t.Fatalf("%v: %s", in, out)
		}
		if strings.Contains(out, "archo ") {
			t.Errorf("%v: a CLI command in an MCP result:\n%s", in, out)
		}
		if in["cluster"] == "E4" && !strings.Contains(out, "next:\n  read_source ref=iqmu type=ModA.CompBroken\n") {
			t.Errorf("E4:\n%s", out)
		}
	}
	// The CLI keeps its commands.
	if out := e.MustRun("log", "--cluster", "E4"); !strings.Contains(out, "next:\n  archo source iqmu --type ModA.CompBroken\n") {
		t.Errorf("CLI:\n%s", out)
	}
}
