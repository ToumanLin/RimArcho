package scan

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// 10 §1: an About.xml saved as UTF-16 (with a BOM) is read like UTF-8.
func TestReadDeclUTF16(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "About"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `<?xml version="1.0" encoding="utf-16"?>
<ModMetaData><packageId>Tester.Wide</packageId><name>宽字符 Mod</name><supportedVersions><li>1.6</li></supportedVersions></ModMetaData>`
	b := binary.LittleEndian.AppendUint16(nil, 0xFEFF)
	for _, u := range utf16.Encode([]rune(doc)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	if err := os.WriteFile(filepath.Join(dir, "About", "About.xml"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	d := ReadDecl(dir, false)
	if d.PackageID != "tester.wide" || d.Name != "宽字符 Mod" {
		t.Fatalf("decl = %q %q", d.PackageID, d.Name)
	}
}
