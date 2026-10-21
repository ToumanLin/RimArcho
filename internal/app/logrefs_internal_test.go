package app

import (
	"strings"
	"testing"
)

func TestXMLRefs(t *testing.T) {
	src := `<?xml version="1.0"?>
<Defs>
  <!-- <texPath>Commented/Out</texPath> -->
  <ThingDef ParentName="Base">
    <defName>Own_Name</defName>
    <graphicData Class="X"><texPath> Things/A </texPath></graphicData>
    <costList><Steel>5</Steel></costList>
    <empty/>
    <li Class="Y"/>
  </ThingDef>
</Defs>`
	var got []string
	xmlRefs([]byte(src), func(tag, s []byte, elem bool, off int) {
		k := "text:"
		if elem {
			k = "elem:"
		} else if string(s) == "Things/A" && string(tag) != "texPath" {
			t.Errorf("tag of Things/A: %q", tag)
		}
		got = append(got, k+string(s))
	})
	all := strings.Join(got, " ")
	for _, want := range []string{"text:Things/A", "elem:Steel", "text:5", "elem:Defs", "elem:costList"} {
		if !strings.Contains(" "+all+" ", " "+want+" ") {
			t.Errorf("missing %s in %s", want, all)
		}
	}
	for _, bad := range []string{"Own_Name", "Commented/Out", "elem:ThingDef", "elem:graphicData", "empty", "elem:li"} {
		if strings.Contains(all, bad) {
			t.Errorf("unexpected %s in %s", bad, all)
		}
	}
}

func TestLogMessageNames(t *testing.T) {
	if typ, name, ok := xrefName("Failed to find RimWorld.BiomeDef named TropicalRainforest. There are 31 defs of this type loaded."); !ok || typ != "RimWorld.BiomeDef" || name != "TropicalRainforest" {
		t.Errorf("GetNamed: %q %q %v", typ, name, ok)
	}
	if typ, name, ok := xrefName("Could not load reference to Verse.ThingDef named Gone_Thing"); !ok || typ != "Verse.ThingDef" || name != "Gone_Thing" {
		t.Errorf("save reference: %q %q %v", typ, name, ok)
	}
	if p := texturePath("Failed to find any textures at Storage/BarrelBigDown while constructing Multi(initPath=Storage/BarrelBigDown, color=RGBA(1, 1, 1, 1))"); p != "Storage/BarrelBigDown" {
		t.Errorf("Multi texture: %q", p)
	}
	for name, game := range map[string]bool{"UnityEngine.InputLegacyModule": true, "System": true, "mscorlib": true, "Assembly-CSharp": true, "0Harmony": false, "VEF": false} {
		if gameAssembly(name) != game {
			t.Errorf("gameAssembly(%s) != %v", name, game)
		}
	}
}
