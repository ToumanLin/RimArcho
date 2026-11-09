package app

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// Can it load? (12 §4.3): a save compared with the list it matched, as that list is now.
// Only what needs attention becomes an issue; each issue names its fixes and the archo
// command performing each, so the CLI, MCP and the page offer the same actions.

// Issue levels, lowest first.
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// Issue kinds (12 §4.3). Malformed and class-missing need the full read (12 §1.7 L1).
// Dangling references are not implemented.
const (
	IssueUnreadable   = "unreadable"    // no <meta> or not XML at the start
	IssueCutOff       = "cut-off"       // no </savegame> at the end
	IssueMalformed    = "malformed"     // L1: the XML is not well formed (Text: where reading stopped)
	IssueClassMissing = "class-missing" // L1: Class="…" values no loaded Mod nor the game defines (Owns.Classes)
	IssueNoList       = "no-list"       // matches no list or pack
	IssueGameVersion  = "game-version"  // made for another major.minor than the game
	IssueNotInstalled = "not-installed" // a Mod in the save is not installed (in the list or not)
	IssueDisabled     = "disabled"      // installed but not in the list
	IssueAdded        = "added"         // in the list, not in the save
	IssueReordered    = "reordered"     // same Mods, another order
	IssueLastLoad     = "last-load"     // the game's log has errors or warnings from loading this save (Load)
)

// Fix actions (12 §4.4).
const (
	FixSubscribe    = "subscribe"     // subscribe to a Workshop Mod (and enable it when the list lacks it)
	FixEnable       = "enable"        // enable an installed Mod in save order
	FixAll          = "fix"           // every Mod fix, then launch
	FixLaunch       = "launch"        // nothing to fix: launch the save's list
	FixDisableAdded = "disable-added" // disable the list's Mods the save lacks
	FixOrder        = "order"         // use the save's order
	FixRecycle      = "recycle"       // move the file to the recycle bin
	FixOpenSave     = "open-save"     // look at another save of the colony
	FixNewList      = "new-list"      // make a list from the save's Mods
	FixOpenLog      = "open-log"      // open the game log at the save's last load
)

// SaveMod is one Mod of a save or of its list.
type SaveMod struct {
	PackageID string `json:"packageId"`
	Name      string `json:"name"`
	SteamID   string `json:"steamId,omitempty"`
	Instance  string `json:"instance,omitempty"` // what this game would load ("" not installed)
	InList    bool   `json:"inList"`
	InSave    bool   `json:"inSave"`
	Moved     bool   `json:"moved,omitempty"` // in both, out of the save's order
	// Library is the installed copy the Mod library shows for it ("" none): what this
	// environment resolves the packageId to, also for a pack's save.
	Library string `json:"library,omitempty"`
}

// SaveFix is one way to resolve an issue.
type SaveFix struct {
	Action  string `json:"action"`
	Label   string `json:"label"`   // English; interfaces translate it
	Command string `json:"command"` // the archo command line performing it
	Target  string `json:"target,omitempty"`
}

// SaveIssue is one row of "Can it load?".
type SaveIssue struct {
	Kind  string    `json:"kind"`
	Level string    `json:"level"`
	Mods  []SaveMod `json:"mods,omitempty"`
	Text  string    `json:"text,omitempty"` // e.g. the versions of a game-version issue
	// Owns is what the row's Mod owns in the save (12 §4.6), from the full read: on
	// not-installed and disabled rows (nil without the full read, or when neither Defs nor
	// classes could be checked), and the classes of a class-missing row.
	Owns *SaveOwned `json:"owns,omitempty"`
	// Load is the last load's errors on a last-load row (12 §4.3).
	Load  *SaveLoad `json:"load,omitempty"`
	Fixes []SaveFix `json:"fixes,omitempty"`
}

// SaveCheck is a save against its list.
type SaveCheck struct {
	Save     *Save
	Mods     []SaveMod   // the save's Mods in load order
	ListOnly []SaveMod   // the list's Mods the save does not have, in the list's order
	Issues   []SaveIssue // most severe first
	// Level is the verdict: "" loads with the list, warning: content will be lost, error:
	// will not load cleanly, "writing": the file is still being written (no verdict).
	Level string
	Quick *SaveFix // the quick-fix bar's one action
	Game  string   // the game version of the save's environment
	// GoneOwns is what the not-installed Mods own together, what nothing installed
	// provides counted once (12 §4.6); nil without the full read or such Mods.
	GoneOwns *SaveOwned
}

