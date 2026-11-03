package app

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Changing Mod settings (06 §2.9 "Mod settings", 01 §4). RimArcho changes only simple values
// (on/off, whole numbers, numbers, one line of text, a HugsLib choice) of a Mod's settings
// file or HugsLib pack, only while no game runs. It first copies the file to
// <file>.rimarcho.bak (one copy) and writes a temporary file that it renames over the file;
// every other byte stays as it was. Reset removes the value, so the Mod uses its default. A
// Config file is changed only where it already stores the value (its other keys are not
// known); a HugsLib pack can also get a value its Archo Monitor record names.

// SettingEdit is one change: Name set to Value, or Reset. Old, when set, is the stored text
// ("" not stored) the change was made against; a file changed since is refused.
type SettingEdit struct {
	Name  string
	Value string
	Reset bool
	Old   *string
}

// SettingsWrite is what a change wrote (or would write, DryRun).
type SettingsWrite struct {
	Mod     string
	File    string // as the page lists it
	Path    string
	Backup  string // the backup written; "" when there was no file
	Changes []SettingChange
	DryRun  bool
}

// SettingChange is one value changed: From "" when it was not stored, To "" when removed.
type SettingChange struct {
	Name, From, To string
	Removed        bool // reset (or set to its default): the Mod uses its default
}

// SettingInputError is a change refused; Code says why: bool, integer, number, line, choice
// (the value; Choices then lists the choices), fixed (RimArcho cannot change the setting),
// unknown (the file does not store it, or no such setting), changed (the file changed since
// it was read), nofile (no settings file yet), running (a game runs).
type SettingInputError struct {
	Code    string
	Input   string
	Choices []string
	Msg     string
}

func (e *SettingInputError) Error() string { return e.Msg }

func inputErr(code, input, msg string) error {
	return &SettingInputError{Code: code, Input: input, Msg: msg}
}

// SettingEditor is how a value can be changed here: Kind "" when it cannot; bool, integer,
// number, text or choice (one of Choices) when it can.
type SettingEditor struct {
	Kind    string
	Choices []string
	// Fraction: a number stored with a decimal point; a whole number is written with ".0", so
	// the next read still offers fractions.
	Fraction bool
}

// ConfigEditor is how a value a Config file stores can be changed.
func ConfigEditor(v SettingValue) SettingEditor {
	switch v.Kind {
	case "bool":
		return SettingEditor{Kind: "bool"}
	case "number":
		if isWhole(v.Value) {
			return SettingEditor{Kind: "integer"}
		}
		return SettingEditor{Kind: "number", Fraction: true}
	case "text":
		if !strings.ContainsAny(v.Value, "\r\n") {
			return SettingEditor{Kind: "text"}
		}
	}
	return SettingEditor{}
}

// HugsLibEditor is how a HugsLib setting can be changed: by its recorded type, else as its
// stored text reads.
func HugsLibEditor(r HugsLibRow) SettingEditor {
	if !r.Recorded {
		if r.Stored {
			return ConfigEditor(r.Value)
		}
		return SettingEditor{}
	}
	switch r.Type {
	case "Boolean":
		return SettingEditor{Kind: "bool"}
	case "Byte", "SByte", "Int16", "UInt16", "Int32", "UInt32", "Int64", "UInt64":
		return SettingEditor{Kind: "integer"}
	case "Single", "Double", "Decimal":
		return SettingEditor{Kind: "number"}
	case "String":
		if !strings.ContainsAny(r.Value.Value, "\r\n") {
			return SettingEditor{Kind: "text"}
		}
		return SettingEditor{}
	}
	if len(r.Choices) > 0 {
		return SettingEditor{Kind: "choice", Choices: r.Choices}
	}
	return SettingEditor{} // a type RimArcho does not know
}

var numberText = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

