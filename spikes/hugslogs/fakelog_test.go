package main

import (
	"fmt"
	"strings"
)

// fakeLog is a HugsLib upload: a mod list naming id (so contents differ), the game version line,
// n filler lines and, with problem, one unresolved cross-reference.
func fakeLog(id, mm string, problem bool, n int) string {
	var b strings.Builder
	b.WriteString("Loaded mods:\nCore(Ludeon.RimWorld): (no assemblies)\nTest(test.m" + id + ")[mv:1.0]: (no assemblies)\n\nLog file contents:\n")
	b.WriteString("RimWorld " + mm + ".4630 rev1\n")
	for i := range n {
		fmt.Fprintf(&b, "Loading step %d\n", i)
	}
	if problem {
		b.WriteString("Could not resolve cross-reference to Verse.ThingDef named FooBar (wanter=costList)\n")
	}
	return b.String()
}
