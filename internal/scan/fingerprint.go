package scan

import (
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/zeebo/xxh3"
)

// Hash is an xxh3-128 digest, big-endian (hi, lo).
type Hash = [16]byte

// File is one entry of the structure manifest (02 §4.3). Semantic files carry a content hash.
type File struct {
	Rel     string // forward slashes, original case
	Size    int64
	MTime   int64 // unix nanoseconds; only used to skip re-hashing, never for identity
	Hash    Hash
	HasHash bool
}

// SemanticSet maps lower-cased relative paths of semantic files to their hashes.
type SemanticSet = map[string]Hash

// Fingerprint is the layered content fingerprint of one mod folder.
type Fingerprint struct {
	Files        []File // sorted by lower-cased Rel; nil once released (see Options.PrevHash)
	Count        int    // number of files, kept when Files is released
	SnapshotHash Hash
	Semantic     SemanticSet
	TotalSize    int64
	Hashed       int // semantic files whose content was read this time
	Reused       int // semantic files whose hash was reused (same size + mtime)
}

// IsSemantic reports whether a relative path (forward slashes) belongs to the semantic layer:
// About/, LoadFolders.xml, XML under any Defs/ or Patches/ directory, and DLLs under any
// Assemblies/ directory — including version folders such as 1.6/Defs/.
func IsSemantic(rel string) bool {
	l := strings.ToLower(rel)
	if strings.HasPrefix(l, "about/") || l == "loadfolders.xml" {
		return true
	}
	segs := strings.Split(l, "/")
	dirs := segs[:len(segs)-1]
	has := func(name string) bool {
		for _, s := range dirs {
			if s == name {
				return true
			}
		}
		return false
	}
	switch {
	case strings.HasSuffix(l, ".xml"):
		return has("defs") || has("patches")
	case strings.HasSuffix(l, ".dll"):
		return has("assemblies")
	}
	return false
}

// skipDir lists folders that are not mod content: VCS metadata and RimArcho's own marker.
func skipDir(name string) bool {
	return name == ".git" || name == ".rimarcho" || name == ".svn"
}

// Compute walks dir and builds its fingerprint. prev maps lower-cased Rel to the previous
// snapshot's entries; a semantic file whose size and mtime are unchanged reuses its hash.
// Links inside the mod folder are not followed.
func Compute(dir string, prev map[string]File) (Fingerprint, error) {
	var fp Fingerprint
	fp.Semantic = SemanticSet{}
	if source.IsLink(dir) {
		// WalkDir does not descend into a junction or symlink root; walk its target.
		if real, err := source.RealPath(dir); err == nil {
			dir = real
		}
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil // unreadable entry: skip, like the game would fail to load it
		}
		if d.IsDir() {
			if p != dir && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // links, junctions and devices inside a mod are not followed
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		f := File{Rel: rel, Size: info.Size(), MTime: info.ModTime().UnixNano()}
		if IsSemantic(rel) {
			key := strings.ToLower(rel)
			if pf, ok := prev[key]; ok && pf.HasHash && pf.Size == f.Size && pf.MTime == f.MTime {
				f.Hash, f.HasHash = pf.Hash, true
				fp.Reused++
			} else if h, err := hashFile(p); err == nil {
				f.Hash, f.HasHash = h, true
				fp.Hashed++
			}
			if f.HasHash {
				fp.Semantic[key] = f.Hash
			}
		}
		fp.TotalSize += f.Size
		fp.Files = append(fp.Files, f)
		return nil
	})
	if err != nil {
		return fp, err
	}
	SortFiles(fp.Files)
	fp.SnapshotHash = SnapshotHash(fp.Files)
	fp.Count = len(fp.Files)
	return fp, nil
}

// SortFiles orders a manifest by lower-cased path, the canonical order for hashing.
func SortFiles(files []File) {
	sort.Slice(files, func(i, j int) bool {
		a, b := strings.ToLower(files[i].Rel), strings.ToLower(files[j].Rel)
		if a != b {
			return a < b
		}
		return files[i].Rel < files[j].Rel
	})
}

// SnapshotHash hashes the sorted manifest: path, size and (for semantic files) content hash.
// Paths are lower-cased so a case-only rename on Windows is not an update.
func SnapshotHash(files []File) Hash {
	h := xxh3.New()
	var num [8]byte
	for _, f := range files {
		h.WriteString(strings.ToLower(f.Rel))
		h.Write([]byte{0})
		binary.BigEndian.PutUint64(num[:], uint64(f.Size))
		h.Write(num[:])
		if f.HasHash {
			h.Write([]byte{1})
			h.Write(f.Hash[:])
		} else {
			h.Write([]byte{0})
		}
		h.Write([]byte{'\n'})
	}
	return h.Sum128().Bytes()
}

func hashFile(p string) (Hash, error) {
	f, err := os.Open(p)
	if err != nil {
		return Hash{}, err
	}
	defer f.Close()
	h := xxh3.New()
	if _, err := io.Copy(h, f); err != nil {
		return Hash{}, err
	}
	return h.Sum128().Bytes(), nil
}

// SemanticOnly computes just the semantic layer of dir (used for source re-binding samples).
func SemanticOnly(dir string) (SemanticSet, error) {
	fp, err := Compute(dir, nil)
	return fp.Semantic, err
}

// Similarity of two semantic sets (02 §4.3, revised after M1): the larger of
//   - J(paths): Jaccard over relative paths only — "moved and updated" keeps its structure;
//   - J(hashes): Jaccard over content hashes regardless of path (as multisets, so several
//     identical files count once each) — "restructured" moves files into e.g. 1.6/.
func Similarity(a, b SemanticSet) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	paths := 0
	for k := range a {
		if _, ok := b[k]; ok {
			paths++
		}
	}
	jPaths := float64(paths) / float64(len(a)+len(b)-paths)
	count := make(map[Hash]int, len(a))
	for _, h := range a {
		count[h]++
	}
	common := 0
	for _, h := range b {
		if count[h] > 0 {
			count[h]--
			common++
		}
	}
	jHashes := float64(common) / float64(len(a)+len(b)-common)
	return max(jPaths, jHashes)
}
