package app

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/textenc"
)

// Mod list formats of other tools (10 §4): RimSort (ModsConfig XML, JSON, clipboard text,
// Rentry), RimWorld's own saved list (.rml, which RimCrow also writes), RimCrow's
// ModList.xml, JSON and share code, and lists of workshop IDs. Detected by content.

// More export formats.
const (
	FormatRimSort = "rimsort" // = ModsConfig XML
	FormatRML     = "rml"
	FormatRimCrow = "rimcrow" // share code
	// FormatRimCrowXML is RimCrow's ModList.xml.
	FormatRimCrowXML = "rimcrow-xml"
)

// ParseList reads a list in any supported format. Tokens are lower-case packageIds (with
// _steam kept); entries known only by workshop ID are in WorkshopOnly.
func ParseList(b []byte) (*ImportedList, error) {
	b = textenc.UTF8(b)
	t := bytes.TrimSpace(b)
	switch {
	case shareCodeRe.Match(t):
		return parseShareCode(string(t))
	case bytes.HasPrefix(t, []byte("{")) || bytes.HasPrefix(t, []byte("[")):
		return parseJSONList(t)
	case bytes.Contains(t[:min(len(t), 4096)], []byte("<savegame")):
		return readSaveMeta(bytes.NewReader(t))
	case bytes.HasPrefix(t, []byte("<")):
		return parseXMLList(t)
	}
	return parseTextList(string(t)), nil
}

func newList(format string) *ImportedList {
	return &ImportedList{Format: format, Names: map[string]string{}, Workshop: map[string]string{}}
}

func (l *ImportedList) add(pkg, name, ws string) {
	pkg = strings.ToLower(strings.TrimSpace(pkg))
	ws = strings.TrimSpace(ws)
	if ws == "0" {
		ws = ""
	}
	if pkg == "" {
		if ws != "" {
			l.WorkshopOnly = append(l.WorkshopOnly, ws)
		}
		return
	}
	for _, t := range l.Tokens {
		if t == pkg {
			return
		}
	}
	l.Tokens = append(l.Tokens, pkg)
	if name = strings.TrimSpace(name); name != "" {
		l.Names[pkg] = name
	}
	if ws != "" {
		l.Workshop[pkg] = ws
	}
}

// ---- XML: ModsConfig, .rml, RimCrow ModList.xml ----

// xmlLists collects the <li> texts of an XML document by their parent path
// ("savedModList/meta/modIds") and the text of leaf elements.
func xmlLists(b []byte) (lists map[string][]string, leaf map[string]string, root string, err error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	lists, leaf = map[string][]string{}, map[string]string{}
	var path []string
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root == "" {
				root = t.Name.Local
			}
			path = append(path, t.Name.Local)
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if len(path) == 0 {
				continue
			}
			s := strings.TrimSpace(text.String())
			if path[len(path)-1] == "li" && len(path) >= 2 {
				parent := strings.Join(path[:len(path)-1], "/")
				lists[parent] = append(lists[parent], s)
			} else if s != "" {
				leaf[strings.Join(path, "/")] = s
			}
			path = path[:len(path)-1]
			text.Reset()
		}
	}
	return lists, leaf, root, nil
}

func parseXMLList(b []byte) (*ImportedList, error) {
	lists, leaf, root, err := xmlLists(b)
	if err != nil {
		return nil, err
	}
	at := func(i int, l []string) string {
		if i < len(l) {
			return l[i]
		}
		return ""
	}
	switch root {
	case "ModsConfigData":
		l := newList(FormatModsConfig)
		l.Game = leaf["ModsConfigData/version"]
		for _, id := range lists["ModsConfigData/activeMods"] {
			l.add(id, "", "")
		}
		return l, nil
	case "savedModList":
		l := newList(FormatRML)
		l.Game = leaf["savedModList/meta/gameVersion"]
		ws := lists["savedModList/meta/modSteamIds"]
		ids, names := lists["savedModList/modList/ids"], lists["savedModList/modList/names"]
		if len(ids) == 0 {
			ids, names = lists["savedModList/meta/modIds"], lists["savedModList/meta/modNames"]
		}
		metaIDs := lists["savedModList/meta/modIds"]
		for i, id := range ids {
			w := ""
			for j, m := range metaIDs {
				if strings.EqualFold(m, id) {
					w = at(j, ws)
				}
			}
			l.add(id, at(i, names), w)
		}
		return l, nil
	case "ModList":
		l := newList("rimcrow-modlist")
		ids, names, ws := lists["ModList/modIds"], lists["ModList/modNames"], lists["ModList/modSteamWorkshopIds"]
		for i, id := range ids {
			l.add(id, at(i, names), at(i, ws))
		}
		return l, nil
	}
	return nil, fmt.Errorf("unknown XML list <%s> (ModsConfigData, savedModList, ModList or a save)", root)
}

