package view

import "regexp"

// Can it run's card text (needs.go, evidence.go): the sections, the evidence chips, each
// item's bold line and its explanation (06 §2.4, §5).
func init() {
	interfaceFormats = append(interfaceFormats,
		interfaceFormat{"no usable installed copy for missing package %s", regexp.MustCompile(`^no usable installed copy for missing package (.+)$`)},
		interfaceFormat{"installed copy for %s is no longer available", regexp.MustCompile(`^installed copy for (.+) is no longer available$`)},
	)

}
