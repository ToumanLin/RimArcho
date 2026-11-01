package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// RimArcho's own update (07 §5.1): a row in settings › about, a line on the launch
// page and a quiet notice when a new version is found.

const (
	TaskSelfUpdate = "self-update"
	TaskSelfCheck  = "self-check"
)

// SelfUpdateLine is the launch page's line (06 §2.3): nil when nothing is to install.
type SelfUpdateLine struct {
	Title string `json:"title"` // "RimArcho"
	Text  string `json:"text"`  // "0.9.1 can be updated"
}

func SelfUpdateHome(a *app.App) *SelfUpdateLine {
	r := a.SelfUpdatePending()
	if r == nil {
		return nil
	}
	return &SelfUpdateLine{Title: "RimArcho", Text: Tf("%s can be updated", r.Version)}
}

// SelfUpdateItem is the first row of the About group (06 §2.14): the running
// version, what the last check found, its notes folded under the row, and the buttons.
func SelfUpdateItem(a *app.App) SettingItem {
	cur := a.SelfVersionNow()
	it := SettingItem{Label: "RimArcho", Value: cur}
	if _, ok := selfupdate.Compare(cur, cur); !ok {
		it.ValueNote = T("a development build is not updated")
		return it
	}
	info := a.SelfUpdateInfo()
	it.Note = lastChecked(a, info.Checked)
	check := SettingAction{Label: T("check for updates"), Argv: []string{"update", "--check"}, Task: TaskSelfCheck, Pending: TaskPending(TaskSelfCheck)}
	r := a.SelfUpdatePending()
	if r == nil {
		if !info.Checked.IsZero() {
			it.ValueNote = selfCheckText(info.State)
		}
		it.Actions = []SettingAction{check}
		return it
	}
	it.ValueNote = Tf("new version available: %s", r.Version)
	it.Details = &SettingDetails{Summary: T("release notes"), Text: strings.TrimSpace(r.Notes), Link: r.Page}
	if err := r.Installable(); err != nil {
		it.ValueNote += "  " + T("download it from the release page")
		it.Actions = []SettingAction{check}
		return it
	}
	install := SettingAction{Label: T("install update"), Argv: []string{"update"}, Task: TaskSelfUpdate, Pending: TaskPending(TaskSelfUpdate),
		Confirm: Tf("Download RimArcho %s (%s), check it against its SHA-256 and replace the program files, then restart RimArcho. Your data is not changed.", r.Version, SizeText(r.Zip.Size))}
	if program, err := a.SelfProgramDir(); err == nil {
		if n := len(a.OtherArchoIn(program)); n > 0 {
			install.Confirm += " " + Tf("%d other RimArcho are open: the files are replaced once they close.", n)
		}
	}
	it.Actions = []SettingAction{install, check}
	return it
}

func selfCheckText(state string) string {
	switch state {
	case "new version available", "up to date", "":
		return T("RimArcho is up to date")
	}
	if strings.HasPrefix(state, "installed ") {
		return T("RimArcho is up to date")
	}
	return Text(state)
}

// SelfUpdatePhase is the task's phase while an install runs.
func SelfUpdatePhase(s app.SelfUpdateStep) string {
	switch s.Phase {
	case "download":
		if s.Total > 0 {
			return Tf("downloading %s of %s", SizeText(s.Done), SizeText(s.Total))
		}
		return T("downloading…")
	case "unpack":
		return T("checking the download…")
	case "wait":
		return Tf("waiting for %d other RimArcho to close", len(s.Others))
	case "replace":
		return T("replacing the program files…")
	case "check":
		return T("starting the new version…")
	}
	return ""
}

// SelfCheckResult is the check task's result line.
func SelfCheckResult(a *app.App) string {
	if r := a.SelfUpdatePending(); r != nil {
		return Tf("RimArcho %s can be updated", r.Version)
	}
	return selfCheckText(a.SelfUpdateInfo().State)
}
