package view

func init() {

	for _, key := range []string{
		"database compacted: freed %s (now %s)",
		"database maintenance: would remove %d old file records and compact the database",
		"cannot compact database: game %s is running (pid %d); close the game first",
		"cannot compact database: it needs %s of free disk space",
	} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}
