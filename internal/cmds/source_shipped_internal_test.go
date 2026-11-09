package cmds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/csproj"
)

// read_source on a Mod that ships a project names its source, the file of the type asked for
// (not a copy under obj/), and the rebuild route (05 §5.6; MCP eval h8: agents compiled the
// shipped source with csc).
func TestShippedSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("1.6/Source/Edu.csproj", `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net48</TargetFramework></PropertyGroup>
  <ItemGroup><PackageReference Include="Krafs.Publicizer" Version="2.*" /><Publicize Include="Assembly-CSharp" /></ItemGroup></Project>`)
	write("1.6/Source/obj/Debug/Copy.cs", "namespace Edu { public class LordToil_AttendClass {} }")
	write("1.6/Source/AI/LordToil_AttendClass.cs", "namespace Edu {\n  public class LordToil_AttendClass : LordToil {}\n}")
	write("1.6/Source/AI/Other.cs", "namespace Edu { class LordToil_AttendClassHelper {} }")
	b := &app.BuildInfo{Source: csproj.Find(root)}
	want := "the Mod ships its source: 1.6/Source/Edu.csproj; this type: 1.6/Source/AI/LordToil_AttendClass.cs. To change the Mod, edit that source and rebuild it with dotnet build (get_mod build:, guide topic=rebuild); it uses a publicizer, so a DLL compiled with csc throws FieldAccessException in the game"
	if got := shippedSource(root, b, "Edu.LordToil_AttendClass"); got != want {
		t.Errorf("type:\n got %s\nwant %s", got, want)
	}
	if got := shippedSource(root, b, ""); got != "the Mod ships its source: 1.6/Source/Edu.csproj. To change the Mod, edit that source and rebuild it with dotnet build (get_mod build:, guide topic=rebuild); it uses a publicizer, so a DLL compiled with csc throws FieldAccessException in the game" {
		t.Errorf("no type: %s", got)
	}
	if got := shippedSource(root, b, "Edu.Missing"); got == "" || strings.Contains(got, "this type:") {
		t.Errorf("unknown type: %s", got)
	}
}
