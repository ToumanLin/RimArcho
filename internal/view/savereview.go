package view

import (
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// PageReview is Saves › Play review (16 §2.11, 12 §3).
const PageReview = "review"

// ReviewPage is the playthrough review as the page shows it: the colonies to choose from
// and, for the one chosen, a map of its Mods in two groups.
type ReviewPage struct {
	Lead string `json:"lead"`
	Note string `json:"note,omitempty"` // saves left out
	// Colonies are the colonies to choose from, by last played; Colony the one reviewed
	// ("*" all of them); All the line under "all colonies".
	Colonies []ReviewColony `json:"colonies"`
	Colony   string         `json:"colony"`
	All      string         `json:"all"`
	Done     int            `json:"done"`
	Total    int            `json:"total"`
	Groups   []ReviewGroup  `json:"groups"`
	// Foot counts what the map leaves out: the Mods that add nothing a save can show.
	Foot string `json:"foot,omitempty"`
	// Functional are those Mods, by name, so the player can see and unmark them.
	Functional []ReviewFunctional `json:"functional,omitempty"`
}

// ReviewFunctional is a functional Mod in the foot's list. Marked: the player marked it
// (else it was detected, 12 §3.3).
type ReviewFunctional struct {
	Name    string `json:"name"`
	Library string `json:"library,omitempty"`
	Marked  bool   `json:"marked,omitempty"`
}

// ReviewColony is a colony's card: its name and a line ("day 379, 48 saves").
type ReviewColony struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Line string `json:"line"`
}

// ReviewGroup is one group of the map. "meet": the Mods that add quests, research, events,
// creatures or factions, sized and coloured by those alone; "rest": the others, where a
// square says only whether anything of the Mod is in the saves.
type ReviewGroup struct {
	Key   string          `json:"key"`
	Label string          `json:"label"`
	Hint  string          `json:"hint"`
	Mods  []ReviewModView `json:"mods"`
}

