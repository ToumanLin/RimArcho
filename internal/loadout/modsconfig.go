package loadout

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeebo/xxh3"
)

// ModsConfig is the parsed Config/ModsConfig.xml together with its raw bytes, so a rewrite
// can keep every element RimArcho does not own (knownExpansions, unknown elements).
type ModsConfig struct {
	Raw     []byte
	Version string
	Active  []string
	Known   []string
}

// Hash fingerprints file contents for external-modification detection (04 §2.3).
func Hash(b []byte) []byte {
	h := xxh3.Hash128(b).Bytes()
	return h[:]
}

var bom = []byte{0xEF, 0xBB, 0xBF}

// ReadModsConfig parses a ModsConfig.xml.
func ReadModsConfig(path string) (*ModsConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var x struct {
		Version string   `xml:"version"`
		Active  []string `xml:"activeMods>li"`
		Known   []string `xml:"knownExpansions>li"`
	}
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, bom)))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := d.Decode(&x); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	mc := &ModsConfig{Raw: raw, Version: strings.TrimSpace(x.Version), Known: x.Known}
	for _, a := range x.Active {
		if a = strings.TrimSpace(a); a != "" {
			mc.Active = append(mc.Active, strings.ToLower(a))
		}
	}
	return mc, nil
}

// span is a byte range of the original document.
type span struct{ start, end int }

// locate finds, at depth 1 under the root element, the inner text of <version> and the
// whole <activeMods> element, plus the start of the root's end tag.
func locate(raw []byte) (version, active span, rootEnd int, err error) {
	version, active, rootEnd = span{-1, -1}, span{-1, -1}, -1
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	depth := 0
	prev := 0
	var cur string
	var curStart int
	for {
		t, terr := d.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return version, active, rootEnd, terr
		}
		off := int(d.InputOffset())
		switch t := t.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				cur, curStart = strings.ToLower(t.Name.Local), prev
				if cur == "version" {
					version.start = off
				}
			}
		case xml.EndElement:
			if depth == 2 {
				switch cur {
				case "version":
					version.end = prev
				case "activemods":
					active = span{curStart, off}
				}
			}
			if depth == 1 {
				rootEnd = prev
			}
			depth--
		}
		prev = off
	}
	if rootEnd < 0 {
		return version, active, rootEnd, errors.New("ModsConfig.xml: no root element")
	}
	return version, active, rootEnd, nil
}

// Render produces the new file: the original document with <version> and <activeMods>
// replaced. With no original, a document in the game's own layout is generated.
func Render(orig []byte, version string, active, knownExpansions []string) ([]byte, error) {
	if len(bytes.TrimSpace(orig)) == 0 {
		var b strings.Builder
		b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModsConfigData>\n")
		b.WriteString("  <version>" + escape(version) + "</version>\n")
		b.WriteString(activeBlock(active, "  ", "\n") + "\n")
		b.WriteString("  <knownExpansions>\n")
		for _, k := range knownExpansions {
			b.WriteString("    <li>" + escape(k) + "</li>\n")
		}
		b.WriteString("  </knownExpansions>\n</ModsConfigData>\n")
		return []byte(b.String()), nil
	}
	hasBOM := bytes.HasPrefix(orig, bom)
	body := bytes.TrimPrefix(orig, bom)
	ver, act, rootEnd, err := locate(body)
	if err != nil {
		return nil, err
	}
	nl := "\n"
	if bytes.Contains(body, []byte("\r\n")) {
		nl = "\r\n"
	}
	type edit struct {
		s    span
		text string
	}
	var edits []edit
	if act.start >= 0 {
		edits = append(edits, edit{act, activeBlock(active, indentAt(body, act.start), nl)})
	}
	if ver.start >= 0 && ver.end >= ver.start {
		edits = append(edits, edit{ver, escape(version)})
	}
	var insert string
	if ver.start < 0 {
		insert += "  <version>" + escape(version) + "</version>" + nl
	}
	if act.start < 0 {
		insert += activeBlock(active, "  ", nl) + nl
	}
	if insert != "" {
		edits = append(edits, edit{span{rootEnd, rootEnd}, insert})
	}
	// Apply from the end so earlier offsets stay valid.
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			if edits[j].s.start > edits[i].s.start {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	out := append([]byte(nil), body...)
	for _, e := range edits {
		out = append(out[:e.s.start:e.s.start], append([]byte(e.text), body[e.s.end:]...)...)
		body = out
	}
	if hasBOM {
		out = append(append([]byte(nil), bom...), out...)
	}
	return out, nil
}

func indentAt(b []byte, pos int) string {
	i := bytes.LastIndexByte(b[:pos], '\n')
	ind := b[i+1 : pos]
	if len(bytes.TrimLeft(ind, " \t")) != 0 {
		return "  "
	}
	return string(ind)
}

func activeBlock(active []string, indent, nl string) string {
	var b strings.Builder
	b.WriteString("<activeMods>" + nl)
	for _, a := range active {
		b.WriteString(indent + "  <li>" + escape(a) + "</li>" + nl)
	}
	b.WriteString(indent + "</activeMods>")
	return b.String()
}

func escape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// BackupSuffix is appended to ModsConfig.xml for the single backup (04 §2.2 rule 1).
const BackupSuffix = ".rimarcho.bak"

// WriteFile backs up the current file (if any) and atomically replaces it: write to a
// temporary file in the same directory, sync, then rename over the original.
func WriteFile(path string, data []byte) (backup string, err error) {
	if old, err := os.ReadFile(path); err == nil {
		backup = path + BackupSuffix
		if err := writeSync(backup, old); err != nil {
			return "", fmt.Errorf("backup: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return backup, err
	}
	tmp := path + ".rimarcho.tmp"
	if err := writeSync(tmp, data); err != nil {
		os.Remove(tmp)
		return backup, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return backup, err
	}
	return backup, nil
}

func writeSync(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
