package logs

import (
	"strings"
	"unicode/utf8"
)

// Unity rich text in log messages (16 §2.10): mods colour their messages with <color>, <b>
// and <i>, as RimWorld's debug log shows them. RichSpans parses a message into styled
// spans; nothing in the text ever becomes markup for an interface.

// Span is a run of text with one style. Color is "#rrggbb" ("" for the line's own colour).
type Span struct {
	Text   string
	Color  string
	Bold   bool
	Italic bool
}

// unityColors are the colour names Unity's rich text knows.
var unityColors = map[string]string{
	"aqua": "#00ffff", "black": "#000000", "blue": "#0000ff", "brown": "#a52a2a", "cyan": "#00ffff",
	"darkblue": "#0000a0", "fuchsia": "#ff00ff", "green": "#008000", "grey": "#808080", "gray": "#808080",
	"lightblue": "#add8e6", "lime": "#00ff00", "magenta": "#ff00ff", "maroon": "#800000", "navy": "#000080",
	"olive": "#808000", "orange": "#ffa500", "purple": "#800080", "red": "#ff0000", "silver": "#c0c0c0",
	"teal": "#008080", "white": "#ffffff", "yellow": "#ffff00",
}

// droppedTags are Unity tags that change size or embed things: removed, their text kept.
var droppedTags = map[string]bool{"size": true, "material": true, "quad": true, "sprite": true, "u": true, "s": true, "mark": true, "font": true, "align": true}

// ParseColor reads a Unity colour value: #rgb, #rgba, #rrggbb, #rrggbbaa or a name.
// The alpha is ignored. ok is false for anything else.
func ParseColor(v string) (string, bool) {
	v = strings.ToLower(strings.Trim(strings.TrimSpace(v), `"'`))
	if c, ok := unityColors[v]; ok {
		return c, true
	}
	if !strings.HasPrefix(v, "#") {
		return "", false
	}
	h := v[1:]
	for _, r := range h {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", false
		}
	}
	switch len(h) {
	case 3, 4:
		return "#" + string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]}), true
	case 6, 8:
		return "#" + h[:6], true
	}
	return "", false
}

// RichSpans parses s. Known tags style the text (<color=…>, <b>, <i>, nested); size-like
// tags are removed; a closing tag with nothing open is removed; an unclosed tag lasts to
// the end. Anything else that looks like a tag (<li Class="…">, <script>) stays as text,
// as in the game.
func RichSpans(s string) []Span {
	type style struct {
		color        string
		bold, italic bool
	}
	var out []Span
	stack := []struct {
		tag string
		st  style
	}{}
	cur := style{}
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		t := buf.String()
		buf.Reset()
		if n := len(out); n > 0 && out[n-1].Color == cur.color && out[n-1].Bold == cur.bold && out[n-1].Italic == cur.italic {
			out[n-1].Text += t
			return
		}
		out = append(out, Span{Text: t, Color: cur.color, Bold: cur.bold, Italic: cur.italic})
	}
	for i := 0; i < len(s); {
		if s[i] != '<' {
			_, w := utf8.DecodeRuneInString(s[i:])
			buf.WriteString(s[i : i+w])
			i += w
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			buf.WriteString(s[i:])
			break
		}
		raw := s[i : i+end+1]
		body := strings.TrimSpace(raw[1 : len(raw)-1])
		closing := strings.HasPrefix(body, "/")
		body = strings.TrimPrefix(body, "/")
		name, val := strings.ToLower(body), ""
		if k := strings.IndexByte(body, '='); k >= 0 {
			name, val = strings.ToLower(strings.TrimSpace(body[:k])), body[k+1:]
		}
		handled := true
		switch {
		case droppedTags[name]:
			// removed, the text inside stays
		case name == "color" || name == "b" || name == "i":
			if closing {
				if val != "" {
					handled = false
					break
				}
				// close the innermost open tag of that name; a stray one is removed
				for j := len(stack) - 1; j >= 0; j-- {
					if stack[j].tag == name {
						flush()
						cur = stack[j].st
						stack = stack[:j]
						break
					}
				}
				break
			}
			next := cur
			switch name {
			case "color":
				c, ok := ParseColor(val)
				if !ok {
					handled = false
					break
				}
				next.color = c
			case "b", "i":
				if val != "" {
					handled = false
					break
				}
				if name == "b" {
					next.bold = true
				} else {
					next.italic = true
				}
			}
			if !handled {
				break
			}
			flush()
			stack = append(stack, struct {
				tag string
				st  style
			}{name, cur})
			cur = next
		default:
			handled = false
		}
		if !handled {
			buf.WriteString(raw)
		}
		i += end + 1
	}
	flush()
	return out
}

// PlainText is s without the rich-text tags RichSpans understands.
func PlainText(s string) string {
	var b strings.Builder
	for _, sp := range RichSpans(s) {
		b.WriteString(sp.Text)
	}
	return b.String()
}

// TruncSpans keeps the first n runes of the spans' text, adding "…" when it cut.
func TruncSpans(sp []Span, n int) []Span {
	var out []Span
	for _, s := range sp {
		r := utf8.RuneCountInString(s.Text)
		if r <= n {
			out = append(out, s)
			n -= r
			continue
		}
		cut := s
		cut.Text = string([]rune(s.Text)[:max(n-1, 0)]) + "…"
		return append(out, cut)
	}
	return out
}
