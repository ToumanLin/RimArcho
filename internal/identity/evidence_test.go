package identity_test

import (
	"fmt"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/zeebo/xxh3"
)

func TestQuestionEvidenceRejectsWeakSets(t *testing.T) {
	h := func(s string) scan.Hash { return xxh3.Hash128([]byte(s)).Bytes() }
	for _, files := range []scan.SemanticSet{
		nil,
		{"About/About.xml": h("metadata"), "About/Preview.png": h("preview"), "LoadFolders.xml": h("route")},
		{"Defs/A.xml": h("")},
		{"Defs/A.xml": h("shared"), "Defs/B.xml": h("shared"), "Defs/C.xml": h("shared")},
		{"Defs/A.xml": h("shared"), "Defs/B.xml": h("other")},
	} {
		if s := identity.QuestionScore("", "0", files, "", "0", files); s != 0 {
			t.Fatalf("weak evidence accepted: %v score=%v", files, s)
		}
	}
	if identity.SameDecl("", "0", "", "0") || identity.SameDecl(" ", "", " ", "") {
		t.Fatal("placeholder declaration matched")
	}
	a, b := scan.SemanticSet{}, scan.SemanticSet{}
	for n := 0; n < 3; n++ {
		a[fmt.Sprintf("Defs/%d.xml", n)] = h(fmt.Sprint(n))
		b[fmt.Sprintf("1.6/Defs/%d.xml", n)] = h(fmt.Sprint(n))
	}
	if s := identity.QuestionScore("a", "", a, "b", "", b); s != 1 {
		t.Fatalf("restructured content rejected: %v", s)
	}
}
