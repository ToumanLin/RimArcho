package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// Whole-list actions (06 §9): sort, save and launch show a preview first; the preview is
// the CLI's dry run (read-only), the action itself a typed call.

// Whole-list actions.
const (
	ListSort   = "sort"
	ListSave   = "save"
	ListLaunch = "launch"
)

// Preview is what an interface shows before a whole-list action.
type Preview struct {
	What  string   `json:"what"`
	Title string   `json:"title"`
	Lines []string `json:"lines"` // the dry run's text, #refs replaced by names
	Apply bool     `json:"apply"` // there is something to do and nothing blocks it
}

// DryRun previews sort, save or launch.
func DryRun(a *app.App, what string) (*Preview, error) {
	p := &Preview{What: what}
	switch what {
	case ListSort:
		p.Title = T("sort — preview")
	case ListSave:
		p.Title = T("save to game — preview")
	case ListLaunch:
		p.Title = T("launch")
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
		return Tf("sorted: %d moved", len(r.Changes)), nil
	case ListSave:
		r, err := a.Save(false)
		if err != nil {
			return "", err
		}
		if r.Unchanged {
			return T("nothing to save"), nil
		}
		if len(r.Links) > 0 {
			return Tf("saved to the game as v%d; %d Mods/ links updated", r.Version, len(r.Links)), nil
		}
		return Tf("saved to the game as v%d", r.Version), nil
	case ListLaunch:
		if _, _, err := a.Launch(false, false); err != nil {
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
