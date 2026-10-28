package modsearch

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/textenc"
)

// NameMatch is a top-level Def with a Name attribute, available after enabling Mod.
type NameMatch struct {
	Mod  Mod
	Name string
}

var errNameBudget = errors.New("installed provider lookup limit reached")

// FindNames checks effective Defs only. Limits bound directory entries, XML files,
// bytes per file and total bytes; the caller supplies a wall-clock deadline.
// It returns partial results when a limit or unreadable file prevents a full search.
func FindNames(ctx context.Context, mods []Mod, o Options, names map[string]bool) ([]NameMatch, bool, error) {
	budget := &nameFS{ctx: ctx, entries: 40000, files: 8000, bytes: 128 << 20}
	o.FS, o.Kinds = budget, Defs
	o.AllVersions = false
	var matches []NameMatch
	var keys []string
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	incomplete := len(mods) > 1000
	if incomplete {
		mods = mods[:1000]
	}
	for _, mod := range mods {
		if err := ctx.Err(); err != nil {
			return matches, true, err
		}
		active := o.Active
		o.Active = func(pkg string) bool {
			return strings.EqualFold(strings.TrimSuffix(pkg, "_steam"), strings.TrimSuffix(mod.PackageID, "_steam")) || active != nil && active(pkg)
		}
		listing := listFiles(ctx, mod, o)
		o.Active = active
		if listing.unreadable > 0 {
			incomplete = true
		}
		if listing.err != nil {
			return matches, true, listing.err
		}
		seen := map[string]bool{}
		for _, file := range listing.files {
			data, err := budget.ReadFile(file.path)
			if err != nil {
				if ctx.Err() != nil {
					return matches, true, ctx.Err()
				}
				if errors.Is(err, errNameBudget) {
					return matches, true, nil
				}
				incomplete = true
				continue
			}
			dec := xml.NewDecoder(bytes.NewReader(textenc.UTF8(data)))
			dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
			depth := 0
			// Keep a file's matches only if its XML is well formed.
			found := map[string]bool{}
			for {
				if ctx.Err() != nil {
					return matches, true, ctx.Err()
				}
				tok, err := dec.Token()
				if err == io.EOF {
					for _, name := range keys {
						if found[name] && !seen[name] {
							seen[name] = true
							matches = append(matches, NameMatch{mod, name})
						}
					}
					break
				}
				if err != nil {
					incomplete = true
					break
				}
				switch t := tok.(type) {
				case xml.StartElement:
					depth++
					if depth != 2 {
						continue
					}
					d := analyze.DefFact{}
					for _, attr := range t.Attr {
						switch attr.Name.Local {
						case "Name":
							d.Name = attr.Value
						case "MayRequire":
							d.MayRequire = attr.Value
						case "MayRequireAnyOf":
							d.MayRequireAny = attr.Value
						}
					}
					if names[d.Name] && d.Loaded(func(pkg string) bool {
						return strings.EqualFold(strings.TrimSuffix(pkg, "_steam"), strings.TrimSuffix(mod.PackageID, "_steam")) || active != nil && active(pkg)
					}) {
						found[d.Name] = true
					}
				case xml.EndElement:
					depth--
				}
			}
		}
		if budget.exhausted {
			return matches, true, nil
		}
	}
	return matches, incomplete || budget.skipped, nil
}

// The filesystem cap applies to listings too, before listFiles retains a tree.
// Symlinked entries are skipped rather than following a directory cycle.
type nameFS struct {
	ctx            context.Context
	entries, files int
	bytes          int64
	exhausted      bool
	skipped        bool
}

func (b *nameFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if b.entries <= 0 {
		b.exhausted = true
		return nil, errNameBudget
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(b.entries + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > b.entries {
		entries = entries[:b.entries]
		b.exhausted = true
	}
	b.entries -= len(entries)
	out := entries[:0]
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			out = append(out, e)
		} else {
			b.skipped = true
		}
	}
	return out, nil
}
func (b *nameFS) ReadFile(path string) ([]byte, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if b.files <= 0 || b.bytes <= 0 {
		b.exhausted = true
		return nil, errNameBudget
	}
	b.files--
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	limit := int64(8 << 20)
	if b.bytes < limit {
		limit = b.bytes
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	b.bytes -= int64(len(data))
	if int64(len(data)) > limit {
		b.exhausted = true
		return nil, errNameBudget
	}
	return data, err
}
