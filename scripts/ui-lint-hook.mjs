// Claude Code PostToolUse hook (.claude/settings.json): after an Edit or Write to the GUI's
// assets or to the UI lint itself, run the UI lint (16 §7.3 "Checks") in the checkout that
// holds the file, worktrees included. A failure exits 2, which hands the problems, each
// naming its specification section, back to the agent. Other files pass straight through.
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

const UI = /[\\/]internal[\\/]surface[\\/]gui[\\/](assets[\\/]|ui_rules_test\.go$|style_rules_test\.go$)/;

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8"));
} catch {
  process.exit(0);
}
const file = input?.tool_input?.file_path || "";
if (!UI.test(file)) process.exit(0);

let root = path.dirname(path.resolve(file));
while (!existsSync(path.join(root, "go.mod"))) {
  const up = path.dirname(root);
  if (up === root) process.exit(0);
  root = up;
}
const r = spawnSync("go", ["test", "-count=1", "-run", "TestStyleRules|TestUIRules|TestEndlessAnimations", "./internal/surface/gui/"], { cwd: root, encoding: "utf8" });
if (r.error) process.exit(0); // no Go here: nothing to report
if (r.status !== 0) {
  const lines = (r.stdout + r.stderr).split("\n").filter((l) => /breaks|ui_rules_test|style_rules_test|assets\//.test(l));
  process.stderr.write("UI lint failed (docs/specs/16-interface/7-visual-design.md §7.3 Checks). Fix these, or add a reasoned exception in ui_rules_test.go only if the spec allows it:\n" + (lines.length ? lines : [r.stdout + r.stderr]).join("\n") + "\n");
  process.exit(2);
}
