package view

func init() {
	Register(map[string]string{
		"database compacted: freed %s (now %s)":                                           "数据库已压缩：释放了 %s，现在 %s",
		"database maintenance: would remove %d old file records and compact the database": "数据库整理：将清理 %d 条旧文件记录并压缩数据库",
		"cannot compact database: game %s is running (pid %d); close the game first":      "无法压缩数据库：游戏 %s 正在运行（进程 %d），请先关闭游戏",
		"cannot compact database: it needs %s of free disk space":                         "无法压缩数据库：需要 %s 可用磁盘空间",
		"database is busy; try compacting again later":                                    "数据库正忙，请稍后再压缩",
	})
	for _, key := range []string{
		"database compacted: freed %s (now %s)",
		"database maintenance: would remove %d old file records and compact the database",
		"cannot compact database: game %s is running (pid %d); close the game first",
		"cannot compact database: it needs %s of free disk space",
	} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}
