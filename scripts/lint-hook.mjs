// Claude Code PostToolUse hook (.claude/settings.json): after an Edit or Write, run the static
// checks that cover the edited file (17 §4 "Static checks") in the checkout that holds it,
// worktrees included. A failure exits 2, which hands the problems, each naming its
// specification section, back to the agent. Files no check covers pass straight through.
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

// Most specific first; each entry: which files, which tests, which packages.
const CHECKS = [
  { files: /[\\/]internal[\\/]surface[\\/]gui[\\/](assets[\\/]|ui_\w+_test\.go$|style_rules_test\.go$)/,
    run: "TestStyleRules|TestUI|TestEndlessAnimations", pkgs: ["./internal/surface/gui/"] },
  { files: /[\\/](docs[\\/]specs[\\/].*\.md|docs[\\/]00a-architecture\.md|AGENTS\.md|CHANGELOG\.md|go\.mod)$/,
    run: "TestArch|TestSpec", pkgs: ["./internal/archlint/", "./internal/speccheck/", "./internal/store/", "./internal/cmds/", "./internal/surface/mcp/", "./internal/harmony/", "./internal/app/"] },
  { files: /[\\/](internal|cmd)[\\/].*\.go$/,
    run: "TestArch|TestSpec", pkgs: ["./internal/archlint/", "./internal/speccheck/"] },
];

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8"));
} catch {
  process.exit(0);
}
const file = input?.tool_input?.file_path || "";
const check = CHECKS.find((c) => c.files.test(file));
if (!check) process.exit(0);

let root = path.dirname(path.resolve(file));
while (!existsSync(path.join(root, "go.mod"))) {
  const up = path.dirname(root);
  if (up === root) process.exit(0);
  root = up;
}
const r = spawnSync("go", ["test", "-count=1", "-run", check.run, ...check.pkgs], { cwd: root, encoding: "utf8" });
if (r.error) process.exit(0); // no Go here: nothing to report
if (r.status !== 0) {
  const out = r.stdout + r.stderr;
  const lines = out.split("\n").filter((l) => /§|breaks|stale|exception|\.go:\d+:/.test(l) && !/^(ok|FAIL)\s/.test(l));
  process.stderr.write("Static checks failed (docs/specs/17-engineering.md §4 \"Static checks\"). Each line names the spec section to read. Fix the code or the spec; add a reasoned exception only if the spec allows it:\n" + (lines.length ? lines.join("\n") : out) + "\n");
  process.exit(2);
}
