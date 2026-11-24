package app

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Mod categories (16 §2.5, 07 §6.1): what kind of Mod it is, worked out from what is inside
// it, never from its name. Display only: sorting does not read them (08 §1.5).

// CategoryKeys are the categories in the order shown (filters, the details' picker).
var CategoryKeys = []string{"game", "library", "content", "code", "patches", "translation", "textures"}

// LibraryMinDependents is how many other installed Mods must require a Mod with a DLL before
// it counts as a library.
const LibraryMinDependents = 4

// Category is a Mod's category: the one shown (Key), the automatic one and the facts behind
// it. Player is set when the player chose Key (07 §6.1); Auto stays what the content says.
type Category struct {
	Key    string `json:"key,omitempty"`
	Auto   string `json:"auto,omitempty"`
	Player bool   `json:"player,omitempty"`
	Why    CategoryWhy
}

// CategoryWhy are the facts a category was decided on, for its reason ("only patches: 14
// patch files, no Defs").
type CategoryWhy struct {
	Defs, Patches, DLLs, Dependents int
	Languages, Textures             bool
}

// CategoryOf is the automatic category of a Mod from its content facts and how many other
// installed Mods (Lineages) require it. The first rule that holds wins:
//
//	game        official content (Core and the DLCs)
//	library     ships a DLL and at least LibraryMinDependents other installed Mods require it
//	content     has Def files
//	code        ships a DLL, no Def files
//	patches     has patch files, no Defs or DLL
//	translation has Languages, no Defs, patches or DLL
//	textures    has textures, nothing above
//
// "" when nothing holds or the facts are unknown (no snapshot).
func CategoryOf(m *store.Mod, c *store.SnapContent, dependents int) (string, CategoryWhy) {
	if m == nil {
		return "", CategoryWhy{}
	}
	if m.Kind == source.Official {
		return "game", CategoryWhy{}
	}
	if c == nil {
		return "", CategoryWhy{}
	}
	why := CategoryWhy{Defs: c.Defs, Patches: c.Patches, DLLs: c.DLLs, Dependents: dependents,
		Languages: c.Kinds&store.HasLanguages != 0, Textures: c.Kinds&store.HasPNG != 0}
	dll := c.Kinds&store.HasDLL != 0
	switch {
	case dll && dependents >= LibraryMinDependents:
		return "library", why
	case c.HasDefs:
		return "content", why
	case dll:
		return "code", why
	case c.Kinds&store.HasDefsPatch != 0: // XML under Defs/ or Patches/, and none is Defs
		return "patches", why
	case why.Languages:
		return "translation", why
	case why.Textures:
		return "textures", why
	}
	return "", why
}

// ValidCategory: k is one of CategoryKeys.
func ValidCategory(k string) bool { return slices.Contains(CategoryKeys, k) }

// Categories are the index's Mods' categories by instance ID, worked out once per index (an
// index is rebuilt when a scan or the player's data changes, Store.IndexKey). The content
// facts are kept per snapshot across indexes (snapContentCache), so a rebuilt index only
// counts requirements again.
func (ix *Index) Categories() map[string]Category {
	if ix.cats == nil { // an index not built by readIndex
		return ix.loadCategories()
	}
	ix.cats.once.Do(func() { ix.cats.m = ix.loadCategories() })
	return ix.cats.m
}

// indexCategories is an index's categories, shared by its copies.
type indexCategories struct {
	once sync.Once
	m    map[string]Category
}

// CategoryFor is one Mod's category; Why has no file counts (CategoryDetail).
func (ix *Index) CategoryFor(id string) Category { return ix.Categories()[id] }

// CategoryDetail is one Mod's category with the file counts of its reason (details, CLI):
// one query over that Mod's file rows.
func (ix *Index) CategoryDetail(id string) Category {
	c := ix.CategoryFor(id)
	if m := ix.ByID[id]; m != nil && m.SnapID != 0 && ix.store != nil {
		if d, p, l, err := ix.store.SnapshotCounts(m.SnapID); err == nil {
			c.Why.Defs, c.Why.Patches, c.Why.DLLs = d, p, l
		}
	}
	return c
}

