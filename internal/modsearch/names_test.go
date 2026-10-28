package modsearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindNamesEffectiveProviders(t *testing.T) {
	m := fixture(t, map[string]string{
		"Defs/Base.xml":     `<Defs><PawnKindDef Name="Shadowed" /></Defs>`,
		"1.6/Defs/Base.xml": `<Defs><PawnKindDef Name="ChimeraKind" Abstract="True" MayRequire="test.mod" /><ThingDef><defName>NotAName</defName><li Name="Nested"/></ThingDef><ThingDef Name="Guarded" MayRequire="test.absent"/><ThingDef Name="Any" MayRequireAnyOf="test.absent,test.mod"/></Defs>`,
		"1.5/Defs/Old.xml":  `<Defs><PawnKindDef Name="WrongVersion"/></Defs>`,
		"Defs/Broken.xml":   `<Defs><ThingDef Name="Broken"/>`,
	})
	names := map[string]bool{}
	for _, n := range []string{"ChimeraKind", "Shadowed", "WrongVersion", "NotAName", "Nested", "Guarded", "Any", "Broken"} {
		names[n] = true
	}
	matches, incomplete, err := FindNames(context.Background(), []Mod{m}, Options{MM: "1.6"}, names)
	if err != nil || !incomplete {
		t.Fatalf("malformed XML not noted: %v %v", incomplete, err)
	}
	if len(matches) != 2 || matches[0].Name != "Any" || matches[1].Name != "ChimeraKind" {
		t.Fatalf("providers %+v", matches)
	}
	// The enabling mod itself must satisfy LoadFolders conditions too.
	m.LoadFoldersXML = `<loadFolders><v1.6><li IfModActive="test.mod">1.6</li></v1.6></loadFolders>`
	matches, incomplete, err = FindNames(context.Background(), []Mod{m}, Options{MM: "1.6"}, names)
	if err != nil || incomplete || len(matches) != 2 {
		t.Fatalf("self condition: %+v %v %v", matches, incomplete, err)
	}
}

func TestFindNamesBoundsAndCancellation(t *testing.T) {
	m := fixture(t, map[string]string{"Defs/Base.xml": `<Defs><PawnKindDef Name="ChimeraKind" Abstract="True"/></Defs>`})
	names := map[string]bool{"ChimeraKind": true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := FindNames(ctx, []Mod{m}, Options{MM: "1.6"}, names); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// A mod cap reports partial results, instead of silently claiming no provider.
	mods := make([]Mod, 1001)
	for i := range mods {
		mods[i] = m
	}
	matches, incomplete, err := FindNames(context.Background(), mods, Options{MM: "1.6"}, names)
	if err != nil || !incomplete || len(matches) != 1000 {
		t.Fatalf("mod cap: %d %v %v", len(matches), incomplete, err)
	}
	// Directory and byte caps apply before retaining or parsing oversized input.
	budget := &nameFS{ctx: context.Background(), entries: 1, files: 1, bytes: 4}
	if _, err := budget.ReadFile(filepath.Join(m.Root, "Defs", "Base.xml")); !errors.Is(err, errNameBudget) {
		t.Fatalf("byte cap: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.Root, "extra"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := budget.ReadDir(m.Root)
	if err != nil || len(entries) != 1 || !budget.exhausted {
		t.Fatalf("entry cap: %d %v", len(entries), err)
	}
	if _, err := budget.ReadDir(m.Root); !errors.Is(err, errNameBudget) {
		t.Fatalf("entry cap continued: %v", err)
	}
	if _, err := budget.ReadFile(filepath.Join(m.Root, "Defs", "Base.xml")); !errors.Is(err, errNameBudget) {
		t.Fatalf("file cap: %v", err)
	}
	large := fixture(t, map[string]string{"Defs/Huge.xml": strings.Repeat(" ", 8<<20) + "x"})
	if _, incomplete, err := FindNames(context.Background(), []Mod{large}, Options{MM: "1.6"}, names); err != nil || !incomplete {
		t.Fatalf("oversized file: %v %v", incomplete, err)
	}
}
