package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
)

// Op is a parsed PatchOperation (DirectXmlToObject subset).
type Op struct {
	class     string // short class name, e.g. PatchOperationAdd; raw value when unknown
	known     bool
	xpath     string
	attribute string
	attrValue string
	name      string
	value     *xmlquery.Node // the <value> element
	order     string
	success   string
	mods      []string
	match     *Op
	nomatch   *Op
	ops       []*Op
	mayReq    string // MayRequire on a list item (only honoured inside lists, like the game)
	mayReqAny string

	lastFailed *Op
}

var knownOps = map[string]bool{
	"PatchOperationAdd": true, "PatchOperationInsert": true, "PatchOperationRemove": true,
	"PatchOperationReplace": true, "PatchOperationAttributeAdd": true, "PatchOperationAttributeSet": true,
	"PatchOperationAttributeRemove": true, "PatchOperationSetName": true, "PatchOperationAddModExtension": true,
	"PatchOperationSequence": true, "PatchOperationTest": true, "PatchOperationConditional": true,
	"PatchOperationFindMod": true,
}

func parseOp(n *xmlquery.Node) *Op {
	cls := n.SelectAttr("Class")
	short := strings.TrimPrefix(cls, "Verse.")
	op := &Op{class: short, known: knownOps[short], mayReq: n.SelectAttr("MayRequire"), mayReqAny: n.SelectAttr("MayRequireAnyOf")}
	if cls == "" {
		op.class, op.known = "PatchOperation", true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xmlquery.ElementNode {
			continue
		}
		switch c.Data {
		case "xpath":
			op.xpath = c.InnerText()
		case "attribute":
			op.attribute = c.InnerText()
		case "name":
			op.name = c.InnerText()
		case "order":
			op.order = strings.TrimSpace(c.InnerText())
		case "success":
			op.success = strings.TrimSpace(c.InnerText())
		case "value":
			if op.class == "PatchOperationAttributeAdd" || op.class == "PatchOperationAttributeSet" {
				op.attrValue = c.InnerText()
			} else {
				op.value = c
			}
		case "mods":
			for li := c.FirstChild; li != nil; li = li.NextSibling {
				if li.Type == xmlquery.ElementNode {
					op.mods = append(op.mods, li.InnerText())
				}
			}
		case "match":
			op.match = parseOp(c)
		case "nomatch":
			op.nomatch = parseOp(c)
		case "operations":
			for li := c.FirstChild; li != nil; li = li.NextSibling {
				if li.Type == xmlquery.ElementNode {
					op.ops = append(op.ops, parseOp(li))
				}
			}
		}
	}
	return op
}

func (o *Op) unknown() bool { return !o.known }

// String mirrors the game's ToString overrides (used in "Patch operation X failed").
func (o *Op) String() string {
	full := o.class
	if o.known {
		full = "Verse." + o.class
	}
	switch o.class {
	case "PatchOperationSequence":
		s := fmt.Sprintf("%s(count=%d", full, len(o.ops))
		if o.lastFailed != nil {
			s += ", lastFailedOperation=" + o.lastFailed.String()
		}
		return s + ")"
	case "PatchOperationFindMod":
		return full + "(" + strings.Join(o.mods, ", ") + ")"
	case "PatchOperationAttributeAdd", "PatchOperationAttributeSet", "PatchOperationAttributeRemove":
		return full + "(" + o.xpath + ")(" + o.attribute + ")"
	case "PatchOperation":
		return full
	}
	if o.known {
		return full + "(" + o.xpath + ")"
	}
	return full
}

type Sim struct {
	doc            *xmlquery.Node
	activeNoSuffix map[string]bool
	activeNames    map[string]bool
	cache          map[string]*xpath.Expr
	compileErrs    int
	runtimeErrs    int
	evals          int
	applied        int
	badXPaths      map[string]string
	plans          map[string]*fastPlan
	ix             defIndex
}

var errRuntime = errors.New("runtime exception")

