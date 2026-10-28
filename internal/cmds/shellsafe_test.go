package cmds_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/ids"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Printed commands survive a paste into a shell (05 §3.1 rule 11). Every archo command
// the problem fixture prints (fix:, next:, "… N more (…)", error hints) is checked three
// ways: no word starts with "#"; PowerShell's own parser, where "#" starts a comment,
// reads the same words; and the command runs with its references as printed.
func TestPrintedCommandsSurviveShells(t *testing.T) {
	e := problemsFixture(t)
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	runs := [][]string{
		{"problems", "--all", "--detail"}, {"launch", "--dry-run"}, {"patches", "--failed"},
		{"why", ref(e, "Needy"), ref(e, "Over One")}, {"def", "--mod", ref(e, "Over One")},
		{"move", ref(e, "Lib"), "--after", ref(e, "Compat")}, // error: not enabled (archo enable …)
	}
	for _, m := range ix.Mods {
		runs = append(runs, []string{"mod", ix.Ref(m.ID)})
	}
	var all, withRefs []string
	for _, argv := range runs {
		out, _ := e.Run(argv...)
		for _, line := range strings.Split(out, "\n") {
			for _, sp := range render.CommandSpans(line) {
				c := strings.TrimSpace(line[sp[0]:sp[1]])
				if slices.Contains(all, c) {
					continue
				}
				all = append(all, c)
				// with or without "#": the subtests say which form was printed
				if slices.ContainsFunc(shellWords(c), func(w string) bool {
					w = strings.TrimPrefix(w, "#")
					_, err := ix.ResolveOne(w)
					return ids.Valid(w) && err == nil
				}) {
					withRefs = append(withRefs, c)
				}
			}
		}
	}
	bare := func(name string) string { return strings.TrimPrefix(ref(e, name), "#") }
	for _, want := range []string{"archo enable " + bare("Lib"), "archo why " + bare("Needy") + " " + bare("Over One")} {
		if !slices.ContainsFunc(withRefs, func(c string) bool { return strings.ReplaceAll(c, " #", " ") == want }) {
			t.Fatalf("the fixture no longer prints %q; commands with references:\n%s", want, strings.Join(withRefs, "\n"))
		}
	}

	t.Run("rule", func(t *testing.T) {
		for _, c := range all {
			for _, w := range shellWords(c) {
				if strings.HasPrefix(w, "#") {
					t.Errorf("%q: the word %q starts a comment in PowerShell and bash", c, w)
				}
			}
		}
	})

	t.Run("powershell", func(t *testing.T) {
		shells := 0
		for _, name := range []string{"powershell", "pwsh"} {
			exe, err := exec.LookPath(name)
			if err != nil {
				continue
			}
			shells++
			control := "archo enable #nyh3"
			got := psParse(t, exe, append([]string{control}, withRefs...))
			if got[0].Comments != 1 || len(got[0].Words) != 2 {
				t.Fatalf("%s: the control %q parsed as %+v; PowerShell should read #nyh3 as a comment", name, control, got[0])
			}
			for i, c := range withRefs {
				p := got[i+1]
				if p.Comments != 0 || p.Errors != 0 || !slices.Equal(p.Words, shellWords(c)) {
					t.Errorf("%s reads %q as %q (comments %d, errors %d), not %q", name, c, p.Words, p.Comments, p.Errors, shellWords(c))
				}
			}
		}
		if shells == 0 {
			t.Skip("no PowerShell on this machine")
		}
	})

	t.Run("run", func(t *testing.T) {
		// Commands that only read or edit the fixture's list; nothing that moves folders.
		safe := map[string]bool{"mod": true, "why": true, "changes": true, "patches": true, "files": true, "def": true,
			"source": true, "harmony": true, "problems": true, "list": true, "enable": true, "disable": true, "move": true}
		ran := 0
		for _, c := range withRefs {
			argv := shellWords(c)[1:]
			if !safe[argv[0]] {
				continue
			}
			ran++
			out, code := e.Run(argv...)
			if code == cmds.ExitUsage || code == cmds.ExitAmbiguous || strings.Contains(out, "matches nothing") || strings.Contains(out, "not a reference") {
				t.Errorf("%s: exit %d\n%s", c, code, out)
			}
		}
		if ran == 0 {
			t.Fatal("no printed command was run")
		}
		t.Logf("%d printed commands, %d with references, %d run", len(all), len(withRefs), ran)
	})
}

// shellWords splits a command as cmd, PowerShell and bash do for these commands: at
// spaces, with double quotes grouping and removed.
func shellWords(c string) []string {
	var out []string
	var w strings.Builder
	quoted, in := false, false
	for _, r := range c {
		switch {
		case r == '"':
			quoted, in = !quoted, true
		case r == ' ' && !quoted:
			if in {
				out = append(out, w.String())
			}
			w.Reset()
			in = false
		default:
			w.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, w.String())
	}
	return out
}

type psLine struct {
	Comments int      `json:"comments"`
	Errors   int      `json:"errors"`
	Words    []string `json:"words"`
}

// psScript parses each line of $env:ARCHO_PS_LINES with PowerShell's parser: its comment
// tokens, parse errors, and the words of the command (string values, other elements as
// written, e.g. --mod).
const psScript = `
foreach ($line in [IO.File]::ReadAllLines($env:ARCHO_PS_LINES)) {
  $tokens = $null; $errs = $null
  $ast = [System.Management.Automation.Language.Parser]::ParseInput($line, [ref]$tokens, [ref]$errs)
  $words = @()
  $c = $ast.Find({ param($n) $n -is [System.Management.Automation.Language.CommandAst] }, $true)
  if ($c) {
    foreach ($el in $c.CommandElements) {
      if ($el -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $words += $el.Value }
      else { $words += $el.Extent.Text }
    }
  }
  [pscustomobject]@{ comments = @($tokens | Where-Object { $_.Kind -eq 'Comment' }).Count; errors = @($errs).Count; words = $words } | ConvertTo-Json -Compress
}
`

func psParse(t *testing.T, exe string, lines []string) []psLine {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lines.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", psScript)
	c.Env = append(os.Environ(), "ARCHO_PS_LINES="+path)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", exe, err, out)
	}
	var got []psLine
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			var p psLine
			if err := json.Unmarshal([]byte(l), &p); err != nil {
				t.Fatalf("%s: %v in %q", exe, err, l)
			}
			got = append(got, p)
		}
	}
	if len(got) != len(lines) {
		t.Fatalf("%s parsed %d of %d lines:\n%s", exe, len(got), len(lines), out)
	}
	return got
}
