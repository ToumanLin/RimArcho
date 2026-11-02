package view

func init() {

	for _, key := range []string{"community data destination is not a regular file: %s", "community data rollback failed; backups kept at %s: %w"} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}
