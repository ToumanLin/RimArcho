# ToDo

待研究、还没定的事。定了就写进规格（实现计划放 01 §5），研究过程放 docs/research/。

## 研究：archo.db 里的文本 ID 和路径改成整数

已定，见 01 §5.1 第 24 步和 [研究报告](docs/research/2026-10-30_archo-db-整数键.md)（§10 是决定）。

## RimArcho自动更新
需要做RimArcho的自动更新。现有定时行为和未实现项见 06 §2.16.1，自动更新接进同一套自动维护（新增一个类别），不要另起计时器。

## TUI
TUI不再渲染图片, 不再使用WezTerm
TUI的界面应该更新和GUI尽量一致
增加输入框允许玩家输入指令

## 设置页
增加反馈选项, 反馈内容可以发送到github issue
增加社区链接, 如Discord, github
设置页面重新排序, 将常用的放到一起

## 本地化
docs应该做成英语, 项目中除了本地化和词表外不留中文;
需做中文ReadMe

## Archo Agent
Archo 要做的是能动手的 Mod 开发 agent，不是翻译、打标签这类小功能 
