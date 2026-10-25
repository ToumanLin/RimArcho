package app

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The reference index (03 §6.5): which effective Defs/ and Patches/ files of an enabled
// set mention which names, the way a def is referenced. A "Could not resolve
// cross-reference" cluster asks which enabled mods want the missing def; without the index
// that meant listing and reading every file of every enabled mod for every new log (5,750
// files, 77 MB for 485 mods). The index is built once per enabled set and game version — by
// the analysis child after an analysis, or by the first lookup that finds none — and stored
// in <data>/cache/refindex/. A lookup reads only the files whose entries match a name's hash.

// maxRefName bounds the text looked up: def names are short, element text can be long.
const maxRefName = 200

// refIndexVersion invalidates stored indexes when the tokens or the format change.
const refIndexVersion = 1

// refIndexKeep is how many indexes (enabled sets) stay on disk.
const refIndexKeep = 8

// refTokens calls fn with every piece of text that could be a def reference, in order: the
// text between a '>' and the next '<' (<thingDef>X</thingDef>, <li>X</li>) and a tag name up
// to the next '>' (<X>1</X> in ThingDefCountClass lists such as costList), with the offset
// of the '>' or '<' it starts at.
func refTokens(b []byte, fn func(tok []byte, at int)) {
	prev := 0 // just after the previous '<'
	for {
		k := bytes.IndexByte(b[prev:], '<')
		if k < 0 {
			return
		}
		k += prev
		if j := bytes.LastIndexByte(b[prev:k], '>'); j >= 0 && prev+j+1 < k && k-(prev+j+1) <= maxRefName {
			fn(b[prev+j+1:k], prev+j)
		}
		if m := bytes.IndexByte(b[k+1:], '>'); m > 0 && m <= maxRefName {
			fn(b[k+1:k+1+m], k)
		}
		prev = k + 1
	}
}

// refScan reports the first line on which each wanted name appears as a reference (see
// refTokens). want maps a name to the index found is called with, once per name.
func refScan(b []byte, want map[string]int, found func(i, line int)) {
	var seen map[int]bool
	at, line := 0, 1 // line of byte offset at
	refTokens(b, func(tok []byte, p int) {
		i, ok := want[string(tok)]
		if !ok || seen[i] {
			return
		}
		if seen == nil {
			seen = map[int]bool{}
		}
		seen[i] = true
		line += bytes.Count(b[at:p], []byte{'\n'})
		at = p
		found(i, line)
	})
}

// indexable: a def name has no white space (the log's name is \S+).
func indexable(tok []byte) bool { return !bytes.ContainsAny(tok, " \t\r\n") }

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func refHash(tok []byte) uint32 { return crc32.Checksum(tok, castagnoli) }

type refFile struct{ Inst, Rel, Path string }

type refIndex struct {
	key   string
	files []refFile
	ents  []uint64 // hash<<32 | file, sorted
}

// filesWith returns the files that may mention name (the hash matches: read them to be sure).
func (x *refIndex) filesWith(name string) []int {
	h := uint64(refHash([]byte(name))) << 32
	i := sort.Search(len(x.ents), func(i int) bool { return x.ents[i] >= h })
	var out []int
	for ; i < len(x.ents) && x.ents[i]&^0xffffffff == h; i++ {
		out = append(out, int(x.ents[i]&0xffffffff))
	}
	return out
}

// refMods are the enabled mods of st with their declarations loaded.
func (a *App) refMods(st *LoadoutState) ([]*store.Mod, error) {
	var mods []*store.Mod
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			mods = append(mods, m)
		}
	}
	return mods, a.Store.LoadDecls(mods)
}

// refIndexKey names an enabled set's index: the game version and each enabled mod's
// instance, snapshot and folder (LoadFolders conditions depend on the set, not its order).
func refIndexKey(st *LoadoutState, mods []*store.Mod) string {
	var l []string
	for _, m := range mods {
		l = append(l, fmt.Sprintf("%s %d %s", m.ID, m.SnapID, st.Ix.ModDir(m)))
	}
	sort.Strings(l)
	h := sha1.New()
	fmt.Fprintf(h, "v%d|%s", refIndexVersion, st.GameMM())
	for _, s := range l {
		fmt.Fprintf(h, "|%s", s)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:20]
}

