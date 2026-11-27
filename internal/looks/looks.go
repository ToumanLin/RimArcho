// Package looks builds the looks index of a plan (21 §2): which Mods ship a texture at
// the same content path, grouped into the looks a player chooses between, with the Defs
// and styles that use each look. It reads Mod folders and its own header cache; it
// decides nothing about what is written to the game (internal/app does, 21 §4).
package looks

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/fsscan"
)

// Look kinds (21 §2.2).
const (
	KindPlain  = "plain"
	KindWorn   = "worn"
	KindFolder = "folder"
)

// All is the choice key for a whole look (21 §3.1).
const All = "all"

const maskSuffix = "_mask"

// Mod is one plan entry, in load order (Pos ascending).
type Mod struct {
	Pos      int
	ID       string // instance id
	Name     string
	Root     string   // absolute Mod folder
	Folders  []string // effective load folders, highest priority first, absolute (content.Resolve)
	Official bool     // Core or a DLC: Defs are read, textures are not (21 §2)
}

// Options controls one Build.
type Options struct {
	CachePath string // header and hash cache file (fsscan.OpenCache); "" = no cache
	Workers   int    // 0 = 4
	Progress  func(done, total int)
}

// Index is the looks of one plan.
type Index struct {
	Looks      []*Look // sorted by ID
	ByID       map[string]*Look
	Styles     map[string][]StyleRef // thing defName -> styles
	Categories []string              // all StyleCategoryDef defNames, sorted
	// Things: the thing Defs whose graphic or worn path leads to a look, by defName (21 §5).
	Things   map[string]*Thing
	Warnings []string // files that could not be read; never fatal
}

// Look is the textures that together draw one thing.
type Look struct {
	ID        string // lower-case base path
	Kind      string // plain, worn or folder
	Category  string // first three folders of the path
	Tags      []string
	Defs      []DefRef
	Styles    []LookStyle
	Providers []*Provider // load order
}

// Provider is a Mod of the plan that supplies at least one slot of a look.
type Provider struct {
	Pos   int
	Slots map[string]*File
}

// File is one texture file a provider supplies for a slot.
type File struct {
	Rel    string // relative to the Mod root, original case, forward slashes
	Tex    string // relative to the load folder's Textures/, original case, forward slashes
	Path   string // absolute
	Format string // png, jpg, psd, dds:DXT1, dds:DX10, …
	W, H   int
	Size   int64
	MTime  int64  // unix nanoseconds
	Alt    *File  // second format of the same key
	Hash   string // set only when hashed
	SameAs int    // Pos of an earlier provider with identical content, -1 if none

	ext    string // lower case, with the dot
	cached string // a hash the cache holds; Hash is set only when another provider needs the comparison
}

// DefRef is one Def field that names a look.
type DefRef struct {
	Type, Name, Label, Field, GraphicClass string
	ModPos                                 int
	Abstract                               bool
}

// LookStyle is a ThingStyleDef that uses a look, with the style categories and thing Defs
// that use the style.
type LookStyle struct {
	StyleDef              string
	Categories, ThingDefs []string
}

// Thing is a concrete ThingDef's graphic as its Def, with its parents, writes it.
type Thing struct {
	Label        string
	TexPath      string // graphicData/texPath
	GraphicClass string
	Graphic      string // the other children of graphicData, as XML
	WornPath     string // apparel/wornGraphicPath
	ModPos       int    // the Mod that defines the Def
}

// StyleRef is one style of a thing Def.
type StyleRef struct{ StyleDef, Category, LookID string }

// Win is what the game shows for a slot.
type Win struct {
	Pos      int
	File     *File
	ByChoice bool
}

