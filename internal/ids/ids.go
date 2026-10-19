// Package ids mints RimArcho's internal identifiers and renders their short references.
//
// IDs are 128-bit random values written as lower-case base32 without padding (02 §7).
package ids

import (
	"encoding/base32"
	"io"
	"sort"
	"strings"
)

var enc = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// New reads 16 bytes from r and returns a 26-character ID.
func New(r io.Reader) string {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		panic("ids: random source failed: " + err.Error())
	}
	return enc.EncodeToString(b[:])
}

// MinPrefix is the shortest prefix length a reference may use.
const MinPrefix = 4

// Shortener maps full IDs to their shortest unique prefix (at least MinPrefix characters)
// within one ID space, e.g. all instances of an environment.
type Shortener struct {
	sorted []string
	short  map[string]string
}

func NewShortener(all []string) *Shortener {
	s := &Shortener{sorted: append([]string(nil), all...), short: make(map[string]string, len(all))}
	sort.Strings(s.sorted)
	for i, id := range s.sorted {
		n := MinPrefix
		if i > 0 {
			n = max(n, lcp(id, s.sorted[i-1])+1)
		}
		if i+1 < len(s.sorted) {
			n = max(n, lcp(id, s.sorted[i+1])+1)
		}
		s.short[id] = id[:min(n, len(id))]
	}
	return s
}

func lcp(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// Short returns the shortest unique prefix of id (its first 8 characters if unknown).
func (s *Shortener) Short(id string) string {
	if v, ok := s.short[id]; ok {
		return v
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Lookup returns every known ID starting with prefix.
func (s *Shortener) Lookup(prefix string) []string {
	prefix = strings.ToLower(prefix)
	i := sort.SearchStrings(s.sorted, prefix)
	var out []string
	for ; i < len(s.sorted) && strings.HasPrefix(s.sorted[i], prefix); i++ {
		out = append(out, s.sorted[i])
	}
	return out
}

// Valid reports whether s could be (a prefix of) an ID.
func Valid(s string) bool {
	if len(s) < MinPrefix || len(s) > 26 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}
