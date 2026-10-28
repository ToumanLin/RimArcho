package view

func init() {
	Register(map[string]string{
		"does not list %s as supported (lists %s)":                               "没有标明支持 %s（标明了 %s）",
		"it may still work, but its author has not said it does on this version": "它可能仍能用，但作者还没有标明支持这个版本。",
		"ignore the %s warning for this mod":                                     "忽略此 Mod 的 %s 提醒",
		"the %s warning for this mod is ignored":                                 "已忽略此 Mod 的 %s 提醒",
		"restore the %s warning":                                                 "恢复 %s 提醒",
		"the %s warning for this mod is restored":                                "已恢复此 Mod 的 %s 提醒",
		"ignore these %d mods' %s warnings":                                      "忽略这 %d 个 Mod 的 %s 提醒",
		"%d mods' %s warnings are ignored":                                       "%d 个 Mod 的 %s 提醒已忽略",
		"%d mods' %s warnings are restored":                                      "已恢复 %d 个 Mod 的 %s 提醒",
		"press M again to ignore these version warnings":                         "再按一次 M：忽略这些 Mod 的版本提醒",
		"Ignore these %d mods' %s warnings in every list and game? You can restore them from the ignored list.": "忽略这 %d 个 Mod 的 %s 提醒？对所有列表和游戏生效，可在已忽略提醒的清单里恢复。",
		"does not list %s as supported": "没有标明支持 %s",
		"lists %s":                      "标明了 %s",
		"only lists %s or earlier":      "只标明了 %s 或更早",
		"view or restore":               "查看或恢复",
		"ignore all":                    "全部忽略",
		"restore all":                   "全部恢复",
		"ignore group":                  "忽略这组",
		"restore group":                 "恢复这组",
		"restore":                       "恢复",
		"no version warnings":           "没有版本提醒",
		"show in list":                  "在列表中显示",
		"the game version changed; reopen the version warnings": "游戏版本已改变，请重新打开版本提醒清单。",
	})
}
