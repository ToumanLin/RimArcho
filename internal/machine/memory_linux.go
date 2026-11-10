package machine

func Read() Memory { return readMemoryAt("/proc") }
