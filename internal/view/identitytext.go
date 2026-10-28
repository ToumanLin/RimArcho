package view

func init() {
	Register(map[string]string{
		"why this question?": "为什么这样问",
		"treat as one mod in lists and combine their change history":                        "列表按同一份 Mod 处理，合并变化历史",
		"keep two copies in lists with separate change histories; share mod notes and tags": "列表保留两份，各记变化历史，共用备注和标签",
		"keep lists and histories separate; close this question":                            "列表和历史各自保留，关闭这条提问",
		"this answer cannot be undone with u":                                               "这个回答不能用 u 撤销",
		"the comparison records are no longer available":                                    "当时的比较记录已不在，无法说明原因",
		"not enough matching content to support this question":                              "没有足够的相同内容支持这条提问",
		"not recorded":                          "未记录",
		"package ID":                            "包名",
		"mod name":                              "Mod 名称",
		"Workshop ID":                           "工坊编号",
		"mod authors":                           "作者",
		"comparison folder":                     "文件夹",
		"%d content files are identical":        "%d 个内容文件完全相同",
		"%d content files match (%d%% overlap)": "%d 个内容文件相同（重合 %d%%）",
		"%s; differences: %s":                   "%s，但%s不同",
		"%s, %s":                                "%s、%s",
		"%s matches: %s":                        "%s相同：%s",
		"%s: %s / %s":                           "%s不同：%s / %s",
		"matching file: %s":                     "相同文件：%s",
		"and %d more files":                     "另有 %d 个",
		"matching files":                        "相同的内容文件",
		"newly found":                           "新发现的",
		"previously recorded":                   "之前记录的",
		"matching values":                       "相同",
		"different values":                      "不同",

		"do not list as a copy":                                                  "不列为副本",
		"%s is now listed independently":                                         "已将 %s 单独列出",
		"choose one copy to list independently":                                  "请选择一份要单独列出的副本",
		"this copy is already listed independently or is no longer available":    "这一份已单独列出，或已不在 Mod 库中",
		"list this copy as an independent mod; package ID conflicts still apply": "将这一份作为独立 Mod 列出；同包名的加载冲突仍然存在",
	})
}
