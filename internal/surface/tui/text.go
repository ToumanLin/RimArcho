package tui

import "github.com/ToumanLin/RimArcho/internal/view"

// The interface text lives in internal/view (08 §1.1); the TUI adds its own help text.

func init() { view.Register(map[string]string{helpText: helpTextZh}) }

// T translates an interface string.
func T(s string) string { return view.T(s) }

// Tf translates a format string, then formats.
func Tf(f string, a ...any) string { return view.Tf(f, a...) }

// SetLang selects the interface language.
func SetLang(v string) { view.SetLang(v) }

const helpTextZh = `方向键只移动光标、焦点和滚动位置；改动东西的键立即生效，u 撤销
焦点区     1 未启用  2 已启用  3 暂存  4 详情      ←→ 相邻的区    tab 依次经过所有区
  ↑↓ j k   移动光标（详情区中滚动）    PgUp PgDn  在任何区滚动详情    g G  到顶 / 到底
选择       shift+↑↓ 连续选择    ctrl+space（v）选中或取消一个    esc 取消选择（换到另一个列表也会取消）
动作，作用于选中的 Mod，没有选中时作用于光标所在的 Mod：
  space    启用（放到排序建议的位置）/ 停用 / 从暂存放回
  t        放入暂存；在暂存区中放回原来的位置     T  全部从暂存放回原位置
  m        移动到…：输入序号或 Mod 名（tab 切换之前 / 之后），直接 enter 移到建议位置
  J K      下移 / 上移 1 位       alt+J alt+K  10 位
  #        标签                   enter  全屏详情（←→ 上一个 / 下一个 Mod，tab 子区，I 技术页面）
  o        工坊页面               i  技术信息     x  换成另一份副本    r  添加规则
  S        自动排序（先看差异）   w  保存到游戏（先看差异）    L  启动游戏
  u ctrl+r 撤销 / 重做
/          搜索聚焦的列表；tab 跳到下一个有匹配的列表；esc 清除
!          需要处理               c  最近变化     R  重新扫描     E  切换环境
< > =      列宽（= 恢复默认）     z  名称换行    [ ]  切换列表（窄终端）
ctrl+k     命令面板               :  运行 archo 命令      ?  本帮助    q  退出
鼠标：点击移动光标；ctrl+点击选中或取消一个；shift+点击连续选择；
滚轮滚动详情；拖动列间空隙调整列宽`
