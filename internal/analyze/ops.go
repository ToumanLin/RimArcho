package analyze

import (
	"fmt"
	"strings"

	"github.com/rimarcho/rimarcho/internal/xdoc"
)

// Op is a parsed PatchOperation (the DirectXmlToObject subset the game uses).
type Op struct {
	Class     string // short class name, e.g. PatchOperationAdd; raw value when unknown
	Known     bool
	XPath     string
	attribute string
	attrValue string
	name      string
	valDoc    *xdoc.Doc
	value     int32 // <value> element, xdoc.None if absent
	order     string
	success   string
	mods      []string
	match     *Op
	nomatch   *Op
	ops       []*Op
	mayReq    string // MayRequire / MayRequireAnyOf: honoured only on list items, like the game
	mayReqAny string
	Line      int

	// filled by the simulation
	lastFailed *Op
	hits       int
	ran        bool
	result     bool
	err        bool
}

var knownOps = map[string]bool{
	"PatchOperationAdd": true, "PatchOperationInsert": true, "PatchOperationRemove": true,
	"PatchOperationReplace": true, "PatchOperationAttributeAdd": true, "PatchOperationAttributeSet": true,
	"PatchOperationAttributeRemove": true, "PatchOperationSetName": true, "PatchOperationAddModExtension": true,
	"PatchOperationSequence": true, "PatchOperationTest": true, "PatchOperationConditional": true,
	"PatchOperationFindMod": true,
}

func parseOp(d *xdoc.Doc, n int32) *Op {
	cls, _ := d.Attr(n, "Class")
	short := strings.TrimPrefix(cls, "Verse.")
	mr, _ := d.Attr(n, "MayRequire")
	mra, _ := d.Attr(n, "MayRequireAnyOf")
	op := &Op{Class: short, Known: knownOps[short], mayReq: mr, mayReqAny: mra, valDoc: d, value: xdoc.None, Line: d.Line(n)}
	if cls == "" {
		op.Class, op.Known = "PatchOperation", true
	}
	for _, c := range d.Children(n) {
		switch d.Name(c) {
		case "xpath":
			op.XPath = d.InnerText(c)
		case "attribute":
			op.attribute = d.InnerText(c)
		case "name":
			op.name = d.InnerText(c)
		case "order":
			op.order = strings.TrimSpace(d.InnerText(c))
		case "success":
			op.success = strings.TrimSpace(d.InnerText(c))
		case "value":
			if op.Class == "PatchOperationAttributeAdd" || op.Class == "PatchOperationAttributeSet" {
				op.attrValue = d.InnerText(c)
			} else {
				op.value = c
			}
		case "mods":
			for _, li := range d.Children(c) {
				op.mods = append(op.mods, d.InnerText(li))
			}
		case "match":
			op.match = parseOp(d, c)
		case "nomatch":
			op.nomatch = parseOp(d, c)
		case "operations":
			for _, li := range d.Children(c) {
				op.ops = append(op.ops, parseOp(d, li))
			}
		}
	}
	return op
}

// String mirrors the game's ToString overrides, as printed in "Patch operation X failed".
func (o *Op) String() string {
	full := o.Class
	if o.Known {
		full = "Verse." + o.Class
	}
	switch o.Class {
	case "PatchOperationSequence":
		s := fmt.Sprintf("%s(count=%d", full, len(o.ops))
		if o.lastFailed != nil {
			s += ", lastFailedOperation=" + o.lastFailed.String()
		}
		return s + ")"
	case "PatchOperationFindMod":
		return full + "(" + strings.Join(o.mods, ", ") + ")"
	case "PatchOperationAttributeAdd", "PatchOperationAttributeSet", "PatchOperationAttributeRemove":
		return full + "(" + o.XPath + ")(" + o.attribute + ")"
	case "PatchOperation":
		return full
	}
	if o.Known {
		return full + "(" + o.XPath + ")"
	}
	return full
}

// walk visits o and every nested operation.
func (o *Op) walk(fn func(*Op)) {
	if o == nil {
		return
	}
	fn(o)
	o.match.walk(fn)
	o.nomatch.walk(fn)
	for _, c := range o.ops {
		c.walk(fn)
	}
}

// failedLeaf follows the failure down to the innermost operation that failed.
func (o *Op) failedLeaf() *Op {
	switch o.Class {
	case "PatchOperationSequence":
		if o.lastFailed != nil {
			return o.lastFailed.failedLeaf()
		}
	case "PatchOperationConditional", "PatchOperationFindMod":
		for _, b := range []*Op{o.match, o.nomatch} {
			if b != nil && b.ran && !b.result {
				return b.failedLeaf()
			}
		}
	}
	return o
}
