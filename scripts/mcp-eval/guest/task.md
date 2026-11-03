# Task

A RimWorld 1.6 game is installed at `C:\Eval\RimWorld`. The player's mods are in `C:\Eval\RimWorld\Mods`. Their active list is `{USERDATA}\Config\ModsConfig.xml`, and the log of the last game start is `{USERDATA}\Player.log`.

The last start showed errors. Find the mod or mods responsible and fix them.

You have PowerShell, the .NET SDK (`dotnet`) and `ilspycmd` (a decompiler). The game's assemblies are in `C:\Eval\RimWorld\RimWorldWin64_Data\Managed`.

## What to hand in

- Save every file you changed or rebuilt under `C:\Eval\submit`, at `<packageId>\<path inside that mod's folder>`. For example `C:\Eval\submit\some.package.id\Defs\Things.xml`, or a rebuilt assembly at `C:\Eval\submit\some.package.id\1.6\Assemblies\Some.dll`. A changed list of active mods goes to `C:\Eval\submit\ModsConfig.xml`.
- Then write `C:\Eval\submit\answer.json`:

```json
{"culprit": ["<packageId of each mod responsible>"], "explanation": "<one or two sentences>"}
```

Your work is checked by starting the game with your files applied. You have about 15 minutes.
