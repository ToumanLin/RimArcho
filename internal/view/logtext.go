package view

import (
	"fmt"
	"math"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Log text in RimWorld's own debug-log colours (06 §2.10, §7): a line's level colours it
// as the game's log window does (Verse.LogMessage.Color), and a mod's <color>, <b> and <i>
// (Unity rich text) style parts of it. Both interfaces use the same spans: the GUI as CSS
// colours, the TUI mapped to its terminal colours.

// Log levels of a line.
const (
	LevelError   = "error"
	LevelWarning = "warning"
	LevelInfo    = "info"
)

// GameLogColors are the game's colours (LudeonTK.EditWindow_Log draws message.Color):
// Error ColorLibrary.LogError (1, .4, .4), Warning Color.yellow (1, .922, .016), Message
// Color.white.
var GameLogColors = map[string]string{LevelError: "#ff6666", LevelWarning: "#ffeb04", LevelInfo: "#ffffff"}

// The card backgrounds the text sits on (style.css --card).
const (
	darkCard  = "#232327"
	lightCard = "#ffffff"
	// minContrast is the WCAG contrast a coloured line keeps against its card.
	minContrast = 4.5
)

// LightLogColors are GameLogColors for the light theme: the same hue, darkened until it
// reads on white (info uses the theme's text colour instead). style.css carries them.
var LightLogColors = map[string]string{
	LevelError:   ForBackground(GameLogColors[LevelError], lightCard),
	LevelWarning: ForBackground(GameLogColors[LevelWarning], lightCard),
}

// TextSpan is a styled run of a log line: C its colour on the dark theme, L on the light
// one ("" for the line's level colour).
type TextSpan struct {
	T string `json:"t"`
	C string `json:"c,omitempty"`
	L string `json:"l,omitempty"`
	B bool   `json:"b,omitempty"`
	I bool   `json:"i,omitempty"`
}

// LogSpans is a message's first line as spans, at most max runes of text.
func LogSpans(msg string, max int) []TextSpan {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.TrimSpace(strings.TrimRight(msg, "\r"))
	var out []TextSpan
	for _, s := range logs.TruncSpans(logs.RichSpans(msg), max) {
		t := TextSpan{T: s.Text, B: s.Bold, I: s.Italic}
		if s.Color != "" {
			t.C, t.L = ForBackground(s.Color, darkCard), ForBackground(s.Color, lightCard)
		}
		out = append(out, t)
	}
	return out
}

// SpanText is the spans' plain text.
func SpanText(sp []TextSpan) string {
	var b strings.Builder
	for _, s := range sp {
		b.WriteString(s.T)
	}
	return b.String()
}

// LevelOf names a line's level: the log's own level when it has one (Waylith, the
// companion log), else from the row's status.
func LevelOf(level string, status int) string {
	switch strings.ToUpper(level) {
	case "ERROR", "FATAL", "CRITICAL", "EXCEPTION":
		return LevelError
	case "WARN", "WARNING":
		return LevelWarning
	case "INFO", "DEBUG", "TRACE", "MESSAGE":
		return LevelInfo
	}
	switch status {
	case StatusErr:
		return LevelError
	case StatusWarn:
		return LevelWarning
	}
	return LevelInfo
}

// ForBackground keeps a colour's hue and saturation and moves its lightness until it has
// minContrast against bg (the nearest lightness that does, towards white on a dark
// background and towards black on a light one).
func ForBackground(c, bg string) string {
	r, g, b, ok := hexRGB(c)
	br, bgc, bb, ok2 := hexRGB(bg)
	if !ok || !ok2 {
		return c
	}
	bgL := luminance(br, bgc, bb)
	if contrast(luminance(r, g, b), bgL) >= minContrast {
		return c
	}
	h, s, l := rgbToHSL(r, g, b)
	step := 0.01
	if bgL > 0.5 {
		step = -0.01
	}
	for l = l + step; l >= 0 && l <= 1; l += step {
		r2, g2, b2 := hslToRGB(h, s, l)
		if contrast(luminance(r2, g2, b2), bgL) >= minContrast {
			return rgbHex(r2, g2, b2)
		}
	}
	if step < 0 {
		return "#000000"
	}
	return "#ffffff"
}

func hexRGB(c string) (r, g, b float64, ok bool) {
	var ri, gi, bi int
	if len(c) != 7 || c[0] != '#' {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(c[1:], "%02x%02x%02x", &ri, &gi, &bi); err != nil {
		return 0, 0, 0, false
	}
	return float64(ri) / 255, float64(gi) / 255, float64(bi) / 255, true
}

func rgbHex(r, g, b float64) string {
	c := func(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", c(r), c(g), c(b))
}

func luminance(r, g, b float64) float64 {
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return hue(h + 1.0/3), hue(h), hue(h - 1.0/3)
}
