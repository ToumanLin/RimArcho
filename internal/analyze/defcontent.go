package analyze

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/xdoc"
)

const DefContentVersion = 1

// DefKeySuffix ends a Mod's Def fact-set key: the analyzer and Def content versions, so a
// change to either makes new sets instead of reusing ones without the new fields (a set
// with an existing key is never rewritten, store.ensureFactSet).
func DefKeySuffix() string { return fmt.Sprintf("|a%d|c%d", Version, DefContentVersion) }

// DefContent retains only predicate inputs, never a full Def subtree (05 §5.1).
type DefContent struct {
	Version    int        `json:"v"`
	Incomplete bool       `json:"incomplete,omitempty"`
	Fields     []DefField `json:"fields,omitempty"`
}

// DefField preserves presence, null, list append/reset and conditional-child semantics.
type DefField struct {
	Name          string     `json:"n"`
	Value         string     `json:"v,omitempty"`
	Reset         bool       `json:"reset,omitempty"`
	Null          bool       `json:"null,omitempty"`
	MayRequire    string     `json:"mr,omitempty"`
	MayRequireAny string     `json:"mra,omitempty"`
	Children      []DefField `json:"c,omitempty"`
}

func (f DefField) Loaded(active func(string) bool) bool {
	return mayRequireOK(f.MayRequire, f.MayRequireAny, active)
}

func extractDefContent(d *xdoc.Doc, root int32) *DefContent {
	c := &DefContent{Version: DefContentVersion}
	left := 256
	bounded := func(v string) string {
		if len(v) > 512 {
			c.Incomplete = true
			v = v[:512]
		}
		return strings.Clone(v)
	}
	var read func(int32, int) DefField
	read = func(n int32, depth int) DefField {
		left--
		f := DefField{Name: d.Name(n)}
		inherit, _ := d.Attr(n, "Inherit")
		f.Reset = strings.EqualFold(inherit, "false")
		null, _ := d.Attr(n, "IsNull")
		f.Null = strings.EqualFold(null, "true")
		f.MayRequire, _ = d.Attr(n, "MayRequire")
		f.MayRequireAny, _ = d.Attr(n, "MayRequireAnyOf")
		f.MayRequire = bounded(f.MayRequire)
		f.MayRequireAny = bounded(f.MayRequireAny)
		// Lists need only eligible entry presence, not verb/tool bodies.
		if f.Name == "verbs" || f.Name == "tools" {
			for _, child := range d.Children(n) {
				if left <= 0 {
					c.Incomplete = true
					break
				}
				left--
				a, _ := d.Attr(child, "MayRequire")
				b, _ := d.Attr(child, "MayRequireAnyOf")
				f.Children = append(f.Children, DefField{Name: "li", MayRequire: bounded(a), MayRequireAny: bounded(b)})
			}
		} else if depth < 2 && (f.Name == "race" || f.Name == "building") {
			if len(d.Children(n)) == 0 {
				f.Value = bounded(strings.TrimSpace(d.InnerText(n)))
			}
			for _, child := range d.Children(n) {
				switch d.Name(child) {
				case "intelligence", "fleshType", "isNaturalRock", "isResourceRock":
					if left <= 0 {
						c.Incomplete = true
						break
					}
					f.Children = append(f.Children, read(child, depth+1))
				}
			}
		} else if f.Name != "apparel" && f.Name != "plant" {
			f.Value = bounded(strings.TrimSpace(d.InnerText(n)))
		}
		return f
	}
	for _, n := range d.Children(root) {
		switch d.Name(n) {
		case "category", "thingClass", "race", "apparel", "plant", "building", "verbs", "tools", "hidden", "isOrganic":
			if left <= 0 {
				c.Incomplete = true
				break
			}
			c.Fields = append(c.Fields, read(n, 0))
		}
	}
	return c
}
