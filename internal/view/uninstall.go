package view

import (
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// 设置 › 卸载 RimArcho (06 §2.14): the plan as both interfaces show it — one row per step, the
// player's choices on the rows that have them, then the same rows as the report.

// UninstallChoice is the player's answers (app.UninstallOptions for the page).
type UninstallChoice struct {
	KeepLocked    bool   `json:"keepLocked"`
	LockedTo      string `json:"lockedTo"`
	RemovePacks   bool   `json:"removePacks"`
	PacksTo       string `json:"packsTo"`
	RemoveSaves   bool   `json:"removeSaves"`
	SavesTo       string `json:"savesTo"`
	RemoveGames   bool   `json:"removeGames"`
	RestoreBackup bool   `json:"restoreBackup"`
}

// Options are the answers for app.Uninstall.
func (c UninstallChoice) Options() app.UninstallOptions {
	return app.UninstallOptions{KeepLocked: c.KeepLocked, LockedTo: c.LockedTo, RemovePacks: c.RemovePacks, PacksTo: c.PacksTo,
		RemoveSaves: c.RemoveSaves, SavesTo: c.SavesTo, RemoveGames: c.RemoveGames, RestoreBackup: c.RestoreBackup}
}

// UninstallPage is the plan, or after a run the report.
type UninstallPage struct {
	Steps    []UnStep `json:"steps"`
	Envs     []string `json:"envs"`     // where kept copies can go
	LockedTo string   `json:"lockedTo"` // the environment they go to
	SavesTo  string   `json:"savesTo"`
	PacksTo  string   `json:"packsTo"`
	Blocked  string   `json:"blocked,omitempty"` // why it cannot run now
	// After a run.
	Ran    bool   `json:"ran,omitempty"`
	Failed int    `json:"failed,omitempty"`
	Result string `json:"result,omitempty"`
}

// UnStep is one step. Toggle names the answer its checkbox sets ("" none): checked is On.
type UnStep struct {
	Key    string   `json:"key"`
	Title  string   `json:"title"`
	Text   string   `json:"text"`
	Size   string   `json:"size,omitempty"`
	Toggle string   `json:"toggle,omitempty"`
	Label  string   `json:"label,omitempty"` // the checkbox's words
	On     bool     `json:"on,omitempty"`
	Items  []UnItem `json:"items"`
}

// UnItem is one folder or file of a step.
type UnItem struct {
	Path  string `json:"path"`
	To    string `json:"to,omitempty"`
	Name  string `json:"name,omitempty"`
	Verb  string `json:"verb"`
	Size  string `json:"size,omitempty"`
	Note  string `json:"note,omitempty"`
	Done  bool   `json:"done,omitempty"`
	Later bool   `json:"later,omitempty"`
	Err   string `json:"err,omitempty"`
}

var unTitles = map[string]string{
	app.UnAccounts: "忘记账户会话",
	app.UnLinks:    "RimArcho's links", app.UnModsConfig: "the games' mod lists", app.UnLocked: "lock library copies",
	app.UnMonitor: "Archo Monitor", app.UnPacks: "packs", app.UnSaves: "own saves", app.UnGames: "downloaded games",
	app.UnSteamCMD: "SteamCMD", app.UnData: "RimArcho's data", app.UnProgram: "the program",
}

var unVerbs = map[string]string{
	"forget-accounts": "忘记会话",
	app.DoUnlink:      "unlink", app.DoClean: "clean", app.DoRestore: "put the backup back", app.DoTrash: "recycle bin", app.DoMove: "move",
	app.DoKeep: "keep", app.DoDelete: "delete", app.DoLater: "after closing", app.DoLeave: "left alone",
}

// UninstallTitle names a step.
func UninstallTitle(key string) string { return T(unTitles[key]) }

// UninstallView builds the page from a plan (or a report: ran).
func UninstallView(p *app.UninstallPlan, ran bool) *UninstallPage {
	o := p.Options
	sizePrefix := ""
	if p.EstimatedSizes {
		sizePrefix = "≈ "
	}
	pg := &UninstallPage{Envs: p.Envs, LockedTo: p.LockedTo, SavesTo: p.SavesTo, PacksTo: p.PacksTo, Ran: ran}
	if pg.LockedTo == "" && len(p.Envs) > 0 {
		pg.LockedTo = p.Envs[0]
	}
	if err := p.Blocked(); err != nil && !ran {
		pg.Blocked = Humanize(nil, strings.TrimPrefix(err.Error(), app.ErrUninstallBlocked.Error()+": "))
	}
	for _, s := range p.Steps {
		st := UnStep{Key: s.Key, Title: T(unTitles[s.Key]), Text: uninstallText(p, s)}
		if b := s.Bytes(); b > 0 {
			st.Size = sizePrefix + render.Size(b)
		}
		switch s.Key {
		case app.UnModsConfig:
			for _, it := range s.Items {
				if _, err := os.Stat(it.Path + loadout.BackupSuffix); err == nil {
					st.Toggle, st.Label, st.On = "restoreBackup", T("put back the list from before RimArcho's last save instead"), o.RestoreBackup
				}
			}
		case app.UnLocked:
			st.Toggle, st.Label, st.On = "keepLocked", T("keep them as ordinary mods"), o.KeepLocked
		case app.UnPacks:
			st.Toggle, st.Label, st.On = "removePacks", T("delete them (recycle bin)"), o.RemovePacks
		case app.UnSaves:
			st.Toggle, st.Label, st.On = "removeSaves", T("delete them (recycle bin)"), o.RemoveSaves
		case app.UnGames:
			st.Toggle, st.Label, st.On = "removeGames", T("delete them (recycle bin)"), o.RemoveGames
		}
		for _, it := range s.Items {
			u := UnItem{Path: it.Path, To: it.To, Name: it.Name, Verb: T(unVerbs[it.Do]), Note: unNote(it.Note), Done: it.Done, Err: it.Err, Later: ran && it.Do == app.DoLater}
			if lines := it.Links.Lines(); len(lines) > 0 {
				u.Note += "\n" + strings.Join(lines, "\n")
			}
			if it.Bytes > 0 {
				u.Size = sizePrefix + render.Size(it.Bytes)
			}
			if len(it.Drop) > 0 {
				u.Note = Tf("takes out %s", strings.Join(it.Drop, ", "))
			}
			if it.Err != "" {
				pg.Failed++
			}
			st.Items = append(st.Items, u)
		}
		pg.Steps = append(pg.Steps, st)
	}
	if ran {
		if pg.Failed > 0 {
			pg.Result = Tf("%d steps failed; RimArcho's data stays so you can run it again", pg.Failed)
		} else {
			pg.Result = Tf("done. RimArcho deletes its data and the program once it has closed; anything left is listed in %s", p.Log)
		}
	}
	return pg
}

func unNote(n string) string {
	if parts := strings.Split(n, "; "); len(parts) > 1 {
		for i, part := range parts {
			parts[i] = unNote(part)
		}
		return strings.Join(parts, T("; "))
	}
	if rest, ok := strings.CutPrefix(n, "includes the downloaded "); ok {
		return Tf("includes the downloaded %s", rest)
	}
	if rest, ok := strings.CutPrefix(n, "keeps "); ok {
		return Tf("keeps %s", rest)
	}
	return T(n)
}

// uninstallText says in one sentence what a step does with its answers.
func uninstallText(p *app.UninstallPlan, s *app.UninstallStep) string {
	n := map[string]int{}
	for _, it := range s.Items {
		n[it.Do]++
	}
	var parts []string
	add := func(do, f string, a ...any) {
		if n[do] == 0 {
			return
		}
		if strings.Contains(f, "%d") {
			a = append([]any{n[do]}, a...)
		}
		parts = append(parts, Tf(f, a...))
	}
	switch s.Key {
	case app.UnAccounts:
		return "先忘记账户会话；外部 SteamCMD 缓存不接管，失败时保留工具和数据。"
	case app.UnLinks:
		add(app.DoUnlink, "delete %d links in the games' Mods folders; the folders they point to stay")
		add(app.DoLeave, "%d folders named like them are not links: left alone")
	case app.UnModsConfig:
		add(app.DoClean, "keep each game's current list (%d), without Archo Monitor and the mods only there through a link; RimArcho's backup is deleted")
		add(app.DoRestore, "put back the list from before RimArcho's last save (%d)")
	case app.UnLocked:
		add(app.DoTrash, "%d copies to the recycle bin")
		add(app.DoMove, "%d copies into %s's Mods folder, as ordinary mods", p.LockedTo)
	case app.UnMonitor:
		add(app.DoDelete, "delete it (%d versions)")
	case app.UnPacks:
		add(app.DoKeep, "keep %d where they are")
		add(app.DoMove, "keep %d: moved to the folder below")
		add(app.DoTrash, "%d to the recycle bin")
		add(app.DoDelete, "delete %d unfinished packs")
	case app.UnSaves:
		add(app.DoMove, "keep %d save folders: moved to the folder below")
		add(app.DoTrash, "%d save folders to the recycle bin")
	case app.UnGames:
		add(app.DoKeep, "keep %d game folders downloaded with SteamCMD")
		add(app.DoTrash, "%d downloaded game folders to the recycle bin")
	case app.UnSteamCMD:
		add(app.DoDelete, "delete the SteamCMD RimArcho downloaded")
		add(app.DoLeave, "it holds mods downloaded with SteamCMD: left alone")
	case app.UnData:
		add(app.DoLater, "once RimArcho has closed, delete its database, settings and caches (%d folders)")
		add(app.DoLeave, "%d folders kept")
	case app.UnProgram:
		add(app.DoLater, "once RimArcho has closed, delete the program")
		add(app.DoLeave, "the program stays")
	}
	return strings.Join(parts, T("; "))
}
