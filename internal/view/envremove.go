package view

import "github.com/ToumanLin/RimArcho/internal/app"

// Removing an environment (06 §2.14): the dry run says what goes and what stays, then one
// confirmation removes it.

// EnvRemovalTitle is the confirmation's title.
func EnvRemovalTitle(r *app.EnvRemoval) string { return Tf("remove game %s", r.Env.Name) }

// EnvRemovalLines says what removing the environment forgets and what stays.
func EnvRemovalLines(r *app.EnvRemoval) []string {
	out := []string{
		Tf("RimArcho forgets: %d lists, %d launch records, %d mods read from its Data/ and Mods/, its history", r.Plans, r.Launches, r.Mods),
		T("stays: the game folder, every mod and save; the shared mod folders and what you noted on mods"),
	}
	if r.Current {
		if r.Next != "" {
			out = append(out, Tf("then %s is in use", r.Next))
		} else {
			out = append(out, T("no game is left"))
		}
	}
	return out
}

func init() {
	Register(map[string]string{
		"remove game %s":  "移除游戏 %s",
		"remove the game": "移除游戏",
		"RimArcho forgets: %d lists, %d launch records, %d mods read from its Data/ and Mods/, its history": "RimArcho 忘掉：%d 个列表、%d 条启动记录、从它的 Data/ 和 Mods/ 读到的 %d 个 Mod，以及它的历史",
		"stays: the game folder, every mod and save; the shared mod folders and what you noted on mods":     "保留：游戏文件夹、所有 mod 和存档；共享的 Mod 文件夹和你给 mod 记下的内容",
		"then %s is in use": "之后使用 %s",
		"no game is left":   "之后不再有游戏",
		"removed game %s":   "已移除游戏 %s",
	})
}
