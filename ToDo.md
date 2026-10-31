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

## 本地化
docs应该做成英语, 项目中除了本地化和词表外不留中文;
需做中文ReadMe

## 加载动画
也许可做界面加载动画--暂时低优先级

## Archo Agent
Archo 要做的是能动手的 Mod 开发 agent，不是翻译、打标签这类小功能 
