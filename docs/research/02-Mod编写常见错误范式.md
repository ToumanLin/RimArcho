# Mod 编写常见错误范式（v1）

> 状态：v1，2026-10-22。面向 Mod 作者：真实玩家日志里最常见的**加载阶段**错误，各自是作者写错了什么、应该怎么写。
> 依据：6,734 份 RimWorld 1.6 公开 HugsLib 日志（研究设计见 `00-日志语料顶层设计.md`），G5 错误范式库 `spikes/patterns/patterns-v0.json`（分支 `claude/corpus-g345-f84a49`），全量分析结果 results-v4，G6 标注集，以及本次对 120 个真实错误簇的逐个核对（打开 Mod 的 XML、反编译 DLL、对照 1.6.4871 的 `Assembly-CSharp.dll` 和 Core/DLC Defs）。
> 范围：v1 只写数据和核对都够扎实的加载阶段范式。运行时 NRE、Harmony 栈类错误、存档错误需要另做读代码研究，列在 §5。

## 0. 怎么读

**数字的含义。** 每条范式给三个数：

- **日志**：出现该范式的日志份数（共 6,734 份）。“占比”= 日志数 / 6,734。
- **会话**：同一玩家 48 小时内、Mod 列表几乎相同的连续上传合并为一个会话（共约 5,000 个）。会话数比日志数更接近“多少玩家遇到过”。
- **簇**：同一份日志里同一模板的错误合并为一簇。簇数远大于日志数，说明一份日志里通常有很多条。

数字来自 patterns-v0（v3 归因）；模板的日志数来自 results-v4。两者口径一致，误差在几个百分点内。

**样本偏差。** 这些是玩家**主动分享**的日志，多半是出了问题才分享，所以比例远高于全体玩家。比较范式之间的**相对**频率是可靠的，“55% 的玩家会遇到”这种绝对说法不成立。语料绝大多数是 1.6（Unity）玩家日志；1.7 的情况未覆盖。

**排序按频率，不按严重度。** 每条都单独写了“后果”。最常见的两条（交叉引用、声音）多数只丢一个小功能；排在后面的程序集、静态构造函数错误少见，但一出就是整个 Mod 失效。

**玩家自己很难修。** 同一玩家的连续上传里，加载阶段错误在“只改了 1–3 个 Mod”之后消失的比例很低：`xref.def` 3%、`xref.sound` 1%、补丁 xpath 10%、`xml.no-field` 9%。玩家增删的往往不是出问题的那个 Mod。这类错误基本只能靠作者修。

**核对等级。** 每条范式核对了 3–11 个真实错误簇，标注为：

- **已证实**：亲眼在 Mod 文件（或反编译代码）里看到原因，且与日志一致；
- **较可能**：机制已证实，但日志里那个版本的 Mod 已不在本地库（库里多为更新后的版本），或触发条件看不到；
- **未复现**：现有文件里找不到原因，或归因本身是错的（玩家环境问题、别的 Mod 的锅）。

“仍存在”指截至本地库版本（2025-06 至 2026-09 间下载）该问题是否还在。只有作者明确就是出错方的已证实案例才写出 packageId。

## 1. 总览

| # | 范式 | 典型消息 | 日志 | 占比 | 会话 | 簇 | 后果 | 核对（证实/较可能/未复现） |
|---|---|---|---|---|---|---|---|---|
| 1 | `xref.def` | Could not resolve cross-reference … | 3,706 | 55% | 2,730 | 94,223 | 该引用被丢弃，少数引发后续错误 | 4 / 0 / 4 |
| 2 | `xref.sound` | No Verse.SoundDef named X … (using undefined sound) | 3,532 | 52% | 2,632 | 21,909 | 没有声音（警告） | 6 / 1 / 2 |
| 3 | `patch.op-failed` + `patch.xpath` | Patch operation … failed / Failed to find a node with the given xpath | 2,501 / 1,790 | 37% / 27% | 1,899 / 1,322 | 27,947 / 20,705 | 该补丁不生效；在 Sequence 里会中止后续操作 | 7 / 5 / 1 |
| 4 | `xml.type-missing` | Could not find type named X | 2,248 | 33% | 1,733 | 45,124 | 整个 Def 加载失败，可连锁 | 3 / 1 / 0 |
| 5 | `xml.no-field` | <x> doesn't correspond to any field in type T | 2,044 | 30% | 1,540 | 19,603 | 该字段被忽略 | 3 / 2 / 0 |
| 6 | `xml.config-error` | Config error in X: … | 2,004 | 30% | 1,421 | 46,760 | 多数无害，少数影响游戏 | 5 / 1 / 1 |
| 7 | `asm.version-mismatch` | MissingMethodException / TypeLoadException … | 1,950 | 29% | 1,505 | 17,513 | 功能或整个程序集失效 | 4 / 6 / 0 |
| 8 | `asm.harmony-target` | Undefined target method for patch method … | 1,867 | 28% | 1,425 | 3,690 | 该补丁失效；在 PatchAll 里会拖垮整个 Mod | 5 / 4 / 0 |
| 9 | `asset.texture-missing` | Could not load Texture2D at 'X' … | 1,821 | 27% | 1,411 | 68,115 | 粉色/空白贴图、无声 | 3 / 1 / 5 |
| 10 | `xref.getnamed` | Failed to find T named X. There are N defs … | 1,561 | 23% | 1,197 | 12,098 | 代码拿到 null，常接着 NRE | 2 / 2 / 7 |
| 11 | `patch.findmod` | Patch operation PatchOperationFindMod(X) failed | 1,548 | 23% | 1,165 | 3,429 | 兼容补丁不生效 | 3 / 2 / 3 |
| 12 | `asm.static-ctor` | Error in static constructor of X / Error while instantiating a mod | 1,098 | 16% | 858 | 3,398 | 常常整个 Mod 不工作 | 4 / 2 / 1 |
| 13 | `asset.maintex` | Material 'X' with Shader 'Y' doesn't have … '_MainTex' | 537 | 8% | 414 | 562 | 图标缺失（警告） | 1 / 1 / 1 |
| 14 | `xml.asset-load` | Exception loading UnityEngine.Texture2D from file. | 450 | 7% | 338 | 461 | 粉色贴图 / 无声 | 5 / 1 / 5 |
| 15 | `xml.def-exception` | Exception loading def from file X.xml: … | 413 | 6% | 314 | 1,806 | 整个 Def（有时整个文件）丢失 | 2 / 1 / 2 |

另有 `asm.dependency-missing`（774 份日志，“Cannot resolve dependency to assembly 'UnityEngine.InputLegacyModule' … ReflectionOnly”）：已证实是 Prepatcher 自身机制产生的噪声，776/776 份带此消息的日志都装了 Prepatcher，**不是其他 Mod 作者的错**，本文不展开。

合计核对 120 个案例：已证实 58，较可能 30，未复现 32（另有 1 个目标 Mod 不在库里无法检查）。

## 2. 五个根因

15 条范式背后，作者侧的原因可以归成五类。很多条范式其实是同一个根因的不同表现。

### 2.1 1.5 的内容在 1.6 上加载

这是最大的一类，横跨 `xml.no-field`、`xml.type-missing`、`asm.version-mismatch`、`asm.harmony-target`、`asm.static-ctor`、`xml.def-exception` 和 `xref.sound`。

游戏的行为（反编译 1.6.4871 `ModContentPack.InitLoadFolders` 确认）：

- `LoadFolders.xml` 里**没有 `<v1.6>` 条目**时，游戏取**不超过 1.6 的最高版本**的条目。最新只写到 `<v1.5>` 的 Mod，在 1.6 上加载的就是 1.5 的 Defs 和 DLL。
- 没有 `LoadFolders.xml` 时，版本文件夹的回退规则类似。
- `<li>/</li>` 会把根目录也加载进来。根目录 `Assemblies/` 里的 DLL，只有在版本文件夹里有**同名**文件时才会被覆盖；文件名不同就两个都加载（见 §3.7）。

1.5 → 1.6 改动过、在语料里反复出现的东西：

| 1.5 | 1.6 | 出现在 |
|---|---|---|
| `<race><wildness>` | `<statBases><Wildness>` | xml.no-field（约 400 份日志） |
| `<placingDraggableDimensions>` | `<drawStyleCategory>` | xml.no-field（261） |
| `TerrainDef.holdSnow` / `hideAtSnowDepth` | `holdSnowOrSand` / `hideAtSnowOrSandDepth` | xml.no-field |
| 建筑 `<building><soundAmbient>` | `CompProperties_AmbientSound` | xml.no-field |
| SoundDef `Pawn_Melee_Punch_HitBuilding` | `…_Generic` / `_Metal` / `_Stone` / `_Wood` | xref.sound（约 2,000） |
| `LongEventHandler.QueueLongEvent`（6 个参数） | 多了 `bool forceHideUI` | asm.version-mismatch（约 250） |
| `void GenText.AppendInNewLine` | 返回 `StringBuilder` | asm.version-mismatch |
| `int Map.Tile`，`GenerateThings(int tile)` | `PlanetTile` | asm.version-mismatch、xml.def-exception（113） |
| `Pawn_EquipmentTracker.MakeRoomFor(eq)` | 多了 `MakeRoomFor(eq, out dropped)` 重载 | asm.static-ctor（41） |
| `PawnRenderTree.ProcessApparel` | 移到 `DynamicPawnRenderNodeSetup_Apparel` | asm.harmony-target |
| `Log.Error(string, bool)` | 只剩 `Log.Error(string)` | asm.version-mismatch（经过打包的旧库） |

