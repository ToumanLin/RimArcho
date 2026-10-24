// Package fsscan lists directory trees and caches results by listing fingerprint.
package fsscan

import (
	"context"
	"encoding/binary"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/zeebo/xxh3"
)

// Entry describes a file or subdirectory below the root.
type Entry struct {
	Rel   string // Path relative to the root, with forward slashes and original case.
	Size  int64  // Size in bytes.
	MTime int64  // Modification time in Unix nanoseconds.
	Dir   bool   // Whether this entry is a directory.
}

// Options controls traversal and which entries are returned.
type Options struct {
	Workers int // Directories listed concurrently; <= 0 uses Workers(root).
	// SkipDir returns true to avoid descending into a subdirectory.
	// It receives the relative path and base name. Calls may overlap.
	SkipDir func(rel, name string) bool
	// Keep selects regular files by relative path and base name. Calls may overlap.
	Keep   func(rel, name string) bool
	Dirs   bool // Include subdirectories, including those excluded by SkipDir.
	Sorted bool // Order entries by lower-cased Rel, then original Rel.
}

// Tree holds the selected entries and totals from a walk.
type Tree struct {
	Root    string  // Absolute root path, with a root link resolved to its target.
	Entries []Entry // Selected entries; the root itself is excluded.
	Files   int     // Number of selected regular files.
	Bytes   int64   // Total size of selected regular files.
	Skipped int     // Subdirectories that could not be opened or fully listed.
}

// Walk lists root without following links inside it. Unreadable subdirectories
// are skipped; an unreadable root is an error. Files with unreadable metadata
// are omitted. Cancellation returns ctx.Err() after the workers have stopped.
func Walk(ctx context.Context, root string, o Options) (*Tree, error) {
	return walk(ctx, root, o, true)
}

// Size counts regular files and bytes without retaining entries.
func Size(ctx context.Context, root string, workers int) (files int, bytes int64, err error) {
	t, err := walk(ctx, root, Options{Workers: workers}, false)
	if err != nil {
		return 0, 0, err
	}
	return t.Files, t.Bytes, nil
}

type directory struct {
	path string
	rel  string
}

type walkResult struct {
	entries []Entry
	files   int
	bytes   int64
	skipped int
}

