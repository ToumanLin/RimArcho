package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// Whole-list actions (06 §6): sort, save and launch act on one key or click and say what
// they did (sort: u undoes it). The preview is the CLI's dry run (read-only), reached from
// the palette (预览排序 / 预览写入 / 预览启动) or `archo … --dry-run`; it never asks to go on.

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
}

// Outcome is what a whole-list action did, for the status line.
type Outcome struct {
	What    string   `json:"what"`
	Text    string   `json:"text"`
	Undo    bool     `json:"undo,omitempty"`    // u takes it back (sort)
	Refused []string `json:"refused,omitempty"` // refused: why, then the commands that resolve it
}

// Run carries out sort, save or launch at once (06 §6). A refusal (a game is running, an
// outside change not loaded, no user data folder …) is an Outcome with Refused set, not an
// error: the interface shows it and asks nothing.
func Run(a *app.App, what string) *Outcome {
	o := &Outcome{What: what}
	text, err := Apply(a, what)
	if err != nil {
		st, _ := a.Loadout()
		o.Refused = RefusalLines(Humanize(st, err.Error()))
		o.Text = o.Refused[0]
		return o
	}
	o.Text = text
	o.Undo = what == ListSort && text != T("the order is already right")
	return o
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
		p.Title = T("preview writing to the game")
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
func Apply(a *app.App, what string) (string, error) {
	switch what {
	case ListSort:
		r, err := a.Sort(false)
		if err != nil {
			return "", err
		}
		if len(r.Changes) == 0 {
			return T("the order is already right"), nil
		}
		return Tf("sorted: %d moved", len(r.Changes)), nil
	case ListSave:
		r, err := a.Save(false)
		if err != nil {
			return "", err
		}
		if r.Unchanged {
			return T("nothing to save"), nil
		}
		msg := Tf("saved to the game as v%d", r.Version)
		if len(r.Links) > 0 {
			msg = Tf("saved to the game as v%d; %d Mods/ links updated", r.Version, len(r.Links))
		}
		if r.Running != nil {
			msg += " · " + T("the game reads it on its next start")
		}
		return msg, nil
	case ListLaunch:
		if _, err := a.Launch(false); err != nil {
			return "", err
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
	return Tf("swapped %s for another copy", Named(st, []string{id})), nil
}
