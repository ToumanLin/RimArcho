// Package rimarcho carries files from the repository's root into the program: CHANGELOG.md,
// shown in Settings › About (16 §2.14). go:embed reaches only files under the package's own
// folder, so this one package lives at the root.
package rimarcho

import _ "embed"

// Changelog is CHANGELOG.md as built; it is never fetched from the network.
//
//go:embed CHANGELOG.md
var Changelog string
