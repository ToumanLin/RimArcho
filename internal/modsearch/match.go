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

func (m *matcher) search(ctx context.Context, j fileJob, o Options, scratch *[]byte) fileHits {
	binary := j.kind&(Textures|Sounds|Assemblies) != 0
	var b []byte
	if binary {
		b = []byte(j.asset)
	} else {
		var err error
		b, err = o.FS.ReadFile(j.path)
		if err != nil {
			return fileHits{err: err}
		}
		b = textenc.UTF8(b)
	}
	found := fileHits{}
	var offsets, keptMeta []int
	var allHits []Match
	var allOffsets []int
	lastLine, line, lineStart, cursor := -1, 1, 0, 0
	add := func(start int) {
		// Line numbers advance only across the gaps between hits; no per-line scan on misses.
		gap := b[cursor:start]
		line += bytes.Count(gap, []byte{'\n'})
		if n := bytes.LastIndexByte(gap, '\n'); n >= 0 {
			lineStart = cursor + n + 1
		}
		cursor = start
		if o.Def {
			allHits = append(allHits, Match{Line: line, Col: start - lineStart + 1})
			allOffsets = append(allOffsets, start)
		}
		if line == lastLine {
			return
		}
		lastLine = line
		found.count++
		if len(found.matches) >= o.PerFile {
			return
		}
		if o.Def {
			keptMeta = append(keptMeta, len(allHits)-1)
		}
		end := bytes.IndexByte(b[start:], '\n')
		if end < 0 {
			end = len(b)
		} else {
			end += start
		}
		hit := Match{Line: line, Col: start - lineStart + 1, Text: snippet(string(b[lineStart:end]), 160)}
		if binary {
			hit.Line = 0
		}
		found.matches = append(found.matches, hit)
		offsets = append(offsets, start)
	}
	if m.re != nil {
		if len(m.prefix) > 0 && !bytes.Contains(b, m.prefix) {
			return found
		}
		for _, hit := range m.re.FindAllIndex(b, -1) {
			if ctx.Err() != nil {
				return fileHits{err: ctx.Err()}
			}
			if !m.word || boundary(b, hit[0], hit[1]) {
				add(hit[0])
			}
		}
	} else {
		haystack := b
		if m.fold {
			*scratch = foldASCII(*scratch, b)
			haystack = *scratch
		}
		for at := 0; at <= len(haystack); {
			if ctx.Err() != nil {
				return fileHits{err: ctx.Err()}
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
			add(start)
			if o.Def {
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
	}
	if len(found.matches) > 0 {
		if !binary && (j.kind == Defs || j.kind == Patches) {
			if o.Def {
				found.summary = contexts(ctx, b, j.kind, o, allHits, allOffsets)
				for i, index := range keptMeta {
					found.matches[i].Role, found.matches[i].Within = allHits[index].Role, allHits[index].Within
				}
			} else {
				found.offsets = offsets // contexts only for the shown page, after paging
			}
		} else if o.Def {
			role := "ref"
			if j.kind == Languages {
				role = "translation"
			}
			found.summary.Roles = map[string]int{role: found.count}
			for i := range found.matches {
				found.matches[i].Role = role
			}
		}
	}
	return found
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
