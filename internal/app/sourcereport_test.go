package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// mcpRun runs a command as an MCP tool call does (cmds.ForMCP).
func mcpRun(e *testkit.Env, argv ...string) string {
	out, _ := cmds.ExecuteContext(cmds.ForMCP(context.Background()), e.App, argv)
	return out
}

// 05 §5.4 (h7 G2): every source search ends with what it did not search.
func TestSourceSearchCoverage(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	st, _ := e.App.Loadout()
	enabled := len(st.List)
	if enabled < 3 {
		t.Fatalf("fixture enables %d mods", enabled)
	}
	modA := ref(t, e, "Mod A")
	last := func(out string) string {
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		return strings.TrimSpace(lines[len(lines)-1])
	}

	// Named: a hit, and the rest of the list is reported as not searched.
	out := e.MustRun("source", "search", "AllComps.Remove(", "--in", modA)
	wantRest := fmt.Sprintf("searched 1 of %d enabled mods (named); not searched: %d (--active --yes)", enabled, enabled-1)
	if !strings.Contains(out, "1 matches") || last(out) != wantRest {
		t.Fatalf("named search, want last line %q:\n%s", wantRest, out)
	}
	if out := mcpRun(e, "source", "search", "AllComps.Remove(", "--in", modA, "--budget", "10"); !strings.HasSuffix(strings.TrimRight(out, "\n"),
		fmt.Sprintf(`searched 1 of %d enabled mods (named); not searched: %d (scope="active": repeat while pending)`, enabled, enabled-1)) {
		t.Fatalf("MCP wording:\n%s", out)
	}

	// Nothing found in a restricted scope: say so and name the next call.
	out = e.MustRun("source", "search", "NoSuchCallAnywhere(", "--in", modA)
	if !strings.Contains(out, "0 matches") || !strings.Contains(last(out), "no matches in 1 mods: the code you look for may be in one of the") ||
		!strings.Contains(last(out), "--active --yes") {
		t.Fatalf("empty named search:\n%s", out)
	}
	out = mcpRun(e, "source", "search", "NoSuchCallAnywhere(", "--implicated", "--budget", "10")
	if !strings.Contains(out, "(implicated); not searched: ") || !strings.Contains(last(out), `no matches in `) || !strings.Contains(last(out), `search them with scope="active"`) {
		t.Fatalf("empty implicated search:\n%s", out)
	}

	// Every enabled mod: nothing left out, no suggestion even without a match.
	out = e.MustRun("source", "search", "NoSuchCallAnywhere(", "--active", "--yes")
	if want := fmt.Sprintf("searched %d of %d enabled mods (active)", enabled, enabled); last(out) != want {
		t.Fatalf("active search, want last line %q:\n%s", want, out)
	}

	// The game alone leaves every enabled mod out.
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), "synthetic game assembly")
	out = e.MustRun("source", "search", "NoSuchCallAnywhere(", "--game")
	if !strings.Contains(out, "searched the game only; not searched: ") || !strings.Contains(last(out), "no matches in the game:") {
		t.Fatalf("game only:\n%s", out)
	}
	out = e.MustRun("source", "search", "AllComps.Remove(", "--game", "--in", modA)
	if !strings.Contains(out, "searched 1 of ") || !strings.Contains(out, " enabled mods and the game (named)") {
		t.Fatalf("mod and game:\n%s", out)
	}
}

// A call that decompiled something says so (why it took long); the next one does not.
func TestSourceSearchDecompiledNote(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.App.Decompile = func(ctx context.Context, dll, out, typ string, _ []string) error {
		time.Sleep(60 * time.Millisecond)
		return os.WriteFile(filepath.Join(out, "X.cs"), []byte("class X { void CompTick() { parent.AllComps.Remove(this); } }\n"), 0o644)
	}
	// A budget of one second dispatches nothing new after the first wave; with 4 DLLs and
	// 6 workers everything starts at once, so use the lower bound the CLI allows and read
	// the "decompiled" note instead of expecting pending.
	out := e.MustRun("source", "search", "AllComps.Remove(", "--active", "--yes", "--budget", "1")
	if !regexp.MustCompile(`decompiled: \d+ assemblies now, \d+ s`).MatchString(out) {
		t.Fatalf("no note about the decompile this call did:\n%s", out)
	}
	out = e.MustRun("source", "search", "AllComps.Remove(", "--active", "--yes", "--budget", "1")
	if strings.Contains(out, "decompiled:") {
		t.Fatalf("the cached search still says it decompiled:\n%s", out)
	}
}