// ---- JSON: RimSort and RimCrow ----

func parseJSONList(b []byte) (*ImportedList, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("not a JSON list: %v", err)
	}
	str := func(x any) string {
		switch y := x.(type) {
		case string:
			return y
		case float64:
			return fmt.Sprintf("%.0f", y)
		}
		return ""
	}
	item := func(l *ImportedList, x any) {
		switch y := x.(type) {
		case string:
			l.add(y, "", "")
		case map[string]any:
			pick := func(keys ...string) string {
				for _, k := range keys {
					if s := str(y[k]); s != "" {
						return s
					}
				}
				return ""
			}
			l.add(pick("packageId", "package_id", "packageid", "id"), pick("name"), pick("workshopId", "workshop_id", "publishedFileId"))
		}
	}
	switch y := v.(type) {
	case []any:
		l := newList("rimcrow-json")
		for _, x := range y {
			item(l, x)
		}
		return l, nil
	case map[string]any:
		if y["format"] == "rimcrow_json" {
			l := newList("rimcrow-json")
			if ml, ok := y["modlist"].([]any); ok && len(ml) > 0 {
				for _, x := range ml {
					item(l, x)
				}
			} else if ids, ok := y["package_ids"].([]any); ok {
				for _, x := range ids {
					item(l, x)
				}
			}
			return l, nil
		}
		l := newList("rimsort-json")
		l.Game = str(y["version"])
		for _, k := range []string{"activeMods", "active_mods", "mods", "mod_list", "modList"} {
			if xs, ok := y[k].([]any); ok {
				for _, x := range xs {
					item(l, x)
				}
				return l, nil
			}
		}
		return nil, errors.New("JSON list without activeMods or mods")
	}
	return nil, errors.New("JSON list is neither an object nor an array")
}

// ---- text: packageIds, RimSort clipboard and Rentry, workshop IDs ----

var (
	clipboardRe  = regexp.MustCompile(`^(.*?)\s*\[([A-Za-z0-9_.\-]+)\]\[([^\]]*)\]\s*$`)
	rentryPkgRe  = regexp.MustCompile(`(?i)\{packageid:\s*([\w.\-]+)\}|packageid:\s*([\w.\-]+)`)
	wsIDRe       = regexp.MustCompile(`[?&]id=(\d+)`)
	digitsRe     = regexp.MustCompile(`^\d{7,}$`)
	rentryNameRe = regexp.MustCompile(`\[([^\]]+)\]\(`)
)

func parseTextList(s string) *ImportedList {
	l := newList(FormatText)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if m := clipboardRe.FindStringSubmatch(line); m != nil {
			l.Format = "rimsort-text"
			ws := ""
			if w := wsIDRe.FindStringSubmatch(m[3]); w != nil {
				ws = w[1]
			}
			l.add(m[2], m[1], ws)
			continue
		}
		if m := rentryPkgRe.FindStringSubmatch(line); m != nil {
			l.Format = "rentry"
			pkg := m[1] + m[2]
			ws, name := "", ""
			if w := wsIDRe.FindStringSubmatch(line); w != nil {
				ws = w[1]
			}
			if n := rentryNameRe.FindAllStringSubmatch(line, -1); len(n) > 0 {
				name = n[len(n)-1][1]
			}
			l.add(pkg, name, ws)
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if digitsRe.MatchString(f[0]) {
			l.add("", "", f[0])
			continue
		}
		if w := wsIDRe.FindStringSubmatch(f[0]); w != nil && strings.Contains(f[0], "steamcommunity.com") {
			l.add("", "", w[1])
			continue
		}
		if pkgShapeRe.MatchString(f[0]) {
			l.add(f[0], "", "")
		}
	}
	return l
}

