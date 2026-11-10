package app

import (
	"bytes"
	"os"
	"regexp"
	"strings"
)

// searchWorkers is how many files a source search reads at once. A decompiled game
// holds thousands of small files: opening them is most of the time, so it overlaps.
const searchWorkers = 8

// fileMatches are one file's matching lines; hits holds at most the search limit.
type fileMatches struct {
	total int
	hits  []lineHit
}

type lineHit struct {
	line int
	text string
}

// lineMatcher matches lines of decompiled C#: a case-insensitive literal, or a regexp.
type lineMatcher struct {
	re    *regexp.Regexp
	lower []byte // the literal, lower-cased
	ascii bool   // the literal is ASCII: matched without copying the file
	lit   string
}

func newLineMatcher(pattern string, re *regexp.Regexp) *lineMatcher {
	m := &lineMatcher{re: re, lit: strings.ToLower(pattern)}
	m.lower = []byte(m.lit)
	m.ascii = true
	for _, c := range m.lower {
		if c >= 0x80 {
			m.ascii = false
		}
	}
	return m
}

// file reads one file and lists its matching lines (limit 0: all of them).
func (m *lineMatcher) file(path string, limit int) fileMatches {
	b, err := os.ReadFile(path)
	if err != nil {
		return fileMatches{}
	}
	if m.re == nil {
		// Most files have no match: look for it in the whole file before splitting lines.
		if m.ascii {
			if !bytesContainsFold(b, m.lower) {
				return fileMatches{}
			}
		} else if !bytes.Contains(bytes.ToLower(b), m.lower) {
			return fileMatches{}
		}
	}
	var out fileMatches
	for n, rest := 1, b; len(rest) > 0; n++ {
		var l []byte
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			l, rest = rest[:i], rest[i+1:]
		} else {
			l, rest = rest, nil
		}
		l = bytes.TrimSuffix(l, []byte{'\r'})
		var ok bool
		switch {
		case m.re != nil:
			ok = m.re.Match(l)
		case m.ascii:
			ok = bytesContainsFold(l, m.lower)
		default:
			ok = strings.Contains(strings.ToLower(string(l)), m.lit)
		}
		if !ok {
			continue
		}
		out.total++
		if limit == 0 || len(out.hits) < limit {
			out.hits = append(out.hits, lineHit{line: n, text: strings.TrimSpace(string(l))})
		}
	}
	return out
}

// bytesContainsFold reports whether b holds lower (an ASCII, lower-case pattern) in any case,
// without copying b.
func bytesContainsFold(b, lower []byte) bool {
	n := len(lower)
	if n == 0 {
		return true
	}
	first := lower[0]
	up := first
	if 'a' <= first && first <= 'z' {
		up = first - 'a' + 'A'
	}
	for i := 0; i+n <= len(b); i++ {
		if c := b[i]; c != first && c != up {
			continue
		}
		j := 1
		for ; j < n; j++ {
			c := b[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lower[j] {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
}