注意：**只改返回值也是二进制不兼容**（`AppendInNewLine` 一例）。“源码不用改”不等于“旧 DLL 能用”，每个游戏版本都要重新编译。

### 2.2 可选的 DLC / Mod 内容没有守卫

`xref.def`、`xref.sound`、`patch.xpath` 里最常见的作者错误：引用或补丁了某个 DLC / Mod 的东西，却没有在它不存在时跳过。

- 交叉引用：`xref.def` 的缺失名字里，9% 是 DLC 的 Def，52% 是别的 Mod 的 Def；`xref.sound` 里 11% 是 DLC 的声音，14% 是别的 Mod 的声音。
- 补丁：Rabbie、Grimstone: Beasts、MoeLotl 等案例都是对 Royalty / Odyssey 的 Def 打补丁却没有守卫，没装该 DLC 的玩家每次启动都报错。
- 常见的半吊子：同一个文件里给相邻几行加了 `MayRequire`，漏了一行；或者守卫写成了**另一个** DLC（把 Ideology 的声音挂在 Royalty 的条件下）。

### 2.3 依赖的框架或目标 Mod 改了，自己没跟上

- **VEF 1.6 改了命名空间**：`VFECore.*` → `VEF.*`，`VanillaGenesExpanded.*` → `VEF.Genes.*`，`VanillaFurnitureExpanded.*` → `VEF.Buildings.*`。还写旧名的 XML 在 1.6 发布初期大量报 `xml.type-missing`，并连带出 `xml.no-field`（类找不到时游戏退回基类，每个子元素都报“不是字段”）。
- 目标 Mod 把内容挪进了条件文件夹（例：Vanilla Animals Expanded 把一批动物放进只在没有 Odyssey 时加载的文件夹），或删了某个节点（Vanilla Genetics Expanded 的杂交动物去掉了 `<verbs>`），给它打补丁的 Mod 就失败。
- `MayRequire` 只防“Mod 不在”，不防“Mod 在但类改名了”（§3.4 案例）。

### 2.4 写法脆弱

能用但一碰就碎的写法，在别人更新或别的 Mod 先改过之后出错：

- xpath 按位置选节点（`li[5]`）、按会被别人改掉的值选节点（`li[clipFolderPath="Weapon/Revolver"]`）、按完整 C# 类名选节点；
- 用 `PatchOperationSequence` + `PatchOperationTest` 做“没有就跳过”，但没写 `<success>Always</success>`，守卫本身就报错；
- 在静态构造函数或 `Mod` 构造函数里裸调 `harmony.PatchAll()`：一个目标找不到，**整个 Mod 的所有补丁**都不打了；
- `AccessTools.Method(type, "Name")` 不带参数类型，游戏加了重载就变成歧义；不检查 null 就传给 `Harmony.Patch`；
- Transpiler 写死 IL 形状（局部变量下标、闭包类名），游戏小版本热更就失配；
- `DefDatabase<T>.GetNamed` / `ThingDef.Named` 查可能不存在的 Def，且不判空。

### 2.5 资源本身的问题

贴图路径和文件名对不上、引用了没打包的美术、尺寸不是 4 的倍数、WAV 编码游戏读不了、自定义 shader 没有 `_MainTex`。单条频率不高，但每个都是作者发布前能自查出来的。

## 3. 逐条范式

### 3.1 `xref.def`：引用了不存在的 Def

- **频率**：3,706 份日志（55%），2,730 个会话，94,223 簇。最常见的模板：
  - `Could not resolve cross-reference: No RimWorld.StatDef named X found to give to RimWorld.StatModifier (null stat)`（810 份日志）
  - `Could not resolve cross-reference to Verse.ThingDef named X (wanter=thingDefs)`（663）
  - `Could not resolve cross-reference to Verse.ResearchProjectDef named X (wanter=researchPrerequisites)`（484）
  - `… ThingDef named X (wanter=recipeUsers)`（399），`… (wanter=prerequisites)`（339）
- **后果**：该引用被丢弃（属性没加上、研究前置没了、配方没挂到工作台上）。多数只丢一个小功能；个别会在运行时遇到 null。
- **缺失名字的来源**（按 (日志, 名字) 去重，64,212 对）：别的 Mod 的 Def 但该 Mod 没启用 52%；哪里都不存在（拼错、已删、或 Mod 不在本地库）31%；DLC 的 Def 但没装 DLC 9%；原版 Def 本身加载失败的连锁 5%；DLC 显示已启用但 Def 缺失 3%（疑似玩家安装问题）。
- **作者做错了什么**：
  1. 用了 DLC 的 Def 不加守卫。例：护甲 `statBases` 里写了 Royalty 的 `JumpRange`，旁边几行都加了 `MayRequire`，这一行漏了（`com.zoshsgahdnkc.wrmegacorpedition`，仍存在）。
  2. 整个 Mod 就是某 DLC 的附加内容，却只把 DLC 写在 `loadAfter`，没写进 `modDependencies`（`kearril.fishingspotsandanglerkits`，Odyssey 钓鱼，仍存在）。
  3. 从 DLC 复制了一个组件到自己的武器上，组件引用的特效是 DLC 独有的（`kindseal.ufpc`，Anomaly 的焚化器特效，仍存在）。
  4. **连锁**：补丁给**原版** Def 加了一个 C# 类（ModExtension），类名拼错或程序集没加载上，结果原版 Def 本身加载失败，几十条 `xref` 报到别的 Mod 头上。例：ReGrowth 的旧版补丁把命名空间写成 `RegrowthCore`，DLL 实际是 `ReGrowthCore`，导致所有原版 BiomeDef 丢失（25/25 份相关日志都有对应的 Def 加载异常）。当前 1.6 版已修。
  5. 重新定义了原版的抽象父 Def（`<ThingDef Name="BuildingBase" Abstract="True">`）并带上旧值，所有继承它的原版和 Mod 建筑都继承了失效引用。这种情况错误会报在无辜的子 Def 上（VFE Spacer、Tavern 的两例都是这样，真正的来源 Mod 不在库里）。
- **正确写法**：

  单个字段、列表项都可以直接挂 `MayRequire`（游戏会把这条引用当作允许缺失，不报错）：

  ```xml
  <statBases>
    <ArmorRating_Sharp>0.8</ArmorRating_Sharp>
    <JumpRange MayRequire="Ludeon.RimWorld.Royalty">20</JumpRange>
  </statBases>
  <researchPrerequisites>
    <li MayRequire="Ludeon.RimWorld.Odyssey">Fishing</li>
  </researchPrerequisites>
  <comps>
    <li Class="CompProperties_AbilityBurner" MayRequire="Ludeon.RimWorld.Anomaly">
      <!-- 引用 Anomaly 的 mote / effecter -->
    </li>
  </comps>
  ```

  整个 Def 都依赖某 DLC 时，把 `MayRequire` 挂在 Def 上，或者放进条件文件夹（§3.2）。整个 Mod 都依赖时，写进 `modDependencies`：

  ```xml
  <modDependencies>
    <li>
      <packageId>Ludeon.RimWorld.Odyssey</packageId>
      <displayName>Odyssey</displayName>
    </li>
  </modDependencies>
  ```

  不要重新定义原版的抽象父 Def；要改父 Def 的值，用补丁。
- **把握**：已证实 4 例（含连锁机制），未复现 4 例（1 例当前版已修，3 例是玩家缺依赖或其他 Mod 改了父 Def）。

### 3.2 `xref.sound`：引用了不存在的声音

