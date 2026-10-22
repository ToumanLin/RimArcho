# 贴图优化：PNG 转 DDS

2026-10-22 研究。依据：反编译本机 1.7.4985（Waylith 引擎）和 1.6.4871 的 `Assembly-CSharp.dll` / `Waylith.dll` / `RimWorld.dll`，RimCrow（`../RimModManager`）的 `backend/managers/mgr_texture_opt.py` 和 `doc/07-贴图优化说明.md`，RimSort 自带的 todds 0.4.1，以及对本机工坊目录的统计和一次实际转换。

## 1. 结论

1. **游戏原生支持，不需要前置 Mod。** 1.6 和 1.7 都会读 Mod 里的 `.dds`，同名 `.dds` 存在时不加载 `.png`；只有 `.dds` 没有 `.png` 也能加载。所以“普通优化（保留 PNG）”和“整合包精简（删掉 PNG）”都可行。
2. **工具用 todds 0.4.1。** 它是 RimSort、RimCrow 共用的编码器，MPL-2.0，但仓库已在 2026-03-31 归档，0.4.1 是最后一版。按固定版本 + SHA-256 处理，问题不大。
3. **DDS 比 PNG 大得多。** 实测原尺寸 9.5 倍，缩到 50% 仍是 2.5 倍（§4）。整合包精简删掉 PNG 后，**不缩放时包会变大而不是变小**。只有缩放才能真正省盘、省显存。
4. **1.7 收益比 1.6 小。** 1.7 自己会把 PNG 编码成 BC1/BC7 并写进持久缓存，第二次启动起就不再解码。DDS 在 1.7 上主要省掉三种情况下的那次编码：第一次启动、Mod 更新之后、**每个整合包的第一次启动**（整合包用 `-savedatafolder`，缓存跟着进了包自己的 `userdata/`，是空的）。
5. **在原 Mod 里生成 DDS 有两个风险必须处理**：Mod 更新后旧 DDS 盖住新 PNG（玩家看到旧贴图），以及低优先级目录里的 DDS 盖住高优先级目录里的 PNG（§2.3）。RimArcho 要自己记录生成了哪些文件，不能靠 todds 的 mtime 比较。

## 2. 游戏怎么读贴图

### 2.1 扩展名与遮蔽（1.6、1.7 相同）

`Verse.ModContentLoader<Texture2D>`：

- 可接受的扩展名：`.png .jpg .jpeg .psd .dds`。
- `ShadowedByDds`：同一个 Mod 里，相对路径相同的 `x.dds` 存在时，`x.png` / `x.jpg` / `x.psd` 不加载。比较不分大小写。
  - 小缺陷：代码去掉最后 4 个字符再拼 `.dds`，所以 `.jpeg` 永远不会被遮蔽（`x.jpeg` → `x..dds`）。
- `.dds` 本身是一等公民，单独存在也照常加载。

### 2.2 DDS 解码（`Verse.ModDdsLoader` / `DdsPixelFormat`）

- 头部校验：魔数 `DDS `、头大小 124、像素格式大小 32，不对就抛异常，贴图变成 `BadTex`（1.7 是品红占位）。
- 支持的格式：DXT1（BC1）、DXT5（BC3）、BC7（DX10 扩展头，数据从 148 字节开始）、未压缩 RGB24 / RGBA32 / BGR888 / RGB565 / ARGB4444 / Alpha8。其他压缩格式（BC4、BC5、BC6H 等）抛 `NotSupportedException`。
- 带 mipmap 就用文件里的 mip；不带时调用 `Apply(updateMipmaps: true)`。所以**要生成 mipmap**（todds 默认就生成）。
- 用 `LoadRawTextureData` 直接上传，原点在左下角，所以要**垂直翻转**（todds `-vf`，RimCrow 也这么传）。
- DDS 的 `anisoLevel` 是 0，PNG 是 2，远看时 DDS 略糊一点，可以忽略。

### 2.3 加载目录之间的遮蔽（容易踩的坑）

`ModContentPack.GetAllFilesForMod` 按加载目录的优先级（高到低）收集文件，键是“相对于加载目录的路径”，先到先得。遮蔽检查在这个合并后的字典上做，所以：

- `1.6/Textures/a.png`（高优先级）和 `Textures/a.dds`（公共目录，低优先级）同时存在时，`a.dds` 会遮蔽 `a.png`，游戏显示的是公共目录那张旧图。
- 只要每个加载目录里的每张 PNG 都有自己的 DDS，就不会出错（高优先级目录的 `a.dds` 会赢过低优先级的 `a.dds`）。
- 规则：**某个相对路径只要有一张 PNG 没转成功，就不能让任何低优先级目录里留着这个路径的、由我们生成的 DDS**。转换失败时要回退整组，而不是只跳过那一张。

### 2.4 1.7 的贴图缓存（Waylith）