// ReviewModView is one Mod's square and its details. Size and Part are what the square
// shows: in "meet" the group's categories only, in "rest" everything. Sum is the line of
// the whole Mod ("12 of 425 seen"); Where what the numbers rest on ("3 colonies, 65 saves").
type ReviewModView struct {
	Name      string          `json:"name"`
	PackageID string          `json:"packageId"`
	Library   string          `json:"library,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	Main      string          `json:"main"` // the label of its largest category
	Sum       string          `json:"sum"`
	Where     string          `json:"where"`
	Adds      int             `json:"adds"`
	Seen      int             `json:"seen"`
	Size      int             `json:"size"`
	Part      int             `json:"part"`
	Tip       string          `json:"tip"`
	Cats      []ReviewCatView `json:"cats"`
	// Group is app.ReviewSeen, ReviewConfigured or ReviewNeverSeen (12 §3.1); Set the names
	// only set up (at most ReviewNames; SetMore counts the rest); Mark the player's mark
	// ("not functional" is the only one a Mod on the map can have).
	Group   string   `json:"group"`
	Set     []string `json:"set,omitempty"`
	SetMore int      `json:"setMore,omitempty"`
	Mark    string   `json:"mark,omitempty"`
}

// ReviewCatView is one category of a Mod: its label, how much is seen, and the names not
// seen (at most ReviewNames; More counts the rest).
type ReviewCatView struct {
	Label string   `json:"label"`
	Adds  int      `json:"adds"`
	Seen  int      `json:"seen"`
	Under bool     `json:"under,omitempty"`
	Names []string `json:"names,omitempty"`
	More  int      `json:"more,omitempty"`
}

// ReviewNames is how many names not seen a category carries to the page.
const ReviewNames = 200

// reviewMeet are the categories a game brings the player to; the others are picked from,
// and how much of them is used says little (403 floors, one laid).
var reviewMeet = map[string]bool{"quests": true, "research": true, "events": true, "factions": true,
	"animals": true, "mechanoids": true, "creatures": true, "biomes": true}

func countColonies(n int) string {
	if n == 1 {
		return T("1 colony")
	}
	return Tf("%d colonies", n)
}

func countSaves(n int) string {
	if n == 1 {
		return T("1 save")
	}
	return Tf("%d saves", n)
}

// ReviewPageOf builds the page from the review of colony ("*" all) among cols; tags are the
// player's tags by library copy.
func ReviewPageOf(r *app.SaveReview, cols []*app.Colony, colony string, tags map[string][]string) *ReviewPage {
	p := &ReviewPage{Colony: colony, Colonies: []ReviewColony{}, Done: r.Done, Total: r.Total}
	saves := 0
	// Every Game Config's colonies are reviewed (12 §4.8): with two or more Saves folders
	// each card also names whose folder it is in.
	owners := map[string]bool{}
	for _, c := range cols {
		if f := c.Latest().Folder; f != nil {
			owners[FolderOwner(f)] = true
		}
	}
	for _, c := range cols {
		line := countSaves(len(c.Saves))
		if d, ok := SaveDay(c.Latest()); ok {
			line = Tf("day %d", d) + T(", ") + line
		}
		if f := c.Latest().Folder; f != nil && len(owners) > 1 {
			line += "  " + FolderOwner(f)
		}
		saves += len(c.Saves)
		p.Colonies = append(p.Colonies, ReviewColony{Key: c.Key, Name: colonyName(c), Line: line})
	}
	p.All = countColonies(len(cols)) + T(", ") + countSaves(saves)
	p.Lead = countColonies(r.Colonies) + T(", ") + countSaves(r.Saves)
	if r.Unread > 0 {
		p.Note = Tf("%s could not be read and are not counted", countSaves(r.Unread))
	}
	var meet, rest []ReviewModView
	functional := 0
	for _, m := range r.Mods {
		if m.Functional {
			functional++
			p.Functional = append(p.Functional, ReviewFunctional{Name: m.Name, Library: m.Library, Marked: m.Marked == app.MarkFunctional})
			continue
		}
		v := ReviewModView{Name: m.Name, PackageID: m.PackageID, Library: m.Library, Tags: tags[m.Library], Adds: m.Adds, Seen: m.Seen,
			Sum: Tf("%d of %d seen", m.Seen, m.Adds), Where: countColonies(m.Colonies) + T(", ") + countSaves(m.Saves), Cats: []ReviewCatView{},
			Group: m.Group(), Set: m.Configured, Mark: m.Marked}
		if len(v.Set) > ReviewNames {
			v.SetMore = len(v.Set) - ReviewNames
			v.Set = v.Set[:ReviewNames]
		}
		var tip []string
		largest := 0
		for _, c := range m.Cats {
			label := contentCategoryLabel(c.Key)
			cv := ReviewCatView{Label: label, Adds: c.Adds, Seen: c.Seen, Under: c.Under, Names: c.NotSeen}
			if len(cv.Names) > ReviewNames {
				cv.More = len(cv.Names) - ReviewNames
				cv.Names = cv.Names[:ReviewNames]
			}
			v.Cats = append(v.Cats, cv)
			if c.Adds > largest {
				largest, v.Main = c.Adds, label
			}
			if reviewMeet[c.Key] {
				v.Size += c.Adds
				v.Part += c.Seen
				tip = append(tip, Tf("%s %d/%d", label, c.Seen, c.Adds))
			}
		}
		slices.SortStableFunc(v.Cats, func(a, b ReviewCatView) int { return b.Adds - a.Adds })
		if v.Size > 0 {
			v.Tip = strings.Join(tip, T(", "))
			meet = append(meet, v)
			continue
		}
		v.Size, v.Part, v.Tip = m.Adds, m.Seen, v.Sum
		if m.Adds == 0 { // marked not functional: nothing to count, never seen
			v.Size, v.Tip = 1, T("adds nothing a save can show; you marked it not functional")
		}
		rest = append(rest, v)
	}
	bySize := func(a, b ReviewModView) int { return b.Size - a.Size }
	slices.SortStableFunc(meet, bySize)
	slices.SortStableFunc(rest, bySize)
	p.Groups = []ReviewGroup{}
	if len(meet) > 0 {
		p.Groups = append(p.Groups, ReviewGroup{Key: "meet", Label: T("quests, research, events, creatures, factions"),
			Hint: T("the colour is how much of these is in the saves"), Mods: meet})
	}
	if len(rest) > 0 {
		none := 0
		for _, m := range rest {
			if m.Seen == 0 {
				none++
			}
		}
		p.Groups = append(p.Groups, ReviewGroup{Key: "rest", Label: T("buildings, floors, clothes, looks and the rest"),
			Hint: Tf("%d with nothing in the saves (outlined)", none), Mods: rest})
	}
	slices.SortStableFunc(p.Functional, func(a, b ReviewFunctional) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	if functional == 1 {
		p.Foot = T("1 interface, performance or fix Mod: a save can't show it")
	} else if functional > 1 {
		p.Foot = Tf("%d interface, performance and fix Mods: a save can't show them", functional)
	}
	return p
}
