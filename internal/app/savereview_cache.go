package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
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
	mm, err := a.reviewMM(sv)
	if err != nil {
		return nil, err
	}
	a.Store.MM = mm
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

// ReviewInputKey is what every save's kept attribution rests on: the versions of the
// parser, the Def extractor and the review, the installed copies and their analysed Defs
// (Store.ReviewContentKey) and the packs' manifests. What a save's Mods resolve to beyond
// that (its Game Config, game version and list) is the save's own context
// (reviewContext), so the Game Config shown and the lists edited do not touch the saves
// that do not load them (12 §3.4).
func (a *App) ReviewInputKey() (string, error) {
	key, err := a.Store.ReviewContentKey()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%d %d %d %s\n", reviewCacheVersion, savefile.ParserVersion, analyze.DefContentVersion, key)
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

// ReviewContextKey is the context a save without a Game Config or list of its own is
// attributed in: the current Game Config, game version and open list. The window keys
// its kept pages by it.
func (a *App) ReviewContextKey() (string, error) {
	return a.reviewContext(&Save{}, map[int64]string{})
}

// ReviewSavesContextKey is the contexts of every save of cols together (reviewContext):
// a kept review page is current only while none of them changed, including a list that
// is not the current one but that some listed save is attributed under.
func (a *App) ReviewSavesContextKey(cols []*Colony) (string, error) {
	h := sha256.New()
	lists := map[int64]string{}
	for _, c := range cols {
		for _, sv := range c.Saves {
			k, err := a.reviewContext(sv, lists)
			if err != nil {
				return "", err
			}
			fmt.Fprintln(h, k)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:12]), nil
}

// reviewContext names what a save's Mods resolve with besides ReviewInputKey: the Game
// Config and game version it is attributed in (its own, or the current one), the list
// whose copies RimArcho deploys there (by the copies it names, not their order) and its
// pack. lists memoizes the lists' copies within one review.
func (a *App) reviewContext(sv *Save, lists map[int64]string) (string, error) {
	var out string
	err := a.inSaveList(sv, func() error {
		e, err := a.Env()
		if err != nil {
			return err
		}
		mm, err := a.reviewMM(sv)
		if err != nil {
			return err
		}
		plan, err := a.planFor(e.ID)
		if err != nil {
			return err
		}
		var id int64
		if plan != nil {
			id = plan.ID
			if _, ok := lists[id]; !ok {
				ws, err := a.Store.Workspace(id)
				if err != nil {
					return err
				}
				var insts []string
				if ws != nil {
					for _, en := range loadout.Decode(ws.Entries) {
						if en.Instance != "" {
							insts = append(insts, en.Instance)
						}
					}
				}
				sort.Strings(insts)
				lists[id] = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(slices.Compact(insts), "\n"))))
			}
		}
		out = fmt.Sprintf("%s|%s|%d|%s|%s", e.ID, mm, id, lists[id], sv.Pack)
		return nil
	})
	return out, err
}

// reviewMM is the game version whose analysis a save's catalog reads: its Game Config's,
// or for a save of none the version the current one reads (LoadoutState.GameMM).
func (a *App) reviewMM(sv *Save) (string, error) {
	if sv.Env != nil {
		return env.MajorMinor(sv.Env.GameVersion), nil
	}
	if a.GameVersion != "" {
		return env.MajorMinor(a.GameVersion), nil
	}
	e, err := a.Env()
	if err != nil {
		return "", err
	}
	return env.MajorMinor(e.GameVersion), nil
}

// reviewRun is what one SaveReview shares between its saves.
type reviewRun struct {
	input    string
	src      *defSource
	catalogs *reviewCatalogs
	lists    map[int64]string // reviewContext's memo
	tidied   bool             // obsolete review rows are dropped before the run's first write
}

func (a *App) reviewSave(sv *Save, read bool, run *reviewRun) (*reviewSaveResult, bool, error) {
	// The matcher may choose another environment when launches or plans change.
	context, err := a.reviewContext(sv, run.lists)
	if err != nil {
		return nil, false, err
	}
	key := run.input + "|" + context
	src, catalogs := run.src, run.catalogs
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
		if !run.tidied {
			// Old rows duplicated full catalogs. Drop that format even for saves outside
			// this request, while retaining their reusable L1 facts; only a review that
			// writes does, so one that only merges kept rows writes nothing.
			if err := a.Store.PruneSaveReviewFormat(fmt.Sprintf("review%d:", reviewCacheVersion)); err != nil {
				return nil, false, err
			}
			run.tidied = true
		}
		err = a.Store.PutSaveReviewCache(&store.SaveReviewCache{Path: saveKey(sv), Size: sv.Size, MTime: sv.ModTime.UnixNano(), Parser: savefile.ParserVersion, InputKey: key, Result: b, CatalogKey: catalog.key, Catalog: catalog.raw})
		if err != nil {
			return nil, false, err
		}
		catalog.raw = nil
	}
	return out, false, nil
}
