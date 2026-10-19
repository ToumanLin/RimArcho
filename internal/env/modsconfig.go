package env

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strings"
)

// ModsConfig is the read-only view of the game's Config/ModsConfig.xml. M1 never writes it.
type ModsConfig struct {
	Version         string   `xml:"version"`
	ActiveMods      []string `xml:"activeMods>li"`
	KnownExpansions []string `xml:"knownExpansions>li"`
}

// ReadModsConfig parses ModsConfig.xml, tolerating a BOM.
func ReadModsConfig(path string) (*ModsConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var mc ModsConfig
	if err := d.Decode(&mc); err != nil {
		return nil, err
	}
	for i, m := range mc.ActiveMods {
		mc.ActiveMods[i] = strings.ToLower(strings.TrimSpace(m))
	}
	return &mc, nil
}
