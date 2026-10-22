package textures

import (
	"slices"
	"sort"
)

// ScaleSteps are the scales offered (percent); 100 is none.
var ScaleSteps = []int{20, 25, 40, 50, 60, 75, 80, 100}

// DefaultMinSide keeps small textures readable: scaling never takes the shorter side below it.
const DefaultMinSide = 128

// Options decide how textures are encoded.
type Options struct {
	Scale   int  // percent; 0 or 100: full size
	MinSide int  // 0: DefaultMinSide
	NoMips  bool // no mipmaps (not recommended: the game then has none for compressed textures)
}

// Normal returns o with its defaults filled in.
func (o Options) Normal() Options {
	if o.Scale <= 0 || o.Scale > 100 {
		o.Scale = 100
	}
	if o.MinSide <= 0 {
		o.MinSide = DefaultMinSide
	}
	return o
}

// ScaleFor is the scale a source is encoded at: masks stay full size; a scale must keep
// the shorter side at MinSide or more and both sides a multiple of 4 (block-compressed
// textures are made of 4×4 blocks; todds rounds the source up to 4 first, then scales
// without rounding again, and 1.6's Unity may refuse other sizes), else the next larger
// step is taken, or full size.
func (o Options) ScaleFor(s *Source) int {
	o = o.Normal()
	if o.Scale == 100 || s.Mask {
		return 100
	}
	w, h := fix4(s.W), fix4(s.H)
	for _, st := range ScaleSteps {
		if st < o.Scale {
			continue
		}
		if st == 100 {
			return 100
		}
		sw, sh := w*st/100, h*st/100
		if min(sw, sh) >= o.MinSide && sw%4 == 0 && sh%4 == 0 && w*st%100 == 0 && h*st%100 == 0 {
			return st
		}
	}
	return 100
}

// Skip reasons for a texture key the encoder leaves alone.
const (
	SkipOwnDDS   = "dds"   // the mod (or another tool) already has a .dds for it
	SkipOtherFmt = "other" // a .jpg, .psd or a .png that is not a PNG shares its name
	SkipFresh    = "fresh" // RimArcho's DDS is there and its PNG has not changed
)

// Job is one source to encode, at a scale.
type Job struct {
	*Source
	Scale int
}

// Plan is what an encoder run does to a Set.
type Plan struct {
	Jobs    []Job
	Skipped map[string]int // reason → number of PNG sources
	Fresh   []*Source      // sources whose DDS is RimArcho's and up to date
	Est     Estimate       // of Jobs
}

// Owned tells the planner about DDS files RimArcho made: ours reports whether a .dds is
// one of them; fresh whether the DDS of a source is up to date (made from this PNG, with
// these options).
type Owned struct {
	Ours  func(ddsPath string) bool
	Fresh func(s *Source) bool
}

// MakePlan decides, key by key, what to encode (docs/research §2.3): a .dds shadows the
// texture of the same key in every load folder, so a key is converted as a whole (each of
// its PNGs, in every folder) or not at all. Keys with a .dds of their own, or with a file
// the encoder cannot take, are left as they are. force re-encodes fresh ones too.
func MakePlan(set *Set, o Options, own Owned, force bool) *Plan {
	o = o.Normal()
	p := &Plan{Skipped: map[string]int{}}
	byKey := map[string][]*Source{}
	var keys []string
	for _, s := range set.Sources {
		if _, ok := byKey[s.Key]; !ok {
			keys = append(keys, s.Key)
		}
		byKey[s.Key] = append(byKey[s.Key], s)
	}
	foreign, other := map[string]bool{}, map[string]bool{}
	for _, x := range set.Others {
		switch {
		case x.Ext == ".dds":
			if own.Ours == nil || !own.Ours(x.Path) {
				foreign[x.Key] = true
			}
		default:
			other[x.Key] = true
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		srcs := byKey[k]
		switch {
		case foreign[k]:
			p.Skipped[SkipOwnDDS] += len(srcs)
			continue
		case other[k]:
			p.Skipped[SkipOtherFmt] += len(srcs)
			continue
		}
		allFresh := !force && own.Fresh != nil
		if allFresh {
			for _, s := range srcs {
				if !own.Fresh(s) {
					allFresh = false
					break
				}
			}
		}
		if allFresh {
			p.Skipped[SkipFresh] += len(srcs)
			p.Fresh = append(p.Fresh, srcs...)
			continue
		}
		for _, s := range srcs {
			j := Job{Source: s, Scale: o.ScaleFor(s)}
			p.Jobs = append(p.Jobs, j)
			p.Est.Add(j, o)
		}
	}
	return p
}

// Keys are the content keys of jobs (a key whose job failed must lose all its DDS).
func Keys(jobs []Job) []string {
	var out []string
	for _, j := range jobs {
		if !slices.Contains(out, j.Key) {
			out = append(out, j.Key)
		}
	}
	return out
}

// Estimate is what encoding some textures is expected to cost and save.
type Estimate struct {
	Files      int
	PNGBytes   int64 // the PNGs on disk
	DDSBytes   int64 // the DDS files expected
	VRAMBefore int64 // what the game makes of the PNGs: one byte a pixel (BC7 in 1.7, DXT5 in 1.6), with mipmaps
	VRAMAfter  int64 // the DDS: BC1 (half a byte a pixel) when opaque, BC7 with alpha
}

// Add counts one job.
func (e *Estimate) Add(j Job, o Options) {
	e.Files++
	e.PNGBytes += j.Size
	dds := DDSSize(j.W, j.H, j.Alpha, j.Scale, !o.NoMips)
	e.DDSBytes += dds
	e.VRAMAfter += dds
	e.VRAMBefore += blockBytes(fix4(j.W), fix4(j.H), 16, true)
}

// Plus adds another estimate.
func (e *Estimate) Plus(o Estimate) {
	e.Files += o.Files
	e.PNGBytes += o.PNGBytes
	e.DDSBytes += o.DDSBytes
	e.VRAMBefore += o.VRAMBefore
	e.VRAMAfter += o.VRAMAfter
}

// DDSSize is the size of the DDS todds writes for a w×h PNG at scale percent: the size
// made a multiple of 4 (-fs), then scaled, BC7 (16 bytes a 4×4 block) with alpha, else BC1
// (8), with the mip chain, and the header (with the DX10 extension for BC7).
func DDSSize(w, h int, alpha bool, scale int, mips bool) int64 {
	w, h = fix4(w), fix4(h)
	if scale > 0 && scale < 100 {
		w, h = max(1, w*scale/100), max(1, h*scale/100)
	}
	block, head := int64(8), int64(128)
	if alpha {
		block, head = 16, 148
	}
	return head + blockBytes(w, h, block, mips)
}

func fix4(n int) int { return (n + 3) / 4 * 4 }

func blockBytes(w, h int, block int64, mips bool) int64 {
	var n int64
	for {
		bw, bh := max(1, (w+3)/4), max(1, (h+3)/4)
		n += int64(bw) * int64(bh) * block
		if !mips || (w <= 1 && h <= 1) {
			return n
		}
		w, h = max(1, w/2), max(1, h/2)
	}
}
