package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// naiveRef is the search refScan replaced: one bytes.Index per name and form.
func naiveRef(b []byte, name string) int {
	k := bytes.Index(b, []byte(">"+name+"<"))
	if k2 := bytes.Index(b, []byte("<"+name+">")); k2 >= 0 && (k < 0 || k2 < k) {
		k = k2
	}
	if k < 0 {
		return 0
	}
	return bytes.Count(b[:k], []byte{'\n'}) + 1
}

func TestRefScan(t *testing.T) {
	b := []byte(`<?xml version="1.0"?>
<Defs>
  <!-- >Steel< in a comment counts, as it did -->
  <ThingDef ParentName="BuildingBase">
    <defName>Wall</defName>
    <costList>
      <Steel>5</Steel>
      <WoodLog>2</WoodLog>
    </costList>
    <li>ComponentIndustrial</li>
    <thingDef>Gun_Revolver</thingDef>
    <label>a > b</label><x>Gun_Revolver</x>
  </ThingDef>
</Defs>`)
	names := []string{"Steel", "WoodLog", "ComponentIndustrial", "Gun_Revolver", "Wall", "BuildingBase", "Missing", "costList"}
	want := map[string]int{}
	for i, n := range names {
		want[n] = i
	}
	got := map[int]int{}
	refScan(b, want, func(i, line int) {
		if _, dup := got[i]; dup {
			t.Errorf("%s reported twice", names[i])
		}
		got[i] = line
	})
	for i, n := range names {
		if got[i] != naiveRef(b, n) {
			t.Errorf("%s: line %d, the per-name search says %d", n, got[i], naiveRef(b, n))
		}
	}
}

func TestRefIndexFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	files := []refFile{
		{"a", "Defs/A.xml", write("A.xml", "<Defs><ThingDef><costList><Steel>1</Steel></costList></ThingDef></Defs>")},
		{"b", "Defs/B.xml", write("B.xml", "<Defs><RecipeDef><li>Steel</li><li>Plasteel</li><description>no Gold here</description></RecipeDef></Defs>")},
		{"b", "Defs/gone.xml", filepath.Join(dir, "gone.xml")},
	}
	x := buildRefIndex("k1", files)
	check := func(x *refIndex, name string, want ...int) {
		t.Helper()
		got := x.filesWith(name)
		if len(got) != len(want) {
			t.Fatalf("%s: files %v, want %v", name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: files %v, want %v", name, got, want)
			}
		}
	}
	check(x, "Steel", 0, 1)
	check(x, "Plasteel", 1)
	check(x, "Gold") // inside text with spaces: not a reference
	check(x, "Uranium")
	a := &App{Home: dir}
	path := filepath.Join(dir, "refindex", "k1.idx")
	if err := a.writeRefIndex(path, x); err != nil {
		t.Fatal(err)
	}
	y, err := readRefIndex(path, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if len(y.files) != 3 || y.files[1] != files[1] || len(y.ents) != len(x.ents) {
		t.Fatalf("read back %+v", y)
	}
	check(y, "Steel", 0, 1)
	if _, err := readRefIndex(path, "k2"); err == nil {
		t.Fatal("an index read under another set's key")
	}
	b, _ := os.ReadFile(path)
	os.WriteFile(path, b[:len(b)-3], 0o644)
	if _, err := readRefIndex(path, "k1"); err == nil {
		t.Fatal("a truncated index was read")
	}
	// only refIndexKeep indexes stay
	for i := 0; i < refIndexKeep+3; i++ {
		a.writeRefIndex(filepath.Join(dir, "refindex", string(rune('a'+i))+".idx"), x)
	}
	es, _ := os.ReadDir(filepath.Join(dir, "refindex"))
	if len(es) != refIndexKeep {
		t.Fatalf("%d indexes kept, want %d", len(es), refIndexKeep)
	}
}
