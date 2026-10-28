package view

import (
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
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
	Text   string `json:"text"`
	Action Action `json:"action"`
}

func versionReason(st *app.LoadoutState, m *store.Mod, muted bool) Reason {
	mm := st.GameMM()
	r := Reason{Status: StatusWarn, Text: Tf("does not list %s as supported (lists %s)", mm, strings.Join(strings.Fields(m.SupportedVersions), ", ")),
		Why: T("it may still work, but its author has not said it does on this version")}
	a := Action{Kind: ActVersionMute, Key: "M", Label: Tf("don't warn about %s for this mod", mm), Refs: []string{"pkg:" + store.VersionMuteKey(m.PackageID)}, GameVersion: mm,
		Done: Tf("warnings about %s are muted for this mod", mm)}
	if muted {
		r.Status, r.Text, r.Why = StatusNone, a.Done, ""
		a.Kind, a.Key, a.Label = ActVersionUnmute, "U", Tf("warn about %s again", mm)
		a.Done = Tf("warnings about %s are on again", mm)
	}
	r.Actions = []Action{a}
	return r
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
	r := versionReason(st, m, muted)
	return &r, nil
}

func versionBatch(st *app.LoadoutState, ps []*app.Problem) Action {
	a := Action{Kind: ActVersionMute, Key: "M", Label: Tf("don't warn about these %d mods", len(ps)), GameVersion: st.GameMM(),
		Done: T("version warnings muted"), Confirm: T("press M again to mute these version warnings"),
		Warn: Tf("Stop warning about %s for these %d mods in every list and game? You can turn warnings back on from the muted list.", st.GameMM(), len(ps))}
	for _, p := range ps {
		if m := st.Ix.ByID[p.SubjectInstance()]; m != nil {
			a.Refs = append(a.Refs, "pkg:"+store.VersionMuteKey(m.PackageID))
		}
	}
	return a
}

func mutedVersionsNeed(st *app.LoadoutState, ps []*app.Problem) Need {
	text := Tf("%d mods muted", len(ps))
	rows := make([]VersionEntry, 0, len(ps))
	for _, p := range ps {
		m := st.Ix.ByID[p.SubjectInstance()]
		r := versionReason(st, m, true)
		rows = append(rows, VersionEntry{Text: m.DisplayName(), Action: r.Actions[0]})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Text < rows[j].Text })
	return Need{rank: 5, Group: GroupList, Text: text, Dim: true, code: "version.muted",
		Actions: []Action{{Kind: ActVersionList, Key: "enter", Label: T("see muted mods"), Title: text, Entries: rows}}}
}
