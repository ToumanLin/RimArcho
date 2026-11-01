package view

import "strings"

var selfUpdateErrors = []string{
	"a development build is not updated; build or download a release",
	"RimArcho %s is the newest version known; check for updates first",
	"cannot write to the program folder %s; download the new version from %s",
	"download failed; nothing was changed: %w",
	"the release zip cannot be used; nothing was changed: %w",
	"stopped while waiting for the other RimArcho to close; nothing was changed (the download is kept): %w",
	"could not replace the program files; RimArcho %s is unchanged: %w",
	"the new program did not start (%s), and putting RimArcho %s back failed: %w; the old files are in %s",
	"the new program did not start, so RimArcho %s was put back: %w",
	"the downloaded RimArcho %s does not match its checksum; nothing was changed",
	"another RimArcho is already installing the update",
	"cannot check for updates: %w",
}

func init() {

	for _, key := range selfUpdateErrors {
		if strings.Contains(key, "%") {
			interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
		}
	}
}