// refFiles lists the effective Defs/ and Patches/ XML files of the enabled mods.
func refFiles(st *LoadoutState, mods []*store.Mod) []refFile {
	active := map[string]bool{}
	for _, m := range mods {
		active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
	}
	var out []refFile
	for _, m := range mods {
		dir := st.Ix.ModDir(m)
		r := content.Resolve(dir, m.Decl().LoadFolders, st.GameMM(), func(p string) bool { return active[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
		for _, sub := range []string{"Defs", "Patches"} {
			for _, as := range content.Assets(dir, r.Folders, sub) {
				out = append(out, refFile{m.ID, as.Rel, as.Path})
			}
		}
	}
	return out
}

func (a *App) refIndexDir() string { return a.cacheDir("refindex") }

// RefIndex returns the reference index of st's enabled set: from disk (about 10 ms for 485
// mods), or built now and stored. It is not kept in memory (~5 MB).
func (a *App) RefIndex(st *LoadoutState) (*refIndex, error) {
	mods, err := a.refMods(st)
	if err != nil {
		return nil, err
	}
	key := refIndexKey(st, mods)
	path := filepath.Join(a.refIndexDir(), key+".idx")
	x, err := readRefIndex(path, key)
	if err != nil {
		x = buildRefIndex(key, refFiles(st, mods))
		a.writeRefIndex(path, x) // best effort: a failed write only costs the next build
	}
	return x, nil
}

// buildRefIndex reads every file once (8 at a time) and records the hash of each
// indexable token per file.
func buildRefIndex(key string, files []refFile) *refIndex {
	x := &refIndex{key: key, files: files}
	var mu sync.Mutex
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen := map[uint32]bool{}
			var mine []uint64
			for i := range next {
				b, err := os.ReadFile(files[i].Path)
				if err != nil {
					continue
				}
				clear(seen)
				refTokens(b, func(tok []byte, _ int) {
					if !indexable(tok) {
						return
					}
					if h := refHash(tok); !seen[h] {
						seen[h] = true
						mine = append(mine, uint64(h)<<32|uint64(i))
					}
				})
			}
			mu.Lock()
			x.ents = append(x.ents, mine...)
			mu.Unlock()
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	slices.Sort(x.ents)
	return x
}

const refMagic = "RAREFIDX"

func (a *App) writeRefIndex(path string, x *refIndex) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(tmp, 1<<20)
	var buf [binary.MaxVarintLen64]byte
	uv := func(n uint64) { w.Write(buf[:binary.PutUvarint(buf[:], n)]) }
	str := func(s string) { uv(uint64(len(s))); w.WriteString(s) }
	w.WriteString(refMagic)
	uv(refIndexVersion)
	str(x.key)
	uv(uint64(len(x.files)))
	for _, f := range x.files {
		str(f.Inst)
		str(f.Rel)
		str(f.Path)
	}
	uv(uint64(len(x.ents)))
	for _, e := range x.ents {
		binary.LittleEndian.PutUint64(buf[:8], e)
		w.Write(buf[:8])
	}
	err = w.Flush()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	a.pruneRefIndexes(path)
	return nil
}

// pruneRefIndexes keeps the refIndexKeep newest indexes.
func (a *App) pruneRefIndexes(keep string) {
	dir := filepath.Dir(keep)
	es, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		path string
		mod  int64
	}
	var fs []f
	for _, e := range es {
		p := filepath.Join(dir, e.Name())
		if fi, err := e.Info(); err == nil && p != keep && strings.HasSuffix(p, ".idx") {
			fs = append(fs, f{p, fi.ModTime().UnixNano()})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].mod > fs[j].mod })
	for i := refIndexKeep - 1; i < len(fs); i++ {
		os.Remove(fs[i].path)
	}
}

var errRefIndex = errors.New("reference index: bad file")

func readRefIndex(path, key string) (*refIndex, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(b)
	magic := make([]byte, len(refMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != refMagic {
		return nil, errRefIndex
	}
	bad := false
	uv := func() uint64 {
		n, err := binary.ReadUvarint(r)
		if err != nil {
			bad = true
		}
		return n
	}
	str := func() string {
		n := uv()
		if bad || n > uint64(r.Len()) {
			bad = true
			return ""
		}
		s := make([]byte, n)
		io.ReadFull(r, s)
		return string(s)
	}
	if uv() != refIndexVersion || str() != key || bad {
		return nil, errRefIndex
	}
	n := uv()
	if bad || n > uint64(r.Len()) {
		return nil, errRefIndex
	}
	x := &refIndex{key: key, files: make([]refFile, n)}
	for i := range x.files {
		x.files[i] = refFile{str(), str(), str()}
	}
	m := uv()
	if bad || m*8 != uint64(r.Len()) {
		return nil, errRefIndex
	}
	rest := b[len(b)-r.Len():]
	x.ents = make([]uint64, m)
	for i := range x.ents {
		x.ents[i] = binary.LittleEndian.Uint64(rest[i*8:])
	}
	return x, nil
}