// Apply runs a top-level operation; exceptions count as failure (LoadedModManager.ApplyPatches).
func (s *Sim) Apply(o *Op) bool {
	ok, err := s.apply(o)
	if err != nil {
		return false
	}
	return ok
}

func (s *Sim) apply(o *Op) (bool, error) {
	s.applied++
	r, err := s.worker(o)
	if *traceSub != "" && strings.Contains(o.xpath, *traceSub) {
		fmt.Printf("TRACE %v err=%v %s\n", r, err, o)
	}
	if err != nil {
		return false, err
	}
	switch o.success {
	case "Always":
		r = true
	case "Never":
		r = false
	case "Invert":
		r = !r
	}
	return r, nil
}

func (s *Sim) compile(expr string) (*xpath.Expr, error) {
	if e, ok := s.cache[expr]; ok {
		if e == nil {
			return nil, errRuntime
		}
		return e, nil
	}
	e, err := xpath.Compile(expr)
	if err != nil {
		s.compileErrs++
		if s.badXPaths == nil {
			s.badXPaths = map[string]string{}
		}
		s.badXPaths[expr] = err.Error()
		s.cache[expr] = nil
		return nil, errRuntime
	}
	s.cache[expr] = e
	return e, nil
}

func (s *Sim) selectNodes(expr string) ([]*xmlquery.Node, error) {
	e, err := s.compile(expr)
	if err != nil {
		return nil, err
	}
	s.evals++
	if *useIndex {
		if p := s.plan(expr); p != nil {
			return s.fastSelect(p, false), nil
		}
	}
	var out []*xmlquery.Node
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errRuntime
			}
		}()
		out = xmlquery.QuerySelectorAll(s.doc, e)
	}()
	return out, err
}

func (s *Sim) selectOne(expr string) (*xmlquery.Node, error) {
	e, err := s.compile(expr)
	if err != nil {
		return nil, err
	}
	s.evals++
	if *useIndex {
		if p := s.plan(expr); p != nil {
			if r := s.fastSelect(p, true); len(r) > 0 {
				return r[0], nil
			}
			return nil, nil
		}
	}
	var out *xmlquery.Node
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errRuntime
			}
		}()
		out = xmlquery.QuerySelector(s.doc, e)
	}()
	return out, err
}

// isAttr: .NET XmlAttribute.ParentNode is null, so tree edits on attributes throw.
func isAttr(n *xmlquery.Node) bool { return n.Type == xmlquery.AttributeNode }

func (s *Sim) valueChildren(o *Op) []*xmlquery.Node {
	if o.value == nil {
		return nil
	}
	return o.value.ChildNodes()
}

