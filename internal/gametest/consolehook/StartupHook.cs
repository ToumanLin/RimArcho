// A .NET startup hook that RimArcho names in DOTNET_STARTUP_HOOKS for RimWorld 1.7 test runs
// (docs/specs 20 §2). RimWorld.exe is a GUI program without a console, so .NET encodes console
// output in the system ANSI code page and turns characters it cannot represent into '?'.
// Setting the encoding before Main makes the captured stdout and stderr UTF-8. Without a
// console, .NET ignores the failing SetConsoleOutputCP and keeps the encoding.
internal class StartupHook
{
    public static void Initialize()
    {
        try
        {
            System.Console.OutputEncoding = new System.Text.UTF8Encoding(false);
        }
        catch (System.Exception)
        {
            // Never stop the game from starting; the log then keeps the ANSI encoding.
        }
    }
}