func isWhole(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// Normalize checks in against the editor and returns the text to store.
func (ed SettingEditor) Normalize(in string) (string, error) {
	s := strings.TrimSpace(in)
	switch ed.Kind {
	case "bool":
		switch strings.ToLower(s) {
		case "true", "on", "yes", "1":
			return "True", nil
		case "false", "off", "no", "0":
			return "False", nil
		}
		return "", inputErr("bool", in, fmt.Sprintf("%q is not on or off", in))
	case "integer":
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return "", inputErr("integer", in, fmt.Sprintf("%q is not a whole number (RimArcho writes this value only as one)", in))
		}
		return strconv.FormatInt(n, 10), nil
	case "number":
		if !numberText.MatchString(s) {
			return "", inputErr("number", in, fmt.Sprintf("%q is not a number (write a decimal point, not a comma)", in))
		}
		if isWhole(s) && ed.Fraction {
			s += ".0" // keeps the value reading as a fraction next time
		}
		return s, nil
	case "text":
		if strings.ContainsAny(in, "\r\n") {
			return "", inputErr("line", in, "the text must be one line")
		}
		return in, nil
	case "choice":
		for _, c := range ed.Choices {
			if strings.EqualFold(c, s) {
				return c, nil
			}
		}
		return "", &SettingInputError{Code: "choice", Input: in, Choices: ed.Choices, Msg: fmt.Sprintf("%q is not one of %s", in, strings.Join(ed.Choices, ", "))}
	}
	return "", inputErr("fixed", in, "RimArcho cannot change this value: change it in the game")
}

// EditModSettings changes m's settings file (key: its ModSettingsFile.Key; "" when m has one
// file) as edits say. It refuses while a game runs. dryRun checks and reports without
// writing.
func (a *App) EditModSettings(m *store.Mod, key string, edits []SettingEdit, dryRun bool) (*SettingsWrite, error) {
	if len(edits) == 0 {
		return nil, errors.New("nothing to change")
	}
	if !dryRun {
		g, err := a.RunningGame()
		if err != nil {
			return nil, err
		}
		if g != nil {
			return nil, inputErr("running", "", "the game is running ("+g.Describe(a.Loc)+"): close it first; a running game keeps its settings and writes them back")
		}
	}
	sm, err := a.ModSettingsOf(m)
	if err != nil {
		return nil, err
	}
	f, err := pickSettingsFile(sm, key)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.Path)
	switch {
	case errors.Is(err, os.ErrNotExist) && f.Kind == SettingsKindHugsLib:
		raw = nil
	case errors.Is(err, os.ErrNotExist):
		return nil, inputErr("nofile", "", "the Mod has no settings file yet: change a setting in the game once")
	case err != nil:
		return nil, err
	}
	out := raw
	w := &SettingsWrite{Mod: sm.Name, File: f.File, Path: f.Path, DryRun: dryRun}
	for _, ed := range edits {
		var ch *SettingChange
		if f.Kind == SettingsKindHugsLib {
			out, ch, err = editHugsLib(out, f, ed)
		} else {
			out, ch, err = editConfig(out, ed)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ed.Name, err)
		}
		if ch != nil {
			w.Changes = append(w.Changes, *ch)
		}
	}
	if dryRun || len(w.Changes) == 0 {
		return w, nil
	}
	w.Backup, err = loadout.WriteFile(f.Path, out)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// pickSettingsFile is sm's file of key; "" picks the only one.
func pickSettingsFile(sm *ModSettingsMod, key string) (*ModSettingsFile, error) {
	var keys []string
	for i := range sm.Files {
		f := &sm.Files[i]
		if strings.EqualFold(f.Key(), key) || key == "" && len(sm.Files) == 1 {
			return f, nil
		}
		keys = append(keys, f.Key())
	}
	switch {
	case len(keys) == 0:
		return nil, fmt.Errorf("%s has no settings known", sm.Name)
	case key == "":
		return nil, fmt.Errorf("%s has %d settings files; name one: %s", sm.Name, len(keys), strings.Join(keys, ", "))
	}
	return nil, fmt.Errorf("%s has no settings file %q; it has %s", sm.Name, key, strings.Join(keys, ", "))
}

