// Light colouring for read-only XML and C#; every token remains text, never HTML.
export function syntaxParts(text, language) {
  const pattern =
    language === "cs"
      ? /(\/\/[^\n]*|\/\*.*?\*\/)|("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|\b(using|namespace|class|struct|interface|public|private|protected|internal|static|virtual|override|return|new|if|else|for|foreach|while|void|bool|int|string|var|null|true|false|this|base|throw|try|catch|async|await)\b|\b(\d+(?:\.\d+)?)\b/g
      : /(<!--.*?-->)|("[^"]*"|'[^']*')|(<\/?[\w:.-]+|\/?>)|\b([\w:.-]+)(?=\s*=)/g;
  const out = [];
  let at = 0;
  for (const match of text.matchAll(pattern)) {
    if (match.index > at) {
      out.push({
        text: text.slice(at, match.index),
        cls: ""
      });
    }
    const cls = match[1]
      ? "comment"
      : match[2]
        ? "string"
        : match[3]
          ? language === "cs"
            ? "keyword"
            : "tag"
          : language === "cs"
            ? "number"
            : "attribute";
    out.push({
      text: match[0],
      cls: "fs-syntax-" + cls
    });
    at = match.index + match[0].length;
  }
  if (at < text.length) {
    out.push({
      text: text.slice(at),
      cls: ""
    });
  }
  return out;
}