- **频率**：3,532 份日志（52%），2,632 个会话，21,909 簇。模板都是 `Could not resolve cross-reference: No Verse.SoundDef named X found to give to <类型> … (using undefined sound instead)`，按接收方分：`RaceProperties`（2,085 份日志）、`ThingDef`（831）、`VerbProperties`（699）、`Tool`、`SkyfallerProperties`（各 626）、`IngestibleProperties`（568）。
- **后果**：警告，不是错误；对应的动作没有声音。
- **缺失名字的来源**（16,463 对）：哪里都不存在 73%，其中原版 1.6 改名的 `Pawn_Melee_Punch_HitBuilding` 至少 13%、把音频文件路径填进了 SoundDef 字段约 15%，其余是拼错或悬空的第三方名字；别的 Mod 的声音 14%；DLC 的声音 11%。
- **一个引擎细节**：在字段上加 `MayRequire` **对声音无效**。其他 Def 类型缺失时，`MayRequire` 能让游戏静默跳过；SoundDef 查不到时游戏会塞一个“未定义声音”占位，然后**无条件**打这条警告（反编译 `DirectXmlCrossRefLoader.WantedRefForObject.TryResolve` 确认）。所以 DLC 声音只能在 Def 级别守卫。
- **作者做错了什么**（6 例已证实，涉及 7 个 Mod，均仍存在）：
  1. 1.5 的种族 XML 原样复制进 1.6，还在用 `Pawn_Melee_Punch_HitBuilding`（`xmb.breadmech`、`w.zhp`）。
  2. DLC 声音用在总会加载的 Def 里：Anomaly 的安全门声音（`ods.simpledoors`），Odyssey 的护盾声音（`kyzy.shipwallenhanced`）。
  3. 守卫挂错了 DLC：基础 Def 抄自 Ideology 的角色技能，用了 Ideology 的 `CombatCommand_Warmup`，子 Def 却挂 `MayRequire="Ludeon.RimWorld.Royalty"`（`kalospacer.dragonianmix`）。
  4. 引用一个任何版本的任何 Mod 都没有定义过的第三方声音（`aoba.fortress.industrial.citadel` 的 `VFE_Shot_InfernoSpewer`，715 份日志）。
  5. 拼错：`Reload_Standard`，原版是 `Standard_Reload`（`aoba.rimthunder.mobileinfantry`，167 份日志；本地库里有 17 个文件复制了这个拼写错误）。
  6. 把 `clipPath` 填进了要求 SoundDef 名字的字段（`sustainStartSound` 等），较可能（当前版已改）。
- **正确写法**：

  ```xml
  <!-- 原版 1.6 的名字 -->
  <soundMeleeHitBuilding>Pawn_Melee_Punch_HitBuilding_Generic</soundMeleeHitBuilding>

  <!-- DLC 声音：整个 Def 挂 MayRequire -->
  <ThingDef ParentName="DoorBase" MayRequire="Ludeon.RimWorld.Anomaly">
    <defName>MyMod_SecurityDoor</defName>
    <building>
      <soundDoorOpenManual>SecurityDoor_Open</soundDoorOpenManual>
    </building>
  </ThingDef>
  ```

  Def 本身不依赖 DLC、只是想在有 DLC 时换声音，就用守卫过的补丁覆盖（`MayRequire` 要挂在 Sequence 里的 `<li>` 上，见 §3.3 的说明）：

  ```xml
  <Operation Class="PatchOperationSequence">
    <operations>
      <li Class="PatchOperationReplace" MayRequire="Ludeon.RimWorld.Anomaly">
        <xpath>Defs/ThingDef[defName="MyMod_Door"]/building/soundDoorOpenManual</xpath>
        <value><soundDoorOpenManual>SecurityDoor_Open</soundDoorOpenManual></value>
      </li>
    </operations>
  </Operation>
  ```

  或者用条件文件夹：

  ```xml
  <loadFolders>
    <v1.6>
      <li>/</li>
      <li>1.6</li>
      <li IfModActive="Ludeon.RimWorld.Anomaly">1.6/Mods/Anomaly</li>
    </v1.6>
  </loadFolders>
  ```

  SoundDef 字段填 SoundDef 的 `defName`；音频文件路径只出现在 SoundDef 内部的 `<grains><li Class="AudioGrain_Clip"><clipPath>` 里。升级版本文件夹时，把自己的 Def 和新版 Core 的同名字段对一遍。
- **把握**：已证实 6 例，较可能 1 例，未复现 2 例（1 例是别的 Mod 改了原版父 Def，1 例是玩家缺声明过的依赖）。

### 3.3 `patch.op-failed` / `patch.xpath`：XML 补丁找不到目标

- **频率**：一次补丁失败通常同时打出两行：汇总行 `[Mod] Patch operation Verse.PatchOperationX(xpath) failed` 加 `file:`（`patch.op-failed`，2,501 份日志，37%，27,947 簇），和明细行 `[Mod] Verse.PatchOperationX(xpath="…"): Failed to find a node with the given xpath`（`patch.xpath`，1,790 份，27%，20,705 簇）。最常见的来源：Combat Extended 对其他 Mod 的兼容补丁（327 份日志）、Dead Man's Switch – Synthetic（151）、Better Architect Menu（97）、Soundscape Enhanced（79）、Layers: Capes and Armor（73）。
- **后果**：这一个操作不生效。如果它在 `PatchOperationSequence` 里，后面的操作全部中止。
- **作者做错了什么**：
  1. **对 DLC 的 Def 打补丁，没有守卫**（已证实 3 例，均仍存在）：给 Royalty 的 `Empire` 阵营加敌对关系（`runnelatki.rabbieracemod`）；给 Odyssey 的五个生物群系加野生动物，文件里甚至用注释标了“Odyssey”（`abrolo.grimstone.beasts`）；给 Royalty 的 `Throne` 加属性，放在 `success=Normal` 的 Sequence 里（`fxz.moelotlzombie.update`）。
  2. **目标 Mod 更新后没跟进**（已证实 2 例，均已修）：CE 给 Vanilla Animals Expanded 的动物打补丁，VAE 1.6 把这些动物挪进了只在没有 Odyssey 时加载的文件夹；CE 给 Vanilla Genetics Expanded 的杂交动物替换 `<verbs>`，VGE 1.6 已删掉这个节点。
  3. **把内容挪到不加载的文件夹，补丁却留着**（已证实，已修）：Def 被移进 `1.6/to be updated/`，对它的补丁还在 `1.6/Patches/` 里生效。
  4. **守卫本身报错**（已证实，仍存在）：想做“节点存在才改”，写成了 `PatchOperationSequence` + `PatchOperationTest`，没写 `<success>Always</success>`。原版有这个节点，但几个流行的调整类 Mod 会删掉它，于是装了它们的玩家每次都看到红字。
  5. **按位置选节点**（较可能）：`root/children/li[5]`，目标的子节点列表一变就指错。
  6. **按会被别人改的值选节点**（较可能）：`li[clipFolderPath="Weapon/Revolver"]`，别的声音 Mod 先改了这个值就匹配不上。
  7. 已有 G6 标注：PatchOperationAdd 选 `li[label="neurotic"]` 失败（PF05）；目标 Mod 删了 `HelixienGasBoiler` 的字段（PF15）。
- **正确写法**：

  DLC 目标有三种守卫，按稳妥程度排列：放进 `IfModActive` 条件文件夹（见 §3.2）；用 `PatchOperationFindMod`（按显示名，见 §3.11）；或在 Sequence 里的 `<li>` 上挂 `MayRequire`。

  **注意**：1.6.4871 的游戏代码只在**列表项**上检查 `MayRequire`（Sequence 的 `<operations>` 里的 `<li>` 会被跳过），**不检查补丁文件顶层的 `<Operation MayRequire="…">`**（反编译 `ModContentPack.LoadPatches`、`DirectXmlToObject.ObjectFromXml` 确认）。语料里有 Mod 的“修复”就是在顶层 Operation 上加了 `MayRequire`，按游戏代码这不会生效。

  ```xml
  <Operation Class="PatchOperationSequence">
    <operations>
      <li Class="PatchOperationAdd" MayRequire="Ludeon.RimWorld.Royalty">
        <xpath>Defs/FactionDef[defName="Empire"]/permanentEnemyToEveryoneExcept</xpath>
        <value><li>MyMod_Faction</li></value>
      </li>
    </operations>
  </Operation>
  ```

  节点可能不存在（别的 Mod 会删，或父 Def 才有）：用 `PatchOperationConditional`，不要用 Sequence + Test。

  ```xml
  <Operation Class="PatchOperationConditional">
    <xpath>Defs/MeditationFocusDef[defName="Natural"]/requiredBackstoriesAny</xpath>
    <match Class="PatchOperationAdd">
      <xpath>Defs/MeditationFocusDef[defName="Natural"]/requiredBackstoriesAny</xpath>
      <value><li><categoryName>MyMod_Tribal</categoryName></li></value>
    </match>
    <!-- nomatch 省略：限制已被别的 Mod 去掉，就什么都不做 -->
  </Operation>
  ```

  选节点用稳定的键，不用位置：

  ```xml
  <!-- 不要：root/children/li[5] -->
  <xpath>Defs/PawnRenderTreeDef[defName="MyRace"]/root/children/li[debugLabel="Head"]/drawData</xpath>
  ```

  其他：可选目标不要放进 `success=Normal` 的 Sequence；把 Def 挪出加载路径时，它的补丁一起挪；目标 Mod 大版本更新后，把补丁对着新版重新跑一遍（开发者模式启动看日志即可）。
