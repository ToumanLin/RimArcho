# Offline documentation reader

Double-click `.cache/docs-reader/rimarcho-docs-reader.html` to read. The page embeds all Markdown documents as of generation time; no server, network connection or browser extension is needed.

The generated repository snapshot lives in the Git-ignored `.cache/` directory, not in `docs/` or a file named `index.html`, to reduce the chance that other agents load it into context while searching or reading source documents. Original Markdown is the only maintained documentation source.

Supports full-text search and body highlighting, a section table of contents, relative links between documents, browser back/forward, source viewing, font-size adjustment, light/dark themes, printing and narrow layouts. The last document, theme and font size are saved when the browser allows local storage.

## Update the documentation snapshot

Run from the RimArcho repository root:

```powershell
node scripts/docs-reader/build.cjs
```

The generator prefers an installed `markdown-it` and can also use the existing `../RimModManager/frontend/node_modules/markdown-it` in this workspace. Elsewhere, specify the module path:

```powershell
node scripts/docs-reader/build.cjs "C:/path/to/node_modules/markdown-it"
```

Only `.cache/docs-reader/rimarcho-docs-reader.html` is regenerated; source documents are unchanged. To change page styles and interactions, edit `template.html` and regenerate.

Document images and links to code, UI drafts and other files still reference the original repository files; keep the repository directory structure to access them. Text, tables and code blocks in the HTML can be read offline on their own. Raw HTML in Markdown is displayed as text to prevent document tags and scripts from executing.