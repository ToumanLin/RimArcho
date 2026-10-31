package view

import (
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// 设置 › 反馈 (06 §2.14): a GitHub issue written in RimArcho and opened in the browser.
// RimArcho, the game and the system always go with it; the enabled mods, the paths and the
// game log's errors only when the player ticks them, and the page shows the whole text first.

// FeedbackOptions are the attachments the player ticked.
type FeedbackOptions struct {
	Mods  bool `json:"mods"`
	Paths bool `json:"paths"`
	Log   bool `json:"log"`
}

// FeedbackPart is one optional attachment: its name, what it holds, and whether there is any.
type FeedbackPart struct {
	Key   string `json:"key"` // mods, paths, log
	Label string `json:"label"`
	Note  string `json:"note"`
	None  bool   `json:"none,omitempty"` // nothing to attach (no game, no log yet)
}

// FeedbackAttach is what goes with the report under the given options.
type FeedbackAttach struct {
	Basic    []string           `json:"basic"`
	Parts    []FeedbackPart     `json:"parts"`
	Sections []feedback.Section `json:"sections"`
	// Pending: the game log is still being analysed; ask again.
	Pending bool `json:"pending,omitempty"`
}

// FeedbackAttachments reads the attachments: always the basic lines and the parts' notes,
// and the sections the options ask for. Paths in sections show the home folder as
// %USERPROFILE%.
func FeedbackAttachments(a *app.App, st *app.LoadoutState, o FeedbackOptions) *FeedbackAttach {
	out := &FeedbackAttach{Basic: []string{"RimArcho " + buildinfo.String()}}
	if st != nil && st.Ix != nil && st.Ix.Env != nil && st.Ix.Env.GameVersion != "" {
		out.Basic = append(out.Basic, "RimWorld "+st.Ix.Env.GameVersion)
	}
	out.Basic = append(out.Basic, feedback.System())
	home, _ := os.UserHomeDir()
	scrub := func(lines []string) []string {
		for i := range lines {
			lines[i] = feedback.Scrub(lines[i], home)
		}
		return lines
	}
	loaded := st != nil && st.Ix != nil && st.Ix.Env != nil

	mods := FeedbackPart{Key: "mods", Label: T("enabled mods"), Note: T("no game yet"), None: true}
	if loaded && st.Plan != nil {
		mods.Note, mods.None = Tf("list “%s”, %d mods", st.Plan.Name, len(st.List)), false
		if o.Mods {
			out.Sections = append(out.Sections, feedback.Section{Heading: Tf("enabled mods (list “%s”, %d)", st.Plan.Name, len(st.List)), Lines: feedbackMods(st)})
		}
	}
	paths := FeedbackPart{Key: "paths", Label: T("paths"), Note: T("the game, its data, the mod folders and RimArcho's data; your user name becomes %USERPROFILE%")}
	if o.Paths {
		out.Sections = append(out.Sections, feedback.Section{Heading: T("paths"), Lines: scrub(feedbackPaths(a, st))})
	}
	logPart := FeedbackPart{Key: "log", Label: T("game log"), Note: T("no game log yet"), None: true}
	if loaded {
		if files := LogFiles(a, st); len(files) > 0 {
			logPart.Note, logPart.None = Tf("the errors in Player.log of %s", files[0].When), false
			if o.Log {
				lines, pending := feedbackLog(a, st)
				out.Pending = pending
				if !pending {
					out.Sections = append(out.Sections, feedback.Section{Heading: Tf("game log errors (Player.log of %s)", files[0].When), Lines: scrub(lines)})
				}
			}
		}
	}
	out.Parts = []FeedbackPart{mods, paths, logPart}
	return out
}

// feedbackMods is the current list in load order: name, packageId, Workshop ID.
func feedbackMods(st *app.LoadoutState) []string {
	var out []string
	for i, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; e.Instance != "" && m != nil {
			out = append(out, strings.TrimSpace(fmt.Sprintf("%d. %s  %s  %s", i+1, m.DisplayName(), m.PackageID, m.WorkshopID())))
			continue
		}
		name := e.Name
		if name == "" {
			name = e.Missing
		}
		out = append(out, fmt.Sprintf("%d. %s  (%s)", i+1, name, T("not installed")))
	}
	return out
}

// feedbackPaths: the game, its data folder, every mod folder and RimArcho's data.
func feedbackPaths(a *app.App, st *app.LoadoutState) []string {
	var out []string
	if st != nil && st.Ix != nil && st.Ix.Env != nil {
		e := st.Ix.Env
		out = append(out, "game: "+e.GameRoot, "game data: "+e.UserDataRoot)
		for _, s := range st.Ix.Sources {
			if s.Status != source.StatusRetired {
				out = append(out, string(s.Kind)+": "+s.Root)
			}
		}
	}
	return append(out, "RimArcho: "+a.Home)
}