- `Waylith.Rendering.GpuTextureService`：PNG 解码后，引擎自己编码成 BC1/BC7（CPU 或 GPU），写进 `<数据目录>/TextureCache/v23/<设置标签>/`，LZ4 压缩。键是完整路径 + 文件大小 + 修改时间。
- 本机缓存 209 MB。
- `RimWorldWaylith.Bootstrap.ResolveCacheRootOverride`：带 `-savedatafolder` 启动时，缓存根目录就是那个文件夹。**所以每个整合包都有自己的空缓存**，第一次启动要把所有 PNG 重新编码一遍。
- DDS 加载后也会被写进同一个缓存（`PersistToTextureCache`），多占一份盘。

### 2.5 本机现状

| 项 | 数量 |
| --- | --- |
| 有 PNG 贴图的工坊 Mod | 827 |
| PNG 贴图 | 132,847 张，4.24 GiB |
| 估算全部转成 DDS（BC1/BC7 + mipmap，不缩放） | 约 26.7 GiB（6.3 倍） |
| 已经有 `.dds` 的 Mod | 292 个，32,448 个文件，5.68 GiB |

已有的 DDS 一部分是作者自己发布的（例如 `[NL] Facial Animation`：PNG 1.2 MB，DDS 51.7 MB），一部分是以前用 RimSort / RimCrow / RimPy 生成的。RimArcho 分不出来，所以**默认不碰别人的 DDS**。

## 3. 工具：todds

| 项 | 内容 |
| --- | --- |
| 版本 | 0.4.1（最后一版），仓库 `todds-encoder/todds`，2026-03-31 归档 |
| 许可 | MPL-2.0，可以随包分发，但要附带许可和源码地址 |
| 获取 | RimCrow：GitHub release 的 `todds_Windows_0.4.1.zip`；RimSort：随安装包附带 `todds/todds.exe` |
| 本机 | `D:\SteamLibrary\steamapps\common\RimWorld\RimSort\todds\todds.exe`，`--help` 显示 0.4.1 |

**RimCrow 的参数**（RimArcho 照用）：

```
todds -f BC1 -af BC7 -vf -fs -o|-on -t -p [-nm] [-sc <百分比> | -ms <最大边>] <文件列表.txt>
```

- `-f BC1 -af BC7`：不透明图用 BC1（每像素 0.5 字节），有透明通道的用 BC7（1 字节）。
- `-fs`：宽高补到 4 的倍数（BC 格式要求按 4×4 分块）。
- 输入用 txt 文件列表，方便只转选中的文件。txt 输入时 todds 不支持另指输出目录，DDS 一定写在 PNG 旁边。
- `-on` 按 mtime 判断“DDS 比 PNG 旧才重转”。工坊更新可能保留旧的 mtime，不可靠，RimArcho 用自己的清单判断（§5.1）。

**RimCrow 的其他规则**（值得照搬）：

- 跳过任何叫 `Source` 的目录（作者的工程素材）。
- 扩展名是 `.png` 但内容不是 PNG 的跳过。
- `_m.png`（遮罩）转换但不缩放。
- 缩放有最小边保护（默认 128）：缩完会小于这个值的图，退回更大的比例或保留原尺寸。
- 分批调用，每批最多 10,000 个文件；单批超时 600 秒。

**来源怎么定（待决定）：**

