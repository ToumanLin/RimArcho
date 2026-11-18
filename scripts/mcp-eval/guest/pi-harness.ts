// Group Q: a Pi extension with harness changes taken from Alma (docs/research/mcp-eval-design.md, Round 6).
//   pi --extension C:\Eval\pi-harness.ts ...
// 1. bash gets a default timeout: Pi's has none, and an unbounded `find /` used up a whole h4 run (h4-P-3).
//    Alma gives every foreground command 120 s unless the model asks for more.
// 2. A prompt section with the run rules Alma's prompt has and Pi's lacks: save the answer early, check it
//    against the task's own terms, and how Git Bash and Windows programs meet (h8-P-3 passed /c/... paths to csc).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const DEFAULT_TIMEOUT_S = 120;

const RULES = [
	`- A bash command without a \`timeout\` stops after ${DEFAULT_TIMEOUT_S} s. Pass \`timeout\` in seconds when a command needs longer (a build, a restore). Search the folders the task names, not whole drives.`,
	"- The shell is Git Bash on Windows. Windows programs (dotnet, csc, ilspycmd, .exe tools) need Windows paths: write C:\\... or convert with `cygpath -w`. Discard output with `>/dev/null`, never `>nul`.",
	"- When the task names a place for the result, put your current best answer there as soon as you have one, then improve it in place: the run can end without warning.",
	"- Before you finish, check the result against the task's own criteria (named paths, files that must exist, what must be handed in), not against your summary of them.",
].join("\n");

export default function (pi: ExtensionAPI) {
	pi.on("tool_call", (event) => {
		if (event.toolName !== "bash") return;
		const input = event.input as { timeout?: number };
		if (input.timeout === undefined) input.timeout = DEFAULT_TIMEOUT_S;
	});
	pi.on("before_agent_start", (event) => {
		event.systemPromptOptions.sections.run_rules = RULES;
	});
}
