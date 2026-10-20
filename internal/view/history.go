package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// The history area (08 §14, 10 §6): where the workspace is, where u takes it, and the saved
// versions to go back to. The interfaces show it where the staging list is when staging is
// empty.

// HistItem is one row of the history area.
type HistItem struct {
	Kind    string `json:"kind"` // "redo", "now", "undo", "version"
	Text    string `json:"text"`
	Steps   int    `json:"steps,omitempty"`   // redo/undo: StepTo(Steps) goes there
	Version int    `json:"version,omitempty"` // version: loadout checkout v<n>
}

// HistoryItems lists the redo steps (furthest first), now, the undo steps (latest first),
// then the versions (newest first, at most 20).
func HistoryItems(a *app.App, st *app.LoadoutState) ([]HistItem, error) {
	h, err := a.History()
	if err != nil {
		return nil, err
	}
	var out []HistItem
	for i := len(h.Redo) - 1; i >= 0; i-- {
		out = append(out, HistItem{Kind: "redo", Text: StepText(st, h.Redo[i]), Steps: h.Redo[i].Steps})
	}
	now := T("now · saved")
	if n := len(st.Changes()); n > 0 {
		now = Tf("now · %d not saved", n)
	}
	out = append(out, HistItem{Kind: "now", Text: now})
	for _, s := range h.Undo {
		out = append(out, HistItem{Kind: "undo", Text: StepText(st, s), Steps: s.Steps})
	}
	for i, v := range h.Versions {
		if i == 20 {
			break
		}
		src := map[string]string{"user": T("saved"), "external": T("changed outside RimArcho"), "import": T("first seen"), "undo": T("restored")}[v.Source]
		if src == "" {
			src = v.Source
		}
		out = append(out, HistItem{Kind: "version", Text: Tf("v%d · %s · %s", v.N, ShortDate(v.Created), src), Version: v.N})
	}
	return out, nil
}

// StepText says what one edit did, in plain words.
func StepText(st *app.LoadoutState, s app.HistoryStep) string {
	name := func(e loadout.Entry) string {
		if e.Instance == "" {
			return e.Missing
		}
		if m := st.Ix.ByID[e.Instance]; m != nil {
			return m.DisplayName()
		}
		return T("a removed mod")
	}
	if len(s.Changes) == 1 {
		c := s.Changes[0]
		switch c.Kind {
		case loadout.Added:
			return Tf("enabled %s · position %d", name(c.Entry), c.To)
		case loadout.Removed:
			return Tf("disabled %s", name(c.Entry))
		case loadout.Moved:
			return Tf("moved %s to %d", name(c.Entry), c.To)
		case loadout.Replaced:
			return Tf("switched to %s", name(c.Entry))
		}
	}
	op := T(s.Op)
	if rest, ok := strings.CutPrefix(s.Op, "import "); ok {
		op = Tf("import %s", rest)
	}
	return Tf("%s · %d changes", op, len(s.Changes))
}
