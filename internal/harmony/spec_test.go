package harmony

import (
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/speccheck"
)

// 05 §5.3 "Skip list" names exactly harmony.DefaultSkip.
func TestSpecDefaultSkip(t *testing.T) {
	t.Parallel()
	sec, err := speccheck.Section("05", "5.3")
	if err != nil {
		t.Fatal(err)
	}
	var doc []string
	var where speccheck.Line
	for _, l := range sec {
		if strings.HasPrefix(l.Text, "- **Skip list**") {
			where = l
			for _, s := range speccheck.Spans(l.Text) {
				if strings.HasSuffix(s, ".dll") {
					doc = append(doc, s)
				}
			}
		}
	}
	if len(doc) == 0 {
		t.Fatal("05 §5.3: no **Skip list** item")
	}
	for _, p := range listParity(doc, DefaultSkip) {
		t.Errorf("%s: 05 §5.3 skip list: %s harmony.DefaultSkip", where.Pos(), p)
	}
}

// listParity names what one list has and the other lacks.
func listParity(doc, code []string) []string {
	var out []string
	for _, c := range code {
		if !slices.Contains(doc, c) {
			out = append(out, c+" is missing; it is in")
		}
	}
	for _, d := range doc {
		if !slices.Contains(code, d) {
			out = append(out, d+" is listed, but it is not in")
		}
	}
	return out
}

func TestSpecListParityDetection(t *testing.T) {
	t.Parallel()
	if got := listParity([]string{"a", "b"}, []string{"b", "c"}); len(got) != 2 {
		t.Errorf("%v", got)
	}
}
