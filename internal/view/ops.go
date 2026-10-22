package view

import (
	"slices"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// What the player can do with a row, the same on the plan editor and Mod 库存 › 全部
// (06 §3.5, §8.5): one table of operations with their label, key and menu group, and per
// row only the codes that apply. The right-click menu, the hint line, a copy's detail
// buttons, the selection bar and the keys all come from it, and the label table goes to the
// page once (OpTable) while each row carries only codes.

// Op is one operation.
type Op struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	// Key runs it ("" only from the menu: the menu key or shift+F10 opens the menu, so the
	// keyboard reaches every op).
	Key   string `json:"key,omitempty"`
	Group int    `json:"group"` // the menu's group: 1 方案 2 整理 3 打开 4 版本 5 磁盘
	// Confirm: it changes files, so the dry run shows first (LibPreview).
	Confirm bool `json:"confirm,omitempty"`
	Danger  bool `json:"danger,omitempty"` // red, last in the menu
	// Detail: a button in a copy's detail (Mod 库存).
	Detail bool `json:"detail,omitempty"`
	// One: it acts on one row at a time (the folder, a page, a note, a position).
	One bool `json:"one,omitempty"`
}

// Op codes. The library's LibDo takes the codes of groups 1, 4 and 5.
const (
	OpEnable    = "enable"     // 启用 (the editor's 未启用)
	OpAdd       = "add"        // 加入当前方案 (Mod 库存)
	OpUse       = "use"        // 换成这一份: the current plan uses another copy of the mod
	OpDisable   = "disable"    // 停用 (the editor's 已启用)
	OpTakeOut   = "take-out"   // 从当前方案拿掉 (Mod 库存)
	OpPutBack   = "put-back"   // 放回原位 (暂存)
	OpEnableAt  = "enable-at"  // 启用到…
	OpMoveTo    = "move-to"    // 移动到…
	OpStage     = "stage"      // 暂存
	OpSwap      = "swap"       // 换成另一份
	OpRemove    = "remove"     // 从所有方案里拿掉
	OpTags      = "tags"       // 标签
	OpNote      = "note"       // 备注 (Mod 库存: this copy's note)
	OpFolder    = "folder"     // 打开文件夹
	OpWorkshop  = "workshop"   // 打开工坊页面
	OpPin       = "pin"        // 锁定这个版本
	OpPinUpdate = "pin-update" // 更新锁定版本
	OpUnpin     = "unpin"      // 解除锁定
	OpMove      = "move"       // 备份锁定版本 (收进锁定库)
	OpUnhide    = "unhide"     // 恢复
	OpResub     = "resubscribe"
	OpUnsub     = "unsubscribe" // 退订, through the Steam client
	OpDelete    = "delete"      // 删除 (to the recycle bin)
)

// opTable is every op in menu order (06 §8.5): within a group in this order, and 退订 /
// 删除 last of all.
var opTable = []Op{
	{Code: OpEnable, Key: "space", Group: 1},
	{Code: OpAdd, Key: "space", Group: 1},
	{Code: OpUse, Key: "x", Group: 1, One: true},
	{Code: OpDisable, Key: "space", Group: 1},
	{Code: OpTakeOut, Key: "space", Group: 1},
	{Code: OpPutBack, Key: "space", Group: 1},
	{Code: OpEnableAt, Key: "m", Group: 1},
	{Code: OpMoveTo, Key: "m", Group: 1},
	{Code: OpStage, Key: "t", Group: 1},
	{Code: OpSwap, Key: "x", Group: 1, One: true},
	{Code: OpRemove, Group: 1},
	{Code: OpTags, Key: "g", Group: 2},
	{Code: OpNote, Key: "n", Group: 2, One: true},
	{Code: OpFolder, Key: "ctrl+o", Group: 3, Detail: true, One: true},
	{Code: OpWorkshop, Key: "ctrl+w", Group: 3, Detail: true, One: true},
	{Code: OpPin, Group: 4, Confirm: true},
	{Code: OpPinUpdate, Group: 4, Confirm: true},
	{Code: OpUnpin, Group: 4, Confirm: true},
	{Code: OpMove, Group: 4, Confirm: true, Detail: true},
	{Code: OpUnhide, Group: 5, Confirm: true, Detail: true},
	{Code: OpResub, Group: 5, Detail: true},
	{Code: OpUnsub, Key: "Del", Group: 5, Confirm: true, Danger: true, Detail: true},
	{Code: OpDelete, Key: "Del", Group: 5, Confirm: true, Danger: true, Detail: true},
}