- **把握**：已证实 7 例，较可能 5 例（4 例日志里的旧版本已不在库里），未复现 1 例；另复用 G6 已审核的 2 例。

### 3.4 `xml.type-missing`：XML 里写的类找不到

- **频率**：2,248 份日志（33%），1,733 个会话，45,124 簇。最常见的模板：`Could not find class Configurable_Deconstruct_Percentage.DeconstructSettings while resolving node ModSettings`（107 份日志），NewRatkin 的一组 `Could not find a type named NewRatkin.*`（各约 80），`Exception loading def from file X.xml: … Could not find type named VFECore.* / VFESecurity.*`。RimArcho 的归因对这条几乎不给来源（0%），本节的来源 Mod 是用“错误日志里哪些 Mod 的出现率远高于全体”找出来再核对文件的。
- **后果**：`Class=` 出现在 Def 的组件或扩展里时，**整个 Def 加载失败**；如果这个 Def 是被补丁改过的原版 Def，就会出现 §3.1 的连锁。出现在 `<Operation Class=…>` 上时，游戏退回基类，补丁不生效，同时每个子元素都报 `xml.no-field`。
- **作者做错了什么**：
  1. **框架改名**（已证实 2 例）：VRE Android 的 GeneDef 还写 `VanillaGenesExpanded.GeneExtension`，VEF 1.6 已改为 `VEF.Genes.GeneExtension`（55 份日志，全部在 1.6 发布当周，当前版已修）；VPE Biotech Integration 用 `VFECore.PatchOperationToggableSequence`（已修）。
  2. **软依赖的类被删了，`MayRequire` 挡不住**（已证实，仍存在）：Grimworld – Hammer of the Imperium（`grimworld.astramilitarum`）的战壕组件写 `<li Class="VFESecurity.CompProperties_TerrainSetter" MayRequire="VanillaExpanded.VFESecurity">`，VFE Security 1.6 已经没有这个类。`MayRequire` 只在 VFE Security 未启用时生效；启用了但类没了，整个战壕 Def 加载失败（同时装了这两个 Mod 的 64 份日志中 62 份报错，没装 VFE Security 的 118 份一份都没有）。
  3. **自己 Mod 的 1.5 内容在 1.6 上加载**（已证实，VFE Security 早期 1.6 版，74 份日志）：见 §2.1。
  4. **XML 和自己的 DLL 不同步**（较可能）：XML 的 `workerClass` 指向 DLL 里已经没有的类。
  5. 设置类改名后，玩家的旧配置文件里还是旧类名（Configurable Deconstruct 一例）：游戏会自动改用新类，**无害**，可以忽略。
- **正确写法**：

  - 依赖的框架升级时，全局搜一遍 `Class="旧命名空间.`，并用开发者模式启动确认没有 `Could not find type`。
  - 软依赖的组件放进该 Mod 的条件文件夹，而不只是挂 `MayRequire`；这样至少可以按目标 Mod 的版本分文件夹。
  - XML 和 DLL 用同一次构建发布。
- **把握**：已证实 3 例，较可能 1 例。

### 3.5 `xml.no-field`：XML 写了类型里没有的字段

- **频率**：2,044 份日志（30%），1,540 个会话，19,603 簇。最常见的模板：`<placingDraggableDimensions>…` on `ThingDef`（261 份日志），`<wildness>…` in `<race>`（两组 250 和 198），`<soundAmbient>MechTurretBig_Call</soundAmbient>` in `<building>`（约 150）。
- **后果**：该字段被忽略。单独看无害，但它几乎总是说明**整份 XML 是给旧版本写的**，同一个 Def 里其他字段大概率也有问题。
- **作者做错了什么**：
  1. **1.6 删除或改名的字段，1.5 的 XML 还在加载**（已证实 2 例，均已修）：NewRatkin 的动物（`solaris.ratkinracemod`）在 1.5 文件里写 `<wildness>`（约 300 份日志），当时 1.6 加载的是这份旧文件；Gloomy Furniture 的墙（`solaris.furniturebase`）写 `<placingDraggableDimensions>`（52 份日志）。同样的修法在 Epochs Pottery、Uncle Boris、SimpleStairs fork 和 VFE Security（74 份日志）里都看得到：1.5 文件夹里是旧字段，1.6 文件夹里改成了 `drawStyleCategory`。完整对照表见 §2.1。
  2. **类找不到导致的假“没有字段”**（已证实）：见 §3.4。修类名即可，不用管这些字段。
  3. **框架版本不匹配**：Mod 写的字段属于比玩家装的更新（或更旧）的框架版本，如 `PipeSystem.ProcessDef` 的 `maxOutputCount`（G6 XL14）。
- **不是 Mod 的问题**：机械族炮塔的 `soundAmbient` 和 Ideology `LayoutRoomDef` 的 `selectionWeight` 两组（约 150 份日志）逐字节等于 1.5 原版 Defs，没有任何 Mod 的出现率偏高，且同时伴随大量原版贴图缺失。这是玩家的游戏文件混了旧版本，应该提示玩家“验证游戏文件完整性”，不算作者错误。
- **正确写法**：

  ```xml
  <!-- 1.5 -->
  <race>
    <wildness>0.55</wildness>
  </race>
  <placingDraggableDimensions>1</placingDraggableDimensions>

  <!-- 1.6 -->
  <statBases>
    <Wildness>0.55</Wildness>
  </statBases>
  <drawStyleCategory>Walls</drawStyleCategory>
  ```

  更重要的是**为 1.6 提供真正的 1.6 文件夹**，并在 `LoadFolders.xml` 写 `<v1.6>` 条目（§2.1），否则 1.6 玩家会退回加载 1.5 的内容。
- **把握**：已证实 3 例，较可能 2 例（其中 1 例确认不是 Mod 问题）。

### 3.6 `xml.config-error`：游戏的配置自检报错

- **频率**：2,004 份日志（30%），1,421 个会话，46,760 簇。最常见的模板：`Config error in FT_ENIAC: description has trailing whitespace`（93 份日志），`Multiple gene categories share the same displayPriorityInXenotype`（84），`Vent_Over: is airtight but Fillage is not Full`（60），`apparel has no layers`（60）。
- **后果**：分三档。

  | 档 | 消息 | 实际影响 |
  |---|---|---|
  | 无害 | description 首尾空格；基因类别 `displayPriorityInXenotype` 重复；研究坐标重复；`category None` 却定义了 Beauty | 只是日志噪声 |
  | 轻微 | `has costStuffCount but no stuffCategories`；`tradeability doesn't allow traders to sell` | 某个数值被静默忽略 |
  | 实际影响 | `weaponMoney` 低于最便宜武器；`apparel has no layers`；`thingClass` / `workerClass` 为 null；有 comps 但类不是 `ThingWithComps`；`Two ThinkNodes have the same unique save key` | 生成的小人没武器、衣服穿不上、组件被丢弃、存读档问题 |
- **作者做错了什么**（已证实 5 例）：
  1. 基因类别的显示优先级用了常见整数，和别的 Mod 撞了（Vanilla Races Expanded – Android 的 `VREA_Hardware`，无害，仍存在）。
  2. **继承了新版父 Def 的新字段**：原版 1.6 给 `Vent` 加了 `isAirtight`，子 Def 把 `fillPercent` 改成 0.6，组合矛盾（`uuugggg.replacestuff` 原版，仍存在；Continued 版的 1.6 文件夹已修，但还附带了根目录的旧文件）。
  3. 复制粘贴留下 `costStuffCount` 却没有 `stuffCategories`（SimpleStairs fork，`rw.mod.simplestairs`，仍存在）。
  4. PawnKind 的 `weaponMoney` 下限低于唯一可选武器的价格，可能生成没枪的坦克（`dms.proton.jointoperations`，仍存在）。
  5. 给父 Def 打补丁，补丁的值泄漏到了 DLC 新增的子 Def（CE 给 `Gun_ChargeLance` 加 `destroyOnDrop`，Odyssey 的 `Gun_ChargeLance_Unique` 继承了它；当前版已修）。
- **正确写法**：

  - 开发者模式启动，打开日志，把自己 Mod 的 `Config error` 全清掉。清不掉的至少要确认属于“无害”一档。
  - 继承原版父 Def 时，每个新版本都对一下父 Def 新增了什么。
  - 给父 Def 打补丁前，搜一下它在新版本里多了哪些子 Def（1.6 Odyssey 给几把原版武器加了 `_Unique` 子 Def）。
  - `weaponMoney` 的下限要高于 `weaponTags` 里最便宜武器的市价。
  - 不要整体替换共享父 Def 的 `<apparel>` 或 `<layers>`，否则所有子 Def 都会丢失层（“apparel has no layers”的一组 60 份日志很可能就是这样，报错落在无辜的子 Def 上，较可能）。