func (ix *Index) loadCategories() map[string]Category {
	out := make(map[string]Category, len(ix.Mods))
	var ids []int64
	for _, m := range ix.Mods {
		if m.SnapID != 0 {
			ids = append(ids, m.SnapID)
		}
	}
	content := snapContent(ix.store, ids)
	// Who requires whom: each requiring Lineage once, live installed copies only.
	need := map[string]map[string]bool{}
	for _, m := range ix.Mods {
		if m.Status != identity.Live || m.Residue {
			continue
		}
		c := content[m.SnapID]
		if c == nil {
			continue
		}
		for _, p := range c.Requires {
			if need[p] == nil {
				need[p] = map[string]bool{}
			}
			need[p][m.LineageID] = true
		}
	}
	for _, m := range ix.Mods {
		by := need[dependencyPackage(m.PackageID)]
		n := len(by)
		if by[m.LineageID] {
			n--
		}
		auto, why := CategoryOf(m, content[m.SnapID], n)
		c := Category{Key: auto, Auto: auto, Why: why}
		if ValidCategory(m.Category) {
			c.Key, c.Player = m.Category, true
		}
		out[m.ID] = c
	}
	return out
}

// snapContentCache keeps the content facts of the snapshots last asked for, by database:
// a snapshot never changes, and holding only the last request's keeps it as large as the
// installed library.
var snapContentCache struct {
	mu sync.Mutex
	db string
	m  map[int64]*store.SnapContent
}

func snapContent(s *store.Store, ids []int64) map[int64]*store.SnapContent {
	if s == nil {
		return nil
	}
	if s.Path == "" { // an in-memory database: snapshot IDs say nothing across databases
		got, _ := s.SnapshotContent(ids)
		return got
	}
	c := &snapContentCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != s.Path {
		c.db, c.m = s.Path, nil
	}
	out := make(map[int64]*store.SnapContent, len(ids))
	var todo []int64
	for _, id := range ids {
		if v := c.m[id]; v != nil {
			out[id] = v
		} else {
			todo = append(todo, id)
		}
	}
	if len(todo) > 0 {
		got, err := s.SnapshotContent(todo)
		if err != nil {
			return out // no categories rather than no list
		}
		for id, v := range got {
			out[id] = v
		}
	}
	c.m = out
	return out
}

// SetCategory sets the player's category of a Mod (all its copies, 07 §6.1), or with ""
// gives it back to the automatic one. It returns the Mod.
func (a *App) SetCategory(ref, key string) (*store.Mod, error) {
	if key != "" && !ValidCategory(key) {
		return nil, fmt.Errorf("unknown category %q: use one of %s, or clear it", key, strings.Join(CategoryKeys, ", "))
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	return m, a.Store.SetCategory(m.LineageID, key)
}

// CategoryReason is why the automatic category is what it is, in English (CLI, MCP).
func CategoryReason(c Category) string { return CategoryReasonWith(c.Auto, c.Why, fmt.Sprintf) }

// CategoryReasonWith words the reason with f (fmt.Sprintf, or the interface's translating
// Tf: the English text is the translation key), so every surface says the same.
func CategoryReasonWith(key string, w CategoryWhy, f func(string, ...any) string) string {
	switch key {
	case "game":
		return f("official: the game or a DLC")
	case "library":
		return f("%d installed Mods require it, and it ships a DLL", w.Dependents)
	case "content":
		return f("%d XML in Defs", w.Defs)
	case "code":
		return f("a DLL and no Def files")
	case "patches":
		return f("only patches: %d XML in Patches, no Defs or DLL", w.Patches)
	case "translation":
		return f("only translations: Languages, no Defs, patches or DLL")
	case "textures":
		return f("only textures: no Defs, patches, DLL or Languages")
	}
	return f("nothing to tell it by")
}