- A. 第一次使用时，经玩家同意，从 GitHub release 下载 0.4.1，用写死的 SHA-256 校验，放到 `%APPDATA%\RimArcho\tools\todds\0.4.1\`。仓库已归档，地址不会再变。
- B. 直接放进 RimArcho 的发布 zip（多约 10 MB，附 MPL 许可）。省掉下载这一步。
- 两种都允许玩家在设置里指定已有的 `todds.exe`（例如 RimSort 自带的那份），用 `--help` 第一行确认版本。

推荐 A，规格原来就要求“玩家同意后下载”。

## 4. 实测

`EGI Holograms and Projectors`（工坊 2979598490）的 `Common/Textures` 复制到临时目录后转换，32 线程：

| 方案 | 大小 | 相对 PNG | 耗时 |
| --- | --- | --- | --- |
| PNG 原图（2,521 张） | 41 MB | 1× | — |
| DDS 原尺寸 | 389 MB | 9.5× | 10.6 秒 |
| DDS 缩到 50% | 102 MB | 2.5× | 4.5 秒 |

按这个速度，本机全部 13 万张 PNG 原尺寸转一遍大约 10 分钟，要 20 多 GiB 盘。

显存估算（RimCrow 的公式）：BC1 = 宽×高×0.5，BC7 = 宽×高×1，有 mipmap 再乘 1.333。缩到 50% 显存约降到 1/4。

## 5. 设计

### 5.1 普通优化（原 Mod 里生成，保留 PNG）

**命令：**

- `archo textures status [<Mod>…]`：每个 Mod 的 PNG 数和大小、已生成的 DDS、过期的 DDS、别人的 DDS、预计大小。
- `archo textures optimize <Mod…> | --plan | --all [--scale <百分比>] [--max-size <像素>] [--no-mipmaps] [--dry-run]`
- `archo textures clean <Mod…> | --all [--dry-run]`：只删清单里的文件。

**写入范围。** 这是 README “从不写 Mod 目录”的第三个例外（前两个是 `save` 和 `copy-local`），只在玩家明确执行时发生：

- 只在 PNG 旁边新建 `.dds`，不改、不删 PNG。
- 已有、但不在清单里的 `.dds` 一律不覆盖，列为“别人的 DDS”。
- 游戏运行时拒绝（文件可能被占用，和 `save` 一样）。

**清单。** 数据库新表，每个生成的文件一行：Instance、相对路径、源 PNG 的大小 + mtime + xxh3、DDS 大小、参数（格式、比例、mipmap）、todds 版本、生成时间。`clean` 和过期检查都靠它。

**过期检查。** 每次扫描后对比清单：

| 情况 | 问题 | 级别 | 动作 |
| --- | --- | --- | --- |
| 源 PNG 变了（大小或哈希不同） | `texture.stale` | warning（游戏会显示旧贴图） | 重新优化 / 清理 |
| 源 PNG 没了 | `texture.orphan` | info（DDS 还在被加载） | 清理 |
| DDS 被删了或被改了 | 从清单移除 | — | — |

**遮蔽规则（§2.3）。** 按 Mod 生效的全部加载目录收集 PNG，不只是 `Textures/` 根目录。某个相对路径只要有一张 PNG 被跳过或转换失败，就删掉这个路径下所有由我们生成、优先级更低的 DDS。

**扫描指纹。** 结构清单现在包含所有文件（02 §4.4），新增的 DDS 会让快照哈希变化，Mod 被当成“更新了”。要把清单里的 DDS 排除在结构层之外。作者自己的 DDS 照常计入。

**工坊更新会不会删掉我们的 DDS：** 不确定，要实测（§6）。不管会不会删，过期检查都能兜住。

### 5.2 整合包精简（包里的副本，删掉 PNG）

在 08 §5.2 的“清洗”和“测量”之间加一步，作用于 `.archo-tmp` 里的副本：

1. 对副本里生效加载目录下的 PNG 跑 todds（参数同上，可选缩放）。
2. 转换成功的 PNG 删掉；失败的保留 PNG，并按 §2.3 回退同一路径下优先级更低的 DDS。
3. `pack.json` 记录 `textures`：工具与版本、参数、转换数、失败数、PNG 字节、DDS 字节。

注意事项：

- **测量要用转换后的大小**。不缩放时包会变成原来的几倍，剩余空间检查要按估算的 DDS 大小算。界面在编译前就显示“PNG x MB → DDS 约 y MB”。
- **默认不缩放**（2026-10-22 决定）。原尺寸转换不省盘，只省加载时间（1.7 上是每个包第一次启动的编码时间），所以不缩放时界面叫它**“预转换”**，选了缩放比例才叫“精简”。要省盘、省显存，得由玩家自己选缩放。
- 只处理 `Textures/`。`About/Preview.png` 等不是通过贴图加载器读的，不碰。
- 语义哈希不含贴图，所以包条目的 `hash` 和轻包的匹配都不受影响。
- 副本的 `origin.json` 加一个标记，说明贴图被精简过，“查看差异”时不把它当成 Mod 被改了。

### 5.3 界面

- Mod 库存页：批量操作“优化贴图 / 清理”，详情里显示贴图统计和过期提示。
- 方案页的“编译成整合包”子页：一个贴图开关、缩放比例（默认“不缩放”），加上编译前的大小估算。不缩放时开关叫“预转换贴图”，选了比例后叫“精简贴图”。
- 第一次使用时的 todds 下载确认（文件名、来源、大小）。

## 6. 动手前要实测

| 项 | 方法 |
| --- | --- |
| 1.6 和 1.7 都能正确显示 todds 生成的 DDS（方向、透明、BC1/BC7、非 4 倍数尺寸） | 一个小 Mod 转换后进游戏看，留截图 |
| 只有 DDS、没有 PNG 时能加载（精简的前提） | 同上，删掉 PNG |
| 工坊更新会不会删掉 Mod 目录里多出来的文件 | 给一个会更新的 Mod 加个标记文件，等它更新 |
| 加载时间：1.6 普通启动、1.7 冷缓存 / 热缓存、整合包第一次启动 | 同一列表各测 3 次，看 Player.log 时间戳 |
| 本机 292 个已有 DDS 的 Mod 里，有多少 DDS 比 PNG 旧（以前的工具留下的过期文件） | 只读统计 |

## 7. 待决定

1. todds 的来源：同意后下载（推荐）还是放进发布 zip。
2. ~~整合包精简的默认缩放比例~~ 已定（2026-10-22）：默认不缩放；不缩放时界面叫“预转换”，选了缩放比例才叫“精简”（§5.2）。
3. 普通优化要不要接管以前的工具留下的 DDS（例如 DDS 比 PNG 旧时提示清理）。默认只读、只提示。
4. 要不要支持 RimCrow 的 `.dds.zstd`（需要 Image Opt 这个 Mod）。建议不做：游戏不能原生读取。
