package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// droppedNeed: the list was loaded from an outside change that left mods out (VM run B8).
// The game does this itself when mods fail to load: it resets ModsConfig.xml to the base
// game, and RimArcho loaded that as the list without a word. The card names the mods and
// puts the version before back in one undoable step, or keeps the change.
func droppedNeed(a *app.App, st *app.LoadoutState) *Need {
	from, to := a.DroppedByOutsideChange(st)
	if to == 0 {
		return nil
	}
	before, err := a.ListAt(st, from)
	if err != nil {
		return nil
	}
	after, err := a.ListAt(st, to)
	if err != nil {
		return nil
	}
	var names []string
	for _, c := range loadout.Diff(before, after, st.LineageOf) {
		if c.Kind == loadout.Removed {
			names = append(names, entryName(st, c.Entry))
		}
	}
	if len(names) == 0 {
		return nil
	}
	shown := names
	if len(shown) > 8 {
		shown = append(shown[:8:8], Tf("and %d more", len(names)-8))
	}
	title := Tf("ModsConfig.xml was changed outside RimArcho: %s left the list", Plural(len(names), "mod"))
	return &Need{Group: GroupConfirm, Counted: true, Status: StatusWarn, Text: title, Title: title,
		Why:      Tf("the game resets its list when mods fail to load; RimArcho loaded the change as v%d. Left out: %s", to, strings.Join(shown, T(", "))),
		Evidence: evidence(EvInferred, Tf("v%d → v%d", from, to)),
		Actions: []Action{
			viewAction(T("see the difference"), "loadout", "diff", fmt.Sprintf("v%d", from), fmt.Sprintf("v%d", to)),
			// One step: u takes it back (06 §6), so no confirmation.
			{Kind: ActLoadExternal, Key: "L", Label: Tf("put back v%d", from), Version: from, Undo: true, Done: Tf("the list is v%d again", from)},
			{Kind: ActKeepOutside, Key: "I", Label: T("keep the change"), Done: Tf("v%d is kept", to)},
		}}
}

func init() {
	Register(map[string]string{
		"ModsConfig.xml was changed outside RimArcho: %s left the list":                                    "ModsConfig.xml 在 RimArcho 之外被改了：%s 不在列表里了",
		"the game resets its list when mods fail to load; RimArcho loaded the change as v%d. Left out: %s": "Mod 加载失败时游戏会重置自己的列表；RimArcho 已把这次修改载入为 v%d。少了：%s",
		"v%d → v%d":             "v%d → v%d",
		"put back v%d":          "恢复 v%d",
		"the list is v%d again": "列表已恢复为 v%d",
		"keep the change":       "保留这次修改",
		"v%d is kept":           "已保留 v%d",
	})
}
