package view

func init() {
	Register(map[string]string{
		"database compacted: %s → %s; freed %s":                                                            "数据库已压缩：%s → %s，释放了 %s",
		"database maintenance: would remove %d old file records and compact the database":                  "数据库整理：将清理 %d 条旧文件记录并压缩数据库",
		"cannot compact database: game %s is running (pid %d); close the game first":                       "无法压缩数据库：游戏 %s 正在运行（进程 %d），请先关闭游戏",
		"cannot compact database: need %d bytes of free disk space; free space is insufficient or unknown": "无法压缩数据库：需要 %d 字节的可用磁盘空间，当前空间不足或无法确定",
		"database is busy; try compacting again later":                                                     "数据库正忙，请稍后再压缩",
	})
	for _, key := range []string{
		"database compacted: %s → %s; freed %s",
		"database maintenance: would remove %d old file records and compact the database",
		"cannot compact database: game %s is running (pid %d); close the game first",
		"cannot compact database: need %d bytes of free disk space; free space is insufficient or unknown",
	} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}
