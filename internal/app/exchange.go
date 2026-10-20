package app

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Loadout exchange (05 §3): a text list of packageIds, a ModsConfig.xml, or the mod list
// stored in a save file's <meta>.

// Export formats.
const (
	FormatText       = "text"
	FormatModsConfig = "modsconfig"
	FormatSave       = "save"
)

// ExportLoadout renders the workspace in a format.
func (a *App) ExportLoadout(format string) ([]byte, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	tokens := st.Reg.Tokens(st.List, func(id string) string {
		if m := st.Ix.ByID[id]; m != nil {
			return m.PackageID
		}
		return ""
	})
	names, ws := map[string]string{}, map[string]string{}
	for i, t := range tokens {
		if i < len(st.List) {
			if m := st.Ix.ByID[st.List[i].Instance]; m != nil {
				names[t] = m.DisplayName()
				if w := m.WorkshopID(); w != "" {
					ws[t] = w
				}
			}
		}
	}
	game := st.Ix.Env.GameVersion
	if game == "" && st.File != nil {
		game = st.File.Version
	}
	switch format {
	case FormatRML:
		return RML(game, tokens, ws, names), nil
	case FormatRimCrow:
		code, err := ShareCode("RimArcho "+st.Ix.Env.Name, game, tokens, ws, names)
		return []byte(code + "\n"), err
	case FormatRimSort:
		format = FormatModsConfig
	}
	switch format {
	case "", FormatText:
		var b strings.Builder
		fmt.Fprintf(&b, "# RimArcho loadout, env:%s, %d mods, %s\n", st.Ix.Env.Name, len(tokens), a.Now().In(a.Loc).Format("2006-01-02 15:04"))
		fmt.Fprintf(&b, "# one packageId per line in load order; text after # is ignored (archo loadout import <file>)\n")
		for i, t := range tokens {
			name := ""
			if i < len(st.List) {
				if m := st.Ix.ByID[st.List[i].Instance]; m != nil {
					name = m.DisplayName()
				} else {
					name = "missing"
				}
			}
			fmt.Fprintf(&b, "%s  # %s\n", t, name)
		}
		return []byte(b.String()), nil
	case FormatModsConfig:
		version := st.Ix.Env.GameVersion
		if version == "" && st.File != nil {
			version = st.File.Version
		}
		var known []string
		for _, m := range st.Ix.Mods {
			if m.Kind == source.Official && m.PackageID != "ludeon.rimworld" && m.Status == identity.Live {
				known = append(known, m.PackageID)
			}
		}
		return loadout.Render(nil, version, tokens, known)
	}
	return nil, fmt.Errorf("unknown format %q (text, modsconfig, rimsort, rml, rimcrow)", format)
}

// ImportedList is a mod list read from a file.
type ImportedList struct {
	Format string
	Tokens []string
	Names  map[string]string // packageId → name, when the file has them (saves)
	Game   string            // game version the file names
	Title  string            // the list's own name (share codes)
	// Workshop maps packageIds to workshop IDs when the list has them; WorkshopOnly are
	// entries known only by workshop ID (resolved against installed mods on import).
	Workshop     map[string]string
	WorkshopOnly []string
	// Unresolved are WorkshopOnly IDs no installed mod has (import notes them, 10 §4).
	Unresolved []string
}

// ReadLoadoutFile reads a list file in any supported format (10 §4), or a RimCrow share
// code given instead of a path.
func ReadLoadoutFile(path string) (*ImportedList, error) {
	if _, err := os.Stat(path); err != nil && shareCodeRe.MatchString(strings.TrimSpace(path)) {
		return parseShareCode(path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".rws") {
		if fi, err := os.Stat(path); err == nil && fi.Size() < 16<<20 {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			if !bytes.Contains(b[:min(len(b), 4096)], []byte("<savegame")) {
				return ParseList(b)
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, _ := br.Peek(4096)
	switch {
	case strings.EqualFold(filepath.Ext(path), ".rws") || bytes.Contains(head, []byte("<savegame")):
		return readSaveMeta(br)
	case bytes.Contains(head, []byte("<ModsConfigData")):
		mc, err := loadout.ReadModsConfig(path)
		if err != nil {
			return nil, err
		}
		return &ImportedList{Format: FormatModsConfig, Tokens: mc.Active, Game: mc.Version}, nil
	}
	l := &ImportedList{Format: FormatText}
	sc := bufio.NewScanner(br)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if fs := strings.Fields(line); len(fs) > 0 {
			l.Tokens = append(l.Tokens, strings.ToLower(fs[0]))
		}
	}
	return l, sc.Err()
}

// readSaveMeta reads <savegame><meta>: gameVersion, modIds and modNames, and stops there
// (saves are tens of MB; the meta block comes first).
func readSaveMeta(r io.Reader) (*ImportedList, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	l := &ImportedList{Format: FormatSave, Names: map[string]string{}}
	var path []string
	var names []string
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
		case xml.EndElement:
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if t.Name.Local == "meta" {
				for i, id := range l.Tokens {
					if i < len(names) {
						l.Names[id] = names[i]
					}
				}
				return l, nil
			}
		case xml.CharData:
			if len(path) < 3 || path[len(path)-1] != "li" || path[len(path)-3] != "meta" {
				if len(path) >= 2 && path[len(path)-1] == "gameVersion" && path[len(path)-2] == "meta" {
					l.Game = strings.TrimSpace(string(t))
				}
				continue
			}
			v := strings.TrimSpace(string(t))
			switch path[len(path)-2] {
			case "modIds":
				l.Tokens = append(l.Tokens, strings.ToLower(v))
			case "modNames":
				names = append(names, v)
			}
		}
	}
	if len(l.Tokens) == 0 {
		return nil, errors.New("no <meta><modIds> in the save")
	}
	return l, nil
}

// ImportLoadout replaces the workspace with a file's list (one undoable step; nothing is
// written to ModsConfig.xml until save).
func (a *App) ImportLoadout(path string) (*EditResult, *ImportedList, error) {
	il, err := ReadLoadoutFile(path)
	if err != nil {
		return nil, nil, err
	}
	if len(il.Tokens) == 0 && len(il.WorkshopOnly) == 0 {
		return nil, il, fmt.Errorf("%s: no mods in the file", path)
	}
	label := filepath.Base(path)
	if il.Format == "rimcrow-share" {
		label = "share code"
	}
	r, err := a.importList(il, label)
	return r, il, err
}

// importList replaces the workspace with a list; workshop-only entries become the
// installed mod with that workshop ID, or are noted in il.Unresolved.
func (a *App) importList(il *ImportedList, label string) (*EditResult, error) {
	return a.edit("import "+label, func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		tokens := il.Tokens
		if len(il.WorkshopOnly) > 0 {
			byWS := map[string]string{}
			for _, m := range st.Ix.Mods {
				if m.Status == identity.Live && m.WorkshopID() != "" {
					if _, ok := byWS[m.WorkshopID()]; !ok || st.Reg.Loadable(m.ID) {
						byWS[m.WorkshopID()] = m.PackageID
					}
				}
			}
			il.Unresolved = nil
			for _, ws := range il.WorkshopOnly {
				if p := byWS[ws]; p != "" {
					tokens = append(tokens, p)
				} else {
					il.Unresolved = append(il.Unresolved, ws)
				}
			}
		}
		list, _ := st.Reg.FromTokens(tokens)
		return list, nil, nil
	})
}