// pkgShapeRe: a packageId is Author.Name; headers and prose lines are skipped.
var pkgShapeRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+(\.[A-Za-z0-9_\-]+)+$`)

// ---- RimCrow share code ----

var shareCodeRe = regexp.MustCompile(`^(?:share://RC/)?(?:RC|RMM1)-[0-9A-Fa-f]{8}-[A-Za-z0-9_\-]+=*$`)

func parseShareCode(code string) (*ImportedList, error) {
	code = strings.TrimPrefix(strings.TrimSpace(code), "share://RC/")
	parts := strings.SplitN(code, "-", 3)
	if len(parts) != 3 {
		return nil, errors.New("not a RimCrow share code")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	if err != nil {
		return nil, fmt.Errorf("share code: %v", err)
	}
	if got := fmt.Sprintf("%08X", crc32.ChecksumIEEE(raw)); !strings.EqualFold(got, parts[1]) {
		return nil, fmt.Errorf("share code checksum %s does not match %s: the code is damaged or incomplete", got, strings.ToUpper(parts[1]))
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("share code: %v", err)
	}
	js, err := io.ReadAll(io.LimitReader(zr, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("share code: %v", err)
	}
	var v []json.RawMessage
	if err := json.Unmarshal(js, &v); err != nil || len(v) < 4 {
		return nil, errors.New("share code: unexpected content")
	}
	var ver int
	json.Unmarshal(v[0], &ver)
	if ver != 1 {
		return nil, fmt.Errorf("share code version %d is not supported", ver)
	}
	l := newList("rimcrow-share")
	json.Unmarshal(v[1], &l.Title)
	json.Unmarshal(v[2], &l.Game)
	var entries []json.RawMessage
	json.Unmarshal(v[3], &entries)
	for _, e := range entries {
		var s string
		if json.Unmarshal(e, &s) == nil {
			l.add(s, "", "")
			continue
		}
		var a []string
		if json.Unmarshal(e, &a) == nil && len(a) > 0 {
			ws, name := "", ""
			if len(a) > 1 {
				ws = a[1]
			}
			if len(a) > 2 {
				name = a[2]
			}
			l.add(a[0], name, ws)
		}
	}
	return l, nil
}

// ShareCode renders a list as a RimCrow share code (RC-<CRC32>-<base64(zlib(json))>).
func ShareCode(title, game string, tokens []string, ws, names map[string]string) (string, error) {
	entries := make([]any, 0, len(tokens))
	for _, t := range tokens {
		switch {
		case names[t] != "":
			entries = append(entries, []string{t, ws[t], names[t]})
		case ws[t] != "":
			entries = append(entries, []string{t, ws[t]})
		default:
			entries = append(entries, t)
		}
	}
	js, err := json.Marshal([]any{1, title, game, entries})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	zw.Write(js)
	zw.Close()
	return fmt.Sprintf("RC-%08X-%s", crc32.ChecksumIEEE(buf.Bytes()), base64.RawURLEncoding.EncodeToString(buf.Bytes())), nil
}

// RML renders a list as RimWorld's saved mod list (the game's mod menu loads it).
func RML(game string, tokens []string, ws, names map[string]string) []byte {
	var b strings.Builder
	esc := func(s string) string {
		var w strings.Builder
		xml.EscapeText(&w, []byte(s))
		return w.String()
	}
	li := func(indent string, xs []string) {
		for _, x := range xs {
			b.WriteString(indent + "<li>" + esc(x) + "</li>\n")
		}
	}
	var wsl, nl []string
	for _, t := range tokens {
		w := ws[t]
		if w == "" {
			w = "0"
		}
		wsl = append(wsl, w)
		n := names[t]
		if n == "" {
			n = t
		}
		nl = append(nl, n)
	}
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<savedModList>\n  <meta>\n")
	b.WriteString("    <gameVersion>" + esc(game) + "</gameVersion>\n    <modIds>\n")
	li("      ", tokens)
	b.WriteString("    </modIds>\n    <modSteamIds>\n")
	li("      ", wsl)
	b.WriteString("    </modSteamIds>\n    <modNames>\n")
	li("      ", nl)
	b.WriteString("    </modNames>\n  </meta>\n  <modList>\n    <ids>\n")
	li("      ", tokens)
	b.WriteString("    </ids>\n    <names>\n")
	li("      ", nl)
	b.WriteString("    </names>\n  </modList>\n</savedModList>\n")
	return []byte(b.String())
}

// RimCrowXML renders a list as RimCrow's ModList.xml (ids, names and workshop IDs in
// parallel lists, as parseXMLList reads them).
func RimCrowXML(tokens []string, ws, names map[string]string) []byte {
	var b strings.Builder
	esc := func(s string) string {
		var w strings.Builder
		xml.EscapeText(&w, []byte(s))
		return w.String()
	}
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModList>\n")
	for _, part := range []struct {
		tag string
		val func(t string) string
	}{
		{"modIds", func(t string) string { return t }},
		{"modNames", func(t string) string {
			if n := names[t]; n != "" {
				return n
			}
			return t
		}},
		{"modSteamWorkshopIds", func(t string) string {
			if w := ws[t]; w != "" {
				return w
			}
			return "0"
		}},
	} {
		b.WriteString("  <" + part.tag + ">\n")
		for _, t := range tokens {
			b.WriteString("    <li>" + esc(part.val(t)) + "</li>\n")
		}
		b.WriteString("  </" + part.tag + ">\n")
	}
	b.WriteString("</ModList>\n")
	return []byte(b.String())
}
