package view

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
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
	groups, lines := linkRows(a, issues)
	actions := []Action{}
	makeAction := func(include string) Action {
		plan := &app.LinkRepairPlan{Links: []*app.GameLink{}, Removed: []string{}}
		for _, l := range issues {
			copy := *l
			if l.Path == include && l.Action == "include explicitly to remove link" {
				copy.Action = "remove link"
			}
			plan.Links = append(plan.Links, &copy)
		}
		previewGroups, preview := linkRows(a, plan.Links)
		label := T("remove the broken or wrong links")
		if include != "" {
			label = Tf("include unproven link: %s", filepath.Base(include))
		}
		for _, l := range issues {
			if l.Code == app.CodeLinkRecovery {
				label = T("repair the previewed links")
				break
			}
		}
		return Action{Kind: ActRepairLinks, Key: "R", Label: label, Refs: []string{a.RememberLinkRepair(plan)}, Confirm: T("press R again to apply the previewed link repair"), Warn: strings.Join(preview, "\n\n"), Lines: preview, LinkGroups: previewGroups, Done: T("applied the previewed link repair")}
	}
	owned := false
	for _, l := range issues {
		if l.Action == "remove link" || l.Action == "recover link" || l.Action == "complete recovery" {
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
	return []Need{{code: "mods.link-health", Group: GroupList, Counted: true, Status: StatusErr, Title: T("game mod links need attention"), Text: T("game mod links need attention"), Why: strings.Join(lines, "\n\n"), Actions: actions, Parts: lines, LinkGroups: groups}}
}

// LinkGroup is shared by the needs panel and its repair confirmation.
type LinkGroup struct {
	Game string    `json:"game"`
	Path string    `json:"path"`
	Rows []LinkRow `json:"rows"`
}
type LinkRow struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Target string `json:"target"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}

func LinkReason(l *app.GameLink) string { return T(l.ShortReason()) }

func linkRows(a *app.App, links []*app.GameLink) ([]LinkGroup, []string) {
	var groups []LinkGroup
	var lines []string
	es, _ := a.Store.Envs()
	for _, l := range links {
		at := -1
		for i := range groups {
			if env.PathKey(groups[i].Path) == env.PathKey(l.Game) {
				at = i
				break
			}
		}
		if at < 0 {
			name := filepath.Base(l.Game)
			for _, e := range es {
				if env.PathKey(realViewPath(e.GameRoot)) == env.PathKey(l.Game) {
					name = e.Name
					if e.GameVersion != "" {
						name += "  " + e.GameVersion
					}
					break
				}
			}
			groups = append(groups, LinkGroup{Game: name, Path: l.Game})
			at = len(groups) - 1
			lines = append(lines, name+"  "+l.Game)
		}
		action := T("keep")
		switch l.Action {
		case "remove link":
			action = T("will delete")
		case "recover link", "complete recovery":
			action = T(l.Action)
		}
		row := LinkRow{Name: filepath.Base(l.Path), Path: l.Path, Target: l.Target, Reason: LinkReason(l), Action: action}
		groups[at].Rows = append(groups[at].Rows, row)
		lines = append(lines, fmt.Sprintf("%s → %s  %s [%s]", l.Path, row.Target, row.Reason, row.Action))
	}
	return groups, lines
}
func realViewPath(p string) string {
	if real, err := source.RealPath(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}
