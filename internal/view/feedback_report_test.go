package view

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Reporting from where the diagnosis shows (16 §2.4, §2.9, §2.14): an item that rests on the
// replacement library names its Mod (DataMod), the Mod's details list it (DataNotes), and an
// outdated-data report started from it carries the Mod and the item, evidence included. The
// CLI drafts the same with --template data --mod --finding.
func TestFeedbackFromItem(t *testing.T) {
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "fan.port", Name: "Unofficial port"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "fan.port")
	e.WriteFile(filepath.Join(e.Home, "cache", "replacements.json.gz"), `{"rules":[{"oldPackageId":"fan.port","newPackageId":"author.official","newName":"Official","newWorkshopId":"2002","newVersions":["1.6"]}]}`)
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := BuildNeeds(e.App, st, ps)
	var it *Need
	for i := range n.Items {
		if n.Items[i].Kind == "mod.replaced" {
			it = &n.Items[i]
		}
		if k := n.Items[i].Kind; n.Items[i].DataMod != "" && k != "mod.replaced" {
			t.Errorf("%s rests on no data but has DataMod %q", k, n.Items[i].DataMod)
		}
	}
	if it == nil || it.DataMod != "fan.port" || it.ID == "" {
		t.Fatalf("replacement item %+v", it)
	}
	inst := it.Mods[0]
	notes := DataNotes(n, st, inst)
	if len(notes) != 1 || notes[0].Finding != it.ID || notes[0].Mod != "fan.port" || notes[0].Evidence == nil || notes[0].Evidence.Kind != EvCommunity {
		t.Fatalf("data notes %+v", notes)
	}

	// the data report the flag starts: the Mod, the item and the data versions, nothing typed
	p := FeedbackProblem{Kind: feedback.KindData, Answers: map[string]string{"mod": "fan.port", feedback.DataFinding: it.ID}}
	o := p.Options()
	if o.Mod != "fan.port" || o.Finding != it.ID {
		t.Fatalf("options %+v", o)
	}
	at := FeedbackAttachments(e.App, st, o, n)
	text := FeedbackPreview(p.Text(), at)
	for _, want := range []string{"the diagnosis " + it.ID, "evidence: community", "the Mod fan.port", "replacement library: Official  author.official  2002", "report: outdated data"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	if strings.Contains(p.Text(), it.ID) {
		t.Errorf("the item's ID is an attachment, not the player's words: %q", p.Text())
	}
	// the wrong-diagnosis report the flag starts
	d := FeedbackProblem{Kind: feedback.KindDiagnosis, Answers: map[string]string{"finding": it.ID}}
	if dt := FeedbackPreview(d.Text(), FeedbackAttachments(e.App, st, d.Options(), n)); !strings.Contains(dt, "kind: mod.replaced") || !strings.Contains(dt, "has a maintained replacement") {
		t.Errorf("diagnosis report:\n%s", dt)
	}

	// the CLI: --template is the kind; data takes --finding; nothing is sent
	out := e.MustRun("feedback", "--template", "data", "--mod", "fan.port", "--finding", it.ID)
	if !strings.Contains(out, "the diagnosis "+it.ID) || !strings.Contains(out, "the Mod fan.port") || !strings.Contains(out, "open on GitHub") {
		t.Fatalf("archo feedback --template data:\n%s", out)
	}
	if out, code := e.Run("feedback", "bug", "--template", "data"); code == 0 {
		t.Fatalf("two kinds accepted:\n%s", out)
	}
	if out, code := e.Run("feedback", "--template", "bug", "--finding", it.ID); code == 0 {
		t.Fatalf("a bug took --finding:\n%s", out)
	}
	if out := e.MustRun("feedback", "--template", "diagnosis", "--finding", it.ID); !strings.Contains(out, "kind: mod.replaced") {
		t.Fatalf("archo feedback --template diagnosis:\n%s", out)
	}
}
