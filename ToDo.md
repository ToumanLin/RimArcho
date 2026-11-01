# ToDo

Things to research, not yet decided. Once decided, put them in the specifications (implementation plans in 01 §5); research goes in docs/research/.

## GUI
按键和说明文字形式一样会令人困惑: confirm enter -> confirm [enter↵]

### subscription and updates
wording: subscription and updates -> updates
chg39展开的列表令人困惑, chg40的警告放在这里毫无意义, "no longer on the workshop"是什么意思
chg95 1 New, 是什么new了也不说
sub29下面的列表应该超链接到mod library对应mod

### search file
search files的界面做得非常差, 令玩家困惑, 建议还是以32:68的默认比例来做, 左栏搜索栏/筛选栏卡片, 左栏下方是match到的mod instance和对应文件tree, 右栏展示该文件的详细内容
提供的搜索选项也很烂, 比如search scope, 默认当前列表但也不能选择其它列表, 也不能基于mod来源tag,版本(排除同mod其它版本文件)等搜索, 本来在mod library就已经做得很好的filter这也可以用上; 搜索反编译代码是搜对应mod, 那如果对应mod有多个instance呢?困惑
提供的搜索结果展示也很烂, 比如提供的地址竟然是相对地址, 玩家搜到的结果也没法直接打开对应文件或文件夹
完全没有思考玩家用这个界面是要干什么
, 做个mock up网页我看下

## 反编译
反编译功能有两个作用, 其一是研究Mod冲突原因, 其二是创作mod
当前把反编译程序集存放在cache的想法应该重新思考, 给我个方案

## 研究如何展示/修改/管理mod config
mod config很重要, 决定了一个mod的内容如何被游玩, 在游戏中, mod config会显示成界面, 但是在游戏外只是plain xml.

## 无障碍
无障碍不是给有障碍人群用的, 是给AI agent在不截屏的情况下用的, 无障碍应该也有验收标准

## save analysis
### 存档管理
预览：殖民地名, 游戏内日期, 叙事者和难度, 游玩时长, 殖民者数, 存档大小, 使用的mod列表
使用 RimSaves 的文件夹逻辑按殖民地分组：它把文件夹名写进文件名，格式是 文件夹#§#存档名.rws。按殖民地分组时必须识别这个前缀，否则用过 RimSaves 的玩家的存档会被分乱
重命名、复制存档
清理建议

### 使用的mod列表
存档 Mod 列表 对存档的影响
列出存档里属于 Mod X 的东西，比如"3 个 X 种族小人、12 个建筑、1 个派系、1 个 WorldComponent", 
补上 §3.3 的漏洞：提取 Class 后，带 Comp 或 Component 的"功能型"Mod 其实会在存档里留痕，至少能部分判断它有没有被用过
回答社区最常问的"中途能不能删这个 Mod"
当前 Mod 列表对比存档 mod 列表

### 检测存档损坏
文件层面：截断（没有 </savegame>，多半是崩溃时存的）、0 字节、XML 损坏。游戏运行时正在写的档不能报成损坏，要先看 08 §4 的运行检测。
引用层面：静态收集存档里定义过的 loadID 和被引用的 loadID，找出指向不存在对象的引用。这相当于提前发现读档时的 Could not resolve reference。
来源层面：
Class="..." 指向的类型在已装的程序集里不存在。读档一定报错，最严重。
Def 找不到来源（§1.5 已经记了"找不到来源"），应该升级为风险提示。
和日志联动：Monitor 已经记录 context.save，可以把上次读这个档时的 save-load 错误挂到存档卡片上。这样静态检查和实际读档两份证据会显示在一起。
待你决定：要不要做修复，即生成一份修好的副本（去掉错误引用、去掉缺失 Mod 的东西）。

### 游玩回顾（给问题三准备数据）
把一个殖民地的多份存档串成时间线：游戏内天数、真实游玩时长、人数、财富、死亡。
研究进度表（§1.4 里待定的那项）；DLC 也算进"没玩到"。
记出现次数，不只记"出现过"。成本很低，能看出玩得多深，可选。

## Archo Agent
Archo should be a Mod development agent that can take action, not just translate or tag
暂定使用Pi agent魔改版
具体怎么做需要研究

# Deferred 
## TUI
TUI 缺乏导航栏
TUI 使用卡顿

## Loading animation
Maybe add a UI loading animation--low priority for now

## 1.6 memory on a real graphics card is inferred
Calibrated in the VM up to 7.5 GiB texture payload (2026-11-01). Most of the VM's texture RAM is the driver's copy, which the VMware adapter (no dedicated VRAM) commits as process memory; the model now counts it as VRAM and adds back only what does not fit in 90% of the card (research: unity16-texture-memory-breakdown). The 720 list: 36–52 GiB resident without dedicated VRAM, 15–30 GiB with a 24 GiB card. Deferred (2026-11-01): the estimate is good enough for now, so no real-hardware run is planned. If false warnings are ever reported, run 1.6 once on a PC with a discrete GPU and compare Task Manager's "Commit size" and "Dedicated GPU memory" with the estimate (vanilla: about 0.9 GiB commit, 0.66 GiB dedicated).