// Verdict texts (12 §4.3).
func (c *SaveCheck) Verdict() string {
	switch c.Level {
	case "writing":
		return "being written"
	case LevelError:
		return "will not load cleanly"
	case LevelWarning:
		return "loads, but content will be lost"
	}
	return "loads with the list"
}

// CheckSave compares a save with its list (12 §4.3). It reads the list's environment and
// plan; nothing is written. With the save's full read cached (12 §1.7 L1; never read here)
// it adds the malformed and class-missing rows and what each missing Mod owns (12 §4.6),
// which makes a Mod owning pawns, factions or classes an error.
func (a *App) CheckSave(s *Save) (*SaveCheck, error) {
	c := &SaveCheck{Save: s}
	if s.Env != nil {
		c.Game = s.Env.GameVersion
	}
	if s.Writing {
		c.Level = "writing"
		return c, nil
	}
	recycle := SaveFix{Action: FixRecycle, Label: "Recycle…", Command: "saves trash " + s.Ref, Target: s.Ref}
	if s.Head == nil {
		c.add(SaveIssue{Kind: IssueUnreadable, Level: LevelError, Text: s.HeadErr, Fixes: []SaveFix{recycle}})
		c.Quick = &recycle
		return c, nil
	}
	unloadable := func() []SaveFix {
		fixes := []SaveFix{recycle}
		if p := s.previousReadable(); p != nil {
			fixes = append(fixes, SaveFix{Action: FixOpenSave, Label: "Open " + p.Name, Command: "saves show " + p.Ref, Target: p.Ref})
		}
		return fixes
	}
	if !s.Head.Complete {
		c.add(SaveIssue{Kind: IssueCutOff, Level: LevelError, Fixes: unloadable()})
		c.Quick = &recycle
	}
	facts, err := a.SaveFacts(s, false)
	if err != nil {
		return nil, err
	}
	// A file that only ends early has the cut-off row already.
	if facts != nil && facts.Malformed != nil && !(facts.Malformed.EOF && !s.Head.Complete) {
		c.add(SaveIssue{Kind: IssueMalformed, Level: LevelError, Text: malformedText(facts.Malformed), Fixes: unloadable()})
		if c.Quick == nil {
			c.Quick = &recycle
		}
	}

	// The list as it is now, and what this game would load for each packageId.
	var list []SaveMod
	hasList := false
	var resolve func(string) string
	var st *LoadoutState
	mm := ""              // the game version whose facts the list's analysis reads
	var packInst []string // the installed copies a pack's Mods come from
	err = a.inSaveList(s, func() error {
		var err error
		if st, err = a.Loadout(); err == nil {
			mm = a.Store.MM
			resolve = func(pkg string) string { return st.Reg.Resolve(pkg).Instance }
		} else {
			st = nil
		}
		switch {
		case s.Pack != "":
			p, perr := a.ReadPack(s.Pack)
			if perr != nil {
				return perr
			}
			hasList = true
			inPack := map[string]bool{}
			for _, e := range p.Entries {
				list = append(list, SaveMod{PackageID: strings.ToLower(e.PackageID), Name: e.Name, SteamID: e.WorkshopID, InList: true})
				inPack[normPkg(e.PackageID)] = true
				inst := e.Instance
				if inst == "" && st != nil {
					inst = st.Reg.Resolve(e.PackageID).Instance
				}
				packInst = append(packInst, inst)
			}
			// A pack carries its Mods: what it has is what loads.
			resolve = func(pkg string) string {
				if inPack[normPkg(pkg)] {
					return pkg
				}
				return ""
			}
		case s.Plan != nil:
			if err != nil {
				return err
			}
			hasList = true
			for _, e := range st.List {
				m := SaveMod{PackageID: e.Missing, Name: e.Name, Instance: e.Instance, InList: true, SteamID: e.WorkshopID}
				if mod := st.Ix.ByID[e.Instance]; mod != nil {
					m.PackageID, m.Name, m.SteamID = strings.ToLower(mod.PackageID), mod.Name, mod.WorkshopID()
				}
				if m.Name == "" {
					m.Name = m.PackageID
				}
				list = append(list, m)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	inList := map[string]bool{}
	for _, m := range list {
		inList[normPkg(m.PackageID)] = true
	}
	inSave := map[string]bool{}
	for i, id := range s.Head.ModIDs {
		m := SaveMod{PackageID: id, Name: s.Head.Name(i), SteamID: s.Head.SteamID(i), InSave: true, InList: inList[normPkg(id)]}
		if resolve != nil {
			m.Instance = resolve(id)
		}
		inSave[normPkg(id)] = true
		c.Mods = append(c.Mods, m)
	}
	for _, m := range list {
		if !inSave[normPkg(m.PackageID)] {
			c.ListOnly = append(c.ListOnly, m)
		}
	}
	markMoved(c.Mods, list)
	library := func(pkg string) string {
		if st == nil || st.Reg == nil {
			return ""
		}
		return st.Reg.Resolve(pkg).Instance
	}
	for i := range c.Mods {
		c.Mods[i].Library = library(c.Mods[i].PackageID)
	}
	for i := range c.ListOnly {
		c.ListOnly[i].Library = library(c.ListOnly[i].PackageID)
	}

	// The last load of this save had errors or warnings (12 §4.3): from the stored report of the
	// environment's logs, never parsing one here.
	if l := a.lastLoadOf(s, st); l != nil && (l.Errors > 0 || l.Warnings > 0) {
		level := LevelWarning
		if l.Errors > 0 {
			level = LevelError
		}
		c.add(SaveIssue{Kind: IssueLastLoad, Level: level, Load: l, Text: l.text(),
			Fixes: []SaveFix{{Action: FixOpenLog, Label: "Open the game log", Command: "log --file " + cmdQuote(l.Log), Target: l.Log}}})
	}
	if mm, gm := env.MajorMinor(s.Head.GameVersion), env.MajorMinor(c.Game); mm != "" && gm != "" && mm != gm {
		c.add(SaveIssue{Kind: IssueGameVersion, Level: LevelWarning, Text: s.Head.GameVersion + " → " + c.Game})
	}
	newList := SaveFix{Action: FixNewList, Label: "Make a list from this save",
		Command: "loadout import " + cmdQuote(s.Path) + " --new " + cmdQuote(s.Colony.Name), Target: s.Ref}
	if !hasList {
		c.add(SaveIssue{Kind: IssueNoList, Level: LevelInfo, Fixes: []SaveFix{newList}})
	}
	editable := s.Plan != nil
	fixRef := s.Ref // what saves fix names: the save, and the current list when checked against it
	if s.Current {
		fixRef += " --current"
	}
	// The Mods with a row of their own. A Mod the list has only as a missing entry (a fix
	// added it, its download is not scanned yet) is as absent from the game as one the list
	// lacks.
	var rows []int
	for i, m := range c.Mods {
		if m.Instance != "" && (m.InList || !hasList) {
			continue
		}
		rows = append(rows, i)
	}
	var owns *saveOwnership
	if facts != nil && st != nil {
		if owns, err = a.saveOwnership(s, st, mm, facts, c.Mods, rows, packInst); err != nil {
			return nil, err
		}
	}
	fixable := 0
	var gone []int
	for _, i := range rows {
		m := c.Mods[i]
		installed := m.Instance != ""
		if !installed {
			gone = append(gone, i)
		}
		is := SaveIssue{Kind: IssueDisabled, Level: LevelWarning, Mods: []SaveMod{m}}
		if o := owns.of(i); o != nil {
			is.Owns = o
			if o.Severe() {
				is.Level = LevelError
			}
		}
		if !installed {
			is.Kind = IssueNotInstalled
		}
		if editable {
			switch {
			case installed:
				is.Fixes = []SaveFix{{Action: FixEnable, Label: "Enable in the list", Command: "saves fix " + fixRef + " --mod " + m.PackageID, Target: m.PackageID}}
				fixable++
			case m.SteamID != "":
				label := "Subscribe and enable"
				if m.InList {
					label = "Subscribe"
				}
				is.Fixes = []SaveFix{{Action: FixSubscribe, Label: label, Command: "saves fix " + fixRef + " --mod " + m.PackageID, Target: m.PackageID}}
				fixable++
			case !m.InList:
				is.Fixes = []SaveFix{newList}
			}
		}
		c.add(is)
	}
	c.GoneOwns = owns.group(gone)
	for _, is := range owns.missingRows() {
		c.add(is)
	}
	if editable && len(c.ListOnly) > 0 {
		c.add(SaveIssue{Kind: IssueAdded, Level: LevelInfo, Mods: c.ListOnly,
			Fixes: []SaveFix{{Action: FixDisableAdded, Label: "Disable to match", Command: "saves fix " + fixRef + " --disable-added"}}})
	}
	var moved []SaveMod
	for _, m := range c.Mods {
		if m.Moved {
			moved = append(moved, m)
		}
	}
	if editable && len(moved) > 0 {
		c.add(SaveIssue{Kind: IssueReordered, Level: LevelInfo, Mods: moved,
			Fixes: []SaveFix{{Action: FixOrder, Label: "Use the save's order", Command: "saves fix " + fixRef + " --order"}}})
	}

	for _, is := range c.Issues {
		if levelRank(is.Level) > levelRank(c.Level) && is.Level != LevelInfo {
			c.Level = is.Level
		}
	}
	if c.Quick == nil {
		switch {
		case !hasList:
			c.Quick = &newList
		case editable && fixable > 0:
			c.Quick = &SaveFix{Action: FixAll, Label: "Fix and launch", Command: "saves fix " + fixRef + " --launch", Target: s.Ref}
		case editable && c.Level == "":
			c.Quick = &SaveFix{Action: FixLaunch, Label: "Launch", Command: "saves fix " + fixRef + " --launch", Target: s.Ref}
		}
	}
	return c, nil
}

func (c *SaveCheck) add(is SaveIssue) {
	// Most severe first; the same level keeps its order.
	i := len(c.Issues)
	for i > 0 && levelRank(c.Issues[i-1].Level) < levelRank(is.Level) {
		i--
	}
	c.Issues = append(c.Issues, SaveIssue{})
	copy(c.Issues[i+1:], c.Issues[i:])
	c.Issues[i] = is
}

func levelRank(l string) int {
	switch l {
	case LevelError:
		return 3
	case LevelWarning:
		return 2
	case LevelInfo:
		return 1
	}
	return 0
}

// malformedText says where reading stopped.
func malformedText(m *savefile.Malformed) string {
	if m.EOF {
		return fmt.Sprintf("the file ends at line %d before the save does", m.Line)
	}
	return fmt.Sprintf("line %d (byte %d): %s", m.Line, m.Offset, m.Msg)
}

// previousReadable is the newest older save of the same colony that is complete and, when
// read in full, well formed.
func (s *Save) previousReadable() *Save {
	if s.Colony == nil {
		return nil
	}
	after := false
	for _, o := range s.Colony.Saves {
		if o == s || o.Path == s.Path { // s may be a copy (SaveAgainstCurrent)
			after = true
			continue
		}
		if after && o.Head != nil && o.Head.Complete && !o.Writing && (o.Summary == nil || !o.Summary.Malformed) {
			return o
		}
	}
	return nil
}

// markMoved marks the save's Mods that are in both lists but out of the save's relative
// order: those outside a longest common subsequence of the two orders.
func markMoved(saveMods []SaveMod, list []SaveMod) {
	inSave := map[string]bool{}
	for _, m := range saveMods {
		inSave[normPkg(m.PackageID)] = true
	}
	var s, l []string
	var idx []int
	for i, m := range saveMods {
		if m.InList {
			s = append(s, normPkg(m.PackageID))
			idx = append(idx, i)
		}
	}
	for _, m := range list {
		if inSave[normPkg(m.PackageID)] {
			l = append(l, normPkg(m.PackageID))
		}
	}
	keep := lcsMask(s, l)
	for k, i := range idx {
		if !keep[k] {
			saveMods[i].Moved = true
		}
	}
}

// lcsMask marks the elements of a in a longest common subsequence of a and b.
func lcsMask(a, b []string) []bool {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	keep := make([]bool, n)
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			keep[i] = true
			i, j = i+1, j+1
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keep
}

// cmdQuote quotes an archo command argument when it needs it.
func cmdQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'#&|;<>()") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
