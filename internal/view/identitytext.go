package view

func init() {
	Register(map[string]string{
		"why this question?": "为什么这样问",
		"treat as one mod in lists and combine their change history":                        "列表按同一份 Mod 处理，合并变化历史",
		"keep two copies in lists with separate change histories; share mod notes and tags": "列表保留两份，各记变化历史，共用备注和标签",
		"keep lists and histories separate; close this question":                            "列表和历史各自保留，关闭这条提问",
		"this answer cannot be undone with u":                                               "这个回答不能用 u 撤销",
		"the comparison records are no longer available":                                    "当时的比较记录已不在，无法说明原因",
		"not recorded": "未记录",
		"recorded files and declarations; folders and their Workshop IDs are current; values are new / previous": "文件和声明取自提问时；文件夹及目录中的工坊编号取自目前记录。斜线前后依次是新发现的、之前记录的 Mod",
		"package ID":               "包名",
		"mod name":                 "Mod 名称",
		"Workshop ID":              "工坊编号",
		"mod authors":              "作者",
		"current folders":          "目前的文件夹",
		"%s: incomplete (%s / %s)": "%s有缺失：%s / %s",
		"%s matches: %s":           "%s相同：%s",
		"%s differs: %s / %s":      "%s不同：%s / %s",
		"file differences: %d shared paths have different contents; %d / %d paths occur on only one side":     "相同路径里有 %d 个文件内容不同；仅一边有的路径各 %d / %d 个",
		"checked files: %d / %d; %d shared paths, %d identical contents":                                      "核对过内容的文件：%d / %d 个；其中 %d 个路径相同，%d 个内容相同",
		"mod content in common: %d of %d / %d distinct nonempty files (%d%% overlap)":                         "Mod 内容有 %d 份相同，两边各有 %d / %d 份不同的非空内容，重合 %d%%",
		"previews, About files, load-folder rules and repeated or empty contents do not count as mod content": "预览图、About 文件、加载目录规则不作为 Mod 内容凭据；空内容不算，重复内容只算一次",
		"content in common: %s":                                                                                   "内容相同的文件：%s",
		"and %d more matching contents":                                                                           "另有 %d 份相同内容",
		"asked because the package or Workshop ID and mod content match":                                          "提问依据：包名或工坊编号相同，Mod 内容也有足够重合",
		"asked because at least three distinct mod contents match and overlap is at least 60%%":                   "提问依据：至少三份不同的 Mod 内容相同，重合达到 60%%",
		"this old question lacks enough content evidence; the next scan removes it if unanswered":                 "这条旧提问缺少足够的内容凭据；未回答时，下次扫描会移除它",
		"folder names and authors alone do not establish a match; textures and audio are not compared by content": "只凭文件夹名或作者不能判断是同一个 Mod；贴图和音频没有核对内容",

		"do not list as a copy":                                                  "不列为副本",
		"%s is now listed independently":                                         "已将 %s 单独列出",
		"choose one copy to list independently":                                  "请选择一份要单独列出的副本",
		"this copy is already listed independently or is no longer available":    "这一份已单独列出，或已不在 Mod 库中",
		"list this copy as an independent mod; package ID conflicts still apply": "将这一份作为独立 Mod 列出；同包名的加载冲突仍然存在",
	})
}
