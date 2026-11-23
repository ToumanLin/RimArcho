package view

import (
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// The steps after a sort (16 §6.1): what the sort found that ordering cannot fix, one step
// per kind of fix, in this order. Each list step is its own undoable edit, previewed before
// the player applies it; the Workshop step subscribes and is not a list edit.
const (
	StepUseInstalled = "use-installed" // list entries with no folder whose copy is installed
	StepReplace      = "replace"       // enabled Mods with an installed recommended replacement
	StepEnable       = "enable"        // required Mods that are installed but not enabled (then sort)
	StepSubscribe    = "subscribe"     // Mods to get from the Workshop
)

var stepOrder = []string{StepUseInstalled, StepReplace, StepEnable, StepSubscribe}

// SortStepItem is one Mod a step changes, or one the steps cannot fix (Unfixable).
type SortStepItem struct {
	// Change: "copy" (an installed copy takes the entry's place), "replace" (Old gives way to
	// Name), "add" (Name is enabled), "get" (Name is subscribed), "none" (Why says why not).
	Change string `json:"change"`
	Name   string `json:"name"`
	Old    string `json:"old,omitempty"`
	// At: the load position after the step; only in the current step's preview.
	At int `json:"at,omitempty"`
	// For: the enabled Mods that require it (add, get).
	For        []string `json:"for,omitempty"`
	WorkshopID string   `json:"workshopId,omitempty"`
	Why        string   `json:"why,omitempty"`
	// ID and OldID: the installed instances Name and Old are, so each name links to its
	// details; a name without one links to its Workshop page (WorkshopID) or to nothing.
	ID    string `json:"id,omitempty"`
	OldID string `json:"oldId,omitempty"`
	// Remove: the reference of a list entry that is not installed, which the row offers to
	// remove from the list (an undoable disable).
	Remove string `json:"remove,omitempty"`
}

// SortStep is one step: what it changes. Only the current step carries a preview (positions
// and Moved, from a dry run of the edit); the others list their Mods.
type SortStep struct {
	Kind  string         `json:"kind"`
	Items []SortStepItem `json:"items"`
	// Preview: the items have positions and Moved is counted.
	Preview bool `json:"preview,omitempty"`
	// Moved: other enabled Mods that change place (enable sorts after enabling).
	Moved int `json:"moved,omitempty"`
	// WorkshopIDs: what the Workshop step subscribes, deduplicated.
	WorkshopIDs []string `json:"workshopIds,omitempty"`
	// Error: the dry run failed; the step cannot apply now.
	Error string `json:"error,omitempty"`
}

// SortSteps is the plan after a sort, read again from the open plan after every step.
type SortSteps struct {
	Steps []SortStep `json:"steps"`
	// Current: the index of the first step whose kind is not in past; -1 when none is left.
	Current int `json:"current"`
	// Unfixable: what no step can fix here (no known source, a copy or replacement that cannot load).
	Unfixable []SortStepItem `json:"unfixable,omitempty"`
}

// sortFixes is what each step would do to the plan as it is now.
type sortFixes struct {
	copies    []app.MissingMod
	replace   []app.ReplacementSuggestion
	enable    []app.MissingDependency
	get       []SortStepItem
	wsIDs     []string
	unfixable []SortStepItem
}

// readSortFixes sorts what the sort found into steps. gone holds the Workshop IDs Steam said
// are removed from the Workshop (delisted, deleted or private): those are never offered for
// download. removeRef gives a missing list entry's reference for its remove button.
func readSortFixes(n app.SortNotices, gone map[string]bool, removeRef func(pkg string) string) sortFixes {
	var f sortFixes
	addGet := func(it SortStepItem) {
		if it.WorkshopID == "" || slices.Contains(f.wsIDs, it.WorkshopID) {
			return
		}
		f.wsIDs = append(f.wsIDs, it.WorkshopID)
		f.get = append(f.get, it)
	}
	for _, m := range n.MissingMods {
		rm := removeRef(m.PackageID)
		switch {
		case m.CanEnable:
			f.copies = append(f.copies, m)
		case m.Installed:
			f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: m.Name, Why: T("an installed copy cannot be loaded"), Remove: rm})
		case m.WorkshopID != "" && gone[m.WorkshopID]:
			f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: m.Name, WorkshopID: m.WorkshopID, Why: T("removed from the Workshop"), Remove: rm})
		case m.WorkshopID != "":
			addGet(SortStepItem{Change: "get", Name: m.Name, WorkshopID: m.WorkshopID, Remove: rm})
		default:
			// a local Mod, or one Steam never had: there is nothing to download
			f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: m.Name, Why: T("in the list, no known source"), Remove: rm})
		}
	}
	for _, r := range n.Replacements {
		switch {
		case r.Enabled:
		case r.CanReplace && r.Installed:
			f.replace = append(f.replace, r)
		case !r.Installed && r.Replacement.WorkshopID != "" && !gone[r.Replacement.WorkshopID]:
			addGet(SortStepItem{Change: "get", Name: r.Replacement.Name, Old: r.Original.Name, OldID: r.Original.Instance, WorkshopID: r.Replacement.WorkshopID})
		case r.Installed && r.BlockedReason != "":
			f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: r.Replacement.Name, Old: r.Original.Name, ID: r.Replacement.Instance, OldID: r.Original.Instance, Why: r.BlockedReason})
		}
	}
	for _, d := range n.MissingDependencies {
		switch d.Kind {
		case "installed":
			f.enable = append(f.enable, d)
		case "workshop":
			if gone[d.WorkshopID] {
				f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: d.Name, For: d.RequiredBy, WorkshopID: d.WorkshopID, Why: T("removed from the Workshop")})
				continue
			}
			addGet(SortStepItem{Change: "get", Name: d.Name, For: d.RequiredBy, WorkshopID: d.WorkshopID})
		default:
			f.unfixable = append(f.unfixable, SortStepItem{Change: "none", Name: d.Name, For: d.RequiredBy, Why: T("required, no known source")})
		}
	}
	return f
}

