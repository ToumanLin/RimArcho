package modsearch

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/textenc"
)

type matcher struct {
	pattern, prefix []byte
	re              *regexp.Regexp
	fold, word      bool
}

func newMatcher(o Options) (*matcher, error) {
	if o.Pattern == "" {
		return nil, fmt.Errorf("search pattern is empty; provide a literal or regular expression")
	}
	m := &matcher{word: o.Word || o.Def, fold: !o.CaseSensitive && !o.Def}
	if o.Regex && !o.Def {
		pattern := o.Pattern
		if !o.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid search regex %q: %w", o.Pattern, err)
		}
		m.re = re
		// LiteralPrefix is case-sensitive; folded regexes generally have no fixed prefix.
		if prefix, _ := re.LiteralPrefix(); len(prefix) >= 3 {
			m.prefix = []byte(prefix)
		}
	} else {
		m.pattern = []byte(o.Pattern)
		if m.fold {
			m.pattern = foldASCII(nil, m.pattern)
		}
	}
	return m, nil
}

func foldASCII(dst, src []byte) []byte {
	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	} else {
		dst = dst[:len(src)]
	}
	for i, c := range src {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst[i] = c
	}
	return dst
}
func ident(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
func boundary(b []byte, start, end int) bool {
	return (start == 0 || !ident(b[start-1])) && (end == len(b) || !ident(b[end]))
}

// hitCollector records one file's hits, advancing line numbers only between hits.
type hitCollector struct {
	b                                 []byte
	def, lineMode                     bool
	perFile                           int
	binary                            bool
	found                             fileHits
	offsets, keptMeta                 []int
	allHits                           []Match
	allOffsets                        []int
	lastLine, line, lineStart, cursor int
}

func (m *matcher) search(ctx context.Context, j fileJob, o Options, scratch *[]byte) fileHits {
	b, binary, err := jobBytes(&j, &o)
	if err != nil {
		return fileHits{err: err}
	}
	c := hitCollector{b: b, def: o.Def, lineMode: o.LineMode, perFile: o.PerFile, binary: binary, lastLine: -1, line: 1}
	if m.re != nil {
		if len(m.prefix) > 0 && !bytes.Contains(b, m.prefix) {
			return c.found
		}
		if o.LineMode {
			err = m.regexLines(ctx, &c)
		} else {
			err = m.regexAll(ctx, &c)
		}
	} else {
		err = m.literal(ctx, &c, scratch)
	}
	if err != nil {
		return fileHits{err: err}
	}
	if len(c.found.matches) > 0 {
		c.annotate(ctx, j.kind, &o)
	}
	return c.found
}

// jobBytes returns the text searched for a job: the asset path for binary kinds, else the file as UTF-8.
func jobBytes(j *fileJob, o *Options) (b []byte, binary bool, err error) {
	binary = j.kind&(Textures|Sounds|Assemblies) != 0
	if binary {
		return []byte(j.asset), true, nil
	}
	b, err = o.FS.ReadFile(j.path)
	if err != nil {
		return nil, false, err
	}
	return textenc.UTF8(b), false, nil
}

// add records a hit at start, counting one per line and keeping up to PerFile matches.
func (c *hitCollector) add(start int) {
	b := c.b
	// Line numbers advance only across the gaps between hits; no per-line scan on misses.
	gap := b[c.cursor:start]
	c.line += bytes.Count(gap, []byte{'\n'})
	if n := bytes.LastIndexByte(gap, '\n'); n >= 0 {
		c.lineStart = c.cursor + n + 1
	}
	c.cursor = start
	if c.def {
		c.allHits = append(c.allHits, Match{Line: c.line, Col: start - c.lineStart + 1})
		c.allOffsets = append(c.allOffsets, start)
	}
	if c.line == c.lastLine {
		return
	}
	c.lastLine = c.line
	c.found.count++
	if len(c.found.matches) >= c.perFile {
		return
	}
	if c.def {
		c.keptMeta = append(c.keptMeta, len(c.allHits)-1)
	}
	end := bytes.IndexByte(b[start:], '\n')
	if end < 0 {
		end = len(b)
	} else {
		end += start
	}
	hit := Match{Line: c.line, Col: start - c.lineStart + 1, Text: snippet(string(b[c.lineStart:end]), 160)}
	if c.lineMode {
		hit.Text = matchSnippet(string(b[c.lineStart:end]), start-c.lineStart, 160)
	}
	if c.binary {
		hit.Line = 0
	}
	c.found.matches = append(c.found.matches, hit)
	c.offsets = append(c.offsets, start)
}

// regexLines adds the first regex hit of each line until PerFile matches are kept.
func (m *matcher) regexLines(ctx context.Context, c *hitCollector) error {
	b := c.b
	for start := 0; start < len(b); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := bytes.IndexByte(b[start:], '\n')
		if end < 0 {
			end = len(b)
		} else {
			end += start
		}
		if hit := m.re.FindIndex(b[start:end]); hit != nil {
			c.add(start + hit[0])
		}
		if len(c.found.matches) >= c.perFile {
			break
		}
		start = end + 1
	}
	return nil
}

