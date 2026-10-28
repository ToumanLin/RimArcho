package view

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

const (
	ActVersionMute   = "version-mute"
	ActVersionUnmute = "version-unmute"
	ActVersionList   = "version-list"
)

// VersionEntry is one mod in a version warning's list, with its own reversible action.
type VersionEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Supported string `json:"supported"`
	Action    Action `json:"action"`
}

type VersionGroup struct {
	Title  string         `json:"title"`
	Action Action         `json:"action"`
	Rows   []VersionEntry `json:"rows"`
}

// VersionWarnings is generated only when the player opens the version list.
type VersionWarnings struct {
	Title  string         `json:"title"`
	All    Action         `json:"all"`
	Groups []VersionGroup `json:"groups"`
}

// Common wording is built once per snapshot; identical declarations share their text.
func versionReasonBuilder(mm string, muted bool) func(*store.Mod) Reason {
	base := Reason{Status: StatusWarn,
		Why: T("it may still work, but its author has not said it does on this version")}
	a := Action{Kind: ActVersionMute, Key: "M", Label: Tf("ignore the %s warning for this mod", mm), GameVersion: mm,
		Done: Tf("the %s warning for this mod is ignored", mm)}
	if muted {
		base.Status, base.Text, base.Why = StatusNone, a.Done, ""
		a.Kind, a.Key, a.Label = ActVersionUnmute, "U", Tf("restore the %s warning", mm)
		a.Done = Tf("the %s warning for this mod is restored", mm)
	}
	texts := map[string]string{}
	return func(m *store.Mod) Reason {
		r, action := base, a
		if !muted {
			r.Text = texts[m.SupportedVersions]
			if r.Text == "" {
				r.Text = Tf("does not list %s as supported (lists %s)", mm, supportedText(m))
				texts[m.SupportedVersions] = r.Text
			}
		}
		action.Refs = []string{"pkg:" + store.VersionMuteKey(m.PackageID)}
		r.Actions = []Action{action}
		return r
	}
}

func supportedText(m *store.Mod) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(m.SupportedVersions, ",", " ")), ", ")
}

// VersionReason is also shown for disabled mods and for mutes whose warning disappeared
// after an update, so the preference remains visible and reversible in the detail.
func VersionReason(a *app.App, st *app.LoadoutState, m *store.Mod) (*Reason, error) {
	mm := st.GameMM()
	if mm == "" || m.Kind == source.Official {
		return nil, nil
	}
	mutes, err := a.Store.VersionMutes(mm)
	if err != nil {
		return nil, err
	}
	muted := mutes[store.VersionMuteKey(m.PackageID)]
	if !muted {
		vs := strings.Fields(strings.ReplaceAll(m.SupportedVersions, ",", " "))
		if len(vs) == 0 {
			return nil, nil
		}
		for _, v := range vs {
			if v == mm {
				return nil, nil
			}
		}
	}
	r := versionReasonBuilder(mm, muted)(m)
	return &r, nil
}

func versionBatch(st *app.LoadoutState, ps []*app.Problem, muted bool) Action {
	mm := st.GameMM()
	a := Action{Kind: ActVersionMute, Key: "M", Label: Tf("ignore these %d mods' %s warnings", len(ps), mm), GameVersion: mm,
		Done: Tf("%d mods' %s warnings are ignored", len(ps), mm), Confirm: T("press M again to ignore these version warnings"),
		Warn: Tf("Ignore these %d mods' %s warnings in every list and game? You can restore them from the ignored list.", len(ps), mm)}
	if muted {
		a.Kind, a.Key, a.Label = ActVersionUnmute, "U", T("restore all")
		a.Done, a.Confirm, a.Warn = Tf("%d mods' %s warnings are restored", len(ps), mm), "", ""
	}
	for _, p := range ps {
		if m := st.Ix.ByID[p.SubjectInstance()]; m != nil {
			a.Refs = append(a.Refs, "pkg:"+store.VersionMuteKey(m.PackageID))
		}
	}
	return a
}

func mutedVersionsNeed(st *app.LoadoutState, ps []*app.Problem) Need {
	mm := st.GameMM()
	text := Tf("%d mods' %s warnings are ignored", len(ps), mm)
	return Need{rank: 5, Group: GroupList, Text: text, Dim: true, code: "version.muted",
		Actions: []Action{{Kind: ActVersionList, Key: "enter", Label: T("view or restore"), Title: text, GameVersion: mm, VersionMuted: true}}}
}

func ReadVersionWarnings(a *app.App, mm string, muted bool) (*VersionWarnings, error) {
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil, err
	}
	st := ps.State
	if st.GameMM() != mm {
		return nil, errors.New(T("the game version changed; reopen the version warnings"))
	}
	list := ps.Muted
	if !muted {
		list = nil
		for _, p := range ps.List {
			if p.Code == "version.unsupported" {
				list = append(list, p)
			}
		}
	}
	out := &VersionWarnings{Title: Tf("does not list %s as supported", mm), All: versionBatch(st, list, muted), Groups: []VersionGroup{}}
	out.All.Label = T("ignore all")
	if muted {
		out.Title, out.All.Label = Tf("%d mods' %s warnings are ignored", len(list), mm), T("restore all")
	}
	// Keep the likely-compatible previous-version group first; other declarations are factual.
	prev := app.PrevMinor(mm)
	older := ""
	if prev != "" {
		older = app.PrevMinor(prev)
	}
	groups := map[string][]*app.Problem{}
	for _, p := range list {
		m := st.Ix.ByID[p.SubjectInstance()]
		versions := strings.Fields(strings.ReplaceAll(m.SupportedVersions, ",", " "))
		key, old := "2:"+supportedText(m), older != "" && len(versions) > 0
		for _, v := range versions {
			if !env.VersionLess(older, v) {
				continue
			}
			old = false
		}
		for _, v := range versions {
			if v == prev {
				key = "0:"
				break
			}
		}
		if old {
			key = "1:"
		}
		groups[key] = append(groups[key], p)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	reasonOf := versionReasonBuilder(mm, muted)
	for _, key := range keys {
		problems := groups[key]
		title := Tf("lists %s", strings.TrimPrefix(key, "2:"))
		if key == "0:" {
			title = Tf("lists %s", prev)
		} else if key == "1:" {
			title = Tf("only lists %s or earlier", older)
		}
		g := VersionGroup{Title: fmt.Sprintf("%s  %d", title, len(problems)), Action: versionBatch(st, problems, muted)}
		g.Action.Label = T("ignore group")
		if muted {
			g.Action.Label = T("restore group")
		}
		for _, p := range problems {
			m := st.Ix.ByID[p.SubjectInstance()]
			action := reasonOf(m).Actions[0]
			action.Label = T("ignore")
			if muted {
				action.Label = T("restore")
			}
			// A row repeats what it lists only when that differs from its group's title.
			supported := Tf("lists %s", supportedText(m))
			if supported == title {
				supported = ""
			}
			g.Rows = append(g.Rows, VersionEntry{ID: m.ID, Name: m.DisplayName(), Supported: supported, Action: action})
		}
		sort.SliceStable(g.Rows, func(i, j int) bool { return g.Rows[i].Name < g.Rows[j].Name })
		out.Groups = append(out.Groups, g)
	}
	return out, nil
}
