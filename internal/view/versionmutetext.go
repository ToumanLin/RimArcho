package view

func init() {
	Register(map[string]string{
		"does not list %s as supported (lists %s)":                               "没有标明支持 %s（标明了 %s）",
		"it may still work, but its author has not said it does on this version": "它可能仍能用，但作者还没有标明支持这个版本。",
		"don't warn about %s for this mod":                                       "这个 Mod 不再提醒 %s",
		"warnings about %s are muted for this mod":                               "这个 Mod 已不再提醒 %s",
		"warn about %s again":                                                    "恢复 %s 的提醒",
		"warnings about %s are on again":                                         "已恢复 %s 的提醒",
		"don't warn about these %d mods":                                         "这 %d 个 Mod 不再提醒",
		"version warnings muted":                                                 "已关闭这些 Mod 的版本提醒",
		"press M again to mute these version warnings":                           "再按一次 M：关闭这些 Mod 的版本提醒",
		"Stop warning about %s for these %d mods in every list and game? You can turn warnings back on from the muted list.": "不再提醒未标明支持 %s 的这 %d 个 Mod？对所有列表和游戏生效，可在已关闭提醒的列表里恢复。",
		"%d mods muted":       "%d 个 Mod 已关闭提醒",
		"see muted mods":      "查看并恢复提醒",
		"no version warnings": "没有版本提醒",
	})
}
