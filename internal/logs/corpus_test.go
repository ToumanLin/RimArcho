package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type corpusCluster struct {
	Class    string `json:"class"`
	Template string `json:"template"`
	Key      string `json:"key"`
	Count    int    `json:"count"`
}

type corpusEntry struct {
	Class     string `json:"class"`
	Level     string `json:"level,omitempty"`
	Frames    int    `json:"frames"`
	Callers   int    `json:"callers,omitempty"`
	Patches   int    `json:"patches,omitempty"`
	Extra     int    `json:"extra"`
	Ref       string `json:"ref,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

type corpusCase struct {
	Version   string          `json:"version"`
	Coverage  string          `json:"coverage"`
	Frequency int             `json:"frequency,omitempty"`
	Skip      string          `json:"skip,omitempty"`
	Count     int             `json:"count"`
	Entries   []corpusEntry   `json:"entries"`
	Clusters  []corpusCluster `json:"clusters"`
}

// RIMARCHO_UPDATE_LOG_CORPUS=1 refreshes active expectations. Review every change
// before committing; skipped cases retain their expectations and issue references.
func TestCorpusRegression(t *testing.T) {
	const dir = "testdata/corpus"
	goldenPath := filepath.Join(dir, "golden.json")
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]corpusCase
	if err = json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus fixtures")
	}
	update := os.Getenv("RIMARCHO_UPDATE_LOG_CORPUS") == "1"
	seen := map[string]bool{}
	for _, path := range files {
		name := filepath.Base(path)
		seen[name] = true
		want, ok := golden[name]
		if !ok {
			t.Errorf("%s: missing golden metadata", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			if want.Skip != "" {
				t.Skip(want.Skip)
			}
			if want.Version != "1.6" && want.Version != "1.7" {
				t.Fatalf("unsupported source version %q", want.Version)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(b)
			// Excerpts contain entry lines only. Supply the format marker in memory.
			if want.Version == "1.7" {
				text = "=== Waylith log ===\n" + text
			}
			f, err := Parse(strings.NewReader(text))
			if err != nil {
				t.Fatal(err)
			}
			got := want
			got.Count = len(f.Entries)
			got.Entries = make([]corpusEntry, 0, len(f.Entries))
			got.Clusters = make([]corpusCluster, 0)
			for _, e := range f.Entries {
				shape := corpusEntry{Class: Classify(e), Level: e.Level, Frames: len(e.Frames), Extra: len(e.Extra), Ref: e.Ref, Duplicate: e.Dup}
				for _, fr := range e.Frames {
					if fr.Caller {
						shape.Callers++
					}
					shape.Patches += len(fr.Harmony)
				}
				got.Entries = append(got.Entries, shape)
			}
			for _, c := range Clusters(f) {
				got.Clusters = append(got.Clusters, corpusCluster{c.Class, c.Template, c.Key, c.Count})
			}
			if update {
				golden[name] = got
				return
			}
			if !reflect.DeepEqual(got, want) {
				a, _ := json.MarshalIndent(got, "", "  ")
				b, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("corpus mismatch\n got: %s\nwant: %s", a, b)
			}
		})
	}
	for name := range golden {
		if !seen[name] {
			t.Errorf("%s: golden has no fixture", name)
		}
	}
	if update && !t.Failed() {
		b, err := json.MarshalIndent(golden, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(goldenPath, append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