func (f sortFixes) has(kind string) bool {
	switch kind {
	case StepUseInstalled:
		return len(f.copies) > 0
	case StepReplace:
		return len(f.replace) > 0
	case StepEnable:
		return len(f.enable) > 0
	case StepSubscribe:
		return len(f.get) > 0
	}
	return false
}

func (f sortFixes) packages() []string {
	out := make([]string, len(f.copies))
	for i, m := range f.copies {
		out[i] = m.PackageID
	}
	return out
}

func (f sortFixes) originals() []string {
	out := make([]string, len(f.replace))
	for i, r := range f.replace {
		out[i] = r.Original.Ref
		if out[i] == "" {
			out[i] = r.Original.Instance
		}
	}
	return out
}

// ReadSortSteps reads the steps for the open plan. past are the kinds the player has applied
// or skipped since the sort: the first other step is current and gets a preview.
func ReadSortSteps(a *app.App, past []string) (*SortSteps, error) {
	st, n, err := a.CurrentSortNotices()
	if err != nil {
		return nil, err
	}
	f := readSortFixes(n, workshopGone(a), func(pkg string) string { return TokRef(st, "pkg:"+pkg) })
	out := &SortSteps{Steps: []SortStep{}, Current: -1, Unfixable: f.unfixable}
	for _, kind := range stepOrder {
		if !f.has(kind) {
			continue
		}
		step := f.step(kind)
		if out.Current < 0 && !slices.Contains(past, kind) {
			out.Current = len(out.Steps)
			if err := f.preview(a, &step); err != nil {
				step.Error = Humanize(st, err.Error())
			}
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}

// workshopGone: the Workshop IDs the last Workshop check found removed from the Workshop.
func workshopGone(a *app.App) map[string]bool {
	gone := map[string]bool{}
	for _, f := range a.SubscriptionSourceCheck(app.LibWorkshop).Failures {
		if f.Gone() {
			gone[f.ID] = true
		}
	}
	return gone
}

// step lists a step's Mods without a dry run.
func (f sortFixes) step(kind string) SortStep {
	s := SortStep{Kind: kind, Items: []SortStepItem{}}
	switch kind {
	case StepUseInstalled:
		for _, m := range f.copies {
			s.Items = append(s.Items, SortStepItem{Change: "copy", Name: m.Name, ID: m.Instance})
		}
	case StepReplace:
		for _, r := range f.replace {
			s.Items = append(s.Items, SortStepItem{Change: "replace", Name: r.Replacement.Name, Old: r.Original.Name, ID: r.Replacement.Instance, OldID: r.Original.Instance})
		}
	case StepEnable:
		for _, d := range f.enable {
			s.Items = append(s.Items, SortStepItem{Change: "add", Name: d.Name, For: d.RequiredBy, ID: d.Instance})
		}
	case StepSubscribe:
		s.Items = append(s.Items, f.get...)
		s.WorkshopIDs = f.wsIDs
	}
	return s
}

// preview fills the current step's positions from a dry run of its edit.
func (f sortFixes) preview(a *app.App, s *SortStep) error {
	var r *app.EditResult
	var err error
	var ids []string // the instance each item puts in the list
	switch s.Kind {
	case StepUseInstalled:
		r, err = a.PreviewUseInstalled(f.packages())
		for _, m := range f.copies {
			ids = append(ids, m.Instance)
		}
	case StepReplace:
		r, err = a.PreviewReplaceRecommended(f.originals())
		for _, x := range f.replace {
			ids = append(ids, x.Replacement.Instance)
		}
	case StepEnable:
		var c *app.DependencyCompletion
		c, err = a.PreviewCompleteDependencies()
		if err == nil {
			r = c.Edit
			// Completing also enables what the newly enabled Mods require: one row each.
			s.Items = s.Items[:0]
			need := map[string][]string{}
			for _, d := range f.enable {
				need[d.Instance] = d.RequiredBy
			}
			for _, id := range c.Enabled {
				name := id
				if m := r.State.Ix.ByID[id]; m != nil {
					name = m.DisplayName()
				}
				s.Items = append(s.Items, SortStepItem{Change: "add", Name: name, For: need[id], ID: id})
				ids = append(ids, id)
			}
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if r == nil || r.State == nil {
		return fmt.Errorf("no preview")
	}
	for i := range s.Items {
		if i < len(ids) {
			s.Items[i].At = r.State.Position(ids[i])
		}
	}
	added := map[string]bool{}
	for _, id := range ids {
		added[id] = true
	}
	for _, ch := range r.Changes {
		if ch.Kind == loadout.Moved && !added[ch.Entry.Instance] {
			s.Moved++
		}
	}
	s.Preview = true
	return nil
}

// ApplySortStep applies one list step to the open plan as it is now, as one undoable edit
// (16 §6.1). The Workshop step is not a list edit: the interface subscribes WorkshopIDs.
func ApplySortStep(a *app.App, kind string) (*Result, error) {
	_, n, err := a.CurrentSortNotices()
	if err != nil {
		return nil, err
	}
	f := readSortFixes(n, workshopGone(a), func(string) string { return "" })
	if !f.has(kind) {
		return &Result{Text: T("nothing left to do in this step")}, nil
	}
	switch kind {
	case StepUseInstalled:
		return UseInstalled(a, f.packages())
	case StepReplace:
		return ReplaceRecommended(a, f.originals())
	case StepEnable:
		return CompleteDependencies(a)
	}
	return nil, fmt.Errorf("%s is not a list step", kind)
}