- **把握**：已证实 5 例，较可能 1 例，未复现 1 例（频率第一的“首尾空格”在当前文件里找不到）。

### 3.7 `asm.version-mismatch`：DLL 调用了已不存在的代码

- **频率**：1,950 份日志（29%），1,505 个会话，17,513 簇。最常见的模板：`ReflectionTypeLoadException`（407 份日志），`TypeLoadException: Could not resolve type … from typeref (expected class … in assembly …)`（342 + 305），`MissingMethodException: Method not found: … QueueLongEvent(…)`（约 250）。
- **后果**：调用点所在的功能失效。出在静态构造函数里就是整个 Mod 失效；`TypeLoadException` 发生在 `GetTypes()` 时，游戏会**丢掉整个程序集**。
- **作者做错了什么**：
  1. **只有 1.5 的 DLL，LoadFolders 只写到 v1.5**（已证实 2 例，仍存在）：Replace Stuff 原版（`uuugggg.replacestuff`）调用 6 参数的 `QueueLongEvent`；RocketMan（`krkr.rocketman`）调用 `void AppendInNewLine`。两者在 1.6 上都通过回退加载 1.5 的 DLL（§2.1）。Replace Stuff 的 Continued 分支已用 1.6 重新编译。
  2. **打包了旧版的共享库**（已证实）：几个 Mod 各自带一份 `0MultiplayerAPI.dll`，程序集按名字绑定，**先加载的那份服务所有 Mod**。0.1 / 0.2 版调用了 1.6 已删除的 `Log.Error(string, bool)`。在全部 33 份报此错的日志里，实际加载的都是 0.1 或 0.2，从来不是 Rimefeller 自带的 0.5；其中 19 份是旧版 Achtung 带进来的。受害的 Mod 自己没有做错任何事。当前的 Achtung 已带 0.6。
  3. **根目录 `Assemblies/` 里的旧 DLL 漏进来**（较可能）：LoadFolders 的 `<v1.6>` 里有 `<li>/</li>`，主 DLL 又改过名，根目录那份 1.0 时代的 DLL 就和新 DLL 一起加载了（引用 Harmony 1 的类型，75 份日志）。
  4. **用 1.5 的引用程序集编译了“1.6 版”**（较可能，已修）。
  5. **目标框架错了**（较可能）：DLL 引用 `System.Collections, Version=10.0.0.0`，是用 .NET 10 的引用程序集编译的，游戏的 Mono 没有。
  6. **对可选 Mod 的硬引用**：正确示范是 Fortified Features Framework，它把引用 CE 类型的代码放进单独的 DLL，只在 `IfModActive="CETeam.CombatExtended"` 的文件夹里加载（G6 AS03/AS04 的报错是玩家排序违反了它的 `loadAfter`，不是作者的错）。
- **正确写法**：

  ```xml
  <!-- LoadFolders.xml：每个支持的版本一个条目，DLL 按版本放 -->
  <loadFolders>
    <v1.5>
      <li>1.5</li>
    </v1.5>
    <v1.6>
      <li>1.6</li>
      <li IfModActive="CETeam.CombatExtended">1.6/CE</li> <!-- 引用 CE 类型的代码单独成一个 DLL -->
    </v1.6>
  </loadFolders>
  ```

  ```xml
  <!-- csproj：目标框架固定 net472，引用游戏自己的 DLL 或对应版本的参考包 -->
  <TargetFramework>net472</TargetFramework>
  <PackageReference Include="Krafs.Rimworld.Ref" Version="1.6.*" />
  ```

  - 不维护新版本了，就不要在 `supportedVersions` 里写 1.6，让玩家看到不兼容提示。
  - 共享库（0Harmony、0MultiplayerAPI 等）要么不打包、依赖提供它的 Mod（Harmony 用 `brrainz.harmony`），要么只打包最新版。
  - 不要在根目录 `Assemblies/` 留 DLL，旧版本的 DLL 放进各自的版本文件夹。
- **把握**：已证实 4 例，较可能 6 例（多数已修，日志里那个版本的 DLL 已拿不到），未复现 0。

### 3.8 `asm.harmony-target`：Harmony 补丁的目标方法不存在

- **频率**：1,867 份日志（28%），1,425 个会话，3,690 簇。最常见的模板：`HarmonyException: Patching exception in method null ---> ArgumentException: Undefined target method for patch method …`（142 + 68），`Error in static constructor of LWM.DeepStorage.ModInit … Patching exception`（99），以及一条 Big & Small 的提示信息 `Failed to patch VFEInsectoids 2's Creep with additional genes.`（405 份日志，Message 级别，不是错误）。
- **后果**：取决于补丁在哪里打。`[StaticConstructorOnStartup]` 或 `Mod` 构造函数里裸调 `PatchAll()` 时，**一个目标失败，整个 Mod 剩下的补丁全部不打**。按类捕获异常的 Mod（VEF、LunarFramework）只丢这一个补丁。
- **作者做错了什么**：
  1. **补丁打在别的 Mod 的方法上，对方删了这个方法**（已证实）：Engine Industries（`aoba.motorization.engine`）补丁 `VehiclePawn.GetExtraFloatMenuOptionsFor`，两个 Vehicle Framework 1.6 版本里都没有这个方法；`PatchAll` 在静态构造函数里，于是 Motorization 的所有补丁都丢了（52 份日志，当前版已修）。
  2. **补丁打在别的 Mod 自己的补丁类上**（已证实，已修）：Map Preview 的兼容模块按字符串补丁 `VEE.FertilityGrid_Patch`，VEE 1.6 删了这个类。好在框架按模块捕获，只丢这一个兼容（146 份日志）。新版加了“目标存在才打”的检查，正是正确写法。
  3. **Transpiler 写死 IL 形状**（已证实，仍存在）：While You're Up 的 1.6 补丁版（`zsbk.patch16.whileyoureup`）在游戏 1.6.4850 / 4871 上 24/24 份日志全部失败，在更早的 118 份上 0 失败，因为热更新重新编译了目标方法的闭包。补丁在 `Mod` 构造函数的 `PatchAll` 里，所以整个 Mod 失效。LWM Deep Storage 的 1.5 版 transpiler 插入写死的 `Ldloc_1`，在 1.6 上生成非法 IL（已修）。
  4. **补丁打在热更新才加入的成员上**（较可能）：VEF 的两个补丁只在较早的 1.6 版本上失败；`TargetMethods()` 返回了 null 却没有过滤。
  5. **原版方法在 1.6 移走了**（较可能，游戏侧已证实）：`PawnRenderTree.ProcessApparel` 移到了 `DynamicPawnRenderNodeSetup_Apparel`；`Dialog_WorldSearch` 本身不声明 `DoWindowContents`（在泛型基类 `Dialog_Search<T>` 上）。
  6. **好的示范**：Big & Small 反射另一个 Mod 的私有字段前先判空，找不到就跳过。唯一的问题是每次启动都打一条看起来像错误的消息。
- **正确写法**：

  按类打补丁，一个类失败不影响其他类：

  ```csharp
  [StaticConstructorOnStartup]
  public static class MyModStartup
  {
      static MyModStartup()
      {
          var harmony = new Harmony("author.mymod");
          foreach (var type in AccessTools.GetTypesFromAssembly(typeof(MyModStartup).Assembly))
          {
              try { harmony.CreateClassProcessor(type).Patch(); }
              catch (Exception e) { Log.Warning($"[MyMod] {type.Name} not applied: {e.Message}"); }
          }
      }
  }
  ```

  跨 Mod 或可能不存在的目标，用 `Prepare()` 先确认存在：

  ```csharp
  [HarmonyPatch]
  public static class Patch_VehicleFloatMenu
  {
      static MethodBase target;

      static bool Prepare()
      {
          target ??= AccessTools.Method("Vehicles.VehiclePawn:GetExtraFloatMenuOptionsFor",
                                        new[] { typeof(IntVec3) });
          return target != null; // false：Harmony 跳过这个补丁类，不报错
      }

      static MethodBase TargetMethod() => target;

      static void Postfix(ref IEnumerable<FloatMenuOption> __result) { /* … */ }
  }
  ```

  `TargetMethods()` 过滤 null：

  ```csharp
  static IEnumerable<MethodBase> TargetMethods() =>
      new[] { AccessTools.DeclaredMethod(typeof(A), "X"), AccessTools.DeclaredMethod(typeof(B), "Y") }
      .Where(m => m != null);
  ```

  Transpiler 找不到要改的位置时，原样返回并记警告，不要抛异常：

  ```csharp
  static IEnumerable<CodeInstruction> Transpiler(IEnumerable<CodeInstruction> instructions)
  {
      var codes = instructions.ToList();
      int i = codes.FindIndex(c => c.Calls(FindResourcesMethod));
      if (i < 0)
      {
          Log.Warning("[MyMod] transpiler anchor not found; feature disabled for this game build");
          return codes;
      }
      // … 插入代码
      return codes;
  }
  ```

  能用 Prefix / Postfix 解决的不用 Transpiler；局部变量和闭包字段用反射按名字找并判空，不写死下标。