// editConfig applies ed to a Config file's bytes.
func editConfig(raw []byte, ed SettingEdit) ([]byte, *SettingChange, error) {
	doc, err := scanSettingsXML(raw)
	if err != nil {
		return nil, nil, err
	}
	if doc.root != "SettingsBlock" {
		return nil, nil, errors.New("not a settings file")
	}
	var box *xmlBox
	for i := range doc.boxes {
		if doc.boxes[i].name == "ModSettings" {
			box = &doc.boxes[i]
		}
	}
	if box == nil {
		return nil, nil, errors.New("not a settings file: no <ModSettings>")
	}
	v := box.child(ed.Name)
	if v == nil {
		if ed.Reset {
			return raw, nil, nil
		}
		return nil, nil, inputErr("unknown", ed.Name, "the file does not store it, and RimArcho does not know the Mod's other settings: change it in the game")
	}
	if err := checkOld(ed, v.value.Value, true); err != nil {
		return nil, nil, err
	}
	editor := ConfigEditor(v.value)
	if editor.Kind == "" {
		return nil, nil, inputErr("fixed", ed.Name, "RimArcho changes only on/off, numbers and one line of text: change it in the game")
	}
	if ed.Reset {
		return removeSpan(raw, v.span), &SettingChange{Name: v.name, From: v.value.Value, Removed: true}, nil
	}
	val, err := editor.Normalize(ed.Value)
	if err != nil {
		return nil, nil, err
	}
	if val == v.value.Value {
		return raw, nil, nil
	}
	return setSpan(raw, v, val), &SettingChange{Name: v.name, From: v.value.Value, To: val}, nil
}

// editHugsLib applies ed to HugsLib's settings file (nil: none yet) for pack f.
func editHugsLib(raw []byte, f *ModSettingsFile, ed SettingEdit) ([]byte, *SettingChange, error) {
	var doc *xmlDoc
	if raw != nil {
		var err error
		if doc, err = scanSettingsXML(raw); err != nil {
			return nil, nil, err
		}
		if doc.root != "settings" {
			return nil, nil, errors.New("not HugsLib's settings file")
		}
	}
	sc := &SettingsContent{Simple: true}
	var box *xmlBox
	if doc != nil {
		for i := range doc.boxes {
			if strings.EqualFold(doc.boxes[i].name, f.Class) {
				box = &doc.boxes[i]
				for _, c := range box.kids {
					sc.Values = append(sc.Values, c.value)
				}
			}
		}
	}
	rows, _ := HugsLibRows(f.HugsLib, sc)
	r := findHugsRow(rows, ed.Name)
	if r == nil {
		return nil, nil, inputErr("unknown", ed.Name, "no such setting shown for this Mod")
	}
	if err := checkOld(ed, r.Value.Value, r.Stored); err != nil {
		return nil, nil, err
	}
	editor := HugsLibEditor(*r)
	if editor.Kind == "" {
		return nil, nil, inputErr("fixed", ed.Name, "RimArcho cannot change this kind of value: change it in HugsLib's window")
	}
	var v *xmlValue
	if box != nil {
		v = box.child(r.Name)
	}
	from := ""
	if v != nil {
		from = v.value.Value
	}
	if ed.Reset {
		if v == nil {
			return raw, nil, nil
		}
		return removeSpan(raw, v.span), &SettingChange{Name: r.Name, From: from, Removed: true}, nil
	}
	val, err := editor.Normalize(ed.Value)
	if err != nil {
		return nil, nil, err
	}
	if r.Recorded && val == r.Default.Value { // HugsLib stores only values off their default
		if v == nil {
			return raw, nil, nil
		}
		return removeSpan(raw, v.span), &SettingChange{Name: r.Name, From: from, Removed: true}, nil
	}
	switch {
	case v != nil && val == from:
		return raw, nil, nil
	case v != nil:
		return setSpan(raw, v, val), &SettingChange{Name: r.Name, From: from, To: val}, nil
	}
	return insertHugsLib(raw, doc, box, f.Class, r.Name, val), &SettingChange{Name: r.Name, To: val}, nil
}