// Build indexes the looks of the plan's Mods. It fails only when ctx ends.
func Build(ctx context.Context, mods []Mod, o Options) (*Index, error) {
	workers := o.Workers
	if workers <= 0 {
		workers = 4
	}
	mods = append([]Mod(nil), mods...)
	sort.SliceStable(mods, func(i, j int) bool { return mods[i].Pos < mods[j].Pos })
	var cache *fsscan.Cache
	if o.CachePath != "" {
		cache = fsscan.OpenCache(o.CachePath)
	}
	scans, err := scanMods(ctx, mods, cache, workers, o.Progress)
	if err != nil {
		return nil, err
	}
	if err := hashContested(ctx, scans, workers); err != nil {
		return nil, err
	}
	idx := &Index{ByID: map[string]*Look{}, Styles: map[string][]StyleRef{}}
	for _, s := range scans {
		idx.Warnings = append(idx.Warnings, s.warnings...)
	}
	if cache != nil {
		for i, s := range scans {
			if !mods[i].Official {
				cache.Put(mods[i].Root, cacheName, cacheStamp, s.next)
			}
		}
		if err := cache.Save(); err != nil {
			idx.Warnings = append(idx.Warnings, "looks cache: "+err.Error())
		}
	}
	facts := resolveDefs(scans)
	group(idx, scans, facts)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return idx, nil
}

// scanMods reads every Mod's textures and Defs with bounded workers.
func scanMods(ctx context.Context, mods []Mod, cache *fsscan.Cache, workers int, progress func(done, total int)) ([]*modScan, error) {
	scans := make([]*modScan, len(mods))
	var mu sync.Mutex
	done := 0
	err := parallel(ctx, len(mods), workers, func(i int) error {
		s, err := scanMod(ctx, mods[i], cache)
		if err != nil {
			return err
		}
		scans[i] = s
		if progress != nil {
			mu.Lock()
			done++
			progress(done, len(mods))
			mu.Unlock()
		}
		return nil
	})
	return scans, err
}