- **把握**：已证实 5 例，较可能 4 例。

### 3.9 `asset.texture-missing`：贴图或音频路径找不到文件

- **频率**：1,821 份日志（27%），1,411 个会话，68,115 簇。模板：`Could not load Texture2D at 'X' in any active mod or in base resources.`（1,187 份日志），`… for def 'Y' …`（636），`Could not load AudioClip at 'X' …`（291），`Failed to find any textures at X while constructing Multi(initPath=…)`（约 30 一组）。
- **后果**：粉色或空白贴图、没有声音。功能不受影响。
- **注意：这条很多不是作者的错。** 核对的 9 组里 5 组是玩家环境：一份日志里同时缺几百张贴图（安装损坏）、某个月里一个 Mod 的贴图整体丢失、原版路径缺失却和任何 Mod 都不相关。判断方法：如果装了这个 Mod 的日志里**几乎每份**都报，是 Mod 的问题；只有 1% 左右报，是玩家环境。
- **作者做错了什么**（已证实 3 例）：
  1. **加了 Def 没加美术**：一个服装样式 Def 指向 `Things/ApparelStyles/…Rustic/…`，Mod 里只有另一个样式的贴图（`lanyv.bunnyarmourfromhoomancute`，装了该 Mod 的 116 份日志里 86 份报错，仍存在）。
  2. **用了别的 Mod 的贴图和声音，却把依赖注释掉了**：About.xml 里 `Solaris.FurnitureBase` 依赖被包在“临时删除”的注释里（`gloomy.furniture.fix`，19 份报错日志全部没装 Gloomy Furniture，装了的 319 份一份都没有，仍存在）。
  3. **`clipPath` 和文件名不一致**：写 `Kill`，文件是 `Kill_.ogg`（`hirokazu.akiyama.music.pack`，修复前 138 份里 136 份报错，2026-10 已修）。
  4. 较可能：附加 Mod 写死了父 Mod 的贴图路径，父 Mod 后来挪了路径。
- **游戏细节**：
  - `Graphic_Multi` 只有在 `_north/_east/_south/_west` 和不带后缀的文件**全都不存在**时才报错，有一个方向就够了。
  - 消息里的 `for def 'Y'` 是正在构建图形的那个 Def，可能是被别的 Mod 改过的原版 Def，不一定是出错的 Mod 自己的 Def。
- **正确写法**：

  - 发布前开开发者模式，进游戏把自己 Mod 的物品、建筑、服装各生成一次，看日志里有没有 `Could not load`。
  - 用别的 Mod 的资源，就把它写进 `modDependencies`；不想硬依赖就自己带一份。
  - `clipPath` / `texPath` 是 `Sounds/` / `Textures/` 下的相对路径，不带扩展名，大小写和文件一致（Linux / Mac 区分大小写）。
- **把握**：已证实 3 例，较可能 1 例，未复现 5 组（均为玩家环境）。另外，G6 的 TX13（Dead Man's Switch“未打包贴图”）需要复查：库里的版本其实带了这张贴图，那 25 份日志都落在 2026-10 该 Mod 贴图整体丢失的时间窗里。

### 3.10 `xref.getnamed`：C# 代码按名字找 Def 没找到

- **频率**：1,561 份日志（23%），1,197 个会话，12,098 簇。模板 `Failed to find <Type> named X. There are N defs of this type loaded.`，类型依次是 ThingDef（487 份日志）、TerrainDef（328）、JobDef（304）、ThoughtDef（267）、HediffDef（214）。
- **后果**：调用方拿到 null。不判空的话，紧接着就是 NullReferenceException；如果在静态构造函数里，整个类都初始化失败。
- **一个引擎细节**：`DefDatabase<T>.GetNamed(name)`、`ThingDef.Named(name)`、`HediffDef.Named(name)` 以及没有 `[MayRequire]` 的 `[DefOf]` 字段，找不到时都打这条；`GetNamedSilentFail(name)` 不打。消息里 **“There are 0 defs”** 说明代码在 Def 加载完之前就运行了。
- **作者做错了什么**：
  1. **对可能被删或改名的 Def 用 `Named` 且不判空**（已证实）：炖菜 Mod（`badoaks.campfirestews`）在静态构造函数里用 `ThingDef.Named` 找原版和其他 Mod 的炉灶，拿到 null 后访问 `.recipes`，静态构造函数崩溃（仍存在）。
  2. **调用得太早**（已证实）：Maru 种族（`vamv.maruracemod`）在一个 RecipeWorker 的静态构造函数里 `HediffDef.Named(...)`，这时 Def 还没加载，58 份日志全部是“There are 0 defs”。同一个程序集里已经有对应的 `[DefOf]` 类（仍存在）。
  3. 较可能：代码查找的 Def 在 XML 里挂了 DLC 条件，代码却没挂同样的条件（已修）。
- **RimArcho 的归因对这条很弱**：抽样的 10 个“来源 Mod”都不是真正的调用方（DLL 里的字符串只是比较、字典键，或者已经用了 SilentFail）。未复现的 7 例大多是这个原因，另有一组是“DLC 显示已启用但 Def 缺失”的玩家环境问题（Odyssey 的 `AncientHeavyBridge` 等，310 份日志）。
- **正确写法**：

  ```csharp
  // 可能不存在的 Def：SilentFail + 判空
  var stove = DefDatabase<ThingDef>.GetNamedSilentFail("MyOtherMod_Stove");
  if (stove != null) stove.recipes.Add(MyDefOf.MyMod_Stew);

  // 固定引用用 [DefOf]，DLC 或可选 Mod 的字段加 [MayRequire]
  [DefOf]
  public static class MyDefOf
  {
      public static RecipeDef MyMod_Stew;
      [MayRequire("Ludeon.RimWorld.Odyssey")] public static ThingDef AncientHeavyBridge;

      static MyDefOf() => DefOfHelper.EnsureInitializedInCtor(typeof(MyDefOf));
  }
  ```

  需要 Def 的初始化放在 `[StaticConstructorOnStartup]` 类里（Def 加载完后才运行），或者推迟到第一次使用时；不要放在 Def 加载期间就会被触发的类型（Worker、Comp 类）的静态构造函数里。
- **把握**：已证实 2 例，较可能 2 例，未复现 7 例（归因错误或玩家环境）。

### 3.11 `patch.findmod`：可选 Mod 的兼容补丁失败

- **频率**：1,548 份日志（23%），1,165 个会话，3,429 簇。模板：`[Mod] Patch operation Verse.PatchOperationFindMod(X) failed`，或 XmlExtensions 的 `FindMod` / `OptionalPatch failed`。最常见的来源：VEF（207 份日志）、Cinders of the Embergarden（74）、Majestic Foliage Framework（71）、Misc. Training Medieval Retexture（64）。
- **先读懂消息**：`FindMod(X) failed` **不表示 X 不存在**，而是 `match` 或 `nomatch` 里面的操作失败了；括号里只是守卫的 Mod 列表，哪个分支都会打印。另外，`PatchOperationFindMod` 比较的是 Mod 的**显示名**（精确相等），不是 packageId：“Humanoid Alien Races” 和 “Humanoid Alien Races ~ Dev” 是两个名字。（反编译 `PatchOperationFindMod.ApplyWorker` / `ModLister.HasActiveModWithName` 确认。）
- **作者做错了什么**（已证实 3 例）：
  1. **守卫对了，里面的 xpath 用了框架的旧类名**：`li[@Class="GraphicCustomization.CompProperties_GraphicCustomization"]`，VEF 1.6 已改为 `VEF.Graphics.…`（`oracle.miscellania`，已修）。
  2. **守卫了错的 Mod**：守卫 Medieval Overhaul，两个分支却都在补丁 Misc. Training 的 Def；Misc. Training 既没写进依赖、也没被守卫，所以没装它就必定失败（`serek.misctrainingmedievalretexture`，仍存在）。
  3. **和另一个 Mod 改的是同一个节点**：给 VFE Security 炮塔的燃料组件打补丁，CE 会先删掉这个组件（`reel.turretpipeline`；作者已在 About.xml 声明与 CE 不兼容，所以主要是玩家混装）。
  4. 较可能：VEF 对原版 `Column` 房间需求的替换没有条件判断，别的 Mod 先把它换了类就匹配不上（VEF 的 Royalty 部分已加条件，Ideology 部分还没有）；LoadFolders 的条件和 FindMod 的条件不一致。