// feedbackLog is the newest Player.log's error groups as the 日志 page lists them, at most
// 40, each one line.
func feedbackLog(a *app.App, st *app.LoadoutState) ([]string, bool) {
	p, err := BuildLogPage(a, st, "", nil)
	if err != nil {
		return []string{err.Error()}, false
	}
	if p.Pending != "" {
		return nil, true
	}
	var out []string
	for _, r := range p.Rows {
		if r.Info {
			continue
		}
		if len(out) == 40 {
			out = append(out, Tf("… %d more error groups", len(p.Rows)-40))
			break
		}
		line := fmt.Sprintf("%s ×%d  %s", r.ID, r.Count, truncateRunes(r.Text, 200))
		if r.Mods != "" {
			line += "  [" + r.Mods + "]"
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		out = append(out, T("no errors"))
	}
	return out, false
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ComposeFeedback is the issue for what the player wrote and the attachments read last.
func ComposeFeedback(title, text string, at *FeedbackAttach) feedback.Result {
	d := feedback.Draft{Title: title, Text: text, Paste: T("The full text was too long for the link; RimArcho copied it. Paste it here.")}
	if at != nil {
		d.Basic, d.Sections = at.Basic, at.Sections
	}
	return feedback.Compose(d)
}

// CommunityGroup is the settings page's 社区 (06 §2.14): 反馈 opens its page, and the
// GitHub repository and the Discord server open in the browser.
func CommunityGroup() SettingGroup {
	g := SettingGroup{Title: T("community"), Items: []SettingItem{
		{Label: T("bugs and suggestions"), Actions: []SettingAction{{Label: T("feedback…"), Argv: []string{"feedback"}}}},
	}}
	for _, l := range CommunityLinks() {
		g.Items = append(g.Items, SettingItem{Label: l.Label, Value: strings.TrimPrefix(l.URL, "https://"), URL: l.URL})
	}
	return g
}

// CommunityLink is one of RimArcho's public pages (the settings page's head, 06 §2.14).
type CommunityLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// CommunityLinks: the GitHub repository and the Discord server.
func CommunityLinks() []CommunityLink {
	return []CommunityLink{{Label: "GitHub", URL: feedback.Repo}, {Label: "Discord", URL: feedback.Discord}}
}

func init() {
	Register(map[string]string{
		"feedback":                     "反馈",
		"feedback…":                    "反馈…",
		"enabled mods":                 "启用的 Mod",
		"paths":                        "路径",
		"no game yet":                  "还没有游戏",
		"no game log yet":              "还没有游戏日志",
		"list “%s”, %d mods":           "列表“%s”，%d 个",
		"enabled mods (list “%s”, %d)": "启用的 Mod（列表“%s”，%d 个）",
		"the game, its data, the mod folders and RimArcho's data; your user name becomes %USERPROFILE%": "游戏、游戏数据、Mod 文件夹和 RimArcho 的数据；用户名换成 %USERPROFILE%",
		"the errors in Player.log of %s":     "%s 的 Player.log 里的错误",
		"game log errors (Player.log of %s)": "游戏日志里的错误（%s 的 Player.log）",
		"… %d more error groups":             "…还有 %d 组报错",
		"no errors":                          "没有报错",
		"The full text was too long for the link; RimArcho copied it. Paste it here.": "全文太长，放不进链接，RimArcho 已经复制好了，请粘贴在这里。",
		"community":            "社区",
		"bugs and suggestions": "问题和建议",
		"storage":              "存储",
		"updates":              "更新",
		// the GUI page (feedback.js)
		"attach":            "附带",
		"basic information": "基本信息",
		"always attached":   "总是附带",
		"title":             "标题",
		"what happened":     "说明",
		"what happened, and what you expected; steps to make it happen again help":                                     "发生了什么，你希望怎样；能重现的话写下步骤",
		"the game log is still being analysed…":                                                                        "游戏日志还在分析…",
		"opens GitHub in the browser; nothing is sent until you submit it there":                                       "在浏览器里打开 GitHub，在那里提交前什么都不会发出去",
		"too long for a link: the link carries the title and the basics, and the full text is copied for you to paste": "太长，放不进链接：链接只带标题和基本信息，全文会先复制好，在 GitHub 的正文里粘贴",
		"copy the text":                "复制全文",
		"open GitHub":                  "打开 GitHub",
		"copy and open GitHub":         "复制全文并打开 GitHub",
		"opened GitHub in the browser": "已在浏览器里打开 GitHub",
		"opened GitHub; paste the copied text into the issue": "已打开 GitHub，把复制的全文粘贴到正文里",
	})
}
