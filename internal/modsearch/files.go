package modsearch

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/textenc"
)

type scope struct {
	dir  string
	kind Kind
}

var scopes = []scope{
	{"About", About}, {"Assemblies", Assemblies}, {"Defs", Defs},
	{"Languages", Languages}, {"Lunar/Components", Assemblies},
	{"Patches", Patches}, {"Sounds", Sounds}, {"Textures", Textures},
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if rel == "." {
		return "/"
	}
	return filepath.ToSlash(rel)
}

type listing struct {
	folders    []string
	files      []fileJob
	unreadable int
	firstError string
	err        error
}

// dirLister reads and caches sorted directory listings, counting failures in result.
type dirLister struct {
	ctx    context.Context
	fs     FS
	cache  map[string][]fs.DirEntry
	result *listing
}

// fileWalk carries the state listFiles keeps while it walks one mod's folders.
type fileWalk struct {
	l    *dirLister
	mod  *Mod
	o    *Options
	seen map[string]bool
	out  []fileJob
}

func listFiles(ctx context.Context, mod Mod, o Options) listing {
	var result listing
	// A listing may be shared by several kinds and the resolver; never sort an FS-owned slice.
	l := dirLister{ctx: ctx, fs: o.FS, cache: map[string][]fs.DirEntry{}, result: &result}
	folders, labels := uniqueFolders(modFolders(&l, &mod, &o), &mod, &o)
	w := fileWalk{l: &l, mod: &mod, o: &o, seen: map[string]bool{}}
	for fi, folder := range folders {
		for _, s := range scopes {
			if fi >= len(labels) && s.kind != About {
				continue
			}
			if o.Kinds&s.kind == 0 || s.kind == About && filepath.Clean(folder) != filepath.Clean(mod.Root) {
				continue
			}
			for _, base := range w.scopeDirs(folder, s.dir) {
				if !w.walkBase(folder, base, s.kind) {
					return result
				}
			}
		}
	}
	result.folders, result.files = labels, w.out
	return result
}

// readDir returns dir's entries sorted case-insensitively, cached, and records errors in the listing.
func (l *dirLister) readDir(dir string) ([]fs.DirEntry, error) {
	if err := l.ctx.Err(); err != nil {
		l.result.err = err
		return nil, err
	}
	if entries, ok := l.cache[dir]; ok {
		return entries, nil
	}
	entries, err := l.fs.ReadDir(dir)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			l.result.err = err
		} else if !errors.Is(err, fs.ErrNotExist) { // LoadFolders may name folders that do not exist
			l.result.unreadable++
			if l.result.firstError == "" {
				l.result.firstError = errText(dir, err)
			}
		}
		l.cache[dir] = nil
		return nil, err
	}
	entries = append([]fs.DirEntry(nil), entries...)
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i].Name(), entries[j].Name()
		if strings.EqualFold(a, b) {
			return a < b
		}
		return strings.ToLower(a) < strings.ToLower(b)
	})
	l.cache[dir] = entries
	return entries, nil
}

// modFolders lists the mod's load folders: every candidate with AllVersions, else the resolved ones.
func modFolders(l *dirLister, mod *Mod, o *Options) []string {
	if !o.AllVersions {
		return content.ResolveFS(mod.Root, mod.LoadFoldersXML, o.MM, o.Active, l.readDir).Folders
	}
	var folders []string
	entries, _ := l.readDir(mod.Root)
	for _, e := range entries {
		if e.IsDir() && content.IsVersionFolder(e.Name()) {
			folders = append(folders, filepath.Join(mod.Root, e.Name()))
		}
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "Common") {
			folders = append(folders, filepath.Join(mod.Root, e.Name()))
		}
	}
	folders = append(folders, mod.Root)
	for _, path := range everyLoadFolder(mod.LoadFoldersXML) {
		folders = append(folders, filepath.Join(mod.Root, path))
	}
	return folders
}

// uniqueFolders drops repeated folders, labels the rest, and appends the root for About when missing.
func uniqueFolders(folders []string, mod *Mod, o *Options) (unique, labels []string) {
	// Repeated entries in LoadFolders do not cause the same physical folder to be searched twice.
	folderSeen := map[string]bool{}
	for _, f := range folders {
		key := strings.ToLower(filepath.Clean(f))
		if !folderSeen[key] {
			folderSeen[key] = true
			unique = append(unique, f)
			labels = append(labels, relative(mod.Root, f))
		}
	}
	// About is metadata of the mod root even when LoadFolders omits the root.
	if !folderSeen[strings.ToLower(filepath.Clean(mod.Root))] && o.Kinds&About != 0 {
		unique = append(unique, mod.Root)
	}
	return unique, labels
}

// scopeDirs resolves each segment of a scope path case-insensitively under folder, including Lunar/Components.
func (w *fileWalk) scopeDirs(folder, scopeDir string) []string {
	dirs := []string{folder}
	for _, segment := range strings.Split(scopeDir, "/") {
		var found []string
		for _, parent := range dirs {
			entries, _ := w.l.readDir(parent)
			for _, e := range entries {
				if e.IsDir() && strings.EqualFold(e.Name(), segment) {
					found = append(found, filepath.Join(parent, e.Name()))
				}
			}
		}
		dirs = found
	}
	return dirs
}

// walkBase adds the accepted files under base breadth-first; it reports false once the listing has an error.
func (w *fileWalk) walkBase(folder, base string, kind Kind) bool {
	o := w.o
	queue := []string{base}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		entries, _ := w.l.readDir(dir)
		if w.l.result.err != nil {
			return false
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if kind != Languages || len(o.Langs) == 0 || dir != base || hasLang(o.Langs, e.Name()) {
					queue = append(queue, path)
				}
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || !acceptFile(kind, path) {
				continue
			}
			asset := relative(base, path)
			if kind == Languages && len(o.Langs) > 0 && !strings.Contains(asset, "/") {
				continue
			}
			key := strings.ToLower(relative(folder, path))
			if !o.AllVersions && w.seen[key] {
				continue
			}
			w.seen[key] = true
			w.out = append(w.out, fileJob{path: path, rel: relative(w.mod.Root, path), folder: relative(w.mod.Root, folder), asset: asset, kind: kind})
		}
	}
	return true
}

func hasLang(langs []string, name string) bool {
	for _, lang := range langs {
		if strings.EqualFold(lang, name) {
			return true
		}
	}
	return false
}
func acceptFile(kind Kind, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch kind {
	case Defs, Patches, About:
		return ext == ".xml"
	case Languages:
		return ext == ".xml" || ext == ".txt"
	case Assemblies:
		return ext == ".dll"
	default:
		return true
	}
}

// AllVersions includes entries from every tag, independent of their conditions (05 §1).
func everyLoadFolder(s string) []string {
	dec := xml.NewDecoder(bytes.NewReader(textenc.UTF8([]byte(s))))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	depth := 0
	var text strings.Builder
	var folders []string
	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 3 {
				text.Reset()
			}
		case xml.CharData:
			if depth == 3 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 3 {
				path := strings.TrimSpace(text.String())
				if path == "/" || path == `\` {
					path = ""
				}
				folders = append(folders, path)
			}
			depth--
		}
	}
	return folders
}

// errText is err with its path, once.
func errText(path string, err error) string {
	if s := err.Error(); strings.Contains(s, path) {
		return s
	}
	return path + ": " + err.Error()
}
