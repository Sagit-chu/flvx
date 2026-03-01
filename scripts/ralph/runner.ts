#!/usr/bin/env bun

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const promptPath = join(scriptDir, "prompt.md");

const { values } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    "max-iterations": { type: "string", default: "50" },
    prompt: { type: "string" },
  },
});

async function main() {
  const baselinePrompt = await Bun.file(promptPath).text();
  const mergedPrompt = values.prompt
    ? `${values.prompt}\n\n---\n\n${baselinePrompt}`
    : baselinePrompt;

  const maxIterations = values["max-iterations"] || "50";
  const escapedPrompt = mergedPrompt.replace(/'/g, "'\\''");
  const command = `/ralph-loop:ralph-loop '${escapedPrompt}' --completion-promise "FINISHED" --max-iterations ${maxIterations}`;

  console.log("[ralph] starting loop");
  console.log(`[ralph] max iterations: ${maxIterations}`);

  const proc = Bun.spawn([
    "sh",
    "-c",
    `claude --permission-mode bypassPermissions --verbose '${command.replace(/'/g, "'\\''")}'`,
  ], {
    stdin: "inherit",
    stdout: "inherit",
    stderr: "inherit",
  });

  const exitCode = await proc.exited;

  if (exitCode === 0) {
    console.log("[ralph] completed successfully");
  } else {
    console.log(`[ralph] exited with code ${exitCode}`);
  }

  process.exit(exitCode);
}

main().catch((error) => {
  console.error("[ralph] error:", error);
  process.exit(1);
});
