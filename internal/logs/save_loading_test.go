package logs

import (
	"os"
	"strings"
	"testing"
)

// This fixture models errors encountered while reading a save. The introductory
// text is illustrative, not a verified phase boundary in either game version.
func TestSyntheticSaveLoadingErrors(t *testing.T) {
	b, err := os.ReadFile("testdata/synthetic-save-loading.log")
	if err != nil {
		t.Fatal(err)
	}
	for _, waylith := range []bool{false, true} {
		text := string(b)
		if waylith {
			lines := strings.Split(text, "\n")
			var out strings.Builder
			out.WriteString("=== Waylith log ===\nVersion: 1.7.4990.1\n\n")
			for _, line := range lines[1:] {
				if line != "" && !strings.HasPrefix(line, " ") {
					level := "ERROR"
					if strings.HasPrefix(line, "Loading game") {
						level = "INFO"
					}
					out.WriteString("2026-10-28 10:00:00.000 " + level + " [Verse] ")
				}
				out.WriteString(line + "\n")
			}
			text = out.String()
		}
		f, err := Parse(strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		var messages []string
		save, xref := false, false
		for _, c := range Clusters(f) {
			if Informational(c.Class) {
				continue
			}
			messages = append(messages, c.First.Message)
			save = save || c.Class == ClassSave
			xref = xref || c.Class == ClassXrefMissing
		}
		if len(messages) != 4 || !save || !xref {
			t.Fatalf("Waylith=%v: save=%v xref=%v messages=%q", waylith, save, xref, messages)
		}
	}
}
