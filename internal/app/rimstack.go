package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// RimStack (07 §6.6) keeps the player's data as JSON files in its data folder
// (%LOCALAPPDATA%\RimStack on Windows, $XDG_DATA_HOME/RimStack on Linux):
//
//   - notes.json: an object keyed by packageId (lower case), each value {tags: [..],
//     note, versionOk, category, ignored: [..]}; the first tag is the Mod's group, and an
//     older file's "sub" is read as a first tag;
//   - rules.json: an array of {first, then}, packageIds: first loads before then;
//   - lists/<name>.json: a saved list {name, ids} (read as any list, 07 §7.2); snapshots/
//     holds the lists it keeps on its own before sorting or saving, which are not offered.
//
// Everything is keyed by packageId, so a Mod maps to the one live Lineage with that
// packageId; none or several is reported, not guessed.

// PartCategories are RimStack's category overrides that RimArcho has an equivalent for:
// Loaders becomes a top rule and Last a bottom rule (08 §1.5).
const PartCategories = "categories"

// maxRimStackFile bounds a RimStack file read (its notes of every Mod, its rules).
const maxRimStackFile = 16 << 20

// RimStackOptions select the notes file and what of it to bring over.
type RimStackOptions struct {
	Notes  string // notes.json in RimStack's data folder
	DryRun bool
	Tags   ManagerTagNames
	Parts  map[string]bool // notes, tags, ignored, categories (Part*); nil takes them all
}

// rimStackNote is one Mod's entry in notes.json.
type rimStackNote struct {
	Tags      []string `json:"tags"`
	Note      string   `json:"note"`
	VersionOK bool     `json:"versionOk"`
	Category  string   `json:"category"`
	Ignored   []string `json:"ignored"`
	Sub       string   `json:"sub"` // an older file's group
}