// parallel runs fn(0..n-1) on at most workers goroutines and stops at the first error or
// when ctx ends.
func parallel(ctx context.Context, n, workers int, fn func(i int) error) error {
	if workers > n {
		workers = n
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		next  int
		first error
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				i := next
				next++
				stop := first != nil
				mu.Unlock()
				if stop || i >= n {
					return
				}
				err := ctx.Err()
				if err == nil {
					err = fn(i)
				}
				if err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

// IsMask reports whether slot is a mask slot.
func IsMask(slot string) bool { return strings.HasSuffix(slot, maskSuffix) }

// MaskOf names the mask slot of a texture slot.
func MaskOf(slot string) string { return slot + maskSuffix }

// Identical reports whether two files are the same texture: the same file, or extension,
// size and content hash match (21 §2.1). Only files another provider also supplies with the
// same extension and size are hashed, so unhashed files are never identical.
func Identical(a, b *File) bool { return identical(a, b) }

// textureOf returns the texture slot a mask slot belongs to.
func textureOf(mask string) string { return strings.TrimSuffix(mask, maskSuffix) }

var facingRank = map[string]int{"single": 0, "south": 1, "east": 2, "north": 3, "west": 4}

// The game's body types in the order of its BodyTypeDefs.
var bodyRank = map[string]int{"male": 1, "female": 2, "thin": 3, "hulk": 4, "fat": 5, "child": 6, "baby": 7}

type slotKey struct {
	group  int // 0 no body, 1.. known bodies, 100 others
	body   string
	facing int
	name   string
	mask   bool
}

func (l *Look) slotKey(slot string) slotKey {
	k := slotKey{mask: IsMask(slot), name: textureOf(slot), facing: 100}
	if l.Kind == KindFolder {
		return k
	}
	facing := k.name
	if i := strings.LastIndexByte(k.name, '_'); i >= 0 {
		k.body, facing = k.name[:i], k.name[i+1:]
		k.group = 100
		if r, ok := bodyRank[k.body]; ok {
			k.group = r
		}
	}
	if r, ok := facingRank[facing]; ok {
		k.facing = r
	} else if k.body == "" {
		k.group = 100
	}
	return k
}

func (a slotKey) less(b slotKey) bool {
	switch {
	case a.group != b.group:
		return a.group < b.group
	case a.body != b.body:
		return a.body < b.body
	case a.facing != b.facing:
		return a.facing < b.facing
	case a.name != b.name:
		return a.name < b.name
	}
	return !a.mask && b.mask
}

// SlotNames lists the slots any provider supplies: single, the facings south, east, north,
// west, the worn body types in the game's order, then the others; a mask follows its texture.
func (l *Look) SlotNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range l.Providers {
		for s := range p.Slots {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return l.slotKey(out[i]).less(l.slotKey(out[j])) })
	return out
}

// OrderWinner returns the last provider that supplies slot: the load-order result.
func (l *Look) OrderWinner(slot string) *Provider {
	for i := len(l.Providers) - 1; i >= 0; i-- {
		if l.Providers[i].Slots[slot] != nil {
			return l.Providers[i]
		}
	}
	return nil
}

func (l *Look) provider(pos int) *Provider {
	for _, p := range l.Providers {
		if p.Pos == pos {
			return p
		}
	}
	return nil
}

// identical reports whether two providers' files are the same texture: extension, size and
// content hash match (21 §2.1).
func identical(a, b *File) bool {
	return a == b || (a != nil && b != nil && a.Hash != "" && a.Hash == b.Hash && a.Size == b.Size && a.ext == b.ext)
}

// Overridden reports whether a later provider's different content wins at least one slot
// the provider at pos supplies.
func (l *Look) Overridden(pos int) bool {
	p := l.provider(pos)
	if p == nil {
		return false
	}
	for slot, f := range p.Slots {
		if w := l.OrderWinner(slot); w != p && !identical(f, w.Slots[slot]) {
			return true
		}
	}
	return false
}

// Overriding reports whether the provider at pos wins a slot over an earlier provider's
// different content.
func (l *Look) Overriding(pos int) bool {
	p := l.provider(pos)
	if p == nil {
		return false
	}
	for slot, f := range p.Slots {
		if l.OrderWinner(slot) != p {
			continue
		}
		for _, q := range l.Providers {
			if q.Pos >= pos {
				break
			}
			if g := q.Slots[slot]; g != nil && !identical(f, g) {
				return true
			}
		}
	}
	return false
}

// InGame resolves every slot (21 §3.2). choice maps a slot name or All to a provider Pos,
// already merged by the caller (a plan's choice over the one for all plans); nil = no
// choice. A choice applies when that provider supplies the slot, otherwise the load-order
// result stands. A mask comes from the provider that wins its texture when that provider
// has the mask, otherwise from the last provider that has it.
func (l *Look) InGame(choice map[string]int) map[string]Win {
	out := map[string]Win{}
	names := l.SlotNames()
	for _, slot := range names {
		if !IsMask(slot) {
			out[slot] = l.textureWin(slot, choice)
		}
	}
	for _, slot := range names {
		if !IsMask(slot) {
			continue
		}
		if tw, ok := out[textureOf(slot)]; ok {
			if f := l.provider(tw.Pos).Slots[slot]; f != nil {
				out[slot] = Win{Pos: tw.Pos, File: f, ByChoice: tw.ByChoice}
				continue
			}
		}
		w := l.OrderWinner(slot)
		out[slot] = Win{Pos: w.Pos, File: w.Slots[slot]}
	}
	return out
}

func (l *Look) textureWin(slot string, choice map[string]int) Win {
	for _, k := range [2]string{slot, All} {
		if pos, ok := choice[k]; ok {
			if p := l.provider(pos); p != nil && p.Slots[slot] != nil {
				return Win{Pos: pos, File: p.Slots[slot], ByChoice: true}
			}
		}
	}
	w := l.OrderWinner(slot)
	return Win{Pos: w.Pos, File: w.Slots[slot]}
}

// Mixed reports whether the in-game result takes texture slots from more than one Mod.
// A look whose every texture slot one provider supplies with the content the game shows is
// not mixed (an identical copy never counts as an override, 21 §2.1). Folder looks are
// never mixed: the game merges a folder's files across Mods by design.
func (l *Look) Mixed(choice map[string]int) bool {
	if l.Kind == KindFolder {
		return false
	}
	wins := l.InGame(choice)
	first, several := -1, false
	for slot, w := range wins {
		if IsMask(slot) {
			continue
		}
		if first >= 0 && w.Pos != first {
			several = true
		}
		first = w.Pos
	}
	if !several {
		return false
	}
	for _, p := range l.Providers {
		all := true
		for slot, w := range wins {
			if !IsMask(slot) && !identical(p.Slots[slot], w.File) {
				all = false
				break
			}
		}
		if all {
			return false
		}
	}
	return true
}
