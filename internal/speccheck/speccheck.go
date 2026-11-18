// Package speccheck reads the specifications (docs/specs, docs/00a-architecture.md and
// AGENTS.md) and the Go sources so that tests can keep the two in agreement. The checks
// themselves are TestSpec… tests: the general ones in this package's tests, the ones that
// need a package's internals in that package's tests. Every failure names the specification
// section to read; exceptions are maps with a reason per entry, and an entry that no longer
// matches anything fails as stale (Exceptions).
//
// Run every spec check with:
//
//	go test -count=1 -run TestSpec ./...
package speccheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Root is the repository root (this file is internal/speccheck/speccheck.go).
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// Line is one line of a document, with its position.
type Line struct {
	Doc  string // slash path relative to the root
	N    int    // 1-based
	Text string
}

// Pos is "doc:line", for failure messages.
func (l Line) Pos() string { return fmt.Sprintf("%s:%d", l.Doc, l.N) }

// Doc is one Markdown file.
type Doc struct {
	Path  string // slash path relative to the root
	Lines []Line
}

var (
	docsOnce sync.Once
	docsAll  []*Doc
	docsErr  error
)

// Read reads one document by its slash path relative to the root.
func Read(rel string) (*Doc, error) {
	b, err := os.ReadFile(filepath.Join(Root(), filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	d := &Doc{Path: rel}
	for i, l := range strings.Split(text, "\n") {
		d.Lines = append(d.Lines, Line{Doc: rel, N: i + 1, Text: l})
	}
	return d, nil
}

// Docs returns every specification file (docs/specs/**.md), docs/00a-architecture.md and
// AGENTS.md, in path order.
func Docs() ([]*Doc, error) {
	docsOnce.Do(func() {
		var paths []string
		err := filepath.WalkDir(filepath.Join(Root(), "docs", "specs"), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
				return err
			}
			rel, _ := filepath.Rel(Root(), p)
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			docsErr = err
			return
		}
		sort.Strings(paths)
		paths = append(paths, "docs/00a-architecture.md", "AGENTS.md")
		for _, p := range paths {
			d, err := Read(p)
			if err != nil {
				docsErr = err
				return
			}
			docsAll = append(docsAll, d)
		}
	})
	return docsAll, docsErr
}

// SpecDocs returns the files of specification nn ("05", "16"): one file, or every file of a
// split specification's directory.
func SpecDocs(nn string) ([]*Doc, error) {
	all, err := Docs()
	if err != nil {
		return nil, err
	}
	var out []*Doc
	for _, d := range all {
		if strings.HasPrefix(d.Path, "docs/specs/"+nn+"-") {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("specification %s not found", nn)
	}
	return out, nil
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(\d+(?:\.\d+)*)\.?\s`)

// Section returns the lines of section num ("4.2") of specification nn, without its
// heading: up to the next heading of the same or a higher level. Section numbers run
// through a split specification's files (specs/README "References").
func Section(nn, num string) ([]Line, error) {
	docs, err := SpecDocs(nn)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		for i, l := range d.Lines {
			m := headingRe.FindStringSubmatch(l.Text)
			if m == nil || m[2] != num {
				continue
			}
			level := len(m[1])
			var out []Line
			for _, n := range d.Lines[i+1:] {
				if h := regexp.MustCompile(`^(#{1,6})\s`).FindStringSubmatch(n.Text); h != nil && len(h[1]) <= level {
					break
				}
				out = append(out, n)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s §%s not found", nn, num)
}

// Row is one table row: its cells (trimmed, with \| unescaped) and its line.
type Row struct {
	Cells []string
	Line  Line
}

// Tables returns the Markdown tables in lines, each as its header row followed by its body
// rows (the --- separator is dropped).
func Tables(lines []Line) [][]Row {
	var out [][]Row
	var cur []Row
	for _, l := range lines {
		t := strings.TrimSpace(l.Text)
		if !strings.HasPrefix(t, "|") {
			if cur != nil {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cells := SplitRow(t)
		if len(cells) > 0 && strings.HasPrefix(strings.TrimSpace(cells[0]), "---") {
			continue
		}
		cur = append(cur, Row{Cells: cells, Line: l})
	}
	if cur != nil {
		out = append(out, cur)
	}
	return out
}

// SplitRow splits a table row on unescaped pipes; \| becomes |.
func SplitRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	if strings.HasSuffix(row, "|") && !strings.HasSuffix(row, `\|`) {
		row = row[:len(row)-1]
	}
	var cells []string
	var b strings.Builder
	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			b.WriteByte('|')
			i++
		case row[i] == '|':
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(row[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

// Column is the index of the header cell that starts with name, or -1.
func Column(header Row, name string) int {
	for i, c := range header.Cells {
		if strings.HasPrefix(c, name) {
			return i
		}
	}
	return -1
}

var spanRe = regexp.MustCompile("`([^`]+)`")

// Spans returns the inline code spans of s, without their backticks.
func Spans(s string) []string {
	var out []string
	for _, m := range spanRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// Exceptions is an allowlist with a reason per entry. Has marks an entry used; Stale lists
// the entries nothing needed, which must fail so the list cannot go stale.
type Exceptions struct {
	name string
	m    map[string]string
	mu   sync.Mutex
	used map[string]bool
}

// NewExceptions wraps m; name is where the map is declared, for the stale message.
func NewExceptions(name string, m map[string]string) *Exceptions {
	return &Exceptions{name: name, m: m, used: map[string]bool{}}
}

// Has reports whether key is an exception and marks it used.
func (e *Exceptions) Has(key string) bool {
	if _, ok := e.m[key]; !ok {
		return false
	}
	e.mu.Lock()
	e.used[key] = true
	e.mu.Unlock()
	return true
}

// Stale returns one message per entry that Has never matched.
func (e *Exceptions) Stale() []string {
	var out []string
	for k := range e.m {
		if !e.used[k] {
			out = append(out, fmt.Sprintf("stale exception %q in %s: nothing needs it any more; remove it", k, e.name))
		}
	}
	slices.Sort(out)
	return out
}
