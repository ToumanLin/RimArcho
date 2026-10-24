package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func linkNeeds(a *app.App, links []*app.GameLink) []Need {
	var issues []*app.GameLink
	for _, l := range links {
		if l.Code != "" {
			issues = append(issues, l)
		}
	}
	if len(issues) == 0 {
		return nil
	}
	lines := []string{}
	for _, l := range issues {
		lines = append(lines, fmt.Sprintf("%s\n%s → %s\n%s\n%s\n%s", l.Game, l.Path, l.Target, l.Reason, l.Evidence, T(l.Action)))
	}
	actions := []Action{}
	makeAction := func(include string) Action {
		plan := &app.LinkRepairPlan{Links: []*app.GameLink{}, Removed: []string{}}
		preview := []string{}
		for _, l := range issues {
			copy := *l
			if l.Path == include && l.Action == "include explicitly to remove link" {
				copy.Action = "remove link"
			}
			plan.Links = append(plan.Links, &copy)
			preview = append(preview, fmt.Sprintf("%s\n%s → %s\n%s\n%s\n%s", l.Game, l.Path, l.Target, l.Reason, l.Evidence, T(copy.Action)))
		}
		label := T("remove the broken or wrong links")
		if include != "" {
			label = Tf("include unproven link: %s", include)
		}
		return Action{Kind: ActRepairLinks, Key: "R", Label: label, Refs: []string{a.RememberLinkRepair(plan)}, Confirm: T("press R again to remove the previewed links"), Warn: strings.Join(preview, "\n\n"), Lines: preview, Done: T("removed the previewed links")}
	}
	owned := false
	for _, l := range issues {
		if l.Action == "remove link" {
			owned = true
		}
	}
	if owned {
		actions = append(actions, makeAction(""))
	}
	for _, l := range issues {
		if l.Action == "include explicitly to remove link" {
			act := makeAction(l.Path)
			act.Key = fmt.Sprint(len(actions) + 1)
			act.Confirm = Tf("press %s again to remove the previewed links", act.Key)
			actions = append(actions, act)
		}
	}
	return []Need{{Group: GroupList, Counted: true, Status: StatusErr, Title: T("game mod links need attention"), Text: T("game mod links need attention"), Why: strings.Join(lines, "\n\n"), Actions: actions, Parts: lines}}
}
