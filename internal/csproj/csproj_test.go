package csproj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func put(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const modernProj = `<?xml version="1.0" encoding="utf-8"?>
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net472</TargetFramework>
    <AssemblyName>MyMod</AssemblyName>
    <OutputPath>..\..\1.6\Assemblies\</OutputPath>
    <AppendTargetFrameworkToOutputPath>false</AppendTargetFrameworkToOutputPath>
    <LangVersion>latest</LangVersion>
  </PropertyGroup>
  <PropertyGroup Condition="'$(Configuration)' == 'Debug'">
    <OutputPath>bin\Debug\</OutputPath>
  </PropertyGroup>
  <ItemGroup>
    <PackageReference Include="Lib.Harmony" Version="2.3.3" ExcludeAssets="runtime" />
    <PackageReference Include="Krafs.Rimworld.Ref" Version="1.6.4633" />
    <PackageReference Include="Krafs.Publicizer" Version="2.2.1"><PrivateAssets>all</PrivateAssets></PackageReference>
    <Publicize Include="Assembly-CSharp:Verse.Pawn.health" />
    <Publicize Include="Assembly-CSharp:RimWorld.Pawn_NeedsTracker.needs" />
  </ItemGroup>
</Project>`

func TestReadModernProject(t *testing.T) {
	root := t.TempDir()
	put(t, root, "About/About.xml", "<ModMetaData/>")
	put(t, root, "Source/MyMod/MyMod.csproj", "\xEF\xBB\xBF"+modernProj)
	put(t, root, "Source/MyMod/A.cs", "class A {}")
	put(t, root, "Source/MyMod/B.cs", "class B {}")
	put(t, root, "Source/MyMod/Sub/C.cs", "class C {}")
	put(t, root, "Source/MyMod/obj/Debug/D.cs", "generated")
	put(t, root, "Source/MyMod/bin/E.cs", "generated")
	put(t, root, "Textures/fake/Z.cs", "not source")
	put(t, root, "1.6/Assemblies/MyMod.dll", "dll")

	s := Find(root)
	if s == nil || len(s.Projects) != 1 || len(s.Loose) != 0 {
		t.Fatalf("Find: %+v", s)
	}
	p := s.Projects[0]
	if p.Err != "" || p.Legacy || p.SDK != "Microsoft.NET.Sdk" || p.Name != "MyMod" || p.AssemblyName != "MyMod" || p.DLL() != "MyMod.dll" {
		t.Errorf("project: %+v", p)
	}
	if !reflect.DeepEqual(p.Frameworks, []string{"net472"}) || p.LangVersion != "latest" {
		t.Errorf("frameworks %v lang %q", p.Frameworks, p.LangVersion)
	}
	if p.Sources != 3 {
		t.Errorf("%d sources, want 3 (obj, bin and Textures are not source)", p.Sources)
	}
	if want := filepath.Join(root, "1.6", "Assemblies"); p.OutputAbs != want || p.TFMAppended || p.OutputDefault {
		t.Errorf("output %q abs %q appended %v, want abs %q", p.Output, p.OutputAbs, p.TFMAppended, want)
	}
	if len(p.Packages) != 3 || p.Packages[0] != (Package{"Lib.Harmony", "2.3.3"}) || p.Packages[2] != (Package{"Krafs.Publicizer", "2.2.1"}) {
		t.Errorf("packages %+v", p.Packages)
	}
	if !p.Publicizer || p.PublicizerPkg != "Krafs.Publicizer" || len(p.Publicize) != 2 || p.Publicize[0] != "Assembly-CSharp:Verse.Pawn.health" {
		t.Errorf("publicizer %v %q %v", p.Publicizer, p.PublicizerPkg, p.Publicize)
	}
}

func TestFrameworkFolderAndConditions(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Source/a.csproj", `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net48</TargetFramework>
    <OutputPath>..\Assemblies</OutputPath>
  </PropertyGroup>
  <PropertyGroup Condition="'$(Configuration)'=='Release' And '$(Platform)'=='AnyCPU'">
    <OutputPath>..\Release\</OutputPath>
  </PropertyGroup>
  <PropertyGroup Condition="Exists('nothing.txt')"><AssemblyName>NotThis</AssemblyName></PropertyGroup>
</Project>`)
	p := Read(filepath.Join(root, "Source", "a.csproj"), root)
	if !p.TFMAppended || filepath.Base(p.OutputAbs) != "net48" || filepath.Base(filepath.Dir(p.OutputAbs)) != "Release" {
		t.Errorf("the Release group and the framework folder: %q (appended %v)", p.OutputAbs, p.TFMAppended)
	}
	if p.AssemblyName != "a" {
		t.Errorf("a condition on a missing file applied: %q", p.AssemblyName)
	}
	// No output path at all: the SDK default.
	put(t, root, "Source/b.csproj", `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net472</TargetFramework></PropertyGroup></Project>`)
	b := Read(filepath.Join(root, "Source", "b.csproj"), root)
	if !b.OutputDefault || b.OutputAbs != filepath.Join(root, "Source", "bin", "Release", "net472") {
		t.Errorf("default output %q", b.OutputAbs)
	}
}

func TestReadOldStyleProject(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Source/Old/Old.csproj", `<?xml version="1.0" encoding="utf-8"?>
<Project ToolsVersion="15.0" DefaultTargets="Build" xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
  <Import Project="$(MSBuildExtensionsPath)\$(MSBuildToolsVersion)\Microsoft.Common.props" Condition="Exists('$(MSBuildExtensionsPath)\$(MSBuildToolsVersion)\Microsoft.Common.props')" />
  <PropertyGroup>
    <AssemblyName>OldMod</AssemblyName>
    <TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion>
    <OutputPath>..\..\Assemblies\</OutputPath>
  </PropertyGroup>
  <ItemGroup>
    <Reference Include="Assembly-CSharp, Version=1.0.0.0, Culture=neutral, PublicKeyToken=null">
      <HintPath>$(RimWorldDir)\Assembly-CSharp.dll</HintPath>
    </Reference>
    <Reference Include="0Harmony"><HintPath>..\..\Lib\0Harmony.dll</HintPath></Reference>
    <Reference Include="System.Core" />
  </ItemGroup>
  <ItemGroup>
    <Compile Include="Main.cs" />
    <Compile Include="Sub\*.cs" />
    <Compile Include="Missing.cs" />
    <Compile Include="Properties\AssemblyInfo.cs" />
  </ItemGroup>
</Project>`)
	put(t, root, "Source/Old/Main.cs", "")
	put(t, root, "Source/Old/Sub/One.cs", "")
	put(t, root, "Source/Old/Sub/Two.cs", "")
	put(t, root, "Source/Old/Properties/AssemblyInfo.cs", "")
	put(t, root, "Source/Old/Unlisted.cs", "") // an old-style project does not compile it
	p := Find(root).Projects[0]
	if !p.Legacy || p.SDK != "" || p.AssemblyName != "OldMod" || !reflect.DeepEqual(p.Frameworks, []string{"v4.7.2"}) {
		t.Errorf("old style: %+v", p)
	}
	if p.Sources != 4 {
		t.Errorf("%d sources, want the 4 listed files that exist", p.Sources)
	}
	if want := filepath.Join(root, "Assemblies"); p.OutputAbs != want || p.TFMAppended {
		t.Errorf("output %q, want %q", p.OutputAbs, want)
	}
	if !reflect.DeepEqual(p.Unresolved, []string{"RimWorldDir"}) {
		t.Errorf("unresolved %v (MSBuildExtensionsPath imports are not reported)", p.Unresolved)
	}
	if len(p.References) != 3 || p.References[0].Name != "Assembly-CSharp" || p.References[0].Missing || !p.References[1].Missing || p.References[2].HintPath != "" {
		t.Errorf("references %+v", p.References)
	}
}

func TestInheritedProps(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Source/Directory.Build.props", `<Project>
  <PropertyGroup><TargetFramework>net472</TargetFramework><OutputPath>$(MSBuildThisFileDirectory)..\Assemblies\</OutputPath>
  <AppendTargetFrameworkToOutputPath>false</AppendTargetFrameworkToOutputPath></PropertyGroup>
  <Import Project="Common.props" />
</Project>`)
	put(t, root, "Source/Common.props", `<Project><ItemGroup><PackageReference Include="Lib.Harmony" /></ItemGroup></Project>`)
	put(t, root, "Source/Directory.Packages.props", `<Project><ItemGroup><PackageVersion Include="Lib.Harmony" Version="2.2.2" /></ItemGroup></Project>`)
	put(t, root, "Source/Mod/Mod.csproj", `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup>
  <ItemGroup><Compile Include="**\*.cs" /><Compile Remove="Tests\**" /><Compile Include="..\Shared\*.cs" /></ItemGroup></Project>`)
	put(t, root, "Source/Mod/A.cs", "")
	put(t, root, "Source/Mod/Deep/B.cs", "")
	put(t, root, "Source/Mod/Tests/T.cs", "")
	put(t, root, "Source/Shared/S.cs", "")
	put(t, root, "Source/Other/Other.cs", "")
	p := Find(root).Projects[0]
	if !reflect.DeepEqual(p.Frameworks, []string{"net472"}) || p.OutputAbs != filepath.Join(root, "Assemblies") {
		t.Errorf("inherited framework %v output %q", p.Frameworks, p.OutputAbs)
	}
	if len(p.Packages) != 1 || p.Packages[0] != (Package{"Lib.Harmony", "2.2.2"}) {
		t.Errorf("central package version: %+v", p.Packages)
	}
	if p.Sources != 3 { // A, Deep/B and the linked Shared/S; Tests removed
		t.Errorf("%d sources, want 3", p.Sources)
	}
	if !reflect.DeepEqual(p.Inherited, []string{"../Directory.Build.props", "../Common.props", "../Directory.Packages.props"}) {
		t.Errorf("inherited %v", p.Inherited)
	}
}

func TestPublicizerForms(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a.csproj", `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><PublicizeAll>true</PublicizeAll></PropertyGroup>
  <ItemGroup><PackageReference Include="Krafs.Publicizer"><Version>2.3.0</Version></PackageReference></ItemGroup></Project>`)
	put(t, root, "b.csproj", `<Project Sdk="Microsoft.NET.Sdk"><ItemGroup>
  <Reference Include="Assembly-CSharp" Publicize="true"><HintPath>x.dll</HintPath></Reference></ItemGroup></Project>`)
	put(t, root, "c.csproj", `<Project Sdk="Microsoft.NET.Sdk"><ItemGroup><PackageReference Include="Lib.Harmony" Version="2.3.3" /></ItemGroup></Project>`)
	a, b, c := Read(filepath.Join(root, "a.csproj"), root), Read(filepath.Join(root, "b.csproj"), root), Read(filepath.Join(root, "c.csproj"), root)
	if !a.Publicizer || !a.PublicizeAll || a.Packages[0].Version != "2.3.0" {
		t.Errorf("a: %+v", a)
	}
	if !b.Publicizer || b.PublicizerPkg != "" {
		t.Errorf("b: %+v", b)
	}
	if c.Publicizer {
		t.Errorf("c: %+v", c)
	}
}

func TestFindWithoutProjects(t *testing.T) {
	root := t.TempDir()
	put(t, root, "About/About.xml", "<ModMetaData/>")
	put(t, root, "Assemblies/Mod.dll", "dll")
	put(t, root, "Textures/Z.cs", "")
	if s := Find(root); s != nil {
		t.Errorf("a Mod with only a DLL: %+v", s)
	}
	put(t, root, "Source/Patch.cs", "")
	put(t, root, "Source/More.cs", "")
	put(t, root, "Other/One.cs", "")
	put(t, root, "Source/Mod.sln", "")
	s := Find(root)
	if s == nil || len(s.Projects) != 0 || len(s.Loose) != 2 || s.Loose[0] != (LooseDir{"Source", 2}) || s.Loose[1] != (LooseDir{"Other", 1}) {
		t.Fatalf("loose source: %+v", s)
	}
	if !reflect.DeepEqual(s.Solutions, []string{"Source/Mod.sln"}) {
		t.Errorf("solutions %v", s.Solutions)
	}
}

func TestBrokenProject(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Source/a.csproj", "this is not xml")
	put(t, root, "Source/b.csproj", `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net472</TargetFramework></Prop`)
	s := Find(root)
	if len(s.Projects) != 2 || s.Projects[0].Err == "" {
		t.Fatalf("broken: %+v", s.Projects)
	}
	if b := s.Projects[1]; b.Err != "" || !reflect.DeepEqual(b.Frameworks, []string{"net472"}) {
		t.Errorf("a truncated file keeps what was read: %+v", b)
	}
}

func TestDotnetOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "dotnet")
	t.Setenv("ARCHO_DOTNET", want)
	if got := Dotnet(); got != want {
		t.Errorf("Dotnet() = %q", got)
	}
}

func TestGlob(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	for _, c := range []struct {
		pat, path string
		want      bool
	}{
		{`**\*.cs`, "a.cs", true}, {`**\*.cs`, "x/y/a.cs", true}, {`*.cs`, "x/a.cs", false},
		{`Tests\**`, "Tests/a/b.cs", true}, {`Tests\**`, "Other/a.cs", false}, {`Sub\?.cs`, "Sub/a.cs", true},
		{`Sub\?.cs`, "Sub/ab.cs", false}, {`A.cs`, "a.cs", true},
	} {
		g := newGlob(dir, c.pat)
		if got := g.match(strings.ToLower(filepath.ToSlash(dir)) + "/" + c.path); got != c.want {
			t.Errorf("%q matches %q: %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}
