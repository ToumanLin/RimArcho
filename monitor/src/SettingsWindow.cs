using System;
using System.Collections.Generic;
using System.Globalization;
using System.Reflection;
using System.Text.RegularExpressions;
using HarmonyLib;
using UnityEngine;
using Verse;

namespace ArchoMonitor
{
    // SettingsControl is one thing a settings window draws: a label, or a control with its
    // current value and, for a slider, its range (03 §6.7.1).
    internal struct SettingsControl
    {
        internal string Kind;  // label, checkbox, radio, slider, text, number, button
        internal string Label; // "" when the control has none
        internal string Value; // "" for a label or a button
        internal string Min, Max;
    }

    // SettingsWindow records what a Mod's settings window draws in the first frame of each
    // opening: its labels and each control's kind, current value and range. RimArcho shows them
    // beside the file's values (06 §2.9). The control patches go in when a settings window first
    // opens; until then nothing is patched, and after it each one only checks a flag. Only what
    // the game draws through Widgets is seen; what a Mod draws by hand is missed.
    internal static class SettingsWindow
    {
        internal const int MaxControls = 300;
        const int MaxText = 200;
        static readonly Regex Tags = new Regex("<[^>]*>", RegexOptions.Compiled);

        static bool tried, recording, lastLabel;
        static int depth;
        static object recorded; // the window whose opening was recorded
        static List<SettingsControl> items;
        static FieldInfo modField;

        // Install patches the settings window itself (Hooks.Install).
        internal static void Install(Harmony h)
        {
            var t = AccessTools.TypeByName("RimWorld.Dialog_ModSettings");
            modField = t == null ? null : AccessTools.Field(t, "mod");
            var draw = t == null ? null : AccessTools.Method(t, "DoWindowContents", new[] { typeof(Rect) });
            if (draw == null || modField == null)
            {
                Monitor.Skipped.Add("Dialog_ModSettings.DoWindowContents");
                return;
            }
            try
            {
                h.Patch(draw, prefix: new HarmonyMethod(AccessTools.Method(typeof(SettingsWindow), nameof(Begin))),
                    finalizer: new HarmonyMethod(AccessTools.Method(typeof(SettingsWindow), nameof(End))));
            }
            catch (Exception e) { Monitor.Skipped.Add("Dialog_ModSettings.DoWindowContents (" + e.GetType().Name + ")"); }
        }

        // Controls patches Widgets the first time a settings window opens; false when it cannot.
        static bool Controls()
        {
            if (tried) return true;
            tried = true;
            try
            {
                var h = new Harmony(Monitor.PackageId + ".settings");
                var w = typeof(Widgets);
                Control(h, AccessTools.Method(w, "Label", new[] { typeof(Rect), typeof(string) }), nameof(LabelText));
                Control(h, AccessTools.Method(w, "Label", new[] { typeof(Rect), typeof(TaggedString) }), nameof(LabelTagged));
                foreach (var m in AccessTools.GetDeclaredMethods(w))
                {
                    var ps = m.GetParameters();
                    switch (m.Name)
                    {
                        case "CheckboxLabeled" when Has(ps, "label", typeof(string)) && Has(ps, "checkOn", typeof(bool).MakeByRefType()):
                            Control(h, m, nameof(Checkbox)); break;
                        case "Checkbox" when Has(ps, "checkOn", typeof(bool).MakeByRefType()):
                            Control(h, m, nameof(CheckboxPlain)); break;
                        case "RadioButtonLabeled" when Has(ps, "labelText", typeof(string)) && Has(ps, "chosen", typeof(bool)):
                            Control(h, m, nameof(Radio)); break;
                        case "ButtonText" when Has(ps, "label", typeof(string)):
                            Control(h, m, nameof(Button)); break;
                        case "TextField" when Has(ps, "text", typeof(string)):
                        case "TextArea" when Has(ps, "text", typeof(string)):
                            Control(h, m, nameof(Text)); break;
                        case "HorizontalSlider" when Has(ps, "value", typeof(float)) && Has(ps, "min", typeof(float)) && Has(ps, "max", typeof(float)) && Has(ps, "label", typeof(string)):
                            Control(h, m, nameof(Slider)); break;
                        case "HorizontalSlider" when Has(ps, "value", typeof(float).MakeByRefType()) && Has(ps, "range", typeof(FloatRange)) && Has(ps, "label", typeof(string)):
                            Control(h, m, nameof(SliderRange)); break;
                        case "IntEntry" when Has(ps, "value", typeof(int).MakeByRefType()):
                            Control(h, m, nameof(Number)); break;
                    }
                }
                return true;
            }
            catch (Exception e)
            {
                Writer.Failure(e);
                return false;
            }
        }

