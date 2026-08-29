import {readFileSync} from "node:fs";
import {spawnSync} from "node:child_process";

const checker = spawnSync(
  process.execPath,
  [
    "node_modules/htmx.org/dist/scripts/upgrade-check.js",
    "--no-color",
    "internal/web",
    "browser",
    "docs/swap-scaling-bench.mjs",
  ],
  {encoding: "utf8"},
);

const output = `${checker.stdout || ""}${checker.stderr || ""}`;
const findings = output.split("\n").filter((line) =>
  /^.+:\d+: \[[^\]]+\]/.test(line));

function isMigratedDisable(line) {
  const match = /^(.+):(\d+): \[renamed-attr\] hx-disable /.exec(line);
  if (!match) return false;
  const lines = readFileSync(match[1], "utf8").split("\n");
  const tail = lines.slice(Number(match[2]) - 1, Number(match[2]) + 20).join("\n");
  const end = tail.indexOf(">");
  const openingTag = end >= 0 ? tail.slice(0, end + 1) : tail;
  return /\bhx-disable="[^"]+"/.test(openingTag);
}

function isPartialRouting(line) {
  const match = /^(.+):(\d+): \[inheritance\] hx-(target|swap) /.exec(line);
  if (!match) return false;
  const lines = readFileSync(match[1], "utf8").split("\n");
  return /<hx-partial\b/.test(lines[Number(match[2]) - 1]);
}

const unexpected = findings.filter((line) =>
  !isMigratedDisable(line) && !isPartialRouting(line));
if (unexpected.length || (checker.status !== 0 && findings.length === 0)) {
  process.stderr.write(output);
  process.exit(checker.status || 1);
}

if (findings.length) {
  process.stdout.write(
    `upgrade-check: ${findings.length} source-verified HTMX 4 false positives ignored\n`,
  );
} else {
  process.stdout.write("upgrade-check: no HTMX 2 migration findings\n");
}
