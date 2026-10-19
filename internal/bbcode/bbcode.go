// Package bbcode parses Steam Workshop descriptions (BBCode) into blocks that a terminal
// can lay out (06 §6.4 item 5). It is tolerant: unknown tags are dropped and their content
// kept, unclosed tags end with the text.
package bbcode

import (
	"regexp"
	"strings"
)

// Span is a run of text with one style.
type Span struct {
	Text   string
	Bold   bool
	Strike bool
	Link   string // target of [url]
}

// Kind of block.
type Kind int

const (
	Para    Kind = iota // a line of text; an empty Para is a paragraph break
	Heading             // Level 1–3
	Item                // a list item; Level is the depth (1 = top), Marker "•" or "3."
	Table               // Rows of cells
	Image               // URL
)

// Block is one laid-out unit.
type Block struct {
	Kind   Kind
	Level  int
	Marker string
	Quote  bool // inside [quote] or [code]: indented, secondary colour
	Spans  []Span
	Rows   [][]string
	URL    string
}

// Text is the block's text without styles.
func (b Block) Text() string {
	var s strings.Builder
	for _, sp := range b.Spans {
		s.WriteString(sp.Text)
	}
	return s.String()
}

var tagRe = regexp.MustCompile(`\[(/?)([a-zA-Z0-9*]+)(?:=([^\]]*))?\]`)

type list struct {
	ordered bool
	n       int
}

type parser struct {
	out     []Block
	cur     *Block
	bold    int
	strike  int
	link    []string
	lists   []list
	quote   int
	table   *Block
	row     []string
	cell    *strings.Builder
	inItem  bool // the current block is a list item (text continues it)
	heading int
}

// Parse turns BBCode into blocks.
func Parse(src string) []Block {
	p := &parser{}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	pos := 0
	for pos < len(src) {
		m := tagRe.FindStringSubmatchIndex(src[pos:])
		if m == nil {
			p.text(src[pos:])
			break
		}
		p.text(src[pos : pos+m[0]])
		closing := src[pos+m[2]:pos+m[3]] == "/"
		name := strings.ToLower(src[pos+m[4] : pos+m[5]])
		arg := ""
		if m[6] >= 0 {
			arg = strings.Trim(src[pos+m[6]:pos+m[7]], `"' `)
		}
		end := pos + m[1]
		switch {
		case name == "noparse" && !closing:
			if i := strings.Index(strings.ToLower(src[end:]), "[/noparse]"); i >= 0 {
				p.text(src[end : end+i])
				end += i + len("[/noparse]")
			}
		case name == "img" && !closing:
			url := src[end:]
			if i := strings.Index(strings.ToLower(url), "[/img]"); i >= 0 {
				url, end = url[:i], end+i+len("[/img]")
			} else {
				end = len(src)
			}
			p.image(strings.TrimSpace(url))
		case name == "code" && !closing:
			// code is verbatim
			if i := strings.Index(strings.ToLower(src[end:]), "[/code]"); i >= 0 {
				p.quote++
				p.flush()
				for _, l := range strings.Split(strings.Trim(src[end:end+i], "\n"), "\n") {
					p.out = append(p.out, Block{Kind: Para, Quote: true, Spans: []Span{{Text: strings.TrimRight(l, " \t")}}})
				}
				p.quote--
				end += i + len("[/code]")
			}
		default:
			p.tag(name, arg, closing)
		}
		pos = end
	}
	p.flush()
	if p.table != nil {
		p.endTable()
	}
	return tidy(p.out)
}

func (p *parser) block() *Block {
	if p.cur == nil {
		p.cur = &Block{Kind: Para, Quote: p.quote > 0}
		if p.heading > 0 {
			p.cur.Kind, p.cur.Level = Heading, p.heading
		}
	}
	return p.cur
}

func (p *parser) flush() {
	if p.cur != nil {
		p.out = append(p.out, *p.cur)
		p.cur = nil
	}
	p.inItem = false
}

func (p *parser) text(s string) {
	if s == "" {
		return
	}
	if p.table != nil {
		if p.cell != nil {
			p.cell.WriteString(strings.Join(strings.Fields(s), " "))
		}
		return
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if i > 0 {
			// a line break ends the line; inside a list item it ends the item's first line
			wasEmpty := p.cur == nil || (len(p.cur.Spans) == 0 && p.cur.Kind == Para)
			p.flush()
			if wasEmpty && len(p.lists) == 0 && p.heading == 0 {
				p.out = append(p.out, Block{Kind: Para}) // blank line: paragraph break
			}
		}
		l = strings.ReplaceAll(l, "\t", " ")
		if strings.TrimSpace(l) == "" && (p.cur == nil || len(p.cur.Spans) == 0) {
			continue
		}
		b := p.block()
		if len(b.Spans) == 0 {
			l = strings.TrimLeft(l, " ")
		}
		sp := Span{Text: l, Bold: p.bold > 0 || b.Kind == Heading, Strike: p.strike > 0}
		if len(p.link) > 0 {
			sp.Link = p.link[len(p.link)-1]
		}
		if n := len(b.Spans); n > 0 && b.Spans[n-1].Bold == sp.Bold && b.Spans[n-1].Strike == sp.Strike && b.Spans[n-1].Link == sp.Link {
			b.Spans[n-1].Text += sp.Text
		} else {
			b.Spans = append(b.Spans, sp)
		}
	}
}

func (p *parser) image(url string) {
	if url == "" {
		return
	}
	if p.table != nil {
		if p.cell != nil {
			p.cell.WriteString("[image]")
		}
		return
	}
	p.flush()
	p.out = append(p.out, Block{Kind: Image, URL: url, Quote: p.quote > 0})
}

