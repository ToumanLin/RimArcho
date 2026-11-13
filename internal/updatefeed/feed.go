// Package updatefeed is an unwired experiment in authenticated update metadata.
// It never installs archives or reads game, Workshop, or application settings.
package updatefeed

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxEnvelope = 96 << 10
const MaxArchive = 200 << 20
const signatureDomain = "RimArcho update feed v1\n"

// Envelope signs the decoded Payload bytes, prefixed by signatureDomain and KeyID + newline.
// Payload and Signature use padded standard base64. No JSON canonicalization is needed.
type Envelope struct {
	KeyID     string `json:"key_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type Manifest struct {
	Format   int       `json:"format"`
	Sequence uint64    `json:"sequence"`
	Issued   time.Time `json:"issued"`
	Expires  time.Time `json:"expires"`
	Version  string    `json:"version"`
	Artifact Artifact  `json:"artifact"`
	Sources  []Source  `json:"sources"`
}

type Artifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Source struct {
	UserAgent string   `json:"user_agent,omitempty"`
	Name      string   `json:"name"`
	Hosts     []string `json:"hosts"`
	Steps     []Step   `json:"steps"`
	Download  string   `json:"download"`
	Challenge string   `json:"challenge,omitempty"`
}

// Step has no general branches, loops, script evaluation, filesystem or process access.
// lanzou-list is a fixed, bounded folder-pagination operation.
// See the research note for the operation and substitution contracts.
type Step struct {
	Op      string            `json:"op"`
	Into    string            `json:"into,omitempty"`
	Input   string            `json:"input,omitempty"`
	Value   string            `json:"value,omitempty"`
	Pattern string            `json:"pattern,omitempty"`
	Path    string            `json:"path,omitempty"`
	Field   string            `json:"field,omitempty"`
	Match   string            `json:"match,omitempty"`
	Equal   string            `json:"equal,omitempty"`
	URL     string            `json:"url,omitempty"`
	Base    string            `json:"base,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Form    map[string]string `json:"form,omitempty"`
}

// Feed keeps the authenticated recipe private so callers cannot accidentally mutate it.
type Feed struct{ manifest Manifest }

func (f *Feed) Artifact() Artifact { return f.manifest.Artifact }
func (f *Feed) Version() string    { return f.manifest.Version }
func (f *Feed) Sequence() uint64   { return f.manifest.Sequence }
func (f *Feed) SourceNames() []string {
	names := make([]string, len(f.manifest.Sources))
	for i, s := range f.manifest.Sources {
		names[i] = s.Name
	}
	return names
}

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var identifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,39}$`)

// Parse authenticates before interpreting payload fields. now must be supplied by
// the caller; minimumSequence is the persisted high-water mark (zero on first use).
func Parse(r io.Reader, keys map[string]ed25519.PublicKey, now time.Time, minimumSequence uint64) (*Feed, error) {
	b, err := limited(r, MaxEnvelope)
	if err != nil {
		return nil, err
	}
	var e Envelope
	if err := strictJSON(b, &e); err != nil {
		return nil, fmt.Errorf("feed envelope: %w", err)
	}
	key := keys[e.KeyID]
	if !identifier.MatchString(e.KeyID) || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("feed signing key is unknown")
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(e.Payload)
	if err != nil || len(payload) > 64<<10 {
		return nil, errors.New("feed payload is invalid or exceeds 64 KiB")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(key, append([]byte(signatureDomain+e.KeyID+"\n"), payload...), sig) {
		return nil, errors.New("feed signature is invalid")
	}
	var m Manifest
	if err := strictJSON(payload, &m); err != nil {
		return nil, fmt.Errorf("feed payload: %w", err)
	}
	if m.Format != 1 {
		return nil, errors.New("unsupported feed format")
	}
	if now.IsZero() || m.Sequence == 0 || m.Sequence < minimumSequence || m.Issued.IsZero() || m.Issued.After(now.Add(5*time.Minute)) || !m.Expires.After(now) || !m.Expires.After(m.Issued) || m.Expires.Sub(m.Issued) > 90*24*time.Hour {
		return nil, errors.New("feed is expired, from the future, or older than the accepted sequence")
	}
	a := m.Artifact
	digest, err := hex.DecodeString(a.SHA256)
	if !stableVersion.MatchString(m.Version) || len(m.Version) > 64 || a.Name != "archo-"+m.Version+"-windows-amd64.zip" || a.Size <= 0 || a.Size > MaxArchive || err != nil || len(digest) != 32 || a.SHA256 != strings.ToLower(a.SHA256) {
		return nil, errors.New("feed needs a stable Windows amd64 zip with a size and lowercase SHA-256")
	}
	if len(m.Sources) == 0 || len(m.Sources) > 4 {
		return nil, errors.New("feed must contain one to four sources")
	}
	names := map[string]bool{}
	for _, s := range m.Sources {
		if !identifier.MatchString(s.Name) || names[s.Name] {
			return nil, errors.New("source names must be unique identifiers")
		}
		names[s.Name] = true
		if err := validateSource(s); err != nil {
			return nil, fmt.Errorf("source %s: %w", s.Name, err)
		}
	}
	return &Feed{manifest: m}, nil
}

func limited(r io.Reader, n int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, n+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > n {
		return nil, errors.New("response exceeds byte limit")
	}
	return b, nil
}

// Reject duplicate keys at every depth as well as unknown fields and trailing JSON.
func strictJSON(b []byte, dest any) error {
	if !utf8.Valid(b) {
		return errors.New("JSON must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := uniqueValue(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(dest)
}

func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON nesting exceeds 32")
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			k, ok := key.(string)
			k = strings.ToLower(k)
			if !ok || seen[k] {
				return errors.New("duplicate JSON key")
			}
			seen[k] = true
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}