// regexAll adds every regex hit in the file, honouring whole-word matching.
func (m *matcher) regexAll(ctx context.Context, c *hitCollector) error {
	for _, hit := range m.re.FindAllIndex(c.b, -1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !m.word || boundary(c.b, hit[0], hit[1]) {
			c.add(hit[0])
		}
	}
	return nil
}

// literal adds the hits of a literal pattern, case-folding into scratch when needed.
func (m *matcher) literal(ctx context.Context, c *hitCollector, scratch *[]byte) error {
	b := c.b
	haystack := b
	if m.fold {
		*scratch = foldASCII(*scratch, b)
		haystack = *scratch
	}
	for at := 0; at <= len(haystack); {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := bytes.Index(haystack[at:], m.pattern)
		if n < 0 {
			break
		}
		start := at + n
		end := start + len(m.pattern)
		if m.word && !boundary(b, start, end) {
			at = start + 1
			continue
		}
		c.add(start)
		if c.lineMode && len(c.found.matches) >= c.perFile {
			break
		}
		if c.def {
			at = end
			continue
		}
		// Like rg, all later occurrences starting on this line count as the same hit.
		n = bytes.IndexByte(b[start:], '\n')
		if n < 0 {
			break
		}
		at = start + n + 1
	}
	return nil
}

// annotate gives the kept matches their roles and contexts, or keeps offsets for later paging.
func (c *hitCollector) annotate(ctx context.Context, kind Kind, o *Options) {
	found := &c.found
	if !c.binary && (kind == Defs || kind == Patches) {
		if o.Def {
			found.summary = contexts(ctx, c.b, kind, *o, c.allHits, c.allOffsets)
			for i, index := range c.keptMeta {
				found.matches[i].Role, found.matches[i].Within = c.allHits[index].Role, c.allHits[index].Within
			}
		} else {
			found.offsets = c.offsets // contexts only for the shown page, after paging
		}
	} else if o.Def {
		role := "ref"
		if kind == Languages {
			role = "translation"
		}
		found.summary.Roles = map[string]int{role: found.count}
		for i := range found.matches {
			found.matches[i].Role = role
		}
	}
}

func snippet(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	end := limit - 3
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "..."
}

// UI snippets keep the matching part visible even on long/minified XML lines.
// The CLI continues to use its established leading-line snippet.
func matchSnippet(s string, at, limit int) string {
	if len(s) <= limit {
		return strings.TrimSpace(s)
	}
	start := max(0, at-limit/3)
	for start > 0 && !utf8.RuneStart(s[start]) {
		start--
	}
	end := min(len(s), start+limit-6)
	for end > start && end < len(s) && !utf8.RuneStart(s[end]) {
		end--
	}
	text := s[start:end]
	if start > 0 {
		text = "..." + text
	}
	if end < len(s) {
		text += "..."
	}
	return strings.TrimSpace(text)
}
