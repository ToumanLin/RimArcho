// Writes spikes/wezterm-v0/wezterm.lua with the product generator (internal/wezterm), for
// the V1/V2 screenshots.
//
//	go run ./spikes/tuiv2-v0/genconfig <font dir> <out file>
package main

import (
	"os"

	"github.com/rimarcho/rimarcho/internal/wezterm"
)

func main() {
	if err := os.WriteFile(os.Args[2], []byte(wezterm.Config(os.Args[1])), 0o644); err != nil {
		panic(err)
	}
}