- **正确写法**：

  ```xml
  <!-- 守卫你实际要补丁的那个 Mod；名字必须和它 About.xml 的 <name> 完全一致 -->
  <Operation Class="PatchOperationFindMod">
    <mods>
      <li>Target Mod Name</li>
    </mods>
    <match Class="PatchOperationConditional">
      <xpath>Defs/ThingDef[defName="ArcheryTarget"]/graphicData</xpath>
      <match Class="PatchOperationReplace"> … </match>
    </match>
  </Operation>
  ```

  - 能用 packageId 的地方优先：`LoadFolders.xml` 的 `IfModActive="packageId"` 和 Sequence 里 `<li>` 上的 `MayRequire="packageId"` 都按 packageId 匹配，不受显示名影响。只针对某个 Mod 的补丁，放进以它为条件的文件夹最稳。
  - xpath 不要以完整 C# 类名为键；用结构选择，或 `contains(@Class,"CompProperties_GraphicCustomization")`。
  - 多个 Mod 都会改的节点，套一层 `PatchOperationConditional`。
- **把握**：已证实 3 例，较可能 2 例，未复现 3 例，无法检查 1 例（目标 Mod 不在库里）。

### 3.12 `asm.static-ctor`：启动代码抛异常

- **频率**：1,098 份日志（16%），858 个会话，3,398 簇。模板：`Error in static constructor of X: System.TypeInitializationException …`，`Error while instantiating a mod of type X: TargetInvocationException …`，以及 `[Harmony: failed to render frame: TypeInitializationException]`（278 份日志，是同一问题在栈追踪里的残影）。
- **后果**：这条说的是代码**在哪里**失败，不是**为什么**失败。静态构造函数一旦抛异常，这个类型此后每次被访问都会再抛一次 `TypeInitializationException`，表现为同一个 Mod 在游戏各处反复报错；`Mod` 构造函数抛异常则这个 Mod 的设置和初始化全部没做。
- **作者做错了什么**：
  1. 静态构造函数里 `PatchAll()`，一个补丁失败全部中止（见 §3.8，最常见）。
  2. 静态构造函数里调用了 1.6 已改的 API（RocketMan、Replace Stuff，见 §3.7）。
  3. **`AccessTools.Method` 只给名字不给参数类型**（已证实）：PawnShields（JecsTools 的 1.5 版）手动补丁 `Pawn_EquipmentTracker.MakeRoomFor`，1.6 新增了带 `out` 参数的重载，按名字查找变成歧义（41 份日志）。非官方 1.6 版已改为带参数类型。
  4. **`AccessTools.Method` 返回 null 不检查就传给 `Harmony.Patch`**（较可能）：消息是 `Null method for <harmonyId>`。[LTO] Colony Groups 在一个静态构造函数里连续约 45 次这样调用，任何一个目标缺失就全部中止。
  5. **打包的共享库自己的静态构造函数失败**（已证实，见 §3.7 的 0MultiplayerAPI）。
  6. **不是这个 Mod 的错**：`harmony.log.txt` 拒绝访问。是**别的** Mod 或环境变量打开了 Harmony 调试模式，而玩家的桌面不可写，之后打补丁的 Mod 都会背锅。作者的教训是：**发布版不要开 `Harmony.DEBUG` 或 `[HarmonyDebug]`**。
  7. 静态构造函数里遍历 `DefDatabase` 改其他 Mod 的 Def，不判空（未复现具体触发条件，但写法本身脆弱）。
- **正确写法**：

  ```csharp
  // 一律带参数类型
  var m = AccessTools.Method(typeof(Pawn_EquipmentTracker), "MakeRoomFor", new[] { typeof(ThingWithComps) });

  // 手动 Patch 前判空
  if (m == null) Log.Warning("[MyMod] MakeRoomFor not found; shield auto-drop disabled");
  else harmony.Patch(m, postfix: new HarmonyMethod(typeof(MyPatches), nameof(MyPatches.MakeRoomFor_Postfix)));
  ```

  静态构造函数里做的事越少越好；遍历和修改 Def 时按 Def 捕获异常，一个坏 Def 不拖垮全部。
- **把握**：已证实 4 例，较可能 2 例，未复现 1 例。

### 3.13 `asset.maintex`：自定义 shader 没有 `_MainTex`

- **频率**：537 份日志（8%），414 个会话，562 簇。模板 `Material 'X' with Shader 'Y' doesn't have a texture property '_MainTex'`。消息里从不带 Mod 名，RimArcho 无法归因。
- **后果**：警告；该物品的 UI 图标为空。
- **机制**：ThingDef 不是 mote、又没有 `uiIconPath` 时，游戏用材质的 `mainTexture` 当图标（`BuildableDef.ResolveIcon`）。材质用的 shader 没有 `_MainTex` 属性，Unity 就打这条警告。比 G5 原先记录的“有 uiIconColor 但没有图标路径”更准确。
- **作者做错了什么**（已证实 1 例）：一个技能特效 Def 不是 mote，却用了自定义闪电 shader，也没有 `uiIconPath`（`ariandel.ariandellibrary`，411 份报错日志中 410 份装了它，仍存在）。较可能的第二例是 VE Gravship 的色键 shader（121/121 份报错日志都装了它，但该 Mod 不在库里无法查看）。
- **正确写法**：给这类 Def 写一个 `uiIconPath`；或者在 shader 里声明一个隐藏的 `_MainTex` 属性：

  ```
  Properties {
      [HideInInspector] _MainTex ("", 2D) = "white" {}
      // … 自己的属性
  }
  ```
- **把握**：已证实 1 例，较可能 1 例，未复现 1 例。

### 3.14 `xml.asset-load`：贴图或音频文件读不了

- **频率**：450 份日志（7%），338 个会话，461 簇。模板：`Exception loading UnityEngine.Texture2D from file.`（381 份日志），`… AudioClip from file.`（78）。下一行的 `absFilePath:` 指出具体文件。
- **后果**：粉色贴图或没有声音。
- **最大的一类不是作者打包的问题**：全语料里 18,374 条（374 份日志）是块压缩 DDS 的宽或高不是 4 的倍数。被点名的几个 Mod **根本没有打包 DDS**，只有 PNG，而且每个只在 1–2% 的日志里报错。这些 DDS 是玩家用贴图优化工具在本地生成的；游戏 1.6 拒绝加载尺寸不是 4 的倍数的块压缩贴图。作者这边的问题只是 PNG 尺寸：4266×2400（Shella Backgrounds）、42×42（Charmweavers）、400×322（Alpha Prefabs）、274×232 和 279×278（Better Info Card）。
- **作者做错了什么**（已证实 5 例）：
  1. PNG 尺寸不是 4 的倍数（上面 4 例，均仍存在）。贴图优化工具很普遍，作者把尺寸做成 4 的倍数就能避免玩家这边出错。
  2. **WAV 用了游戏解码不了的格式**：8 个 WAV 是 `WAVE_FORMAT_EXTENSIBLE`、双声道、96 kHz（`plaxen.hamster`，53 份日志中 43 份报错，仍存在）。真正的错误被掩盖成 `ArgumentNullException: key`，很难从日志看出来。
  3. 较可能：音频文件名含非 ASCII 字符（如 `Khazad-Dûm.ogg`），加载时 404。
  4. Better Info Card 的旧版本文件夹里还留着无用的截图文件（`Snipaste_*.png`），也被当贴图加载。
- **游戏细节**：1.6 的 DDS 加载器只接受 DXT1、DXT5、BC7（DX10 头）和几种未压缩格式；同目录下有同名 `.dds` 时，`.png` 不加载（详见 `贴图优化-DDS.md`）。
- **正确写法**：

  - 贴图宽高做成 4 的倍数，最好是 2 的幂。
  - 音频用 Ogg Vorbis，或普通 PCM WAV（格式标记 1），44.1 / 48 kHz。
  - 资源文件名只用 ASCII。
  - `Textures/` 下不放不被引用的文件。
- **把握**：已证实 5 例，较可能 1 例，未复现 5 例（GPU 不支持、文件被清零、线程加载等玩家环境问题）。

### 3.15 `xml.def-exception`：整个 Def 加载时抛异常

- **频率**：413 份日志（6%），314 个会话，1,806 簇。模板 `Exception loading def from file X.xml: <异常>`。最大的一组是 NewRatkin 的 `TraderKinds_Caravan_Ratkin.xml: TypeLoadException: … invalid vtable method slot`（113 份日志）。
- **后果**：这个 Def 丢失；有些情况下整个文件的 Def 都丢失。
- **作者做错了什么**：
  1. **旧 DLL 覆写了签名已改的抽象方法**（已证实）：旧版 `NewRatkin.dll` 覆写 `GenerateThings(int forTile, …)`，1.6 的抽象方法是 `GenerateThings(PlanetTile forTile, …)`，类型加载失败，整个商队 TraderKindDef 丢失。更深一层：这个种族有多份重传的副本，**DLL 同名、程序集版本号也相同**，游戏只加载其中一份，旧的那份可能胜出。当前各份 1.6 DLL 都已改用 `PlanetTile`，但同名副本的隐患还在。
  2. **XML 里有空的 `<li></li>`**（已证实，已修）：`<workDisables>` 里留了三个空项，空字符串不是合法的 `WorkTags` 值，整个背景故事 Def 丢失。
  3. 较可能：DLL 用 .NET 10 的引用程序集编译（见 §3.7），所有带 `List<T>` 字段的类型都加载失败，连带用到它们的 Def。
  4. 其他：枚举值拼错（`Midworld`、`ture`），框架的 XML 格式和框架版本对不上。
