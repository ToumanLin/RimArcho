package render

import "testing"

// Commands lose the "#" of instance references; everything else keeps it (05 §3.1 rule 11).
func TestShellSafe(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"  archo enable #nyh3", "  archo enable nyh3"},
		{"next:\n  archo why #a3f7 #52aa\n  archo problems --subject #5sed --all\n", "next:\n  archo why a3f7 52aa\n  archo problems --subject 5sed --all\n"},
		{"      fix: archo mod #mdx4 --diff #6olp", "      fix: archo mod mdx4 --diff 6olp"},
		// a bracketed command ends at its ")"; the reference before it is prose
		{"error: #z3bw is not enabled (archo enable #z3bw)", "error: #z3bw is not enabled (archo enable z3bw)"},
		{"  changes: 1, latest 2026-10-01 (archo changes #hnyg)", "  changes: 1, latest 2026-10-01 (archo changes hnyg)"},
		{"    … 3 more (archo problems --subject #a3f7)", "    … 3 more (archo problems --subject a3f7)"},
		// two spaces and ";" end a command
		{"fix: archo enable #nyh3  (#abcd Foo defines Bar)", "fix: archo enable nyh3  (#abcd Foo defines Bar)"},
		{"candidate: Lib; archo subscribe workshop #nyh3; #abcd", "candidate: Lib; archo subscribe workshop nyh3; #abcd"},
		// a "#" inside a word is no comment: lineages, constraint refs and quoted text stay
		{"archo lineage merge L#7c2e #iqmu", "archo lineage merge L#7c2e iqmu"},
		{"archo rule add ignore decl:#iqmu:loadafter:tester.y", "archo rule add ignore decl:#iqmu:loadafter:tester.y"},
		{`archo source search "#region x" --in #iqmu`, `archo source search "#region x" --in iqmu`},
		{`archo source search "AllComps.Remove(" --in #iqmu --in #gpto`, `archo source search "AllComps.Remove(" --in iqmu --in gpto`},
		// not a reference, not a command
		{"archo enable #?", "archo enable #?"},
		{"#a3f7 Combat Extended  [active 34/212]", "#a3f7 Combat Extended  [active 34/212]"},
		{"rimarcho enable #a3f7", "rimarcho enable #a3f7"},
		{"error log.exception  log:E3 attributed #a3f7", "error log.exception  log:E3 attributed #a3f7"},
	} {
		if got := ShellSafe(c.in); got != c.want {
			t.Errorf("ShellSafe(%q)\n got  %q\n want %q", c.in, got, c.want)
		}
		if again := ShellSafe(c.want); again != c.want {
			t.Errorf("ShellSafe is not idempotent on %q: %q", c.want, again)
		}
	}
}

func TestCommandSpans(t *testing.T) {
	t.Parallel()
	line := "x (archo changes hnyg) and archo why a b  (c)"
	var got []string
	for _, sp := range CommandSpans(line) {
		got = append(got, line[sp[0]:sp[1]])
	}
	if len(got) != 2 || got[0] != "archo changes hnyg" || got[1] != "archo why a b" {
		t.Fatalf("CommandSpans = %q", got)
	}
}
