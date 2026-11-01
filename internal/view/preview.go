package view

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
)

// launchHintNow is the current plan's LaunchHint from the stored problems (no analysis).
func launchHintNow(a *app.App) *LaunchHint {
	st, err := a.Loadout()
	if err != nil {
		return nil
	}
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil
	}
	return BuildNeeds(a, st, ps).Launch
}

// Whole-list actions (06 §6): sort, save and launch act on one key or click and say what
// they did (sort: u undoes it). The preview is the CLI's dry run (read-only), reached from
// the palette (preview the sort / preview saving / preview the launch) or `archo … --dry-run`; it never asks to go on.

// Whole-list actions.
const (
	ListSort   = "sort"
	ListSave   = "save"
	ListLaunch = "launch"
)

// Preview is the dry run of a whole-list action, for reading.
type Preview struct {
	What  string   `json:"what"`
	Title string   `json:"title"`
	Lines []string `json:"lines"` // the dry run's text, #refs replaced by names
	Apply bool     `json:"apply"` // there is something to do and nothing blocks it
	// Ok, when set, are the positions (in the refs asked about) of the items that can go
	// ahead; the others cannot now and are left out. Unset: all of them.
	Ok []int `json:"ok,omitempty"`
}

// Outcome is what a whole-list action did, for the status line.
type Outcome struct {
	app.SortNotices
	What    string   `json:"what"`
	Text    string   `json:"text"`
	Undo    bool     `json:"undo,omitempty"`    // u takes it back (sort)
	Refused []string `json:"refused,omitempty"` // refused: why, then the commands that resolve it
	// Moves: refused because Mods/ holds another version that can move into the lock
	// library (08 §3): what moving them does, for the one confirmation; RunMoving goes on.
	Moves []string `json:"moves,omitempty"`
	// Confirm: nothing was done; a launch with required mods missing asks first (RunAsking):
	// what will go wrong, then the missing ones.
	Confirm []string `json:"confirm,omitempty"`
}

// Run carries out sort, save or launch at once (06 §6). A refusal (a game is running, an
// outside change not loaded, no user data folder …) is an Outcome with Refused set, not an
// error: the interface shows it and asks nothing.
func Run(a *app.App, what string) *Outcome { return run(a, what, false) }

// RunMoving is Run after the player agreed to move the folders in the way into the lock
// library (Outcome.Moves).
func RunMoving(a *app.App, what string) *Outcome { return run(a, what, true) }

func run(a *app.App, what string, move bool) *Outcome {
	o := &Outcome{What: what}
	var text string
	var err error
	if what == ListSort {
		var r *app.SortResult
		r, err = a.Sort(false)
		if err == nil {
			text = sortResultText(r)
			o.SortNotices = r.SortNotices
		}
	} else {
		text, err = apply(a, what, move)
	}
	if err != nil {
		return refused(a, o, err)
	}
	o.Text = text
	o.Undo = what == ListSort && text != T("the order is already right")
	return o
}

// refused fills an outcome from a refusal; a movable one carries the moves.
func refused(a *app.App, o *Outcome, err error) *Outcome {
	st, _ := a.Loadout()
	var de *app.DuplicatesError
	if errors.As(err, &de) && de.Movable() {
		o.Text = T("Mods/ holds another version of mods RimArcho must link")
		o.Refused = []string{o.Text}
		o.Moves = MoveLines(de)
		return o
	}
	// an unloaded outside change: the window's words and its page, not CLI commands (VM run B9)
	if errors.Is(err, app.ErrExternalConflict) {
		o.Text = T("ModsConfig.xml was changed outside RimArcho, and this list has edits of its own: nothing was written")
		o.Refused = []string{o.Text, T("choose which to keep on the can it run page: see the difference, use the outside change, or ignore it")}
		return o
	}
	o.Refused = RefusalLines(Humanize(st, err.Error()))
	o.Text = o.Refused[0]
	return o
}

// MoveLines say what moving the folders in the way does (the confirmation's lines).
func MoveLines(de *app.DuplicatesError) []string {
	out := []string{T("these folders move from Mods/ into the lock library; lists that use them keep them and get them through a link:")}
	for _, m := range de.Moves {
		what := m.For
		if m.Pack != "" {
			what = Tf("pack %s's copy of %s", m.Pack, m.Mod)
		}
		out = append(out, Tf("Mods/%s → %s, so that Mods/%s can be linked for %s", m.Copy.Name, m.To, m.Link, what))
	}
	return out
}

// RefusalLines splits a refusal into its reason and the commands it names: "reason (a: archo
// x; b: archo y)" or "reason; do archo x". The app's error kinds ("launch blocked: ") go.
func RefusalLines(msg string) []string {
	for _, pre := range []string{app.ErrLaunchBlocked.Error() + ": ", app.ErrExternalConflict.Error() + ": "} {
		msg = strings.TrimPrefix(msg, pre)
	}
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(msg, ")") && strings.Contains(msg[i:], "archo ") {
		return append([]string{msg[:i]}, strings.Split(msg[i+2:len(msg)-1], "; ")...)
	}
	if i := strings.Index(msg, "; "); i > 0 && strings.Contains(msg[i:], "archo ") {
		return []string{msg[:i], msg[i+2:]}
	}
	return []string{msg}
}