func opLabel(code string) string {
	return map[string]string{
		OpEnable: T("enable"), OpAdd: T("add to the current plan"), OpUse: T("use this copy in the current plan"),
		OpDisable: T("disable"), OpTakeOut: T("take out of the current plan"), OpPutBack: T("put back"),
		OpEnableAt: T("enable at…"), OpMoveTo: T("move to…"), OpStage: T("stage"), OpSwap: T("swap for another copy"),
		OpRemove: T("take out of every plan"), OpTags: T("tags"), OpNote: T("note"),
		OpFolder: T("open the folder"), OpWorkshop: T("open the Workshop page"),
		OpPin: T("lock this version"), OpPinUpdate: T("update the locked version"), OpUnpin: T("unlock"),
		OpMove: T("back up as a locked version"), OpUnhide: T("restore"), OpResub: T("subscribe again"),
		OpUnsub: T("unsubscribe"), OpDelete: T("delete"),
	}[code]
}

// OpTable is every op with its label in the interface language (sent to the page once).
func OpTable() []Op {
	out := make([]Op, len(opTable))
	for i, o := range opTable {
		o.Label = opLabel(o.Code)
		out[i] = o
	}
	return out
}

// OpOf is one op ("" Code: no such op).
func OpOf(code string) Op {
	for _, o := range opTable {
		if o.Code == code {
			o.Label = opLabel(o.Code)
			return o
		}
	}
	return Op{}
}

// sortOps puts codes in menu order and drops duplicates.
func sortOps(codes []string) []string {
	var out []string
	for _, o := range opTable {
		if slices.Contains(codes, o.Code) {
			out = append(out, o.Code)
		}
	}
	return out
}

// Where a copy is in the current plan.
type Where int

const (
	Nowhere  Where = iota // another environment's, a missing item, a copy that cannot be loaded
	Disabled              // installed here, not enabled (未启用)
	Enabled               // in the load order (已启用)
	Staged                // in staging (暂存)
)

// WhereOf is where the copy is in the current plan.
func WhereOf(lib *app.Library, i *app.LibInstance) Where {
	switch {
	case i.Mod == nil || i.Missing():
		return Nowhere
	case i.InPlan:
		return Enabled
	case i.Staged:
		return Staged
	case lib != nil && lib.St != nil && lib.St.Ix.ByID[i.Mod.ID] != nil:
		return Disabled
	}
	return Nowhere
}

// EditorPage and LibraryPage name the page asking: the plan ops read the same on both but
// for space, which is 启用 / 停用 in the editor and 加入 / 拿掉 in Mod 库存.
const (
	EditorPage  = "plan"
	LibraryPage = "library"
	ChangesPage = "changes"
)

