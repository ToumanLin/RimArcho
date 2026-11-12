package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Bump when attribution or the compact representation changes independently of
// the save parser or Def content extractor.
const reviewCacheVersion = 2

// Only seen evidence and a catalog reference belong to a save. Enabled-copy
// metadata and categorized names live once in the content-addressed catalog.
type reviewSaveResult struct {
	Read       bool
	Mods       []SaveMod `json:"-"`
	Names      map[string]map[string][]string
	CatalogKey string
	Catalog    map[string]map[string][]string `json:"-"`
}

// reviewCatalogs is a run-local LRU, bounded independently of the number of saves
// or different enabled sets. Saves sharing a catalog use the same decoded object.
type reviewCatalogs struct {
	entries []reviewCatalogEntry
}

type reviewCatalogEntry struct {
	key     [32]byte
	catalog *reviewCatalog
}

type reviewCatalog struct {
	Mods  []SaveMod
	Names map[string]map[string][]string
	key   string
	raw   []byte // released after its first atomic publication with a save
}

const reviewCatalogLimit = 2

func (cache *reviewCatalogs) keep(entry reviewCatalogEntry) {
	if len(cache.entries) == reviewCatalogLimit {
		cache.entries = slices.Delete(cache.entries, 0, 1)
	}
	cache.entries = append(cache.entries, entry)
}

func (cache *reviewCatalogs) load(a *App, key string) (*reviewCatalog, error) {
	for i, entry := range cache.entries {
		if entry.catalog.key == key {
			cache.entries = append(slices.Delete(cache.entries, i, i+1), entry)
			return entry.catalog, nil
		}
	}
	b, err := a.Store.SaveReviewCatalog(key)
	if err != nil || b == nil {
		return nil, err
	}
	var catalog reviewCatalog
	if json.Unmarshal(b, &catalog) != nil {
		return nil, nil
	}
	catalog.key = key
	cache.keep(reviewCatalogEntry{catalog: &catalog})
	return &catalog, nil
}

func (cache *reviewCatalogs) get(a *App, sv *Save, mods []SaveMod, src *defSource) (*reviewCatalog, error) {
	oldMM := a.Store.MM
	defer func() { a.Store.MM = oldMM }()
	if sv.Env != nil {
		a.Store.MM = env.MajorMinor(sv.Env.GameVersion)
	}
	active := map[string]bool{}
	for _, pkg := range sv.Head.ModIDs {
		active[normPkg(pkg)] = true
	}
	var insts []string
	pos := map[string]int{}
	h := sha256.New()
	fmt.Fprintf(h, "%q\n", a.Store.MM)
	for i, m := range mods {
		active[normPkg(m.PackageID)] = true
		if m.Instance != "" {
			insts = append(insts, m.Instance)
			pos[m.Instance] = i
			fmt.Fprintf(h, "%q\n", m.Instance)
		}
	}
	packages := make([]string, 0, len(active))
	for pkg := range active {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	for _, pkg := range packages {
		fmt.Fprintf(h, "%q\n", pkg)
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	for i, entry := range cache.entries {
		if entry.key == key {
			cache.entries = append(slices.Delete(cache.entries, i, i+1), entry)
			return entry.catalog, nil
		}
	}
	ready, err := a.Store.ContentReady(insts)
	if err != nil {
		return nil, err
	}
	catalog := map[string]map[string][]string{}
	if err := a.contentCatalog(catalog, src, insts, ready, pos, active); err != nil {
		return nil, err
	}
	c := &reviewCatalog{Mods: mods, Names: catalog}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	c.key = fmt.Sprintf("%x", sha256.Sum256(raw))
	for _, entry := range cache.entries {
		if entry.catalog.key == c.key {
			c = entry.catalog
			break
		}
	}
	if c.raw == nil {
		c.raw = raw
	}
	cache.keep(reviewCatalogEntry{key: key, catalog: c})
	return c, nil
}

// ReviewInputKey validates both persistent attributions and the GUI's kept pages.
func (a *App) ReviewInputKey() (string, error) {
	key, err := a.Store.ReviewContentKey()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%d %d %d %s %q %d %q\n", reviewCacheVersion, savefile.ParserVersion, analyze.DefContentVersion, key, a.EnvName, a.OpenPlan, a.GameVersion)
	// Pack entries select different installed copies from the library registry.
	ents, err := os.ReadDir(a.PacksDir())
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(a.PacksDir(), ent.Name(), "pack.json")
		if st, err := os.Stat(path); err == nil {
			fmt.Fprintf(h, "%q %d %d\n", path, st.Size(), st.ModTime().UnixNano())
		}
	}
	return fmt.Sprintf("review%d:%x", reviewCacheVersion, h.Sum(nil)), nil
}

func (a *App) reviewSave(sv *Save, read bool, input string, src *defSource, catalogs *reviewCatalogs) (*reviewSaveResult, bool, error) {
	// The matcher may choose another environment when launches or plans change.
	context := ""
	if sv.Env != nil {
		context = sv.Env.ID + "|" + sv.Env.GameVersion
	}
	key := input + "|" + context + "|" + sv.Pack
	if !sv.Writing && sv.Head != nil {
		b, err := a.Store.SaveReviewCache(saveKey(sv), sv.Size, sv.ModTime.UnixNano(), savefile.ParserVersion, key)
		if err != nil {
			return nil, false, err
		}
		var kept reviewSaveResult
		if len(b) > 0 && json.Unmarshal(b, &kept) == nil && kept.Read {
			catalog, err := catalogs.load(a, kept.CatalogKey)
			if err != nil {
				return nil, false, err
			}
			if catalog != nil {
				kept.Mods, kept.Catalog = catalog.Mods, catalog.Names
				return &kept, true, nil
			}
		}
	}
	var facts *savefile.Facts
	if read && !sv.ReadFailed && sv.Head != nil {
		facts, _ = a.SaveFacts(sv, true) // an unreadable save is counted under Unread
	}
	res, err := a.saveContent(sv, contentOpts{seen: true, src: src, facts: facts})
	if err != nil {
		return nil, false, err
	}
	out := &reviewSaveResult{Read: res.Read && sv.Head != nil, Mods: res.mods, Names: map[string]map[string][]string{}}
	for inst, cats := range res.names {
		out.Names[inst] = map[string][]string{}
		for cat, names := range cats {
			if reviewSkip[cat] {
				continue
			}
			for _, n := range names {
				out.Names[inst][cat] = append(out.Names[inst][cat], n.Name)
			}
		}
	}
	if out.Read && !sv.Writing {
		catalog, err := catalogs.get(a, sv, out.Mods, src)
		if err != nil {
			return nil, false, err
		}
		out.Mods, out.Catalog, out.CatalogKey = catalog.Mods, catalog.Names, catalog.key
		b, err := json.Marshal(out)
		if err != nil {
			return nil, false, err
		}
		err = a.Store.PutSaveReviewCache(&store.SaveReviewCache{Path: saveKey(sv), Size: sv.Size, MTime: sv.ModTime.UnixNano(), Parser: savefile.ParserVersion, InputKey: key, Result: b, CatalogKey: catalog.key, Catalog: catalog.raw})
		if err != nil {
			return nil, false, err
		}
		catalog.raw = nil
	}
	return out, false, nil
}
