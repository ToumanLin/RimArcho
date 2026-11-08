package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadcost"
	"github.com/ToumanLin/RimArcho/internal/machine"
)

// MemoryAssessment is shared by view models and CLI/MCP. Unknown is never zero
// capacity: missing hardware or estimates must not produce a shortage warning.
type MemoryAssessment struct {
	Hardware     machine.Memory `json:"hardware"`
	RAMBudget    int64          `json:"ramBudget"`
	RAMLow       int64          `json:"ramLow"`
	RAMHigh      int64          `json:"ramHigh"`
	VRAMLow      int64          `json:"vramLow"`
	VRAMHigh     int64          `json:"vramHigh"`
	RAMSeverity  string         `json:"ramSeverity"`
	VRAMSeverity string         `json:"vramSeverity"`
	// VRAMInRAM: the part of the VRAM estimate held in system memory on this machine
	// (loadcost.Spill), already included in RAMLow / RAMHigh.
	VRAMInRAMLow  int64 `json:"vramInRAMLow"`
	VRAMInRAMHigh int64 `json:"vramInRAMHigh"`
	// TexturesOnDemand: 1.7 texture VRAM beyond what the game keeps on this card
	// (LoadFit.OnDemand). Not a shortage: those textures load when drawn.
	TexturesOnDemand int64 `json:"texturesOnDemand,omitempty"`
	Estimated        bool  `json:"estimated"`
	// Stale: the totals are over an hour old. They still count; callers re-measure in
	// the background, so the launch button does not flicker back to unknown.
	Stale bool `json:"stale,omitempty"`
}

func (a *App) MemorySnapshot() machine.Memory {
	if a.MemoryProbe != nil {
		return a.MemoryProbe()
	}
	return machine.Cached()
}
func (a *App) measuredMemory() machine.Memory {
	if a.MemoryProbe != nil {
		return a.MemoryProbe()
	}
	return machine.Measure()
}

// RAMBudget uses current available physical RAM (16 §2.3).
// It is not the commit limit and cannot prove that a launch will crash.
func RAMBudget(m machine.Memory) int64 { return m.AvailableRAM }

// CommitBudget is what the system can still commit (physical RAM plus page file),
// the limit the game's private bytes actually run into; available RAM when unknown.
func CommitBudget(m machine.Memory) int64 {
	if m.CommitAvailable > 0 {
		return m.CommitAvailable
	}
	return m.AvailableRAM
}

// AssessMemory: severe when the loading budget's lower bound exceeds what can still be
// committed, warning when the range reaches past available physical RAM (paging).
func AssessMemory(m machine.Memory, r *LoadCost) MemoryAssessment {
	c := MemoryAssessment{Hardware: m, RAMBudget: RAMBudget(m), RAMSeverity: "unknown", VRAMSeverity: "unknown"}
	if r == nil {
		return c
	}
	c.Estimated = true
	f := r.FitTo(m.VRAM())
	c.RAMLow, c.RAMHigh = f.LoadingLow, f.LoadingHigh
	c.VRAMLow, c.VRAMHigh = f.VRAMLow, f.VRAMHigh
	c.VRAMInRAMLow, c.VRAMInRAMHigh = f.HeldLow, f.HeldHigh
	c.TexturesOnDemand = f.OnDemand
	if m.TotalRAM > 0 && !m.SampledAt.IsZero() {
		c.RAMSeverity = "ok"
		if CommitBudget(m) < c.RAMLow {
			c.RAMSeverity = "severe"
		} else if c.RAMBudget < c.RAMHigh {
			c.RAMSeverity = "warning"
		}
	}
	if m.VRAM() > 0 {
		c.VRAMSeverity = "ok"
		// 1.7 never holds more than its watermark, which Cap17 already applied.
		if r.Game != loadcost.Game17 && m.VRAM() < c.VRAMHigh {
			c.VRAMSeverity = "warning"
		}
	}
	return c
}

type memoryEstimate struct {
	Key  string    `json:"key"`
	At   time.Time `json:"at"`
	Cost LoadCost  `json:"cost"`
}

func memoryKey(st *LoadoutState) string {
	h := sha256.New()
	fmt.Fprintf(h, "vram-cap17-v5|%s|%s|%s", st.Ix.Env.ID, st.GameMM(), ListFingerprint(st.List))
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			fmt.Fprintf(h, "|%s:%d:%x", m.ID, m.SnapID, m.SnapHash)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
func memoryMeta(st *LoadoutState) string {
	return fmt.Sprintf("memory-estimate:%s:%d", st.Ix.Env.ID, st.Plan.ID)
}
func (a *App) saveMemoryEstimate(st *LoadoutState, r *LoadCost) error {
	// Keep totals and the few category parts (loadcost.Cap17 needs the texture parts):
	// retaining raw per-mod measurements would duplicate the disk cache.
	copy := *r
	copy.PerMod = nil
	copy.Top = nil
	copy.Capacity = MemoryAssessment{}
	b, err := json.Marshal(memoryEstimate{Key: memoryKey(st), At: time.Now(), Cost: copy})
	if err != nil {
		return err
	}
	return a.Store.SetMeta(memoryMeta(st), string(b))
}

// CachedMemoryAssessment reads SQLite totals only. Edits, scans or model changes
// invalidate the estimate; after an hour it is stale. Measuring files is a background task.
func (a *App) CachedMemoryAssessment(st *LoadoutState) MemoryAssessment {
	var saved memoryEstimate
	if st.GameMM() == "" || json.Unmarshal([]byte(a.Store.Meta(memoryMeta(st))), &saved) != nil || saved.Key != memoryKey(st) {
		return AssessMemory(a.MemorySnapshot(), nil)
	}
	c := AssessMemory(a.MemorySnapshot(), &saved.Cost)
	c.Stale = time.Since(saved.At) > time.Hour
	return c
}

// MemoryFingerprint changes with the environment, list, scan or calibrated model.
func MemoryFingerprint(st *LoadoutState) string { return memoryKey(st) }
