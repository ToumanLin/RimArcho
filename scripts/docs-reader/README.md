# 离线文档阅读器

双击 `.cache/docs-reader/rimarcho-docs-reader.html` 即可阅读。页面内置生成时的全部 Markdown 文档，无需启动服务器、联网或安装浏览器扩展。

生成的整库快照放在已被 Git 忽略的 `.cache/` 目录，不放入 `docs/`，也不命名为 `index.html`，以减少其他 agent 搜索和读取源文档时将其载入上下文的机会。原始 Markdown 是文档的唯一维护来源。

支持全文搜索与正文高亮、章节目录、文档间相对链接、浏览器前进后退、源码查看、字号调整、深浅色主题、打印和窄屏布局。上次阅读的文档、主题和字号会在浏览器允许本地存储时保存。

## 更新文档快照

在 RimArcho 仓库根目录运行：

```powershell
node scripts/docs-reader/build.cjs
```

生成器优先使用已安装的 `markdown-it`，也能使用当前工作区已有的 `../RimModManager/frontend/node_modules/markdown-it`。在其他环境可指定模块路径：

```powershell
node scripts/docs-reader/build.cjs "C:/path/to/node_modules/markdown-it"
```

只重新生成 `.cache/docs-reader/rimarcho-docs-reader.html`，不会修改原始文档。修改页面样式和交互请编辑 `template.html` 后重新生成。

文档图片和指向代码、界面草图等文件的链接仍引用仓库原始文件；保留仓库目录结构即可访问。HTML 中的文字、表格和代码块可以单独离线阅读。Markdown 原始 HTML 按文本显示，避免文档中的标签和脚本被执行。
