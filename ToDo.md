# ToDo

待研究、还没定的事。定了就写进规格（实现计划放 01 §5），研究过程放 docs/research/。

## all lists页面计算RAM, VRAM不准确
720Mods的列表只计算出了3-5GB, 和实际情况不符合
在can-it-run中增加计算机可用RAM VRAM和列表所需RAM VRAM对比, 如不足应该警告; 可用RAM不足必然导致游戏崩溃, 这个警告和其它警告不同, 应该改变主页面的launch按钮样式, 并在玩家启动的时候弹窗警告

## uninstall RimArcho没有confirm
之后做单独页面, 展示uninstall进度, 阻止玩家删一半把软件关掉

## 按`:`的命令弹窗
应该被组合进GUI的一部分, 允许在所有页面使用

## GUI界面
去掉 Windows 原生标题栏，把最小化、最大化、关闭按钮整合进 RimArcho 的导航栏，让它们共用一行

### 下拉框
所有on/off下拉框改成圆角组件: "C:\Users\Touman_Lin\Pictures\Screenshots\Screenshot 2026-10-31 110204.png"
目前的下拉框太丑, 应该做成这样的: "C:\Users\Touman_Lin\Pictures\Screenshots\Screenshot 2026-10-31 110122.png"

### 地址
把当前点击复制地址修改为在地址尾部增加复制图标按钮; 太长的地址导致换行中间应该变成...; 
地址展示/编辑的通用模板使用 settings/folders这样的展示方式, 不允许在界面中出现可误触的输入框; 改要求同样适用于网页地址, 如settings/community data, external tools 都不符合这条

## 设置页
设置的图标看着像"太阳"这不对
settings/community 这样展示: "C:\Users\Touman_Lin\Pictures\Screenshots\Screenshot 2026-10-31 111324.png"
feedback, community, RimArcho version 合并进 About, 放设置页最顶上.

## 本地化
docs应该做成英语, 项目中除了本地化和词表外不留中文;
需做中文ReadMe

## 加载动画
也许可做界面加载动画--暂时低优先级

## Archo Agent
Archo 要做的是能动手的 Mod 开发 agent，不是翻译、打标签这类小功能 
