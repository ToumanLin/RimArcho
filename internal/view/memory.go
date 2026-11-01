package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// MemoryNeed keeps the comparison and the severity common to GUI and TUI.
func MemoryNeed(c app.MemoryAssessment, plan int64) Need {
	it := Need{Group: GroupList, Section: SectionHint, Status: StatusHint, Kind: "memory.capacity", code: "memory.capacity", Title: T("RAM / VRAM"),
		Actions: []Action{{Kind: ActView, Key: "v", Label: T("refresh memory estimate"), Argv: []string{"plan", "cost", fmt.Sprint(plan)}}}}
	ram, vram := T("not measured"), T("not measured")
	if c.Estimated {
		ram = costRange(c.RAMLow, c.RAMHigh)
		vram = costRange(c.VRAMLow, c.VRAMHigh)
	}
	available, total := T("unavailable"), T("unavailable")
	if c.Hardware.TotalRAM > 0 {
		available = SizeText(c.RAMBudget)
		total = SizeText(c.Hardware.TotalRAM)
	}
	video := T("unavailable")
	if c.Hardware.VRAM() > 0 {
		video = SizeText(c.Hardware.VRAM())
	}
	lines := []string{
		Tf("RAM: loading needs %s; available now %s of %s", ram, available, total),
		Tf("VRAM: estimated %s; dedicated capacity %s", vram, video),
	}
	if c.RAMSeverity == "severe" {
		it.Status = StatusErr
		it.Section = SectionBreak
		it.Counted = true
		lines = append(lines, T("RAM may be insufficient: close other apps or reduce the list before launching."))
	} else if c.RAMSeverity == "warning" || c.VRAMSeverity == "warning" {
		it.Status = StatusWarn
		it.Section = SectionBreak
		it.Counted = true
	}
	if c.RAMSeverity == "warning" {
		lines = append(lines, T("RAM is tight: closing other apps leaves more room."))
	}
	if c.VRAMSeverity == "warning" {
		lines = append(lines, T("VRAM may be insufficient: optimize textures or reduce texture mods."))
	}
	if len(c.Hardware.Adapters) > 0 {
		lines = append(lines, Tf("GPU: %s (by the system's preference)", c.Hardware.Adapters[0].Name))
	}
	if it.Counted {
		lines = append(lines, T("Saves and memory mods allocate themselves are not counted; a shortfall is a risk, not a certain crash."))
	}
	it.Evidence = evidence(EvInferred, T("file estimates and OS memory snapshot"))
	it.Why = strings.Join(lines, "\n")
	it.Text = it.Title + ": " + strings.Join(lines, "; ")
	identifyNeed(&it)
	return it
}

func RefreshMemory(a *app.App, st *app.LoadoutState, old *Needs) *Needs {
	if st == nil {
		return old
	}
	n := *old
	n.Items = nil
	for _, it := range old.Items {
		if it.Kind != "memory.capacity" {
			n.Items = append(n.Items, it)
		}
	}
	c := a.CachedMemoryAssessment(st)
	n.Memory = &c
	n.Items = append(n.Items, MemoryNeed(c, st.Plan.ID))
	sort.SliceStable(n.Items, func(i, j int) bool {
		if n.Items[i].Section != n.Items[j].Section {
			return n.Items[i].Section < n.Items[j].Section
		}
		return n.Items[i].Status != StatusHint && n.Items[j].Status == StatusHint
	})
	indexNeeds(&n)
	return &n
}

func init() {
	Register(map[string]string{
		"RAM is tight: closing other apps leaves more room.":             "可用内存偏紧：关闭其他程序可以留出更多空间。",
		"close other apps or reduce the list before launching.":          "关闭其他程序或减少 Mod 后再启动。",
		"RAM may be insufficient":                                        "内存可能不足",
		"launch with RAM risk":                                           "内存不足，启动游戏",
		"launch with limited RAM":                                        "内存偏紧，启动游戏",
		"launch anyway":                                                  "仍然启动",
		"loading RAM needed":                                             "载入预计需要",
		"RAM available now":                                              "当前可用内存",
		"the list changed; review the updated estimate before launching": "列表已变化，请查看更新后的估算再启动",
		"file estimates and OS memory snapshot":                          "文件估算与系统内存快照",
		"RAM / VRAM":                                                     "内存 / 显存",
		"refresh memory estimate":                                        "重新估算内存",
		"not measured":                                                   "尚未估算",
		"unavailable":                                                    "未读取到",
		"RAM: loading needs %s; available now %s of %s":                  "内存：载入时需要 %s；当前可用 %s，共 %s",
		"VRAM: estimated %s; dedicated capacity %s":                      "显存：预计 %s；显卡专用显存 %s",
		"RAM may be insufficient: close other apps or reduce the list before launching.":                         "内存可能不足：建议关闭其他程序或减少 Mod 后再启动。",
		"VRAM may be insufficient: optimize textures or reduce texture mods.":                                    "显存可能不足：建议优化贴图或减少贴图 Mod。",
		"GPU: %s (by the system's preference)":                                                                   "显卡：%s（按系统优先级推测）",
		"Saves and memory mods allocate themselves are not counted; a shortfall is a risk, not a certain crash.": "不含存档和 Mod 自行分配的内存；不足是风险，不一定崩溃。",
		"Loading RAM budget %s": "载入时内存约 %s",
	})
}

// MemoryLaunch binds a cached comparison to the exact environment/list snapshot.
// It performs no file measurement on the page's paint or launch paths.
type MemoryLaunch struct {
	Env        string               `json:"env"`
	Plan       int64                `json:"plan"`
	Token      string               `json:"token"`
	Assessment app.MemoryAssessment `json:"assessment"`
	Needed     string               `json:"needed"`
	Available  string               `json:"available"`
}

func BuildMemoryLaunch(a *app.App, st *app.LoadoutState) *MemoryLaunch {
	if st == nil || st.Plan == nil {
		return nil
	}
	c := a.CachedMemoryAssessment(st)
	return &MemoryLaunch{Env: st.Ix.Env.Name, Plan: st.Plan.ID, Token: app.MemoryFingerprint(st), Assessment: c, Needed: costRange(c.RAMLow, c.RAMHigh), Available: SizeText(c.RAMBudget)}
}
