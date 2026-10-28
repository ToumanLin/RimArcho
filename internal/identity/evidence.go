package identity

import (
	"slices"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/zeebo/xxh3"
)

// ValidWorkshopID excludes empty IDs, placeholders such as 0, and folder names.
func ValidWorkshopID(id string) bool {
	if id == "" || id == "0" {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(id, 10, 64)
	return err == nil && n > 0
}

// FileEvidence distinguishes shared paths from shared content. Payload counts exclude
// About/, routing metadata, empty files and repeated hashes (02 §5.1).
type FileEvidence struct {
	Paths, Contents, Changed          int
	PayloadA, PayloadB, PayloadCommon int
	PayloadSimilarity                 float64
	Examples                          []string // at most eight new-side paths with shared payload content
}

func CompareFiles(a, b scan.SemanticSet) FileEvidence {
	e := FileEvidence{}
	for p, h := range a {
		if other, ok := b[p]; ok {
			e.Paths++
			if h != other {
				e.Changed++
			}
		}
	}
	counts := map[scan.Hash]int{}
	for _, h := range a {
		counts[h]++
	}
	for _, h := range b {
		if counts[h] > 0 {
			e.Contents++
			counts[h]--
		}
	}
	pa, pb := payload(a), payload(b)
	e.PayloadA, e.PayloadB = len(pa), len(pb)
	var paths []string
	for h, p := range pa {
		if _, ok := pb[h]; ok {
			e.PayloadCommon++
			paths = append(paths, p)
		}
	}
	if union := len(pa) + len(pb) - e.PayloadCommon; union > 0 {
		e.PayloadSimilarity = float64(e.PayloadCommon) / float64(union)
	}
	slices.Sort(paths)
	e.Examples = paths[:min(8, len(paths))]
	return e
}

func payload(files scan.SemanticSet) map[scan.Hash]string {
	out := map[scan.Hash]string{}
	empty := xxh3.Hash128(nil).Bytes()
	for p, h := range files {
		l := strings.ToLower(p)
		if h == empty || h == (scan.Hash{}) || strings.HasPrefix(l, "about/") || l == "loadfolders.xml" || !scan.IsSemantic(l) {
			continue
		}
		if old, ok := out[h]; !ok || p < old {
			out[h] = p
		}
	}
	return out
}

// QuestionScore is deliberately stricter than move/copy matching: without a shared
// declaration, paths alone cannot establish identity. Three distinct nonempty payload
// contents and a majority overlap are required; with a declaration, one suffices.
func QuestionScore(pkgA, wsA string, a scan.SemanticSet, pkgB, wsB string, b scan.SemanticSet) float64 {
	return questionScore(SameDecl(pkgA, wsA, pkgB, wsB), payload(a), payload(b))
}

func questionScore(sameDecl bool, a, b map[scan.Hash]string) float64 {
	minCommon := 3
	if sameDecl {
		minCommon = 1
	}
	if len(a) < minCommon || len(b) < minCommon {
		return 0
	}
	common := 0
	if len(a) > len(b) {
		a, b = b, a
	}
	for h := range a {
		if _, ok := b[h]; ok {
			common++
		}
	}
	if common < minCommon {
		return 0
	}
	score := float64(common) / float64(len(a)+len(b)-common)
	if score < QuestionSimilarity {
		return 0
	}
	return score
}