func walk(ctx context.Context, root string, o Options, retain bool) (*Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// EvalSymlinks also resolves Windows directory junctions.
	root, err = filepath.EvalSymlinks(root)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	n := o.Workers
	if n <= 0 {
		n = Workers(root)
	}
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	rootDir := directory{path: dirPath(root)}
	queue := []directory{rootDir}
	head := 0
	pending := 1 // Queued plus in-flight directories, protected by mu.
	var rootErr error

	woke := make(chan struct{})
	stopWake := context.AfterFunc(ctx, func() {
		mu.Lock()
		cond.Broadcast()
		mu.Unlock()
		close(woke)
	})
	results := make([]walkResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Go(func() {
			var result walkResult
			var children []directory
			mu.Lock()
			for {
				for head == len(queue) && pending != 0 && rootErr == nil && ctx.Err() == nil {
					cond.Wait()
				}
				if pending == 0 || rootErr != nil || ctx.Err() != nil {
					mu.Unlock()
					results[i] = result
					return
				}
				var dir directory
				if retain {
					dir = queue[len(queue)-1]
					queue[len(queue)-1] = directory{}
					queue = queue[:len(queue)-1]
				} else {
					// Breadth-first counting keeps pending paths live, reducing GC
					// churn of stdlib's pooled directory buffers on small heaps.
					dir = queue[head]
					queue[head] = directory{}
					head++
					if head == len(queue) {
						queue, head = queue[:0], 0
					} else if head >= 4096 && head >= len(queue)/2 {
						remaining := copy(queue, queue[head:])
						clear(queue[remaining:])
						queue, head = queue[:remaining], 0
					}
				}
				mu.Unlock()

				var err error
				children, err = readDirectory(ctx, dir, o, retain, &result, children[:0])
				if err != nil && dir.path != rootDir.path {
					result.skipped++
				}

				mu.Lock()
				if err != nil && dir.path == rootDir.path {
					rootErr = err
				}
				queue = append(queue, children...)
				pending += len(children) - 1
				if len(children) > 0 || pending == 0 || rootErr != nil {
					cond.Broadcast()
				}
				clear(children)
				// Keep the lock for the next pop, combining completion and dequeue.
			}
		})
	}
	wg.Wait()
	if !stopWake() {
		<-woke // Join a cancellation callback that has already started.
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rootErr != nil {
		return nil, rootErr
	}

	t := &Tree{Root: root}
	entries := 0
	for _, result := range results {
		entries += len(result.entries)
		t.Files += result.files
		t.Bytes += result.bytes
		t.Skipped += result.skipped
	}
	if n == 1 {
		t.Entries = results[0].entries
	} else if entries > 0 {
		t.Entries = make([]Entry, 0, entries)
		for _, result := range results {
			t.Entries = append(t.Entries, result.entries...)
		}
	}
	if o.Sorted {
		slices.SortFunc(t.Entries, compareEntries)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

func readDirectory(ctx context.Context, dir directory, o Options, retain bool, result *walkResult, children []directory) ([]directory, error) {
	if err := ctx.Err(); err != nil {
		return children, err
	}
	f, err := openDir(dir.path)
	if err != nil {
		return children, err
	}
	entries, readErr := f.ReadDir(-1)
	f.Close()
	for _, d := range entries {
		if ctx.Err() != nil {
			break
		}
		typ := d.Type()
		if typ != 0 && typ != fs.ModeDir {
			continue
		}
		name := d.Name()
		var child string
		if (typ == fs.ModeDir && (retain || o.SkipDir != nil)) || o.Keep != nil {
			child = name
			if dir.rel != "" {
				child = dir.rel + "/" + name
			}
		}
		if typ == fs.ModeDir {
			if o.SkipDir == nil || !o.SkipDir(child, name) {
				var path string
				if dir.path[len(dir.path)-1] == filepath.Separator {
					path = dir.path + name
				} else {
					path = dir.path + string(filepath.Separator) + name
				}
				children = append(children, directory{path: path, rel: child})
			}
			if retain && o.Dirs {
				if info, err := d.Info(); err == nil {
					result.entries = append(result.entries, Entry{child, info.Size(), info.ModTime().UnixNano(), true})
				}
			}
			continue
		}
		if o.Keep != nil && !o.Keep(child, name) {
			continue
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		result.files++
		result.bytes += info.Size()
		if retain {
			if o.Keep == nil {
				child = name
				if dir.rel != "" {
					child = dir.rel + "/" + name
				}
			}
			result.entries = append(result.entries, Entry{child, info.Size(), info.ModTime().UnixNano(), false})
		}
	}
	return children, readErr
}

// Hash returns an xxh3-128 fingerprint of lower-cased paths, sizes, modification
// times and directory flags in canonical order. It does not reorder Entries.
func (t *Tree) Hash() [16]byte {
	entries := t.Entries
	if !slices.IsSortedFunc(entries, compareEntries) {
		entries = slices.Clone(entries)
		slices.SortFunc(entries, compareEntries)
	}
	h := xxh3.New()
	var text [512]byte
	var meta [18]byte // Path terminator, size, mtime, directory flag.
	for _, e := range entries {
		n := 0
		for _, r := range e.Rel {
			if len(text)-n < utf8.UTFMax {
				h.Write(text[:n])
				n = 0
			}
			n += utf8.EncodeRune(text[n:], unicode.ToLower(r))
		}
		h.Write(text[:n])
		binary.BigEndian.PutUint64(meta[1:9], uint64(e.Size))
		binary.BigEndian.PutUint64(meta[9:17], uint64(e.MTime))
		meta[17] = 0
		if e.Dir {
			meta[17] = 1
		}
		h.Write(meta[:])
	}
	return h.Sum128().Bytes()
}

func compareEntries(a, b Entry) int {
	if c := compareLower(a.Rel, b.Rel); c != 0 {
		return c
	}
	return strings.Compare(a.Rel, b.Rel)
}

// Comparing folded runes avoids allocating lower-cased paths while sorting.
func compareLower(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		x, nx := lowerRune(a)
		y, ny := lowerRune(b)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		a, b = a[nx:], b[ny:]
	}
	return strings.Compare(a, b)
}

func lowerRune(s string) (rune, int) {
	r, n := rune(s[0]), 1
	if r >= utf8.RuneSelf {
		r, n = utf8.DecodeRuneInString(s)
	}
	return unicode.ToLower(r), n
}
