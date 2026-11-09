package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

const srcProject = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net472</TargetFramework>
    <AssemblyName>SrcMod</AssemblyName>
    <OutputPath>..\..\1.6\Assemblies\</OutputPath>
    <AppendTargetFrameworkToOutputPath>false</AppendTargetFrameworkToOutputPath>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Lib.Harmony" Version="2.3.3" />
    <PackageReference Include="Krafs.Rimworld.Ref" Version="1.6.4633" />
    <PackageReference Include="Krafs.Publicizer" Version="2.2.1" />
    <Publicize Include="Assembly-CSharp:Verse.Pawn.health" />
    <Reference Include="Assembly-CSharp"><HintPath>..\..\..\..\Managed\Assembly-CSharp.dll</HintPath></Reference>
  </ItemGroup>
</Project>`

// 05 §5.6 (h8 G3): get_mod / archo mod says how a Mod that ships source is built, and says
// nothing for one that does not.
func TestModBuildBlock(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	src := testkit.Sample("tester.src", "Source Mod")
	src.Files["Source/Src/SrcMod.csproj"] = srcProject
	src.Files["Source/Src/Patches.cs"] = "class P {}"
	src.Files["Source/Src/Sub/More.cs"] = "class M {}"
	src.Files["Source/Src/obj/Gen.cs"] = "generated"
	e.WriteMod(filepath.Join(e.Game, "Mods", "SrcMod"), src)
	dll := testkit.Sample("tester.dll", "Dll Mod")
	dll.Files["Assemblies/Dll.dll"] = "not source"
	e.WriteMod(filepath.Join(e.Game, "Mods", "DllMod"), dll)
	loose := testkit.Sample("tester.loose", "Loose Mod")
	loose.Files["Source/Fix.cs"] = "class F {}"
	e.WriteMod(filepath.Join(e.Game, "Mods", "LooseMod"), loose)
	scanNow(t, e)

	e.App.Dotnet = func() string { return "" }
	out := e.MustRun("mod", "pkg:tester.src")
	srcDir := filepath.Join(e.Game, "Mods", "SrcMod")
	for _, want := range []string{
		"build: the Mod ships source: 1 project, 2 .cs files",
		"project: Source/Src/SrcMod.csproj  2 .cs files  Microsoft.NET.Sdk  net472",
		"packages: Lib.Harmony 2.3.3 (Harmony), Krafs.Rimworld.Ref 1.6.4633 (the game's reference assemblies), Krafs.Publicizer 2.2.1",
		"publicizer: Krafs.Publicizer makes private game members reachable: Assembly-CSharp:Verse.Pawn.health.",
		"FieldAccessException",
		"references: not found: Assembly-CSharp at ..\\..\\..\\..\\Managed\\Assembly-CSharp.dll",
		"output: " + filepath.Join(srcDir, "1.6", "Assemblies") + "; no SrcMod.dll there yet",
		"run in: " + filepath.Join(srcDir, "Source", "Src"),
		"restore: dotnet restore",
		"build: dotnet build -c Release",
		"dotnet: not found on PATH",
		"help: archo guide rebuild",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(mcpRun(e, "mod", "pkg:tester.src"), "help: guide topic=rebuild") {
		t.Error("MCP wording of the help line")
	}

	e.App.Dotnet = func() string { return filepath.Join(e.Root, "sdk", "dotnet") }
	e.WriteFile(filepath.Join(srcDir, "1.6", "Assemblies", "SrcMod.dll"), strings.Repeat("x", 2048))
	out = e.MustRun("mod", "pkg:tester.src")
	if !strings.Contains(out, "dotnet: "+filepath.Join(e.Root, "sdk", "dotnet")) || !strings.Contains(out, "; SrcMod.dll there now: 2.0 KB") {
		t.Errorf("dotnet found, DLL built:\n%s", out)
	}

	// The built DLL checked against the game: a csc build of publicized code is refused at run
	// time, the same code built unsafe (as Krafs.Publicizer builds it) is not (MCP eval h8).
	managed := filepath.Join(e.Game, "RimWorldWin64_Data", "Managed")
	access := filepath.Join("..", "..", "testdata", "dotnet", "bin", "access")
	copyTo(t, filepath.Join(access, "Assembly-CSharp.dll"), filepath.Join(managed, "Assembly-CSharp.dll"))
	built := filepath.Join(srcDir, "1.6", "Assemblies", "SrcMod.dll")
	copyTo(t, filepath.Join(access, "plain", "AccessMod.dll"), built)
	out = e.MustRun("mod", "pkg:tester.src")
	if !strings.Contains(out, "built: !! SrcMod.dll uses 2 non-public game members (Verse.StatWorker::Secret(), Verse.StatWorker::stat) and does not skip verification: the game throws FieldAccessException / MethodAccessException when it runs that code. Build it with dotnet build and the project's publicizer (guide topic=rebuild), not csc") {
		t.Errorf("csc build:\n%s", out)
	}
	for _, v := range []string{"unsafe", "sub"} {
		copyTo(t, filepath.Join(access, v, "AccessMod.dll"), built)
		if out := e.MustRun("mod", "pkg:tester.src"); strings.Contains(out, "built:") {
			t.Errorf("%s build:\n%s", v, out)
		}
	}

	if out := e.MustRun("mod", "pkg:tester.dll"); strings.Contains(out, "build:") {
		t.Errorf("a Mod with only a DLL has a build block:\n%s", out)
	}
	out = e.MustRun("mod", "pkg:tester.loose")
	if !strings.Contains(out, "build: C# source without a project file") || !strings.Contains(out, "1 .cs files in Source/") {
		t.Errorf("source without a project:\n%s", out)
	}
}

func copyTo(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
