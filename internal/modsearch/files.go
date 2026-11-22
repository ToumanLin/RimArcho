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

// walkScope names one scope folder that walkBase lists, and the options that filter its files.
type walkScope struct {
	root, folder, base string
	langs              []string
	allVersions        bool
	kind               Kind
}

func listFiles(ctx context.Context, mod Mod, o Options) listing {
	var result listing
	// A listing may be shared by several kinds and the resolver; never sort an FS-owned slice.
	cache := map[string][]fs.DirEntry{}
	readDir := func(dir string) ([]fs.DirEntry, error) {
		return readSortedDir(ctx, o.FS, cache, &result, dir)
	}
	folders, labels := uniqueFolders(modFolders(readDir, &mod, &o), &mod, &o)
	seen := map[string]bool{}
	var out []fileJob
	for fi, folder := range folders {
		for _, s := range scopes {
			if fi >= len(labels) && s.kind != About {
				continue
			}
			if o.Kinds&s.kind == 0 || s.kind == About && filepath.Clean(folder) != filepath.Clean(mod.Root) {
				continue
			}
			for _, base := range scopeDirs(readDir, []string{folder}, s.dir) {
				ws := walkScope{root: mod.Root, folder: folder, base: base, langs: o.Langs, allVersions: o.AllVersions, kind: s.kind}
				var ok bool
				if out, ok = walkBase(readDir, &result, seen, out, &ws); !ok {
					return result
				}
			}
		}
	}
	result.folders, result.files = labels, out
	return result
}

// readSortedDir returns dir's entries sorted case-insensitively, cached, and records errors in result.
func readSortedDir(ctx context.Context, fsys FS, cache map[string][]fs.DirEntry, result *listing, dir string) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		result.err = err
		return nil, err
	}
	if entries, ok := cache[dir]; ok {
		return entries, nil
	}
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.err = err
		} else if !errors.Is(err, fs.ErrNotExist) { // LoadFolders may name folders that do not exist
			result.unreadable++
			if result.firstError == "" {
				result.firstError = errText(dir, err)
			}
		}
		cache[dir] = nil
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
	cache[dir] = entries
	return entries, nil
}

// modFolders lists the mod's load folders: every candidate with AllVersions, else the resolved ones.
func modFolders(readDir func(string) ([]fs.DirEntry, error), mod *Mod, o *Options) []string {
	if !o.AllVersions {
		return content.ResolveFS(mod.Root, mod.LoadFoldersXML, o.MM, o.Active, readDir).Folders
	}
	var folders []string
	entries, _ := readDir(mod.Root)
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

// scopeDirs resolves each segment of a scope path case-insensitively under dirs, including Lunar/Components.
func scopeDirs(readDir func(string) ([]fs.DirEntry, error), dirs []string, scopeDir string) []string {
	for _, segment := range strings.Split(scopeDir, "/") {
		var found []string
		for _, parent := range dirs {
			entries, _ := readDir(parent)
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

// walkBase appends the accepted files under s.base breadth-first; it reports false once the listing has an error.
func walkBase(readDir func(string) ([]fs.DirEntry, error), result *listing, seen map[string]bool, out []fileJob, s *walkScope) ([]fileJob, bool) {
	base, kind := s.base, s.kind
	queue := []string{base}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		entries, _ := readDir(dir)
		if result.err != nil {
			return out, false
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if kind != Languages || len(s.langs) == 0 || dir != base || hasLang(s.langs, e.Name()) {
					queue = append(queue, path)
				}
				continue
			}
			if strings.HasPrefix(e.Name(), ".") || !acceptFile(kind, path) {
				continue
			}
			asset := relative(base, path)
			if kind == Languages && len(s.langs) > 0 && !strings.Contains(asset, "/") {
				continue
			}
			key := strings.ToLower(relative(s.folder, path))
			if !s.allVersions && seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, fileJob{path: path, rel: relative(s.root, path), folder: relative(s.root, s.folder), asset: asset, kind: kind})
		}
	}
	return out, true
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
