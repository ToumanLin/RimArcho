package view

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// retired are the words the interface no longer uses (06 §7.2), each with the word that
// replaced it. The code, the CLI and the specs keep their own names (plan, environment,
// instance…); the player only sees the new ones.
var retired = []struct{ Old, New string }{
	{"方案", "列表"},
	{"启用列表", "已启用 / 列表"},
	{"工作区", "当前列表"},
	{"实例", "这一份"},
	{"Instance", "这一份"},
	{"Lineage", "同一个 Mod 的几份"},
	{"上游", "有新版本 / 工坊版"},
	{"原版有更新", "有新版本（原版是 Core 和 DLC）"},
	{"改用原版", "改用工坊版（原版是 Core 和 DLC）"},
	{"使用原版", "改用工坊版（原版是 Core 和 DLC）"},
	{"库存", "Mod 库"},
	{"环境", "游戏"},
	{"编译", "创建"},
	{"收进锁定库", "移入锁定库"},
	{"备份锁定", "移入锁定库"},
	{"已固定", "已锁定"},
	{"固定在", "锁定在"},
	{"固定的副本", "锁定的那份"},
	{"换成另一份", "切换版本…"},
	{"替换实例", "切换版本…"},
	{"写入游戏", "保存"},
	{"未写入", "未保存"},
	{"还没写入游戏", "未保存"},
	{"写入过的版本", "保存过的版本"},
	{"载入版本", "回到旧版"},
	{"载入列表", "回到这一版 / 采用外部修改"},
	{"载入到列表", "采用外部修改"},
	{"恢复给游戏", "取消隐藏"},
	{"拿掉", "停用"},
	{"轻包", "RimArcho 列表（导出整合包的列表）"},
	{"完整包", "整合包"},
}

// retiredEn are the English words the interface no longer uses: list, game, a newer
// version, switch version, locked, go back to a version, and the
// light / full pack (a pack is always full; its list alone is a RimArcho list).
var retiredEn = regexp.MustCompile(`(?i)\b(plans?|environments?|upstream|workspace|swap(s|ped)?|pinned|not written|checkout|light packs?|full packs?)\b`)

// wordingExempt are texts whose Chinese is right although it holds a retired word.
var wordingExempt = map[string]bool{
	"decompiled C#": true, // 反编译
	"%d assemblies are not decompiled yet: search again to go on":            true,
	"the mod was built for another version of the game or of the target mod": true, // a DLL compiled for another version
}

// WordingTextProblems checks text from surface-specific literals as well as tables.
func WordingTextProblems(where, text string) []string {
	if strings.ContainsRune(text, '\u00b7') {
		return []string{fmt.Sprintf("%s: %q uses a middle dot (06 §7.3)", where, text)}
	}
	return nil
}

// WordingProblems lists every interface text that still uses a retired word or middle dot, and every key
// two tables translate differently (the later table wins and the other text is lost). It
// sees the tables of every package linked in, so call it where all surfaces are.
func WordingProblems() []string {
	var out []string
	check := func(where, zh string) {
		for _, r := range retired {
			if strings.Contains(zh, r.Old) {
				out = append(out, fmt.Sprintf("%s: %q uses %q (write %q)", where, zh, r.Old, r.New))
			}
		}
	}
	checkEn := func(where, en string) {
		if w := retiredEn.FindString(en); w != "" {
			out = append(out, fmt.Sprintf("%s: the English %q uses %q", where, en, w))
		}
	}
	for k, v := range zhText {
		out = append(out, WordingTextProblems("key", k)...)
		out = append(out, WordingTextProblems(k, v)...)
		if !wordingExempt[k] {
			check(fmt.Sprintf("%q", k), v)
		}
		en := k
		if e, ok := enText[k]; ok {
			en = e
		}
		checkEn("key", en)
	}
	for k, v := range enText {
		out = append(out, WordingTextProblems("English key", k)...)
		out = append(out, WordingTextProblems(k, v)...)
		checkEn(k, v)
	}
	for _, c := range Commands {
		out = append(out, WordingTextProblems("palette "+c.ID, c.Zh)...)
		out = append(out, WordingTextProblems("palette "+c.ID, c.En)...)
		check("palette "+c.ID, c.Zh)
		checkEn("palette "+c.ID, c.En)
	}
	for k, v := range clashes {
		out = append(out, fmt.Sprintf("%q is translated twice: %q and %q", k, v[0], v[1]))
	}
	sort.Strings(out)
	return out
}
