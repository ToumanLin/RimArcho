// Package render builds the semantic outline text shared by CLI and MCP (05 §2): a subject
// line, two-space indentation per level, `key: value` lines or list items, truncation hints,
// and an optional trailing `next:` section. No ANSI colour, no JSON.
package render

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// width measures display columns; East Asian ambiguous characters (…, →, ·) count as one
// column, matching Windows Terminal regardless of locale (07 §6.1 item 3).
var width = &runewidth.Condition{EastAsianWidth: false}

// DefaultLimit is the default number of list items before truncation (keeps output ≤ 40 lines, 05 §6 I5).
const DefaultLimit = 30

// Node is one outline line with its children.
type Node struct {
	Line     string
	Children []*Node
}

// Doc is a rendered command result: top-level nodes (normally one subject) plus next steps.
type Doc struct {
	Nodes []*Node
	Next  []string
}

// New starts a document with a subject line.
func New(subject string) (*Doc, *Node) {
	n := &Node{Line: subject}
	return &Doc{Nodes: []*Node{n}}, n
}

// Add appends a child line and returns it.
func (n *Node) Add(line string) *Node {
	c := &Node{Line: line}
	n.Children = append(n.Children, c)
	return c
}

// Addf appends a formatted child line.
func (n *Node) Addf(format string, args ...any) *Node { return n.Add(fmt.Sprintf(format, args...)) }

// KV appends "key: value"; an empty value is skipped and returns nil.
func (n *Node) KV(key, value string) *Node {
	if value == "" {
		return nil
	}
	return n.Add(key + ": " + value)
}

// Section appends "key:" and returns it for children.
func (n *Node) Section(key string) *Node { return n.Add(key + ":") }

// Truncate keeps the first limit children (limit ≤ 0 keeps all) and appends the
// "… N more (cmd)" hint (05 §2.1 rule 4).
func (n *Node) Truncate(limit int, expand string) {
	if limit <= 0 || len(n.Children) <= limit {
		return
	}
	more := len(n.Children) - limit
	n.Children = n.Children[:limit]
	if expand != "" {
		n.Addf("… %d more (%s)", more, expand)
	} else {
		n.Addf("… %d more", more)
	}
}

// AddNext appends suggested next commands.
func (d *Doc) AddNext(cmds ...string) { d.Next = append(d.Next, cmds...) }

// String renders the document; every line ends with "\n".
func (d *Doc) String() string {
	var sb strings.Builder
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		sb.WriteString(strings.Repeat("  ", depth))
		sb.WriteString(strings.TrimRight(n.Line, " "))
		sb.WriteByte('\n')
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, n := range d.Nodes {
		walk(n, 0)
	}
	if len(d.Next) > 0 {
		sb.WriteString("next:\n")
		for _, c := range d.Next {
			sb.WriteString("  " + c + "\n")
		}
	}
	return sb.String()
}

// MaxColumn caps column padding so one very long name does not push every row apart.
const MaxColumn = 40

// Columns pads cells so columns line up by display width (CJK-aware); the last column is
// not padded. Empty trailing cells are dropped.
func Columns(rows [][]string) []string {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			if i < len(r)-1 {
				widths[i] = min(max(widths[i], width.StringWidth(c)), MaxColumn)
			}
		}
	}
	out := make([]string, len(rows))
	for ri, r := range rows {
		for len(r) > 0 && r[len(r)-1] == "" {
			r = r[:len(r)-1]
		}
		var sb strings.Builder
		for i, c := range r {
			sb.WriteString(c)
			if i < len(r)-1 {
				sb.WriteString(strings.Repeat(" ", max(widths[i]-width.StringWidth(c), 0)+2))
			}
		}
		out[ri] = sb.String()
	}
	return out
}

// AddColumns appends aligned rows as children.
func (n *Node) AddColumns(rows [][]string) {
	for _, l := range Columns(rows) {
		n.Add(l)
	}
}

// Size formats a byte count.
func Size(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

// Plural returns "1 mod" / "2 mods" ("1 entry" / "2 entries").
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	if strings.HasSuffix(word, "y") && !strings.HasSuffix(word, "ey") {
		return fmt.Sprintf("%d %sies", n, word[:len(word)-1])
	}
	return fmt.Sprintf("%d %ss", n, word)
}