- **正确写法**：

  - 用当前游戏版本的 `Assembly-CSharp.dll` 重新编译；每次发布递增 `AssemblyVersion`。
  - 不要用相同的程序集名重传别人的 Mod。
  - 删掉空的 `<li></li>`；要表示“无”，删掉整个节点或写 `None`。
- **把握**：已证实 2 例，较可能 1 例，未复现 2 例（看起来是框架侧问题，或触发的补丁没找到）。

## 4. 发布前自查清单

按上面的数据，下面几项覆盖了绝大多数作者侧的加载错误：

1. **版本文件夹**：每个支持的版本有自己的文件夹和 `<v1.x>` LoadFolders 条目；不支持的版本不要写进 `supportedVersions`。根目录 `Assemblies/` 不留 DLL。（§2.1）
2. **用开发者模式启动一次，清空自己 Mod 的红字和黄字**：`Could not resolve cross-reference`、`doesn't correspond to any field`、`Could not find type`、`Config error`、`Patch operation … failed`。
3. **只装 Core 再启动一次**（不开任何 DLC 和可选 Mod）：所有 DLC / 可选 Mod 的引用和补丁都应该静默跳过。这一步能抓出 §3.1–§3.3 的大部分问题。
4. **DLC 声音在 Def 级别守卫**，不要在字段上挂 `MayRequire`。（§3.2）
5. **补丁**：可选目标用 `IfModActive` 条件文件夹、`PatchOperationFindMod`、Sequence 里 `<li>` 上的 `MayRequire`（顶层 `<Operation>` 上的不生效）或 `PatchOperationConditional`；xpath 用稳定的键，不用位置和会被别人改的值；FindMod 用对方的显示名，并守卫你实际补丁的那个 Mod。（§3.3、§3.11）
6. **Harmony**：按类打补丁并捕获异常；跨 Mod 目标用 `Prepare()`；`AccessTools.Method` 带参数类型并判空；Transpiler 失配时原样返回；发布版关掉 Harmony 调试。（§3.8、§3.12）
7. **C# 查 Def**：可选的用 `GetNamedSilentFail` 并判空，固定的用 `[DefOf]`（可选字段加 `[MayRequire]`），不在 Def 加载期间查。（§3.10）
8. **编译**：目标框架 `net472`，引用当前游戏版本的程序集；共享库不打包或只打包最新版。（§3.7）
9. **资源**：每个 texPath / clipPath 都有文件；贴图宽高是 4 的倍数；音频用 Ogg 或 PCM WAV；文件名只用 ASCII；自定义 shader 的非 mote Def 给 `uiIconPath`。（§3.9、§3.13、§3.14）
10. **框架或目标 Mod 大版本更新后**：搜一遍旧类名、对着新版重跑一遍补丁。（§2.3）

## 5. 尚未覆盖

以下范式频率不低，但原因要读具体的调用栈和代码，不能只看消息和 XML，v1 不写。频率来自 patterns-v0（日志份数，共 6,734）。

| 范式 | 阶段 | 日志 | 会话 | 簇 | 为什么没写 |
|---|---|---|---|---|---|
| `rt.nre` 裸 NullReferenceException | 运行时 | 2,502 | 1,929 | 27,838 | 消息不含信息，原因在栈里；需要按“最内层游戏方法 + 最内层 Mod 程序集”归组后读代码 |
| `rt.other` 其他运行时异常 | 运行时 | 2,148 | 1,653 | 14,245 | 同上 |
| `rt.post-load` 读档 / 长事件后异常 | 存档 | 1,905 | 1,489 | 7,517 | 多与存档里的旧数据有关 |
| `save.loadid` 存档引用的对象不存在 | 存档 | 1,704 | 1,324 | 20,718 | 多为玩家中途增删 Mod，不全是作者错误 |
| `rt.ui` 界面绘制异常 | 运行时 | 1,258 | 1,014 | 1,892 | 需读 UI 补丁 |
| `save.defref` 存档引用的 Def 不存在 | 存档 | 1,010 | 767 | 90,952 | 多为玩家移除 Mod 后读旧档 |
| `rt.tick` Tick 中异常 | 运行时 | 827 | 674 | 4,618 | 需读代码 |
| `save.other` | 存档 | 676 | 510 | 8,012 | |
| `save.thinknode` 思维树节点变化 | 存档 | 562 | 454 | 883 | |
| `rt.ai-job` 小人 AI / 工作异常 | 运行时 | 547 | 463 | 2,969 | 需读代码 |
| `save.not-deep-saved` | 存档 | 511 | 411 | 2,142 | 作者侧（保存引用但不保存对象），值得后续单独写 |
| `save.abstract-component` | 存档 | 438 | 350 | 819 | 多为移除 Mod |
| `rt.resolve-references` | 加载 | 398 | 326 | 5,815 | Def 的 ResolveReferences 抛异常，需读代码 |
| `asset.texture-create` 贴图参数非法 | 加载 | 389 | 305 | 1,957 | 多由贴图替换 / 缩放类 Mod 引起 |
| `rt.mapgen` 地图生成 | 运行时 | 338 | 275 | 437 | |
| `rt.translation` 翻译注入失败 | 加载 | 311 | 250 | 359 | 翻译 Mod 与被翻译 Mod 版本不匹配 |
| `rt.pawn-generation` 小人生成 | 运行时 | 293 | 231 | 390 | |
| `rt.collection-modified` 迭代中修改集合 | 运行时 | 229 | 189 | 244 | 典型作者错误（Tick 里增删 comps），值得后续写 |
| `rt.comp-init` 组件无法实例化 | 运行时 | 226 | 168 | 235 | |
| `asm.invalid-il` 非法 IL | 运行时 | 105 | 69 | 113 | 多个 Transpiler 冲突 |
| `xml.other` / `patch.other` 未分类 | 加载 | 1,543 / 513 | 1,243 / 409 | 27,885 / 1,468 | G5 规则未细分 |

另外，results-v4 的 `harmony-stack` 类（带 Harmony 补丁栈的异常，38,760 簇）基本落在上面的运行时范式里，同样需要读代码研究。

## 6. 方法与复现

- **范式与频率**：G5 `spikes/patterns/main.go` 的 46 条规则（分支 `claude/corpus-g345-f84a49`，结果 `RESULT.txt`、`patterns-v0.json`）。模板的日志数为本次对 `corpus-kb/results-v4.jsonl` 用同一套规则重新分类所得。
- **案例选取**：每条范式从 results-v4 中抽取恰好有一个来源 Mod、且该 Mod 在本地研究库（`D:\RimWorld16\Mods`、工坊 294100）里的簇，按模板频率和不同的子原因挑选；RimArcho 不归因的范式（`xml.type-missing`、`xml.config-error`、`asset.maintex` 等）改用“报错日志中出现率远高于全体的 Mod”定位，再到库里核对文件。复用了 G6 标注集 `corpus-kb/labels/v2/tasks.jsonl` 中已审核的案例（标注编号写在正文里）。
- **核对方式**：打开 Mod 实际在 1.6 上会加载的文件（按 LoadFolders 解析）；补丁用脚本对 1.6.4871 合并后的 Core/DLC Defs 和目标 Mod 逐条求值 xpath；DLL 用 ilspycmd 反编译，目标签名对照 1.6.4871 的 `Assembly-CSharp.dll`；必要时按月份对比“装了该 Mod 的日志”与“报错日志”。
- **局限**：
  - 本地库的 Mod 版本多数**比日志里玩家用的更新**，所以很多案例是“已修”，只能从旧版本文件夹或日志推断当时的文件，这类记为“较可能”。
  - `D:\RimWorld16\Mods` 里的副本不含 `Textures/` 和 `Sounds/`，资源存在性只能在工坊副本上检查；工坊副本里的 `.dds` 是在本机重新生成的，不能代表作者发布的文件。
  - 归因是 RimArcho v3/v4 的结果（来源准确率约 78%），`xref.getnamed` 这条尤其弱。
  - 语料是 1.6 玩家日志；1.7 的变化（如 Waylith 引擎）未覆盖。
- **核对记录**：本次的逐例记录（含文件路径和代码位置）在会话临时目录，未入库；需要时可按本节方法重做。各条的核对数见 §1 总览表。
