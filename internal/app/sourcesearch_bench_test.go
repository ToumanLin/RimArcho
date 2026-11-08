package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// synthGameFile is one decompiled game type: about 6.5 KB of ILSpy-style C#, like the
// average type of a 5,500-type Assembly-CSharp.dll.
func synthGameFile(i int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "using System;\nusing System.Collections.Generic;\nnamespace Verse\n{\n\tpublic class Type%d_Worker\n\t{\n", i)
	for f := 0; f < 8; f++ {
		fmt.Fprintf(&sb, "\t\tprivate int field%d;\n\t\tpublic List<string> Items%d = new List<string>();\n", f, f)
	}
	for m := 0; m < 4; m++ {
		fmt.Fprintf(&sb, "\t\tpublic int Method%d(int a, string b, List<int> c)\n\t\t{\n\t\t\tint total = a;\n\t\t\tfor (int i = 0; i < c.Count; i++)\n\t\t\t{\n", m)
		fmt.Fprintf(&sb, "\t\t\t\tif (c[i] %% %d == 0 && b != null)\n\t\t\t\t{\n\t\t\t\t\ttotal += c[i] * field%d + b.Length;\n\t\t\t\t\tItems%d.Add(b + total.ToString());\n\t\t\t\t}\n", m+2, m, m)
		fmt.Fprintf(&sb, "\t\t\t}\n\t\t\treturn total;\n\t\t}\n")
	}
	sb.WriteString("\t}\n}\n")
	return sb.String()
}

// BenchmarkGameSourceSearch times a warm search of the game's decompiled project
// (5,500 files, about 36 MB, the size of a real Assembly-CSharp.dll): the cost every
// read_source game=true search pays once the project is in the library (05 §5.4).
func BenchmarkGameSourceSearch(b *testing.B) {
	e := testkit.New(b)
	e.AddEnv(false)
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), "synthetic game assembly")
	e.App.Decompile = func(_ context.Context, _, out, typ string, _ []string) error {
		for i := 0; i < 5500; i++ {
			dir := filepath.Join(out, fmt.Sprintf("Verse%d", i%7))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("Type%d_Worker.cs", i)), []byte(synthGameFile(i)), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	e.MustRun("source", "game") // the cold decompile happens here, outside the timing
	for _, tc := range []struct{ name, pattern string }{
		{"literal-miss", "AllComps.Remove("},
		{"literal-many", "Items3.Add"},
		{"regex-miss", "--regex"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			args := []string{"source", "search", tc.pattern, "--game"}
			if tc.name == "regex-miss" {
				args = []string{"source", "search", `AllComps\.(Remove|Add)\(`, "--game", "--regex"}
			}
			for i := 0; i < b.N; i++ {
				e.MustRun(args...)
			}
		})
	}
}