func (p *parser) endTable() {
	if p.cell != nil {
		p.row = append(p.row, strings.TrimSpace(p.cell.String()))
		p.cell = nil
	}
	if len(p.row) > 0 {
		p.table.Rows = append(p.table.Rows, p.row)
		p.row = nil
	}
	if len(p.table.Rows) > 0 {
		p.out = append(p.out, *p.table)
	}
	p.table = nil
}

func (p *parser) tag(name, arg string, closing bool) {
	switch name {
	case "b", "strong":
		p.bold += step(closing)
	case "strike", "s":
		p.strike += step(closing)
	case "i", "u", "em", "spoiler", "color", "size", "font", "center":
		// shown as plain text (06 §2.1: no italics or underlines)
	case "url":
		if closing {
			if len(p.link) > 0 {
				p.link = p.link[:len(p.link)-1]
			}
		} else {
			p.link = append(p.link, arg)
		}
	case "h1", "h2", "h3", "h4":
		p.flush()
		if closing {
			p.heading = 0
		} else {
			p.heading = min(int(name[1]-'0'), 3)
		}
	case "list", "olist":
		p.flush()
		if closing {
			if len(p.lists) > 0 {
				p.lists = p.lists[:len(p.lists)-1]
			}
		} else {
			p.lists = append(p.lists, list{ordered: name == "olist" || arg == "1"})
		}
	case "*", "li":
		if closing {
			p.flush()
			return
		}
		p.flush()
		depth := max(len(p.lists), 1)
		marker := "•"
		if len(p.lists) > 0 {
			l := &p.lists[len(p.lists)-1]
			l.n++
			if l.ordered {
				marker = itoa(l.n) + "."
			}
		}
		p.cur = &Block{Kind: Item, Level: depth, Marker: marker, Quote: p.quote > 0}
		p.inItem = true
	case "quote":
		p.flush()
		p.quote += step(closing)
	case "hr":
		p.flush()
		p.out = append(p.out, Block{Kind: Para}) // a paragraph break (06 §2.1: no lines)
	case "nl", "br":
		p.flush()
	case "table":
		p.flush()
		if closing {
			if p.table != nil {
				p.endTable()
			}
		} else if p.table == nil {
			p.table = &Block{Kind: Table}
		}
	case "tr":
		if p.table == nil {
			return
		}
		if p.cell != nil {
			p.row = append(p.row, strings.TrimSpace(p.cell.String()))
			p.cell = nil
		}
		if len(p.row) > 0 {
			p.table.Rows = append(p.table.Rows, p.row)
			p.row = nil
		}
	case "td", "th":
		if p.table == nil {
			return
		}
		if closing {
			if p.cell != nil {
				p.row = append(p.row, strings.TrimSpace(p.cell.String()))
				p.cell = nil
			}
		} else {
			if p.cell != nil {
				p.row = append(p.row, strings.TrimSpace(p.cell.String()))
			}
			p.cell = &strings.Builder{}
		}
	}
}

func step(closing bool) int {
	if closing {
		return -1
	}
	return 1
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// tidy trims trailing spaces, drops empty items, collapses paragraph breaks and removes
// them at the ends and around headings, rules and images.
func tidy(in []Block) []Block {
	var out []Block
	for _, b := range in {
		for i := range b.Spans {
			if i == len(b.Spans)-1 {
				b.Spans[i].Text = strings.TrimRight(b.Spans[i].Text, " ")
			}
		}
		empty := b.Kind == Para && strings.TrimSpace(b.Text()) == ""
		if b.Kind == Item && strings.TrimSpace(b.Text()) == "" {
			continue
		}
		if empty {
			if len(out) == 0 || out[len(out)-1].isBreak() {
				continue
			}
			b = Block{Kind: Para}
		}
		out = append(out, b)
	}
	for len(out) > 0 && out[len(out)-1].isBreak() {
		out = out[:len(out)-1]
	}
	// Breaks next to headings and images go: renderers space headings themselves.
	var final []Block
	for i, b := range out {
		if b.isBreak() && ((i > 0 && out[i-1].Kind == Heading) || (i+1 < len(out) && out[i+1].Kind == Heading) ||
			(i > 0 && out[i-1].Kind == Image) || (i+1 < len(out) && out[i+1].Kind == Image)) {
			continue
		}
		final = append(final, b)
	}
	return final
}

func (b Block) isBreak() bool { return b.Kind == Para && len(b.Spans) == 0 }

// Plain renders the blocks as plain text for the CLI and MCP: headings on their own line,
// list items with their markers, table cells joined by " | ", images as [image: url].
func Plain(bs []Block) string {
	var s strings.Builder
	for _, b := range bs {
		ind := ""
		if b.Quote {
			ind = "  "
		}
		switch b.Kind {
		case Para:
			s.WriteString(ind + b.Text() + "\n")
		case Heading:
			if s.Len() > 0 {
				s.WriteString("\n")
			}
			s.WriteString(ind + b.Text() + "\n")
		case Item:
			s.WriteString(ind + strings.Repeat("  ", b.Level-1) + b.Marker + " " + b.Text() + "\n")
		case Table:
			for _, r := range b.Rows {
				s.WriteString(ind + strings.Join(r, " | ") + "\n")
			}
		case Image:
			s.WriteString(ind + "[image: " + b.URL + "]\n")
		}
	}
	return s.String()
}

// TextLen is the length in runes of the text without markup and images (06 §6.4: what
// "short" means when choosing a source).
func TextLen(bs []Block) int {
	n := 0
	for _, b := range bs {
		switch b.Kind {
		case Table:
			for _, r := range b.Rows {
				for _, c := range r {
					n += len([]rune(c))
				}
			}
		default:
			n += len([]rune(strings.TrimSpace(b.Text())))
		}
	}
	return n
}
