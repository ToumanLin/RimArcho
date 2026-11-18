package store

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/speccheck"
)

// 04 §4 "Schema and upgrades" lists every step of store.upgrades by the schema it creates:
// FirstReleaseSchema+1 … SchemaVersion, in order.
func TestSpecSchemaUpgrades(t *testing.T) {
	t.Parallel()
	sec, err := speccheck.Section("04", "4")
	if err != nil {
		t.Fatal(err)
	}
	var para speccheck.Line
	for _, l := range sec {
		if strings.HasPrefix(l.Text, "**Schema and upgrades**") {
			para = l
		}
	}
	if para.Text == "" {
		t.Fatal("04 §4: no **Schema and upgrades** paragraph")
	}
	got, err := upgradeStepsDocumented(para.Text)
	if err != nil {
		t.Fatalf("%s: 04 §4: %v", para.Pos(), err)
	}
	if len(upgrades) != SchemaVersion-FirstReleaseSchema {
		t.Fatalf("store.upgrades has %d steps for schema %d..%d", len(upgrades), FirstReleaseSchema, SchemaVersion)
	}
	for _, p := range upgradeStepProblems(got, FirstReleaseSchema, SchemaVersion) {
		t.Errorf("%s: %s", para.Pos(), p)
	}
}

var upgradeListRe = regexp.MustCompile("appends one step to `store\\.upgrades` \\(([^()]*(?:\\([^()]*\\)[^()]*)*)\\)")
var upgradeStepRe = regexp.MustCompile(`(?:^|; )(\d+): `)

// upgradeStepsDocumented reads the numbers of the step list "(30: …; 31: …)".
func upgradeStepsDocumented(text string) ([]int, error) {
	m := upgradeListRe.FindStringSubmatch(text)
	if m == nil {
		return nil, fmt.Errorf("no step list after \"appends one step to `store.upgrades`\"")
	}
	var out []int
	for _, s := range upgradeStepRe.FindAllStringSubmatch(m[1], -1) {
		n, _ := strconv.Atoi(s[1])
		out = append(out, n)
	}
	return out, nil
}

func upgradeStepProblems(got []int, first, last int) []string {
	var out []string
	want := map[int]bool{}
	for v := first + 1; v <= last; v++ {
		want[v] = true
	}
	for i, n := range got {
		if !want[n] {
			out = append(out, fmt.Sprintf("04 §4: step %d is documented, but store.upgrades creates schema %d..%d", n, first+1, last))
		}
		if i > 0 && n <= got[i-1] {
			out = append(out, fmt.Sprintf("04 §4: step %d is listed after %d; keep the steps in order", n, got[i-1]))
		}
		delete(want, n)
	}
	for v := first + 1; v <= last; v++ {
		if want[v] {
			out = append(out, fmt.Sprintf("04 §4: step %d (store.upgrades[%d]) is not documented in the step list; add what it changes and the section that owns it", v, v-first-1))
		}
	}
	return out
}

func TestSpecSchemaUpgradesDetection(t *testing.T) {
	t.Parallel()
	got, err := upgradeStepsDocumented("A change appends one step to `store.upgrades` (30: a, 05 §5.3; 31: b (x); 33: c). More.")
	if err != nil || fmt.Sprint(got) != "[30 31 33]" {
		t.Fatalf("%v %v", got, err)
	}
	if p := upgradeStepProblems(got, 29, 33); len(p) != 1 || !strings.Contains(p[0], "step 32") {
		t.Errorf("%v", p)
	}
	if p := upgradeStepProblems([]int{30, 32, 31, 34}, 29, 32); len(p) != 2 {
		t.Errorf("%v", p)
	}
}