func (s *Sim) worker(o *Op) (bool, error) {
	switch o.class {
	case "PatchOperationAdd":
		if o.value == nil {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if isAttr(n) {
				return false, errRuntime
			}
			if o.order == "Prepend" {
				for i := len(kids) - 1; i >= 0; i-- {
					prependChild(n, clone(kids[i]))
				}
			} else {
				for _, k := range kids {
					appendChild(n, clone(k))
				}
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationInsert":
		if o.value == nil {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if isAttr(n) || n.Parent == nil {
				return false, errRuntime
			}
			if o.order == "Append" {
				// InsertAfter(child, xmlNode) for each child in order: later children end up first.
				for _, k := range kids {
					insertAfter(n, clone(k))
				}
			} else {
				for i := len(kids) - 1; i >= 0; i-- {
					insertBefore(n, clone(kids[i]))
				}
			}
			if n.Type == xmlquery.TextNode {
				normalize(n.Parent)
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationReplace":
		if o.value == nil {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if isAttr(n) || n.Parent == nil {
				s.runtimeErrs++
				return false, errRuntime
			}
			for _, k := range kids {
				insertBefore(n, clone(k))
			}
			detach(n)
		}
		return len(nodes) > 0, nil
	case "PatchOperationRemove":
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		for _, n := range nodes {
			if isAttr(n) || n.Parent == nil {
				s.runtimeErrs++
				return false, errRuntime
			}
			detach(n)
		}
		return len(nodes) > 0, nil
	case "PatchOperationAttributeAdd", "PatchOperationAttributeSet", "PatchOperationAttributeRemove":
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		res := false
		for _, n := range nodes {
			if n.Type != xmlquery.ElementNode {
				if o.class == "PatchOperationAttributeSet" {
					// Attributes is null on non-elements: .NET throws.
					s.runtimeErrs++
					return false, errRuntime
				}
				continue
			}
			touchAttr(n, o.attribute)
			has := n.HasAttr(o.attribute)
			switch o.class {
			case "PatchOperationAttributeAdd":
				if !has {
					xmlquery.AddAttr(n, o.attribute, o.attrValue)
					res = true
				}
			case "PatchOperationAttributeSet":
				if has {
					n.SetAttr(o.attribute, o.attrValue)
				} else {
					xmlquery.AddAttr(n, o.attribute, o.attrValue)
				}
				res = true
			case "PatchOperationAttributeRemove":
				if has {
					n.RemoveAttr(o.attribute)
					res = true
				}
			}
		}
		return res, nil
	case "PatchOperationSetName":
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		for _, n := range nodes {
			if isAttr(n) || n.Parent == nil {
				s.runtimeErrs++
				return false, errRuntime
			}
			nn := &xmlquery.Node{Type: xmlquery.ElementNode, Data: o.name}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				appendChild(nn, clone(c))
			}
			insertBefore(n, nn)
			detach(n)
		}
		return len(nodes) > 0, nil
	case "PatchOperationAddModExtension":
		if o.value == nil {
			return false, errRuntime
		}
		nodes, err := s.selectNodes(o.xpath)
		if err != nil {
			return false, err
		}
		kids := s.valueChildren(o)
		for _, n := range nodes {
			if isAttr(n) {
				return false, errRuntime
			}
			var ext *xmlquery.Node
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xmlquery.ElementNode && c.Data == "modExtensions" {
					ext = c
					break
				}
			}
			if ext == nil {
				ext = &xmlquery.Node{Type: xmlquery.ElementNode, Data: "modExtensions"}
				appendChild(n, ext)
			}
			for _, k := range kids {
				appendChild(ext, clone(k))
			}
		}
		return len(nodes) > 0, nil
	case "PatchOperationTest":
		n, err := s.selectOne(o.xpath)
		return n != nil, err
	case "PatchOperationConditional":
		n, err := s.selectOne(o.xpath)
		if err != nil {
			return false, err
		}
		if n != nil {
			if o.match != nil {
				return s.apply(o.match)
			}
		} else if o.nomatch != nil {
			return s.apply(o.nomatch)
		}
		if o.match == nil {
			return o.nomatch != nil, nil
		}
		return true, nil
	case "PatchOperationFindMod":
		found := false
		for _, m := range o.mods {
			if s.activeNames[m] {
				found = true
				break
			}
		}
		if found {
			if o.match != nil {
				return s.apply(o.match)
			}
		} else if o.nomatch != nil {
			return s.apply(o.nomatch)
		}
		return true, nil
	case "PatchOperationSequence":
		for _, c := range o.ops {
			if !s.listItemActive(c) {
				continue
			}
			ok, err := s.apply(c)
			if err != nil {
				return false, err
			}
			if !ok {
				o.lastFailed = c
				return false, nil
			}
		}
		return true, nil
	case "PatchOperation":
		return false, nil
	}
	// Mod-defined operation class: cannot simulate; assume it succeeds.
	return true, nil
}

// listItemActive mirrors DirectXmlToObject.ListFromXml's MayRequire handling.
func (s *Sim) listItemActive(o *Op) bool {
	if o.mayReq != "" {
		for _, id := range strings.Split(o.mayReq, ",") {
			if !s.activeNoSuffix[strings.ToLower(strings.TrimSpace(id))] {
				return false
			}
		}
	}
	if o.mayReqAny != "" {
		for _, id := range strings.Split(o.mayReqAny, ",") {
			if s.activeNoSuffix[strings.ToLower(strings.TrimSpace(id))] {
				return true
			}
		}
		return false
	}
	return true
}