// DryRun previews sort, save or launch.
func DryRun(a *app.App, what string) (*Preview, error) {
	p := &Preview{What: what}
	switch what {
	case ListSort:
		p.Title = T("preview the sort")
	case ListSave:
		p.Title = T("preview saving")
	case ListLaunch:
		p.Title = T("preview the launch")
	default:
		return nil, fmt.Errorf("no preview for %s", what)
	}
	text, code := cmds.Execute(a, []string{what, "--dry-run"})
	st, _ := a.Loadout()
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		p.Lines = append(p.Lines, Humanize(st, l))
	}
	p.Apply = code == cmds.ExitOK && !(what == ListSort && strings.HasPrefix(text, "sort: 0 moved"))
	return p, nil
}

// Apply runs sort, save or launch and says what it did.
func Apply(a *app.App, what string) (string, error) { return apply(a, what, false) }

func apply(a *app.App, what string, move bool) (string, error) {
	switch what {
	case ListSort:
		r, err := a.Sort(false)
		if err != nil {
			return "", err
		}
		return sortResultText(r), nil
	case ListSave:
		r, err := a.SaveWith(false, move)
		if err != nil {
			return "", err
		}
		if r.Unchanged {
			return T("no changes to save"), nil
		}
		msg := Tf("saved as v%d", r.Version)
		if len(r.Links) > 0 {
			msg = Tf("saved as v%d; %d Mods/ links updated", r.Version, len(r.Links))
		}
		if r.Running != nil {
			msg += "  " + T("the game reads it on its next start")
		}
		return msg, nil
	case ListLaunch:
		if _, err := a.LaunchWith(false, move); err != nil {
			return "", err
		}
		// The most likely trouble goes with it (06 §2.3, never blocking): the TUI has no
		// launch page, so its status line is where the player reads it.
		if h := launchHintNow(a); h != nil {
			return T("the game is starting") + "  " + Tf("watch for: %s", h.Text), nil
		}
		return T("the game is starting"), nil
	}
	return "", fmt.Errorf("no action %s", what)
}

// UndoRedo steps the workspace back or forward.
func UndoRedo(a *app.App, redo bool) (string, error) {
	if _, _, err := a.UndoRedo(redo); err != nil {
		return "", err
	}
	if redo {
		return T("redid the change"), nil
	}
	return T("undid the last change"), nil
}

// RestoreAll puts every staged mod back where it was (T).
func RestoreAll(a *app.App) (string, error) {
	staged, st, err := a.Staged()
	if err != nil {
		return "", err
	}
	if len(staged) == 0 {
		return T("staging is empty"), nil
	}
	var refs []string
	for _, s := range staged {
		refs = append(refs, st.Ix.Ref(s.Instance))
	}
	if _, err := a.Restore(refs, app.Placement{}); err != nil {
		return "", err
	}
	return Tf("put %s back in place", Plural(len(refs), "mod")), nil
}

// Swap replaces an enabled mod by another copy of it (x).
func Swap(a *app.App, st *app.LoadoutState, id string) (string, error) {
	if _, err := a.Swap(st.Ix.Ref(id), ""); err != nil {
		return "", err
	}
	return Tf("switched the version of %s", Named(st, []string{id})), nil
}

// LaunchPack plays a pack as an Outcome: a refusal because of folders that can move into the
// lock library carries the moves (08 §5.3).
func LaunchPack(a *app.App, name string, move bool) *Outcome {
	o := &Outcome{What: "pack"}
	r, err := a.LaunchPackWith(name, false, move)
	if err != nil {
		return refused(a, o, err)
	}
	o.Text = Tf("pack %s: the game is starting", name)
	if len(r.Moved) > 0 {
		o.Text += "  " + Tf("%d folders moved into the lock library", len(r.Moved))
	}
	return o
}

func sortResultText(r *app.SortResult) string {
	text := sortChangeText(r)
	if r.MonitorSkipped != "" {
		text += "  " + Tf("Archo Monitor not added: %s", T(r.MonitorSkipped))
	}
	return text
}

func sortChangeText(r *app.SortResult) string {
	if len(r.Changes) == 0 {
		return T("the order is already right")
	}
	var added []string
	moved := 0
	for _, ch := range r.Changes {
		if ch.Kind == loadout.Added {
			name := ch.Entry.Instance
			if r.State != nil {
				if m := r.State.Ix.ByID[ch.Entry.Instance]; m != nil {
					name = m.DisplayName()
				}
			}
			added = append(added, name)
		}
		if ch.Kind == loadout.Moved {
			moved++
		}
	}
	if len(added) > 0 {
		// what the sort put in (Core, Archo Monitor: 04 §5.1) by name
		return Tf("sorted: added %s, %d moved", strings.Join(added, T(", ")), moved)
	}
	strong := 0
	for _, b := range order.Broken(r.Input, r.Result) {
		if b.Strength == order.Strong {
			strong++
		}
	}
	if strong > 0 {
		// Strong ones first (04 §5.3): they are what the sort mattered for.
		return Tf("sorted: %d moved, strong requirements put right: %d", len(r.Changes), strong)
	}
	return Tf("sorted: %d moved, only weaker preferences were out of order", len(r.Changes))
}
