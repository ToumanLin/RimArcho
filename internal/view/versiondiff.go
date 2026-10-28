package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// VersionDiff is "see the difference" of an outside change (`loadout diff v<n> workspace`)
// in the interface's words: how the saved version from (0: the newest) and the version to
// (0: the list being edited) differ, one mod a line. The window ran the CLI command, which
// it does not allow, and the line "not a read-only command" was all the player got (VM run
// B9).
func VersionDiff(a *app.App, from, to string) ([]string, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	env := st.Ix.Env.ID
	v := [2]int{}
	if st.Latest != nil {
		v[0] = st.Latest.N
	}
	for k, ref := range []string{from, to} {
		if ref == "" || ref == "workspace" {
			continue
		}
		if v[k], err = a.ParseVersionRef(env, ref); err != nil {
			return nil, err
		}
	}
	la, err := a.ListAt(st, v[0])
	if err != nil {
		return nil, err
	}
	lb, err := a.ListAt(st, v[1])
	if err != nil {
		return nil, err
	}
	side := func(n int) string {
		if n == 0 {
			return T("your list")
		}
		return Tf("v%d", n)
	}
	chs := loadout.Diff(la, lb, st.LineageOf)
	if len(chs) == 0 {
		return []string{Tf("%s and %s are the same", side(v[0]), side(v[1]))}, nil
	}
	out := []string{Tf("%s → %s: %d changes", side(v[0]), side(v[1]), len(chs))}
	for _, c := range chs {
		n := entryName(st, c.Entry)
		switch c.Kind {
		case loadout.Added:
			out = append(out, Tf("+ %s (only in %s, position %d)", n, side(v[1]), c.To))
		case loadout.Removed:
			out = append(out, Tf("− %s (only in %s)", n, side(v[0])))
		case loadout.Moved:
			out = append(out, Tf("↕ %s: position %d → %d", n, c.From, c.To))
		case loadout.Replaced:
			out = append(out, Tf("⇄ %s: another copy (was %s)", n, entryName(st, c.Old)))
		}
	}
	return out, nil
}

// entryName is a list entry's mod name: a missing entry's own name, a forgotten copy said so.
func entryName(st *app.LoadoutState, e loadout.Entry) string {
	if e.Instance == "" {
		return e.Missing
	}
	if m := st.Ix.ByID[e.Instance]; m != nil {
		return m.DisplayName()
	}
	return T("a removed mod")
}

func init() {
	Register(map[string]string{
		"your list":                      "你的列表",
		"%s and %s are the same":         "%s 和 %s 一样",
		"%s → %s: %d changes":            "%s → %s：%d 处不同",
		"+ %s (only in %s, position %d)": "+ %s（只在%s里，第 %d 个）",
		"− %s (only in %s)":              "− %s（只在%s里）",
		"↕ %s: position %d → %d":         "↕ %s：第 %d 个 → 第 %d 个",
		"⇄ %s: another copy (was %s)":    "⇄ %s：换了一个副本（原来是 %s）",
	})
}
