package view

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/textenc"
)

// SearchExpression uses Go's bounded RE2 implementation for both the search and
// viewer highlights; page JavaScript never executes a user-supplied regex.
func SearchExpression(q FileSearchReq) (*regexp.Regexp, error) {
	pattern := q.Pattern
	if !q.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if !q.Case {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, errors.New(T("not a regular expression") + ": " + err.Error())
	}
	return re, nil
}

func searchInside(root, path string, allowRoot bool) bool {
	if root == "" || path == "" {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, realPath)
	return err == nil && (allowRoot || rel != ".") && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type SearchTextPart struct {
	Text string `json:"text"`
	Hit  bool   `json:"hit,omitempty"`
}
type SearchTextLine struct {
	Number int              `json:"number"`
	Parts  []SearchTextPart `json:"parts"`
}
type SearchFileText struct {
	Name   string           `json:"name"`
	Path   string           `json:"path"`
	Lines  []SearchTextLine `json:"lines"`
	Hits   []int            `json:"hits"`
	Type   string           `json:"type"`
	Source bool             `json:"source"`
	Binary bool             `json:"binary"`
	Cut    bool             `json:"cut"`
	Note   string           `json:"note,omitempty"`
}

// ReadSearchFile shares SearchFolder's resolved-root check. Caps cover bytes,
// displayed lines, and highlight fragments, including maliciously dense files.
func ReadSearchFile(ix *app.Index, libraryRoot, id, path string, q FileSearchReq) (*SearchFileText, error) {
	if _, err := SearchFolder(ix, libraryRoot, id, path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New(T("the file is not there"))
	}
	out := &SearchFileText{Name: filepath.Base(path), Path: path, Lines: []SearchTextLine{}, Hits: []int{}, Source: searchInside(libraryRoot, path, false)}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".xml":
		out.Type = "UTF-8 · XML"
	case ".cs":
		out.Type = "UTF-8 · C#"
	case ".txt", ".json", ".md", ".yml", ".yaml", ".log":
		out.Type = "UTF-8"
	default:
		out.Binary = true
		return out, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, FileSearchViewCap+1))
	if err != nil {
		return nil, err
	}
	if len(data) > FileSearchViewCap {
		data = data[:FileSearchViewCap]
		out.Cut = true
	}
	data = textenc.UTF8(data)
	if bytes.IndexByte(data, 0) >= 0 {
		out.Binary = true
		return out, nil
	}
	if out.Cut {
		for n := 0; n < 3 && len(data) > 0 && !utf8.Valid(data); n++ {
			data = data[:len(data)-1]
		}
	}
	if !utf8.Valid(data) {
		out.Binary = true
		return out, nil
	}
	re, err := SearchExpression(q)
	if err != nil {
		return nil, err
	}
	fragments := 0
	for n, line := range strings.Split(string(data), "\n") {
		if n >= 10000 || fragments >= FileSearchLineCap {
			out.Cut = true
			break
		}
		line = strings.TrimSuffix(line, "\r")
		row := SearchTextLine{Number: n + 1, Parts: []SearchTextPart{}}
		at := 0
		// Bound regex output as well as input; only positive-width matches are drawn.
		for _, span := range re.FindAllStringIndex(line, FileSearchLineCap-fragments+1) {
			if span[0] == span[1] {
				continue
			}
			if fragments >= FileSearchLineCap {
				out.Cut = true
				break
			}
			if span[0] > at {
				row.Parts = append(row.Parts, SearchTextPart{Text: line[at:span[0]]})
			}
			row.Parts = append(row.Parts, SearchTextPart{Text: line[span[0]:span[1]], Hit: true})
			at = span[1]
			fragments++
		}
		if fragments >= FileSearchLineCap && at < len(line) {
			out.Cut = true
			line = line[:at]
		}
		if at < len(line) || len(row.Parts) == 0 {
			row.Parts = append(row.Parts, SearchTextPart{Text: line[at:]})
		}
		for _, part := range row.Parts {
			if part.Hit {
				out.Hits = append(out.Hits, n+1)
				break
			}
		}
		out.Lines = append(out.Lines, row)
	}
	if out.Cut {
		out.Note = T("file cut at the viewer limit (1 MiB, 10,000 lines or 5,000 highlights); open the file to read more")
	}
	return out, nil
}

func searchLineParts(text string, re *regexp.Regexp) []SearchTextPart {
	var parts []SearchTextPart
	at := 0
	for _, span := range re.FindAllStringIndex(text, 160) {
		if span[0] == span[1] {
			continue
		}
		if span[0] > at {
			parts = append(parts, SearchTextPart{Text: text[at:span[0]]})
		}
		parts = append(parts, SearchTextPart{Text: text[span[0]:span[1]], Hit: true})
		at = span[1]
	}
	if at < len(text) || len(parts) == 0 {
		parts = append(parts, SearchTextPart{Text: text[at:]})
	}
	return parts
}
