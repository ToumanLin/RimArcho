package corpus_test

import (
	"errors"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/corpus"
	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/patterns"
)

func bundle(t *testing.T, format int, data string, secs map[string]string) []byte {
	t.Helper()
	b := &corpus.Bundle{Format: format, Data: data, Sections: map[string]*corpus.Section{}}
	for g, w := range secs {
		b.Sections[g] = &corpus.Section{Data: w, Logs: 10, Patterns: []byte(`{"version":"t","game":"` + g + `","logs":10,"generic":[{"id":"g:t.` + w + `","match":"x","logs":1,"sessions":1,"clusters":1}]}`),
			Known: []byte(`{"cutoff":"` + w + `","game":"` + g + `","logs":10,"prereqs":[],"combos":[]}`)}
	}
	enc, err := corpus.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func cand(t *testing.T, origin, label string, raw []byte) corpus.Candidate {
	b, err := corpus.Parse(raw)
	return corpus.Candidate{Origin: origin, Label: label, Bundle: b, Err: err}
}

// 05 §4.6: per game version the newest section wins; 1.7 never borrows 1.6; an unreadable
// format is skipped and a newer one is noted.
func TestChoosePerGameVersion(t *testing.T) {
	cs := []corpus.Candidate{
		corpus.EmbeddedCandidate(),
		cand(t, corpus.Monitor, "ArchoMonitor-0.12.0", bundle(t, 1, "2026-W47", map[string]string{"1.6": "2026-W47", "1.7": "2026-W46"})),
		cand(t, corpus.Workshop, "9999", bundle(t, 1, "2026-W48", map[string]string{"1.7": "2026-W48"})),
		cand(t, corpus.Monitor, "ArchoMonitor-0.13.0", bundle(t, 2, "2026-W50", map[string]string{"1.6": "2026-W50"})),
	}
	if !errors.As(cs[3].Err, new(*corpus.FormatError)) {
		t.Fatalf("format 2 should not parse: %v", cs[3].Err)
	}
	c16 := corpus.Choose(cs, "1.6")
	if c16.Data != "2026-W47" || c16.Origin != corpus.Monitor || !c16.NewerFormat {
		t.Fatalf("1.6: %+v", c16.Summarize())
	}
	if c16.String() != "corpus W47, 1.6" {
		t.Fatalf("label %q", c16.String())
	}
	c17 := corpus.Choose(cs, "1.7")
	if c17.Data != "2026-W48" || c17.Origin != corpus.Workshop {
		t.Fatalf("1.7: %+v", c17.Summarize())
	}
	// Only 1.6 data: 1.7 gets none rather than 1.6's.
	only16 := corpus.Choose(cs[:1], "1.7")
	if only16.Data != "" || only16.Section != nil || only16.String() != "no corpus data for 1.7" {
		t.Fatalf("1.7 borrowed: %+v", only16.Summarize())
	}
	// The embedded bundle is 1.6 at EmbeddedData.
	if e := corpus.Choose(cs[:1], "1.6"); e.Data != corpus.EmbeddedData || e.Origin != corpus.Embedded {
		t.Fatalf("embedded: %+v", e.Summarize())
	}
}

// The chosen section is what patterns.Load and known.Get return; a game version without a
// section gets empty data, never another version's.
func TestUseInstallsSection(t *testing.T) {
	defer corpus.Reset()
	cs := []corpus.Candidate{corpus.EmbeddedCandidate(), cand(t, corpus.Monitor, "m", bundle(t, 1, "2026-W47", map[string]string{"1.6": "2026-W47"}))}
	corpus.Use(corpus.Choose(cs, "1.6"))
	if d := patterns.Load(); d.Get("g:t.2026-W47") == nil || known.Get().Cutoff != "2026-W47" {
		t.Fatalf("1.6 section not in use: %d rules, cutoff %q", len(d.Rules), known.Get().Cutoff)
	}
	corpus.Use(corpus.Choose(cs, "1.7"))
	if d := patterns.Load(); len(d.Rules) != 0 || len(d.Mod) != 0 || len(known.Get().Prereqs) != 0 {
		t.Fatalf("1.7 borrowed 1.6 data: %d rules", len(d.Rules))
	}
	corpus.Reset()
	if patterns.Load() != patterns.Embedded() {
		t.Fatal("Reset should restore the embedded library")
	}
}

func TestParseRejects(t *testing.T) {
	if _, err := corpus.Parse([]byte(`{"format":1,"data":"W47","sections":{}}`)); err == nil {
		t.Fatal("a data version that is not an ISO week must not parse")
	}
	if _, err := corpus.Parse([]byte(`{"format":1,"data":"2026-W47","sections":{"1.6":{"data":"2026-W48"}}}`)); err == nil {
		t.Fatal("a section newer than its bundle must not parse")
	}
}
