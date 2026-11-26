package xdoc

import (
	"testing"

	"github.com/antchfx/xpath"
)

const sample = `<?xml version="1.0" encoding="utf-8"?>
<Defs>
  <!-- comment -->
  <ThingDef Name="BaseGun" Abstract="True"><label>base</label></ThingDef>
  <ThingDef ParentName="BaseGun">
    <defName>Gun_A</defName>
    <verbs><li><range>10</range></li></verbs>
  </ThingDef>
  <ThingDef><defName>Gun_B</defName><label>b &amp; c</label></ThingDef>
</Defs>`

func sel(t *testing.T, d *Doc, expr string) []Ref {
	t.Helper()
	e, err := xpath.Compile(expr)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := d.Select(e, 0, false)
	if !ok {
		t.Fatalf("%s panicked", expr)
	}
	return r
}

func TestParseSelectMutate(t *testing.T) {
	d, err := ParseDoc([]byte(sample), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := sel(t, d, `Defs/ThingDef[defName="Gun_A"]/verbs/li/range`); len(r) != 1 || d.InnerText(r[0].N) != "10" {
		t.Fatalf("range: %v", r)
	}
	if r := sel(t, d, `/Defs/ThingDef[@Name="BaseGun"]/@Abstract`); len(r) != 1 || !r[0].IsAttr() {
		t.Fatalf("attr: %v", r)
	}
	if r := sel(t, d, `Defs/ThingDef[defName="Gun_B"]/label`); d.InnerText(r[0].N) != "b & c" {
		t.Fatalf("entity: %q", d.InnerText(r[0].N))
	}
	if n := len(sel(t, d, `Defs/ThingDef`)); n != 3 {
		t.Fatalf("defs: %d", n)
	}
	if n := len(sel(t, d, `//li | //range`)); n != 2 {
		t.Fatalf("union: %d", n)
	}
	// Mutations.
	a := sel(t, d, `Defs/ThingDef[defName="Gun_A"]`)[0].N
	b := sel(t, d, `Defs/ThingDef[defName="Gun_B"]`)[0].N
	d.Detach(b)
	if n := len(sel(t, d, `Defs/ThingDef`)); n != 2 {
		t.Fatalf("after detach: %d", n)
	}
	c := d.Clone(d, b)
	d.InsertBefore(a, c)
	if r := sel(t, d, `Defs/ThingDef[2]/defName`); d.InnerText(r[0].N) != "Gun_B" {
		t.Fatalf("insert order: %q", d.InnerText(r[0].N))
	}
	d.SetAttr(a, "Abstract", "False")
	if n := len(sel(t, d, `Defs/ThingDef[@Abstract="False"]`)); n != 1 {
		t.Fatalf("attr set: %d", n)
	}
	if d.Line(a) != 5 {
		t.Fatalf("line = %d", d.Line(a))
	}
	// Cross-document clone with another name table.
	o := New(nil)
	o.AppendChild(0, o.Clone(d, a))
	if r := sel(t, o, `ThingDef/verbs/li/range`); len(r) != 1 {
		t.Fatalf("cross clone: %v", r)
	}
}

// SelectLimit stops an evaluation at its budget of navigator moves, the same place every
// time, and a later evaluation starts with a fresh count (05 §5.2).
func TestSelectLimit(t *testing.T) {
	d, err := ParseDoc([]byte(sample), nil)
	if err != nil {
		t.Fatal(err)
	}
	e := xpath.MustCompile(`Defs/ThingDef[last()]`)
	all, err := d.SelectLimit(e, 0, false, 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("unlimited: %v %v", all, err)
	}
	need := d.Steps()
	if r, err := d.SelectLimit(e, 0, false, need); err != nil || len(r) != 1 || d.Steps() != need {
		t.Fatalf("at the budget: %v %v, %d steps of %d", r, err, d.Steps(), need)
	}
	for range 2 {
		if r, err := d.SelectLimit(e, 0, false, need-1); err != ErrSteps || r != nil || d.Steps() != need {
			t.Fatalf("over the budget: %v %v, %d steps", r, err, d.Steps())
		}
	}
	if _, ok := d.Select(e, 0, false); !ok {
		t.Fatal("Select after an evaluation over budget")
	}
}
