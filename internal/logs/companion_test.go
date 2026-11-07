package logs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// 06 §7 (L11, L13): Archo Monitor's lines merge by id (counts add up, the highest total
// wins), the session line is read, bad lines are counted, a half-written last line waits.
func TestCompanionMonitor(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "companion", "ArchoMonitor.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.TrimRight(b, "\r\n") // the fixture ends in the middle of a line
	c := NewCompanion(path)
	c.Feed(b, false)
	if !c.Monitor || c.Session != "3f2a9c10ab12" || c.Version != "0.9.0" || c.Started.IsZero() {
		t.Fatalf("session: monitor %v %q %q %v", c.Monitor, c.Session, c.Version, c.Started)
	}
	if len(c.Entries) != 2 || c.BadLines != 1 {
		t.Fatalf("%d entries, %d bad lines", len(c.Entries), c.BadLines)
	}
	e := c.Lookup("Config error in a_Thing: has no label")
	if e == nil || e.Count != 9 || e.Level != "ERROR" || e.At.Format("15:04:05.000") != "14:05:03.120" {
		t.Fatalf("entry %+v", e)
	}
	if len(e.Files) != 1 || e.Roles["tester.b"] != "patch" || e.Def != "a_Thing" || e.Inferred != "DefConfigError" || len(e.Refs) != 1 {
		t.Fatalf("context %+v", e)
	}
	if e.Details == "" {
		t.Fatal("the first line's details are lost")
	}
	if w := c.Lookup("Something odd 7 times"); w == nil || w.Level != "WARNING" {
		t.Fatal("lookup by the normalised message")
	}
	// The half-written last line completes on the next read.
	c.Feed([]byte("ne\",\"details\":\"\",\"count\":1,\"context\":{},\"source\":\"archo-monitor\",\"schema\":1,\"id\":\"11111111\",\"total\":1}\n"), false)
	if len(c.Entries) != 3 || c.BadLines != 1 {
		t.Fatalf("after the rest of the line: %d entries, %d bad", len(c.Entries), c.BadLines)
	}
	if FirstSession(path) != "3f2a9c10ab12" {
		t.Fatal("FirstSession")
	}
}

// RimCrow Companion's format (L5, L11): the "[Mod] " prefix goes, relative paths stay files,
// pseudo entries become refs, lines merge by level and message.
func TestCompanionRimCrow(t *testing.T) {
	c, err := ReadCompanion(filepath.Join("..", "..", "testdata", "companion", "RimCrow_Realtime.log"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Monitor || c.BadLines != 1 || len(c.Entries) != 2 {
		t.Fatalf("monitor %v, bad %d, entries %d", c.Monitor, c.BadLines, len(c.Entries))
	}
	e := c.Lookup("Could not resolve cross-reference to Verse.ThingDef named Missing_Thing (wanter=costList)")
	if e == nil || e.Count != 5 {
		t.Fatalf("entry %+v", e)
	}
	if len(e.Files) != 1 || e.Files[0] != "Defs/ThingDefs/a.xml" || len(e.Refs) != 2 {
		t.Fatalf("files %v refs %v", e.Files, e.Refs)
	}
	if HintClass(e.Inferred, e.Message) != ClassXrefMissing {
		t.Fatal("hint")
	}
}

// The hint only re-keys entries RimArcho classed other or exception.
func TestClusterKeyHint(t *testing.T) {
	c := NewCompanion("x")
	c.Feed([]byte(`{"timestamp":"2026-10-21 14:05:03.120","level":"ERROR","message":"Bad thing in def Foo","count":1,"context":{"inferredType":"XMLSyntaxError"},"source":"archo-monitor","schema":1,"id":"1"}`+"\n"), false)
	e := &Entry{Message: "Bad thing in def Foo"}
	_, _, cls := ClusterKeyHint(e, HintFrom(c))
	if cls != ClassXMLLoad {
		t.Fatalf("class %s", cls)
	}
	e2 := &Entry{Message: "Could not resolve cross-reference to Verse.ThingDef named X"}
	if _, _, cls := ClusterKeyHint(e2, HintFrom(c)); cls != ClassXrefMissing {
		t.Fatalf("own class kept: %s", cls)
	}
}
