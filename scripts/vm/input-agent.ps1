# Input agent for the test VM: real Windows mouse and keyboard input (SendInput) in the
# signed-in desktop session, driven from the host over TCP. Started by `vm.ps1 input start`
# with -interactive, so its input reaches every window on the guest screen: the native window
# frame, WebView2, Windows Terminal, dialogs. The host side is input.mjs; see README.md.
#
# Protocol: one JSON object per line each way. The first request must be
# {"cmd":"hello","token":"..."}. Commands are raw on purpose (one key, one button, one move);
# combos, clicks and drags are built on the host, where they are tested.
# Test VM only. Listens on 127.0.0.1; `vm.ps1 forward` exposes it to the host.
param(
    [int]$Port = 9230,
    [Parameter(Mandatory = $true)][string]$Token,
    [string]$PidFile = '',
    [string]$LogFile = ''
)
$ErrorActionPreference = 'Stop'
function Log([string]$s) { if ($LogFile) { Add-Content -Encoding UTF8 -LiteralPath $LogFile -Value ("{0:HH:mm:ss} {1}" -f (Get-Date), $s) } }
try {
    if ($PidFile) { Set-Content -Encoding ASCII -LiteralPath $PidFile -Value $PID }
    Add-Type -ReferencedAssemblies System.Windows.Forms, System.Web.Extensions, UIAutomationClient, UIAutomationTypes, WindowsBase -TypeDefinition @'
using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Net;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Web.Script.Serialization;
using System.Windows.Automation;
using System.Windows.Forms;

public static class ArchoInputAgent {
    [StructLayout(LayoutKind.Sequential)] struct MOUSEINPUT { public int dx; public int dy; public uint mouseData; public uint dwFlags; public uint time; public IntPtr extra; }
    [StructLayout(LayoutKind.Sequential)] struct KEYBDINPUT { public ushort wVk; public ushort wScan; public uint dwFlags; public uint time; public IntPtr extra; }
    [StructLayout(LayoutKind.Explicit)] struct UNION { [FieldOffset(0)] public MOUSEINPUT mi; [FieldOffset(0)] public KEYBDINPUT ki; }
    [StructLayout(LayoutKind.Sequential)] struct INPUT { public uint type; public UNION u; }
    [StructLayout(LayoutKind.Sequential)] struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
    [StructLayout(LayoutKind.Sequential)] struct POINT { public int X; public int Y; }
    delegate bool EnumProc(IntPtr h, IntPtr l);

    [DllImport("user32.dll", SetLastError = true)] static extern uint SendInput(uint n, INPUT[] inputs, int size);
    [DllImport("user32.dll")] static extern int GetSystemMetrics(int i);
    [DllImport("user32.dll")] static extern bool GetCursorPos(out POINT p);
    [DllImport("user32.dll")] static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] static extern uint MapVirtualKey(uint code, uint type);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc f, IntPtr l);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] static extern bool IsWindow(IntPtr h);
    [DllImport("user32.dll")] static extern bool IsIconic(IntPtr h);
    [DllImport("user32.dll")] static extern bool IsZoomed(IntPtr h);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] static extern bool GetClientRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] static extern bool ClientToScreen(IntPtr h, ref POINT p);
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr h);
    [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr h, int cmd);
    [DllImport("user32.dll")] static extern bool AttachThreadInput(uint a, uint b, bool attach);
    [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] static extern IntPtr GetWindow(IntPtr h, uint cmd);
    [DllImport("user32.dll")] static extern IntPtr OpenInputDesktop(uint flags, bool inherit, uint access);
    [DllImport("user32.dll")] static extern bool CloseDesktop(IntPtr d);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern bool GetUserObjectInformation(IntPtr h, int index, StringBuilder buf, int len, out int needed);
    [DllImport("user32.dll")] static extern bool SetProcessDpiAwarenessContext(IntPtr v);
    [DllImport("user32.dll")] static extern uint GetDpiForWindow(IntPtr h);
    [DllImport("user32.dll")] static extern uint GetDpiForSystem();
    [DllImport("user32.dll")] static extern uint GetDoubleClickTime();

    const uint INPUT_MOUSE = 0, INPUT_KEYBOARD = 1;
    const uint KEYEVENTF_EXTENDEDKEY = 1, KEYEVENTF_KEYUP = 2, KEYEVENTF_UNICODE = 4;
    const uint MOUSEEVENTF_MOVE = 0x1, MOUSEEVENTF_ABSOLUTE = 0x8000, MOUSEEVENTF_VIRTUALDESK = 0x4000;
    const uint MOUSEEVENTF_WHEEL = 0x800, MOUSEEVENTF_HWHEEL = 0x1000;

    static readonly Dictionary<int, bool> heldKeys = new Dictionary<int, bool>();
    static readonly Dictionary<string, bool> heldButtons = new Dictionary<string, bool>();
    static readonly JavaScriptSerializer json = new JavaScriptSerializer { MaxJsonLength = int.MaxValue };

    public static void Run(int port, string token, string logFile) {
        try { SetProcessDpiAwarenessContext(new IntPtr(-4)); } catch { } // per-monitor v2: physical pixels everywhere
        TcpListener listener = new TcpListener(IPAddress.Loopback, port);
        listener.Start();
        Log(logFile, "listening on 127.0.0.1:" + port);
        bool quit = false;
        while (!quit) {
            using (TcpClient client = listener.AcceptTcpClient()) {
                client.NoDelay = true;
                Log(logFile, "client " + client.Client.RemoteEndPoint);
                NetworkStream stream = client.GetStream();
                StreamReader reader = new StreamReader(stream, new UTF8Encoding(false));
                StreamWriter writer = new StreamWriter(stream, new UTF8Encoding(false));
                writer.NewLine = "\n";
                bool authed = false;
                try {
                    string line;
                    while ((line = reader.ReadLine()) != null) {
                        if (line.Trim().Length == 0) continue;
                        Dictionary<string, object> req = null;
                        Dictionary<string, object> res = new Dictionary<string, object>();
                        try {
                            req = json.Deserialize<Dictionary<string, object>>(line);
                            if (req.ContainsKey("id")) res["id"] = req["id"];
                            string cmd = Str(req, "cmd", "");
                            if (!authed) {
                                if (cmd != "hello" || Str(req, "token", "") != token) throw new Exception("bad token: send {\"cmd\":\"hello\",\"token\":...} first");
                                authed = true;
                                res["pid"] = Process.GetCurrentProcess().Id;
                            } else if (cmd == "quit") {
                                quit = true;
                            } else {
                                Handle(cmd, req, res);
                            }
                            res["ok"] = true;
                        } catch (Exception e) {
                            res["ok"] = false;
                            res["error"] = e.Message;
                        }
                        writer.WriteLine(json.Serialize(res));
                        writer.Flush();
                        if (quit || !authed) break;
                    }
                } catch (IOException) {
                } finally {
                    ReleaseAll();
                    Log(logFile, "client gone; held keys and buttons released");
                }
            }
        }
        listener.Stop();
        Log(logFile, "quit");
    }

    static void Log(string file, string s) {
        if (string.IsNullOrEmpty(file)) return;
        try { File.AppendAllText(file, DateTime.Now.ToString("HH:mm:ss") + " " + s + "\r\n"); } catch { }
    }

    static string Str(Dictionary<string, object> d, string k, string def) { object v; return d.TryGetValue(k, out v) && v != null ? v.ToString() : def; }
    static int Int(Dictionary<string, object> d, string k, int def) { object v; return d.TryGetValue(k, out v) && v != null ? Convert.ToInt32(v) : def; }
    static long Long(Dictionary<string, object> d, string k, long def) { object v; return d.TryGetValue(k, out v) && v != null ? Convert.ToInt64(v) : def; }
    static bool Bool(Dictionary<string, object> d, string k, bool def) { object v; return d.TryGetValue(k, out v) && v != null ? Convert.ToBoolean(v) : def; }

    static void Handle(string cmd, Dictionary<string, object> req, Dictionary<string, object> res) {
        switch (cmd) {
            case "screen": Screen(res); return;
            case "windows": res["windows"] = Windows(Str(req, "process", ""), Long(req, "hwnd", 0)); return;
            case "focus": res["foreground"] = Focus(new IntPtr(Long(req, "hwnd", 0))); return;
            case "move": NeedInputDesktop(); Move(Int(req, "x", 0), Int(req, "y", 0)); Cursor(res); return;
            case "button": NeedInputDesktop(); Button(Str(req, "button", "left"), Bool(req, "down", true)); return;
            case "wheel": NeedInputDesktop(); Wheel(Int(req, "delta", 120), Bool(req, "horizontal", false)); return;
            case "key": NeedInputDesktop(); Key(Int(req, "vk", 0), Bool(req, "ext", false), Bool(req, "up", false)); return;
            case "unicode": NeedInputDesktop(); Unicode(Str(req, "text", ""), Int(req, "delay", 5)); return;
            case "release": ReleaseAll(); return;
            case "clip": res["text"] = Clipboard.ContainsText() ? Clipboard.GetText() : null; return;
            case "setclip": { string t = Str(req, "text", ""); if (t.Length == 0) Clipboard.Clear(); else Clipboard.SetText(t); return; }
            case "uiatext": res["texts"] = UiaText(Int(req, "pid", 0), Int(req, "max", 200000)); return;
            case "doubleclick": res["ms"] = GetDoubleClickTime(); return;
            default: throw new Exception("unknown command: " + cmd);
        }
    }

    // The input desktop is not "Default" while the screen is locked or a UAC/secure prompt is
    // up; SendInput would then go nowhere useful.
    static string InputDesktop() {
        IntPtr d = OpenInputDesktop(0, false, 0x0001);
        if (d == IntPtr.Zero) return "";
        try {
            StringBuilder sb = new StringBuilder(256); int n;
            return GetUserObjectInformation(d, 2, sb, sb.Capacity * 2, out n) ? sb.ToString() : "";
        } finally { CloseDesktop(d); }
    }
    static void NeedInputDesktop() {
        string d = InputDesktop();
        if (!string.Equals(d, "Default", StringComparison.OrdinalIgnoreCase))
            throw new Exception("the guest screen is locked or a secure prompt is up (input desktop '" + d + "'); sign in through the VMware window");
    }

    static void Cursor(Dictionary<string, object> res) { POINT p; GetCursorPos(out p); res["cursor"] = new int[] { p.X, p.Y }; }

    static void Screen(Dictionary<string, object> res) {
        res["virtual"] = new int[] { GetSystemMetrics(76), GetSystemMetrics(77), GetSystemMetrics(78), GetSystemMetrics(79) };
        res["primary"] = new int[] { GetSystemMetrics(0), GetSystemMetrics(1) };
        res["dpi"] = GetDpiForSystem();
        res["desktop"] = InputDesktop();
        res["foreground"] = GetForegroundWindow().ToInt64();
        Cursor(res);
    }

    static Dictionary<string, object> WindowInfo(IntPtr h) {
        uint pid; GetWindowThreadProcessId(h, out pid);
        StringBuilder t = new StringBuilder(512); GetWindowText(h, t, t.Capacity);
        StringBuilder c = new StringBuilder(256); GetClassName(h, c, c.Capacity);
        RECT r; GetWindowRect(h, out r);
        RECT cr; GetClientRect(h, out cr);
        POINT o = new POINT(); ClientToScreen(h, ref o);
        string name = "";
        try { name = Process.GetProcessById((int)pid).ProcessName; } catch { }
        Dictionary<string, object> w = new Dictionary<string, object>();
        w["hwnd"] = h.ToInt64(); w["pid"] = pid; w["process"] = name; w["title"] = t.ToString(); w["class"] = c.ToString();
        w["rect"] = new int[] { r.Left, r.Top, r.Right - r.Left, r.Bottom - r.Top };
        w["client"] = new int[] { o.X, o.Y, cr.Right, cr.Bottom };
        w["dpi"] = GetDpiForWindow(h);
        w["visible"] = IsWindowVisible(h); w["minimized"] = IsIconic(h); w["maximized"] = IsZoomed(h);
        w["foreground"] = GetForegroundWindow() == h;
        return w;
    }

    static List<Dictionary<string, object>> Windows(string process, long hwnd) {
        List<Dictionary<string, object>> list = new List<Dictionary<string, object>>();
        if (hwnd != 0) {
            IntPtr h = new IntPtr(hwnd);
            if (IsWindow(h)) list.Add(WindowInfo(h));
            return list;
        }
        EnumWindows(delegate(IntPtr h, IntPtr l) {
            if (!IsWindowVisible(h) || GetWindow(h, 4) != IntPtr.Zero) return true; // visible, unowned
            Dictionary<string, object> w = WindowInfo(h);
            if (process.Length > 0 && !string.Equals((string)w["process"], process, StringComparison.OrdinalIgnoreCase)) return true;
            if (process.Length == 0 && ((string)w["title"]).Length == 0) return true;
            list.Add(w);
            return true;
        }, IntPtr.Zero);
        return list;
    }

    static bool Focus(IntPtr h) {
        if (!IsWindow(h)) throw new Exception("no window " + h.ToInt64());
        if (IsIconic(h)) ShowWindow(h, 9);
        uint dummy;
        uint fg = GetWindowThreadProcessId(GetForegroundWindow(), out dummy);
        uint me = GetCurrentThreadId();
        bool attached = fg != me && AttachThreadInput(me, fg, true);
        try { BringWindowToTop(h); SetForegroundWindow(h); }
        finally { if (attached) AttachThreadInput(me, fg, false); }
        if (GetForegroundWindow() != h) {
            // The process that sent the last input may take the foreground: send a lone Shift
            // release (no app acts on it), then try again.
            Send(KeyInput(0x10, false, true));
            SetForegroundWindow(h);
        }
        Thread.Sleep(50);
        return GetForegroundWindow() == h;
    }

    static void Send(INPUT i) {
        INPUT[] a = new INPUT[] { i };
        if (SendInput(1, a, Marshal.SizeOf(typeof(INPUT))) != 1)
            throw new Exception("SendInput was blocked (error " + Marshal.GetLastWin32Error() + "; a window of higher integrity in front?)");
    }

    static void Move(int x, int y) {
        int vx = GetSystemMetrics(76), vy = GetSystemMetrics(77), vw = GetSystemMetrics(78), vh = GetSystemMetrics(79);
        INPUT i = new INPUT(); i.type = INPUT_MOUSE;
        i.u.mi.dx = (int)Math.Round((x - vx + 0.5) * 65536.0 / vw);
        i.u.mi.dy = (int)Math.Round((y - vy + 0.5) * 65536.0 / vh);
        i.u.mi.dwFlags = MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK;
        Send(i);
        POINT p; GetCursorPos(out p);
        if (p.X != x || p.Y != y) SetCursorPos(x, y); // rounding at odd sizes; the move event already went out
    }

    static void Button(string button, bool down) {
        uint flag, data = 0;
        switch (button) {
            case "left": flag = down ? 0x2u : 0x4u; break;
            case "right": flag = down ? 0x8u : 0x10u; break;
            case "middle": flag = down ? 0x20u : 0x40u; break;
            case "back": flag = down ? 0x80u : 0x100u; data = 1; break;
            case "forward": flag = down ? 0x80u : 0x100u; data = 2; break;
            default: throw new Exception("unknown button " + button);
        }
        INPUT i = new INPUT(); i.type = INPUT_MOUSE; i.u.mi.dwFlags = flag; i.u.mi.mouseData = data;
        Send(i);
        if (down) heldButtons[button] = true; else heldButtons.Remove(button);
    }

    static void Wheel(int delta, bool horizontal) {
        INPUT i = new INPUT(); i.type = INPUT_MOUSE;
        i.u.mi.dwFlags = horizontal ? MOUSEEVENTF_HWHEEL : MOUSEEVENTF_WHEEL;
        i.u.mi.mouseData = unchecked((uint)delta);
        Send(i);
    }

    static INPUT KeyInput(int vk, bool ext, bool up) {
        INPUT i = new INPUT(); i.type = INPUT_KEYBOARD;
        i.u.ki.wVk = (ushort)vk;
        i.u.ki.wScan = (ushort)MapVirtualKey((uint)vk, 0);
        i.u.ki.dwFlags = (ext ? KEYEVENTF_EXTENDEDKEY : 0) | (up ? KEYEVENTF_KEYUP : 0);
        return i;
    }

    static void Key(int vk, bool ext, bool up) {
        if (vk <= 0 || vk > 254) throw new Exception("bad virtual key " + vk);
        Send(KeyInput(vk, ext, up));
        if (up) heldKeys.Remove(vk); else heldKeys[vk] = ext;
    }

    static void Unicode(string text, int delay) {
        foreach (char ch in text) {
            if (ch == '\r') continue;
            if (ch == '\n' || ch == '\t') {
                int vk = ch == '\n' ? 0x0D : 0x09;
                Send(KeyInput(vk, false, false)); Send(KeyInput(vk, false, true));
            } else {
                INPUT d = new INPUT(); d.type = INPUT_KEYBOARD; d.u.ki.wScan = ch; d.u.ki.dwFlags = KEYEVENTF_UNICODE;
                INPUT u = d; u.u.ki.dwFlags = KEYEVENTF_UNICODE | KEYEVENTF_KEYUP;
                Send(d); Send(u);
            }
            if (delay > 0) Thread.Sleep(delay);
        }
    }

    static void ReleaseAll() {
        foreach (KeyValuePair<int, bool> k in new List<KeyValuePair<int, bool>>(heldKeys)) { try { Send(KeyInput(k.Key, k.Value, true)); } catch { } }
        heldKeys.Clear();
        foreach (string b in new List<string>(heldButtons.Keys)) { try { Button(b, false); } catch { } }
        heldButtons.Clear();
    }

    // Text a window exposes through UI Automation's TextPattern: what Windows Terminal shows.
    static List<string> UiaText(int pid, int max) {
        List<string> texts = new List<string>();
        Condition mine = new PropertyCondition(AutomationElement.ProcessIdProperty, pid);
        Condition hasText = new PropertyCondition(AutomationElement.IsTextPatternAvailableProperty, true);
        foreach (AutomationElement top in AutomationElement.RootElement.FindAll(TreeScope.Children, mine)) {
            foreach (AutomationElement e in top.FindAll(TreeScope.Descendants, hasText)) {
                object p;
                if (e.TryGetCurrentPattern(TextPattern.Pattern, out p)) texts.Add(((TextPattern)p).DocumentRange.GetText(max));
            }
        }
        return texts;
    }
}
'@
    Log "agent $PID starting"
    [ArchoInputAgent]::Run($Port, $Token, $LogFile)
} catch {
    Log "FAIL $($_.Exception.Message)"
    throw
} finally {
    if ($PidFile -and (Test-Path -LiteralPath $PidFile)) { Remove-Item -LiteralPath $PidFile -ErrorAction SilentlyContinue }
}
