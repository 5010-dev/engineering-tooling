import { rm } from "node:fs/promises";

const packageRoot = new URL("../", import.meta.url);
const generatedEntries = [
  "authority",
  "cli",
  "constants",
  "hashing",
  "index",
  "metadata",
  "plan",
  "process",
  "report",
  "repository",
  "skill",
];

for (const entry of generatedEntries) {
  for (const suffix of [".d.ts", ".d.ts.map", ".js", ".js.map"]) {
    await rm(new URL(`lib/${entry}${suffix}`, packageRoot), { force: true });
  }
}
await rm(new URL("lib/skills/golden-path/", packageRoot), {
  force: true,
  recursive: true,
});