func longType(name string, methods int) string {
	var sb strings.Builder
	sb.WriteString("using System;\nnamespace Verse\n{\n\tpublic class Long\n\t{\n\t\tprivate int field;\n\n")
	sb.WriteString("\t\tpublic Long(int x)\n\t\t{\n\t\t\tfield = x;\n\t\t}\n\n")
	for i := 0; i < methods; i++ {
		fmt.Fprintf(&sb, "\t\tpublic int Method%d(int a)\n\t\t{\n\t\t\tif (a > %d)\n\t\t\t{\n\t\t\t\treturn a;\n\t\t\t}\n\t\t\treturn field;\n\t\t}\n\n", i, i)
	}
	sb.WriteString("\t}\n}\n")
	return sb.String()
}

// 05 §5.4 (h4 P5, h8 G5): a short type is shown whole; a long one gets a member index
// and member=<name> cuts one member out.
func TestSourceTypeWholeIndexAndMember(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	modA := ref(t, e, "Mod A")
	var body string
	e.App.Decompile = func(_ context.Context, _, out, typ string, _ []string) error {
		return os.WriteFile(filepath.Join(out, strings.ReplaceAll(typ, "+", ".")+".cs"), []byte(body), 0o644)
	}
	lines := func(out string) int { return strings.Count(out, "\n") }

	body = longType("Short", 15) // 149 lines
	out := e.MustRun("source", modA, "--type", "Verse.Short")
	if strings.Contains(out, "more lines") || !strings.Contains(out, "Method14") || strings.Contains(out, "members:") {
		t.Fatalf("a type under the limits is shown whole:\n%s", out)
	}

	body = longType("Long", 40) // 374 lines
	out = e.MustRun("source", modA, "--type", "Verse.Long")
	if !strings.Contains(out, "members: ctor 8, Method0 13, Method1 22,") || !strings.Contains(out, "Method39 364; 1 fields") ||
		!strings.Contains(out, "more lines (archo source "+strings.TrimPrefix(modA, "#")+" --type Verse.Long --all, or --member <name>)") || strings.Contains(out, "Method39(int") {
		t.Fatalf("a long type gets its head and an index:\n%s", out)
	}
	if lines(out) > 45 {
		t.Fatalf("the head is %d lines", lines(out))
	}
	if out := e.MustRun("source", modA, "--type", "Verse.Long", "--all"); !strings.Contains(out, "Method39(int a)") || strings.Contains(out, "more lines") {
		t.Fatalf("--all:\n%s", out)
	}

	out = e.MustRun("source", modA, "--type", "Verse.Long", "--member", "Method7")
	if !strings.Contains(out, "Verse.Long.Method7  (lines 76-83)") || !strings.Contains(out, "  76          public int Method7(int a)") ||
		!strings.Contains(out, "  83          }") || strings.Contains(out, "Method6") || strings.Contains(out, "Method8") {
		t.Fatalf("--member:\n%s", out)
	}
	if out := e.MustRun("source", modA, "--type", "Verse.Long", "--member", "ctor"); !strings.Contains(out, "Verse.Long.ctor  (lines 8-11)") {
		t.Fatalf("--member ctor:\n%s", out)
	}
	out, code := e.Run("source", modA, "--type", "Verse.Long", "--member", "Nothing")
	if code != 1 || !strings.Contains(out, `Verse.Long has no member "Nothing"; members: ctor 8, Method0 13,`) {
		t.Fatalf("unknown member (exit %d):\n%s", code, out)
	}
	if out, code := e.Run("source", modA, "--member", "Method7"); code != 2 || !strings.Contains(out, "--member needs --type") {
		t.Fatalf("--member alone (exit %d):\n%s", code, out)
	}

	// Few lines but many bytes (long literal lines): still the head and the index.
	body = "namespace Verse\n{\n\tpublic class Wide\n\t{\n\t\tpublic string A() { return \"" + strings.Repeat("x", 13<<10) + "\"; }\n\t\tpublic int B() { return 1; }\n\t}\n}\n"
	out = e.MustRun("source", modA, "--type", "Verse.Wide")
	if !strings.Contains(out, "members: A 5, B 6") {
		t.Fatalf("12 KB limit:\n%.400s", out)
	}
}