// CopyOps are the codes that apply to a copy now, in menu order (06 §8.5): the plan ops by
// where it is, 标签 and 备注, 打开, the version ops and the disk ops (06 §2.9's 动作).
func CopyOps(lib *app.Library, l *app.LibLineage, i *app.LibInstance, page string) []string {
	has := func(s string) bool { return slices.Contains(i.States, s) }
	plans := slices.ContainsFunc(i.UsedIn, func(u app.LibUse) bool { return u.Kind == "plan" })
	m := i.Mod
	live := m != nil && !i.Missing()
	workshop := i.Source == app.LibWorkshop || i.Source == app.LibSteamCMD
	own := i.Source == app.LibLocal || i.Source == app.LibLocked
	loadable := live && !has(app.LibUnloadable) && !has(app.LibHidden) && !has(app.LibResidue)
	others := 0
	if l != nil {
		for _, o := range l.Instances {
			if o != i && o.Mod != nil && !o.Missing() {
				others++
			}
		}
	}
	var ks []string
	switch WhereOf(lib, i) {
	case Disabled:
		inPlan := l != nil && l.InPlan()
		switch {
		case page == EditorPage:
			ks = append(ks, OpEnable, OpEnableAt, OpStage)
		case inPlan && loadable:
			ks = append(ks, OpUse) // the plan uses another copy of it
		case loadable && !has(app.LibIgnored):
			ks = append(ks, OpAdd, OpEnableAt, OpStage)
		}
	case Enabled:
		off := OpTakeOut
		if page == EditorPage {
			off = OpDisable
		}
		ks = append(ks, off, OpMoveTo, OpStage)
		if others > 0 {
			ks = append(ks, OpSwap)
		}
	case Staged:
		ks = append(ks, OpPutBack, OpEnableAt)
	}
	if plans && m != nil {
		ks = append(ks, OpRemove)
	}
	if m != nil {
		ks = append(ks, OpTags, OpNote) // the editor: the mod's note; Mod 库存: this copy's
	}
	url := IsNumber(i.WorkshopID)
	if live && i.Path != "" {
		ks = append(ks, OpFolder)
	}
	if url {
		ks = append(ks, OpWorkshop)
	}
	if loadable && workshop {
		ks = append(ks, OpPin)
	}
	if i.Locked != nil && i.Locked.Updated {
		ks = append(ks, OpPinUpdate)
	}
	if i.Locked != nil {
		ks = append(ks, OpUnpin)
	}
	if live && i.Spare && i.Source == app.LibLocal {
		ks = append(ks, OpMove)
	}
	if live && has(app.LibHidden) {
		ks = append(ks, OpUnhide)
	}
	if live && has(app.LibResidue) && url {
		ks = append(ks, OpResub)
	}
	if live && i.Source == app.LibWorkshop && !has(app.LibResidue) && url {
		ks = append(ks, OpUnsub)
	}
	if live && !plans && (own || workshop && has(app.LibResidue)) && !source.IsLink(i.Path) {
		ks = append(ks, OpDelete)
	}
	return sortOps(ks)
}

// RowOps are the ops of the editor's row: its copy's ops when the library knows it (the
// version and disk ops need it), else the plan's own for the column.
func RowOps(lib *app.Library, id string, col Column) []string {
	if lib != nil {
		for _, l := range lib.Lineages {
			for _, i := range l.Instances {
				if i.Mod != nil && i.Mod.ID == id {
					ops := CopyOps(lib, l, i, EditorPage)
					// the column the player sees decides space (the library reads the plan
					// as written; the board may be newer by a step)
					return fixColumn(ops, col)
				}
			}
		}
	}
	return fixColumn([]string{OpTags, OpNote}, col)
}

// fixColumn keeps the plan ops of the row's column.
func fixColumn(ops []string, col Column) []string {
	plan := map[Column][]string{Inactive: {OpEnable, OpEnableAt, OpStage}, Active: {OpDisable, OpMoveTo, OpStage}, Staging: {OpPutBack, OpEnableAt}}[col]
	out := slices.DeleteFunc(slices.Clone(ops), func(c string) bool {
		return slices.Contains([]string{OpEnable, OpAdd, OpUse, OpDisable, OpTakeOut, OpPutBack, OpEnableAt, OpMoveTo, OpStage}, c) ||
			c == OpSwap && col != Active
	})
	return sortOps(append(out, plan...))
}
