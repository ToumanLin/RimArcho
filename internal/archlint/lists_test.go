package archlint

// allExceptionLists names every exception list, for TestArchReasons.
func allExceptionLists() map[string]map[string]string {
	return map[string]map[string]string{
		"importAllowed":        importAllowed,
		"readOnlyAllowed":      readOnlyAllowed,
		"surfaceWritesAllowed": surfaceWritesAllowed,
		"childWindowAllowed":   childWindowAllowed,
		"listenAllowed":        listenAllowed,
		"exitAllowed":          exitAllowed,
		"ansiAllowed":          ansiAllowed,
		"stdioAllowed":         stdioAllowed,
	}
}