// findHugsRow is the row named name, else the one titled so.
func findHugsRow(rows []HugsLibRow, name string) *HugsLibRow {
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i]
		}
	}
	var hit *HugsLibRow
	for i := range rows {
		if strings.EqualFold(rows[i].Name, name) || rows[i].Title != "" && strings.EqualFold(rows[i].Title, name) {
			if hit != nil {
				return nil
			}
			hit = &rows[i]
		}
	}
	return hit
}

func checkOld(ed SettingEdit, now string, stored bool) error {
	if ed.Old == nil {
		return nil
	}
	if !stored {
		now = ""
	}
	if *ed.Old != now {
		return inputErr("changed", now, fmt.Sprintf("the file changed since it was read (now %q): read it again", now))
	}
	return nil
}

// The XML a settings file holds, with byte offsets: a root, its children (boxes: ModSettings,
// or HugsLib's packs) and theirs (values).
type xmlDoc struct {
	root  string
	span  xmlSpan
	boxes []xmlBox
}

type xmlBox struct {
	name string
	span xmlSpan
	kids []xmlValue
}

type xmlValue struct {
	name  string
	span  xmlSpan
	value SettingValue
}

// xmlSpan is an element's bytes: [start, openEnd) its start tag, [closeStart, end) its end
// tag; a self-closing element has openEnd == closeStart == end.
type xmlSpan struct{ start, openEnd, closeStart, end int }

func (s xmlSpan) selfClosing() bool { return s.openEnd == s.end }

func (b *xmlBox) child(name string) *xmlValue {
	for i := range b.kids {
		if b.kids[i].name == name {
			return &b.kids[i]
		}
	}
	return nil
}

// scanSettingsXML reads a settings file's elements to depth 3 with their offsets.
func scanSettingsXML(raw []byte) (*xmlDoc, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	doc := &xmlDoc{}
	depth := 0
	var starts [4]int
	var opens [4]int
	var cur *xmlValue
	var text strings.Builder
	var c childCount
	for {
		before := int(d.InputOffset())
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		after := int(d.InputOffset())
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth <= 3 {
				starts[depth], opens[depth] = before, after
			}
			switch depth {
			case 1:
				doc.root = t.Name.Local
			case 2:
				doc.boxes = append(doc.boxes, xmlBox{name: t.Name.Local})
			case 3:
				if len(doc.boxes) > 0 {
					cur = &xmlValue{name: t.Name.Local, value: SettingValue{Key: t.Name.Local}}
					if strings.EqualFold(attr(t, "IsNull"), "true") {
						cur.value.Kind = "null"
					}
					text.Reset()
					c = childCount{}
				}
			case 4:
				if cur != nil {
					c.kids++
					switch t.Name.Local {
					case "li":
						c.lis++
					case "keys":
						c.keys, c.inKeys = true, true
					case "values":
						c.values = true
					}
				}
			case 5:
				if cur != nil && c.inKeys && t.Name.Local == "li" {
					c.keyItems++
				}
			}
		case xml.EndElement:
			if depth == 4 && t.Name.Local == "keys" {
				c.inKeys = false
			}
			if depth <= 3 {
				sp := xmlSpan{start: starts[depth], openEnd: opens[depth], closeStart: before, end: after}
				if after == opens[depth] { // <x/>: no end tag read
					sp.closeStart, sp.end = opens[depth], opens[depth]
				}
				switch depth {
				case 1:
					doc.span = sp
				case 2:
					doc.boxes[len(doc.boxes)-1].span = sp
				case 3:
					if cur != nil {
						classifySetting(&cur.value, text.String(), c)
						cur.span = sp
						b := &doc.boxes[len(doc.boxes)-1]
						b.kids = append(b.kids, *cur)
						cur = nil
					}
				}
			}
			depth--
		case xml.CharData:
			if depth == 3 && cur != nil {
				text.Write(t)
			}
		}
	}
	if doc.root == "" {
		return nil, errors.New("no XML element")
	}
	return doc, nil
}