        static bool Has(ParameterInfo[] ps, string name, Type type)
        {
            foreach (var p in ps) if (p.Name == name && p.ParameterType == type) return true;
            return false;
        }

        static void Control(Harmony h, MethodInfo m, string prefix)
        {
            if (m == null || m.IsGenericMethodDefinition) return;
            try
            {
                h.Patch(m, prefix: new HarmonyMethod(AccessTools.Method(typeof(SettingsWindow), prefix)),
                    postfix: new HarmonyMethod(AccessTools.Method(typeof(SettingsWindow), nameof(Leave))));
            }
            catch (Exception e) { Monitor.Skipped.Add("Widgets." + m.Name + " (" + e.GetType().Name + ")"); }
        }

        // Begin: Dialog_ModSettings.DoWindowContents prefix. The first frame of an opening records.
        internal static void Begin(object __instance)
        {
            if (recording || !Writer.Ready || ReferenceEquals(__instance, recorded)) return;
            recorded = __instance;
            if (!Controls()) return;
            items = new List<SettingsControl>();
            depth = 0;
            lastLabel = false;
            recording = true;
        }

        // End: its finalizer, so a Mod's drawing that throws still ends the record.
        internal static void End(object __instance)
        {
            if (!recording) return;
            recording = false;
            try
            {
                var f = SettingsRecord.Describe(modField.GetValue(__instance) as Mod);
                if (f.HasValue)
                {
                    // the dialog's own heading is the window's title
                    if (items.Count > 0 && items[0].Kind == "label" && items[0].Label == Clean(f.Value.Title)) items.RemoveAt(0);
                    Writer.WriteSettingsWindow(f.Value, items);
                }
            }
            catch (Exception e) { Writer.Failure(e); }
            items = null;
        }

        // Each control's prefix records it unless it is drawn inside another control (the label a
        // checkbox draws); Leave is its postfix. A control that throws skips Leave, which only
        // hides what that frame draws after it.
        static void Leave() { if (recording) depth--; }

        static void LabelText(string label) { Add("label", label, ""); }
        static void LabelTagged(TaggedString label) { Add("label", label.RawText, ""); }
        static void Checkbox(string label, ref bool checkOn) { Add("checkbox", label, checkOn ? "True" : "False"); }
        static void CheckboxPlain(ref bool checkOn) { Add("checkbox", "", checkOn ? "True" : "False"); }
        static void Radio(string labelText, bool chosen) { Add("radio", labelText, chosen ? "True" : "False"); }
        static void Button(string label) { Add("button", label, ""); }
        static void Text(string text) { Add("text", "", text); }
        static void Number(ref int value) { Add("number", "", value.ToString(CultureInfo.InvariantCulture)); }
        static void Slider(float value, float min, float max, string label) { Add("slider", label, Num(value), Num(min), Num(max)); }
        static void SliderRange(ref float value, FloatRange range, string label) { Add("slider", label, Num(value), Num(range.min), Num(range.max)); }

        static string Num(float f) => f.ToString("0.####", CultureInfo.InvariantCulture);

        static void Add(string kind, string label, string value, string min = "", string max = "")
        {
            if (!recording) return;
            if (depth++ > 0 || items.Count >= MaxControls) return;
            label = Clean(label);
            bool wasLabel = lastLabel;
            lastLabel = false;
            if (kind == "label")
            {
                if (label.Length == 0) return;
                lastLabel = true;
            }
            else if (label.Length == 0 && wasLabel && kind != "button")
            {
                // a field drawn after its own label (Listing_Standard's labeled entries and sliders)
                label = items[items.Count - 1].Label;
                items.RemoveAt(items.Count - 1);
            }
            if (value.Length > MaxText) value = value.Substring(0, MaxText);
            items.Add(new SettingsControl { Kind = kind, Label = label, Value = value, Min = min, Max = max });
        }

        static string Clean(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            s = Tags.Replace(s, "").Trim();
            return s.Length > MaxText ? s.Substring(0, MaxText) : s;
        }
    }
}
