// Package corpus is the log-corpus dataset bundle (05 §4.6 "corpus data", 19 §6): the error
// patterns (internal/patterns), the observed prerequisites and combinations (internal/known)
// and the per-rule load-order judgments (internal/order) of each game version, in one
// versioned file. The program embeds one bundle (A); Archo Monitor's folders carry newer ones
// (B: RimArcho's own Monitor directory, C: a Workshop-subscribed Monitor). Per game version
// the newest usable section wins; 1.6 and 1.7 never borrow from each other.
package corpus

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Format is the bundle format this RimArcho reads. A bundle with another format is skipped;
// a higher one means "newer corpus data needs a newer RimArcho".
const Format = 1

// Dir and File locate a bundle inside a Monitor Mod folder: a folder the game never loads
// (not Defs/, not a version folder, not listed in LoadFolders.xml).
const (
	Dir  = "Corpus"
	File = "corpus.json.gz"
)

// maxBundle caps one decompressed bundle.
const maxBundle = 64 << 20

// Bundle is one versioned dataset file.
type Bundle struct {
	Format    int                 `json:"format"`
	Data      string              `json:"data"` // ISO week of the newest section, e.g. 2026-W47
	Generated string              `json:"generated,omitempty"`
	Sections  map[string]*Section `json:"sections"` // by game major.minor
}

// Section is the data of one game version.
type Section struct {
	Data     string          `json:"data"`             // ISO week this section was built
	Logs     int             `json:"logs"`             // shared logs it counts
	Window   string          `json:"window,omitempty"` // weeks counted, "2026-W22..2026-W47"
	Patterns json.RawMessage `json:"patterns,omitempty"`
	Known    json.RawMessage `json:"known,omitempty"`
	Observed json.RawMessage `json:"observed,omitempty"`
}

var weekRe = regexp.MustCompile(`^[0-9]{4}-W[0-9]{2}$`)

// ValidWeek reports whether s is an ISO week such as 2026-W47 (zero-padded, so weeks order
// as strings).
func ValidWeek(s string) bool { return weekRe.MatchString(s) }

// Week is the ISO week of t.
func Week(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// ShortWeek is the label form of a week: "W47".
func ShortWeek(s string) string {
	if i := strings.Index(s, "-W"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Parse reads a bundle (gzip or plain JSON) and checks its envelope. A bundle of another
// format returns ErrFormat with the format set, so callers can tell newer from broken.
func Parse(b []byte) (*Bundle, error) {
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(zr, maxBundle+1))
		if err != nil {
			return nil, err
		}
		if len(raw) > maxBundle {
			return nil, errors.New("corpus bundle is larger than 64 MB")
		}
		b = raw
	}
	var head struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return nil, err
	}
	if head.Format != Format {
		return &Bundle{Format: head.Format}, &FormatError{Format: head.Format}
	}
	var bu Bundle
	if err := json.Unmarshal(b, &bu); err != nil {
		return nil, err
	}
	if !ValidWeek(bu.Data) {
		return nil, fmt.Errorf("corpus bundle: data %q is not an ISO week", bu.Data)
	}
	for g, s := range bu.Sections {
		if s == nil || !ValidWeek(s.Data) || s.Data > bu.Data {
			return nil, fmt.Errorf("corpus bundle: section %s has no usable data version", g)
		}
	}
	return &bu, nil
}

// FormatError is a bundle in a format this RimArcho does not read.
type FormatError struct{ Format int }

func (e *FormatError) Error() string {
	if e.Format > Format {
		return fmt.Sprintf("corpus format %d is newer than this RimArcho reads (%d)", e.Format, Format)
	}
	return fmt.Sprintf("corpus format %d is not readable (this RimArcho reads %d)", e.Format, Format)
}

// Encode writes a bundle as gzip-compressed JSON.
func Encode(b *Bundle) ([]byte, error) {
	if b.Format == 0 {
		b.Format = Format
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Origins of a candidate bundle.
const (
	Embedded = "embedded" // A: inside this program
	Monitor  = "monitor"  // B: RimArcho's own Monitor directory
	Workshop = "workshop" // C: a Workshop-subscribed Monitor
)

// Candidate is one bundle RimArcho can read.
type Candidate struct {
	Origin string // Embedded, Monitor or Workshop
	Label  string // the Monitor folder name, or "" for the embedded bundle
	Path   string
	Bundle *Bundle
	Err    error
}

// ReadFolder reads the bundle of a Monitor Mod folder; ok is false when it has none.
func ReadFolder(origin, modDir string) (c Candidate, ok bool) {
	p := filepath.Join(modDir, Dir, File)
	b, err := os.ReadFile(p)
	if err != nil {
		return Candidate{}, false
	}
	c = Candidate{Origin: origin, Label: filepath.Base(modDir), Path: p}
	c.Bundle, c.Err = Parse(b)
	return c, true
}

// Choice is the data one game version uses.
type Choice struct {
	Game    string
	Data    string // the section's ISO week; "" when no bundle has this game version
	Origin  string
	Label   string
	Logs    int
	Section *Section
	// NewerFormat: some bundle has a format above Format ("newer corpus data needs a newer
	// RimArcho").
	NewerFormat bool
	Skipped     []string // bundles left out, with the reason
}

// Choose picks, for one game version, the newest section among the usable candidates. A tie
// keeps the earlier candidate (the order given: embedded, then Monitor folders, then Workshop).
func Choose(cands []Candidate, game string) Choice {
	ch := Choice{Game: game}
	for _, c := range cands {
		var fe *FormatError
		if errors.As(c.Err, &fe) {
			if fe.Format > Format {
				ch.NewerFormat = true
			}
			ch.Skipped = append(ch.Skipped, fmt.Sprintf("%s: %v", c.name(), c.Err))
			continue
		}
		if c.Err != nil || c.Bundle == nil {
			ch.Skipped = append(ch.Skipped, fmt.Sprintf("%s: %v", c.name(), c.Err))
			continue
		}
		s := c.Bundle.Sections[game]
		if s == nil || s.Data <= ch.Data {
			continue
		}
		ch.Data, ch.Origin, ch.Label, ch.Logs, ch.Section = s.Data, c.Origin, c.Label, s.Logs, s
	}
	sort.Strings(ch.Skipped)
	return ch
}

func (c Candidate) name() string {
	if c.Label != "" {
		return c.Origin + " " + c.Label
	}
	return c.Origin
}

// String is the brief source line: "corpus W47, 1.7", "no corpus data for 1.7".
func (ch Choice) String() string {
	if ch.Data == "" {
		return "no corpus data for " + ch.Game
	}
	return "corpus " + ShortWeek(ch.Data) + ", " + ch.Game
}

// From names where the chosen section came from: "built in", "Archo Monitor folder
// ArchoMonitor-0.12.0", "Workshop Archo Monitor".
func (ch Choice) From() string {
	switch ch.Origin {
	case Embedded:
		return "built in"
	case Monitor:
		return "Archo Monitor folder " + ch.Label
	case Workshop:
		return "Workshop Archo Monitor"
	}
	return ""
}