// readRimStackJSON reads one of RimStack's files (at most 16 MB, a byte order mark skipped).
func readRimStackJSON(path string, v any) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() > maxRimStackFile {
		return fmt.Errorf("%s is larger than 16 MB", filepath.Base(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	return json.Unmarshal(b, v)
}

// packageLineages maps lower-case packageIds (without _steam) to the live Lineages that
// have them.
func packageLineages(ix *Index) map[string][]string {
	out := map[string][]string{}
	for _, m := range ix.Mods {
		if !m.TombstoneAt.IsZero() || m.PackageID == "" {
			continue
		}
		pkg := strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")
		if !contains(out[pkg], m.LineageID) {
			out[pkg] = append(out[pkg], m.LineageID)
		}
	}
	return out
}

// lineageOfPackage is the one live Lineage with packageId pkg, or why there is none.
func lineageOfPackage(lins map[string][]string, pkg string) (string, string) {
	switch l := lins[strings.TrimSuffix(strings.ToLower(strings.TrimSpace(pkg)), "_steam")]; len(l) {
	case 0:
		return "", "not installed here"
	case 1:
		return l[0], ""
	}
	return "", "more than one installed Mod matches this packageId"
}

// ImportRimStack maps RimStack's notes.json onto Lineages: notes, tags, ignored problems
// (and "works on this version") as one tag, and the Loaders and Last categories as top and
// bottom rules. It only reads the file; running it again changes nothing.
func (a *App) ImportRimStack(opt RimStackOptions) (*RimCrowReport, error) {
	var raw map[string]json.RawMessage
	if err := readRimStackJSON(opt.Notes, &raw); err != nil {
		return nil, fmt.Errorf("RimStack notes: %w", err)
	}
	if raw == nil {
		return nil, errors.New("this is not RimStack's notes.json")
	}
	rep := &RimCrowReport{DB: opt.Notes, DryRun: opt.DryRun, NotMapped: map[string]int{}, NotesAndTags: true}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	lins := packageLineages(ix)
	pd, err := a.Store.PlayerDataAll(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	tags, err := a.newTagImport(ix.Env.ID, opt.DryRun, rep)
	if err != nil {
		return nil, err
	}
	rules, err := a.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	pkgs := make([]string, 0, len(raw))
	for k := range raw {
		pkgs = append(pkgs, k)
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		var n rimStackNote
		if json.Unmarshal(raw[pkg], &n) != nil {
			rep.NotMapped["RimStack entries that are not notes"]++
			continue
		}
		n.keepParts(opt.Parts, rep)
		if n.Note == "" && len(n.Tags) == 0 && !n.ignores() && n.Category == "" {
			continue
		}
		rep.Records++
		lin, why := lineageOfPackage(lins, pkg)
		if lin == "" {
			rep.Unmapped = append(rep.Unmapped, RCItem{pkg, why})
			continue
		}
		rep.ByPackage++
		cur := pd[lin]
		if cur == nil {
			cur = &store.PlayerData{}
			pd[lin] = cur
		}
		if err := a.importNote(rep, ix.Env.ID, lin, pkg, "RimStack", n.Note, cur); err != nil {
			return nil, err
		}
		for _, t := range n.Tags {
			if err := tags.add(PartTags, lin, cur, t); err != nil {
				return nil, err
			}
		}
		if n.ignores() {
			if err := tags.add(PartIgnored, lin, cur, opt.Tags.ignore("RimStack")); err != nil {
				return nil, err
			}
		}
		if kind := map[string]string{"loaders": order.RuleTop, "last": order.RuleBottom}[n.Category]; kind != "" {
			if rules, err = a.importEndRule(rep, ix.Env.ID, lin, kind, rules); err != nil {
				return nil, fmt.Errorf("%s: %w", pkg, err)
			}
		}
	}
	return rep, nil
}

// keepParts trims an entry and clears what the chosen parts leave out. A category RimArcho
// has no equivalent for is counted as not mapped.
func (n *rimStackNote) keepParts(parts map[string]bool, rep *RimCrowReport) {
	n.Note = strings.TrimSpace(n.Note)
	var tags []string
	for _, t := range append([]string{n.Sub}, n.Tags...) {
		if t = strings.TrimSpace(t); t != "" && !containsFold(tags, t) {
			tags = append(tags, t)
		}
	}
	n.Tags = tags
	n.Category = strings.ToLower(strings.TrimSpace(n.Category))
	switch n.Category {
	case "", "loaders", "last":
	default:
		// RimStack's other categories (and the labels older notes kept) only group Mods there
		rep.NotMapped["RimStack categories other than Loaders and Last"]++
		n.Category = ""
	}
	if !wantPart(parts, PartNotes) {
		n.Note = ""
	}
	if !wantPart(parts, PartTags) {
		n.Tags = nil
	}
	if !wantPart(parts, PartIgnored) {
		n.Ignored, n.VersionOK = nil, false
	}
	if !wantPart(parts, PartCategories) {
		n.Category = ""
	}
}

// ignores: the player told RimStack not to warn about this Mod (a problem, or its game
// version).
func (n *rimStackNote) ignores() bool { return n.VersionOK || len(n.Ignored) > 0 }

// importNote brings a note over to lineage lin unless RimArcho has another one, which stays
// and is reported as a conflict.
func (a *App) importNote(rep *RimCrowReport, envID, lin, key, manager, note string, cur *store.PlayerData) error {
	if note == "" {
		return nil
	}
	rep.item(PartNotes, lin, note, "", cur.Note != "")
	switch {
	case cur.Note == note:
		rep.NoteSame++
		rep.part(PartNotes).Same++
	case cur.Note != "":
		rep.part(PartNotes).Same++ // RimArcho's own note stays
		rep.Conflicts = append(rep.Conflicts, RCItem{key, fmt.Sprintf("note: RimArcho has %q, %s %q (kept RimArcho's)", cur.Note, manager, note)})
	default:
		rep.NoteNew++
		rep.part(PartNotes).New++
		cur.Note = note
		if !rep.DryRun {
			return a.Store.SetNote(envID, lin, note)
		}
	}
	return nil
}

// importEndRule adds a top or bottom rule for lin unless it has one of that kind; it
// returns the rules with the new one.
func (a *App) importEndRule(rep *RimCrowReport, envID, lin, kind string, rules []*store.UserRule) ([]*store.UserRule, error) {
	rep.RulesTotal++
	for _, r := range rules {
		if r.Kind == kind && r.A == lin {
			rep.item(PartCategories, lin, kind, "", true)
			rep.RuleSame++
			rep.part(PartCategories).Same++
			return rules, nil
		}
	}
	rep.item(PartCategories, lin, kind, "", false)
	rep.RuleNew++
	rep.part(PartCategories).New++
	r := &store.UserRule{Kind: kind, A: lin, Note: "imported from RimStack", Created: a.Now().UTC()}
	if !rep.DryRun {
		if _, err := a.Store.AddRule(envID, r); err != nil {
			return rules, err
		}
	}
	return append(rules, r), nil
}

// ImportRimStackRules reads RimStack's rules.json ({first, then}: first loads before then)
// as before rules. A rule RimArcho has already, as before A B or after B A, is skipped;
// packageIds must each map to one live Lineage.
func (a *App) ImportRimStackRules(path string, dryRun bool) (*RimCrowReport, error) {
	var raw []map[string]any
	if err := readRimStackJSON(path, &raw); err != nil {
		return nil, fmt.Errorf("RimStack rules: %w", err)
	}
	if raw == nil {
		return nil, errors.New("this is not RimStack's rules.json")
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	existing, err := a.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	lins := packageLineages(ix)
	rep := &RimCrowReport{Rules: path, DryRun: dryRun, NotMapped: map[string]int{}}
	for _, item := range raw {
		first, _ := item["first"].(string)
		then, _ := item["then"].(string)
		first, then = strings.ToLower(strings.TrimSpace(first)), strings.ToLower(strings.TrimSpace(then))
		if first == "" || then == "" {
			rep.NotMapped["RimStack rules without two packageIds"]++
			continue
		}
		rep.RulesTotal++
		label := first + " before " + then
		la, why := lineageOfPackage(lins, first)
		lb, whyB := lineageOfPackage(lins, then)
		if la == "" || lb == "" {
			if why == "" {
				why = whyB
			}
			rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, why})
			continue
		}
		if la == lb {
			rep.RulesSkipped = append(rep.RulesSkipped, RCItem{label, "both are the same Mod here"})
			continue
		}
		duplicate := false
		for _, r := range existing {
			if r.Kind == order.RuleBefore && r.A == la && r.B == lb || r.Kind == order.RuleAfter && r.A == lb && r.B == la {
				duplicate = true
				break
			}
		}
		rep.item(PartRules, la, order.RuleBefore, lb, duplicate)
		if duplicate {
			rep.RuleSame++
			rep.part(PartRules).Same++
			continue
		}
		r := &store.UserRule{Kind: order.RuleBefore, A: la, B: lb, Note: "imported from RimStack", Created: a.Now().UTC()}
		existing = append(existing, r)
		rep.RuleNew++
		rep.part(PartRules).New++
		if !dryRun {
			if _, err := a.Store.AddRule(ix.Env.ID, r); err != nil {
				return nil, fmt.Errorf("%s: %w", label, err)
			}
		}
	}
	return rep, nil
}

// RimStackDataDirs are the places RimStack keeps its data on this system (07 §6.2):
// %LOCALAPPDATA%\RimStack on Windows; $XDG_DATA_HOME/RimStack (~/.local/share/RimStack) on
// Linux, and its Flatpak's own folder; ~/Library/Application Support/RimStack on macOS.
func RimStackDataDirs() []string {
	home, _ := os.UserHomeDir()
	var out []string
	switch runtime.GOOS {
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "RimStack"))
		} else if home != "" {
			out = append(out, filepath.Join(home, "AppData", "Local", "RimStack"))
		}
	case "darwin":
		if home != "" {
			out = append(out, filepath.Join(home, "Library", "Application Support", "RimStack"))
		}
	default:
		if x := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(x) {
			out = append(out, filepath.Join(x, "RimStack"))
		}
		if home != "" {
			out = append(out, filepath.Join(home, ".local", "share", "RimStack"),
				filepath.Join(home, ".var", "app", "io.github.TheForgotten69.RimStack", "data", "RimStack"))
		}
	}
	return out
}

// RimStackFileKind says which of RimStack's data files b is: "rimstack" for notes.json (an
// object of Mods, each an object with note or tags), "rimstack-rules" for rules.json (an
// array of {first, then}), "" otherwise (a saved list reads as a list).
func RimStackFileKind(b []byte) string {
	b = bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))
	var rules []map[string]json.RawMessage
	if json.Unmarshal(b, &rules) == nil && len(rules) > 0 {
		for _, r := range rules {
			if r["first"] == nil || r["then"] == nil {
				return ""
			}
		}
		return "rimstack-rules"
	}
	var notes map[string]map[string]json.RawMessage
	if json.Unmarshal(b, &notes) != nil || len(notes) == 0 {
		return ""
	}
	for _, n := range notes {
		if n["note"] == nil && n["tags"] == nil {
			return ""
		}
	}
	return "rimstack"
}
