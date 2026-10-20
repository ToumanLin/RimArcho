// Command winres writes cmd/archo's Windows resources (08 §15.3): the keystone icon group
// and the version information. The release script runs it before each of the two builds.
//
//	go run ./scripts/winres -version 0.7.0 -exe RimArcho.exe -desc RimArcho
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ToumanLin/RimArcho/internal/icon"
	"github.com/ToumanLin/RimArcho/internal/winres"
)

func main() {
	version := flag.String("version", "0.0.0", "version, 1.2.3 or 1.2.3-rc1")
	exe := flag.String("exe", "archo.exe", "OriginalFilename")
	desc := flag.String("desc", "RimArcho command line", "FileDescription")
	out := flag.String("o", "cmd/archo/rsrc_windows_amd64.syso", "output .syso")
	flag.Parse()
	var icons []winres.Icon
	for _, s := range icon.Sizes {
		icons = append(icons, winres.Icon{Size: s, PNG: icon.PNG(s)})
	}
	b, err := winres.Syso(icons, winres.Version{Version: *version, Description: *desc, Filename: *exe,
		Product: "RimArcho", Copyright: "Copyright (c) 2026 Touman Lin, MIT License"})
	if err == nil {
		err = os.WriteFile(*out, b, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}
}