func escapeText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// setSpan puts val as v's text.
func setSpan(raw []byte, v *xmlValue, val string) []byte {
	if v.span.selfClosing() {
		return splice(raw, v.span.start, v.span.end, "<"+v.name+">"+escapeText(val)+"</"+v.name+">")
	}
	return splice(raw, v.span.openEnd, v.span.closeStart, escapeText(val))
}

// removeSpan removes an element, with its line when it is alone on it.
func removeSpan(raw []byte, sp xmlSpan) []byte {
	from, to := sp.start, sp.end
	i := from
	for i > 0 && (raw[i-1] == ' ' || raw[i-1] == '\t') {
		i--
	}
	if i == 0 || raw[i-1] == '\n' {
		from = i
		if from > 0 {
			from-- // the line break before it
			if from > 0 && raw[from-1] == '\r' {
				from--
			}
		}
	}
	return splice(raw, from, to, "")
}

func splice(raw []byte, from, to int, with string) []byte {
	out := make([]byte, 0, len(raw)-(to-from)+len(with))
	out = append(out, raw[:from]...)
	out = append(out, with...)
	return append(out, raw[to:]...)
}

// indentOf is the white space before position i on its line ("" when other text is there).
func indentOf(raw []byte, i int) string {
	j := i
	for j > 0 && (raw[j-1] == ' ' || raw[j-1] == '\t') {
		j--
	}
	if j > 0 && raw[j-1] != '\n' {
		return ""
	}
	return string(raw[j:i])
}

// insertHugsLib adds <name>val</name> to pack id (box: its element; nil: none yet), creating
// the pack, or the file (raw nil), as needed, indented like the file.
func insertHugsLib(raw []byte, doc *xmlDoc, box *xmlBox, id, name, val string) []byte {
	elem := "<" + name + ">" + escapeText(val) + "</" + name + ">"
	if raw == nil {
		return []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<settings>\n  <" + id + ">\n    " + elem + "\n  </" + id + ">\n</settings>\n")
	}
	nl := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		nl = "\r\n"
	}
	rootIndent := indentOf(raw, doc.span.start)
	packIndent := rootIndent + "  "
	if len(doc.boxes) > 0 {
		packIndent = indentOf(raw, doc.boxes[0].span.start)
	}
	if box != nil {
		kidIndent := indentOf(raw, box.span.start) + "  "
		if len(box.kids) > 0 {
			last := box.kids[len(box.kids)-1]
			if in := indentOf(raw, box.kids[0].span.start); in != "" {
				kidIndent = in
			}
			return splice(raw, last.span.end, last.span.end, nl+kidIndent+elem)
		}
		pi := indentOf(raw, box.span.start)
		if box.span.selfClosing() {
			return splice(raw, box.span.start, box.span.end, "<"+box.name+">"+nl+kidIndent+elem+nl+pi+"</"+box.name+">")
		}
		return splice(raw, box.span.openEnd, box.span.closeStart, nl+kidIndent+elem+nl+pi)
	}
	pack := "<" + id + ">" + nl + packIndent + "  " + elem + nl + packIndent + "</" + id + ">"
	if len(doc.boxes) > 0 {
		last := doc.boxes[len(doc.boxes)-1]
		return splice(raw, last.span.end, last.span.end, nl+packIndent+pack)
	}
	if doc.span.selfClosing() {
		return splice(raw, doc.span.start, doc.span.end, "<"+doc.root+">"+nl+packIndent+pack+nl+rootIndent+"</"+doc.root+">")
	}
	return splice(raw, doc.span.openEnd, doc.span.closeStart, nl+packIndent+pack+nl+rootIndent)
}
