package source

import "strings"

// Rebinding thresholds (02 §3.2).
const (
	RebindSamples    = 20
	RebindSimilarity = 0.9
	RebindRate       = 0.8
)

// SemanticSet maps a lower-cased relative file path to its content hash.
type SemanticSet = map[string][16]byte

// RebindSample is one sampled directory of the candidate root.
type RebindSample struct {
	Name       string
	Similarity float64 // -1 when no old instance has this relative path
	Matched    bool
}

// RebindResult reports whether a new root is the same source as an old one.
type RebindResult struct {
	Samples []RebindSample
	Matched int
	Rate    float64
	Accept  bool
}

// EvaluateRebind samples up to RebindSamples mod folders under newRoot and compares their
// semantic fingerprints to the old source's instances with the same relative path
// (old is keyed by lower-cased directory name). Sampling is evenly spaced over the sorted
// directory list rather than random, so the outcome is reproducible.
func EvaluateRebind(newRoot string, old map[string]SemanticSet,
	fingerprint func(dir string) (SemanticSet, error), similarity func(a, b SemanticSet) float64) RebindResult {
	var r RebindResult
	entries, _, err := List(newRoot)
	if err != nil {
		return r
	}
	var mods []Entry
	for _, e := range entries {
		if HasAbout(e.Path) {
			mods = append(mods, e)
		}
	}
	pick := mods
	if len(mods) > RebindSamples {
		pick = make([]Entry, 0, RebindSamples)
		for i := 0; i < RebindSamples; i++ {
			pick = append(pick, mods[i*len(mods)/RebindSamples])
		}
	}
	for _, e := range pick {
		s := RebindSample{Name: e.Name, Similarity: -1}
		if prev, ok := old[strings.ToLower(e.Name)]; ok {
			if cur, err := fingerprint(e.Path); err == nil {
				s.Similarity = similarity(prev, cur)
				s.Matched = s.Similarity >= RebindSimilarity
			}
		}
		if s.Matched {
			r.Matched++
		}
		r.Samples = append(r.Samples, s)
	}
	switch {
	case len(old) == 0:
		r.Rate, r.Accept = 1, true // nothing to lose: the old source held no instances
	case len(r.Samples) > 0:
		r.Rate = float64(r.Matched) / float64(len(r.Samples))
		r.Accept = r.Rate >= RebindRate
	}
	return r
}
