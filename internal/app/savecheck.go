package app

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Can it load? (10 §4.3): a save compared with the list it matched, as that list is now.
// Only what needs attention becomes an issue; each issue names its fixes and the archo
// command performing each, so the CLI, MCP and the page offer the same actions.

// Issue levels, lowest first.
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// Issue kinds (10 §4.3). Kinds needing the full read (classes, references, last load) are
// added with it.
const (
	IssueUnreadable   = "unreadable"    // no <meta> or not XML at the start
	IssueCutOff       = "cut-off"       // no </savegame> at the end
	IssueNoList       = "no-list"       // matches no list or pack
	IssueGameVersion  = "game-version"  // made for another major.minor than the game
	IssueNotInstalled = "not-installed" // a Mod in the save is not installed
	IssueDisabled     = "disabled"      // installed but not in the list
	IssueAdded        = "added"         // in the list, not in the save
	IssueReordered    = "reordered"     // same Mods, another order
)

// Fix actions (10 §4.4).
const (
	FixSubscribe    = "subscribe"     // subscribe to a Workshop Mod and enable it
	FixEnable       = "enable"        // enable an installed Mod in save order
	FixAll          = "fix"           // every Mod fix, then launch
	FixLaunch       = "launch"        // nothing to fix: launch the save's list
	FixDisableAdded = "disable-added" // disable the list's Mods the save lacks
	FixOrder        = "order"         // use the save's order
	FixRecycle      = "recycle"       // move the file to the recycle bin
	FixOpenSave     = "open-save"     // look at another save of the colony
	FixNewList      = "new-list"      // make a list from the save's Mods
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
	Fixes []SaveFix `json:"fixes,omitempty"`
}

// SaveCheck is a save against its list.
type SaveCheck struct {
	Save     *Save
	Mods     []SaveMod   // the save's Mods in load order
	ListOnly []SaveMod   // the list's Mods the save does not have
	Issues   []SaveIssue // most severe first
	// Level is the verdict: "" loads with the list, warning: content will be lost, error:
	// will not load cleanly, "writing": the file is still being written (no verdict).
	Level string
	Quick *SaveFix // the quick-fix bar's one action
	Game  string   // the game version of the save's environment
}

// Verdict texts (10 §4.3).
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

// CheckSave compares a save with its list (10 §4.3). It reads the list's environment and
// plan; nothing is written.
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
	if !s.Head.Complete {
		fixes := []SaveFix{recycle}
		if p := s.previousReadable(); p != nil {
			fixes = append(fixes, SaveFix{Action: FixOpenSave, Label: "Open " + p.Name, Command: "saves show " + p.Ref, Target: p.Ref})
		}
		c.add(SaveIssue{Kind: IssueCutOff, Level: LevelError, Fixes: fixes})
		c.Quick = &recycle
	}

	// The list as it is now, and what this game would load for each packageId.
	var list []SaveMod
	hasList := false
	var resolve func(string) string
	err := a.inSaveList(s, func() error {
		st, err := a.Loadout()
		if err == nil {
			resolve = func(pkg string) string { return st.Reg.Resolve(pkg).Instance }
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

	if mm, gm := env.MajorMinor(s.Head.GameVersion), env.MajorMinor(c.Game); mm != "" && gm != "" && mm != gm {
		c.add(SaveIssue{Kind: IssueGameVersion, Level: LevelWarning, Text: s.Head.GameVersion + " → " + c.Game})
	}
	newList := SaveFix{Action: FixNewList, Label: "Make a list from this save",
		Command: "loadout import " + cmdQuote(s.Path) + " --new " + cmdQuote(s.Colony.Name), Target: s.Ref}
	if !hasList {
		c.add(SaveIssue{Kind: IssueNoList, Level: LevelInfo, Fixes: []SaveFix{newList}})
	}
	editable := s.Plan != nil
	fixable := 0
	for _, m := range c.Mods {
		if m.InList || (!hasList && m.Instance != "") {
			continue
		}
		is := SaveIssue{Kind: IssueDisabled, Level: LevelWarning, Mods: []SaveMod{m}}
		if m.Instance == "" {
			is.Kind = IssueNotInstalled
		}
		if editable {
			switch {
			case m.Instance != "":
				is.Fixes = []SaveFix{{Action: FixEnable, Label: "Enable in the list", Command: "saves fix " + s.Ref + " --mod " + m.PackageID, Target: m.PackageID}}
				fixable++
			case m.SteamID != "":
				is.Fixes = []SaveFix{{Action: FixSubscribe, Label: "Subscribe and enable", Command: "saves fix " + s.Ref + " --mod " + m.PackageID, Target: m.PackageID}}
				fixable++
			default:
				is.Fixes = []SaveFix{newList}
			}
		}
		c.add(is)
	}
	if editable && len(c.ListOnly) > 0 {
		c.add(SaveIssue{Kind: IssueAdded, Level: LevelInfo, Mods: c.ListOnly,
			Fixes: []SaveFix{{Action: FixDisableAdded, Label: "Disable to match", Command: "saves fix " + s.Ref + " --disable-added"}}})
	}
	var moved []SaveMod
	for _, m := range c.Mods {
		if m.Moved {
			moved = append(moved, m)
		}
	}
	if editable && len(moved) > 0 {
		c.add(SaveIssue{Kind: IssueReordered, Level: LevelInfo, Mods: moved,
			Fixes: []SaveFix{{Action: FixOrder, Label: "Use the save's order", Command: "saves fix " + s.Ref + " --order"}}})
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
			c.Quick = &SaveFix{Action: FixAll, Label: "Fix and launch", Command: "saves fix " + s.Ref + " --launch", Target: s.Ref}
		case editable && c.Level == "":
			c.Quick = &SaveFix{Action: FixLaunch, Label: "Launch", Command: "saves fix " + s.Ref + " --launch", Target: s.Ref}
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

// previousReadable is the newest older save of the same colony that is complete.
func (s *Save) previousReadable() *Save {
	if s.Colony == nil {
		return nil
	}
	after := false
	for _, o := range s.Colony.Saves {
		if o == s {
			after = true
			continue
		}
		if after && o.Head != nil && o.Head.Complete && !o.Writing {
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
