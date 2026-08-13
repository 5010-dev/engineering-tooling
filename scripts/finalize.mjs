import { chmod, cp, mkdir } from "node:fs/promises";

const packageRoot = new URL("../", import.meta.url);
const outputRoot = new URL("lib/skills/golden-path/", packageRoot);

await mkdir(outputRoot, { recursive: true });
await cp(new URL("skills/golden-path/", packageRoot), outputRoot, {
  recursive: true,
});
await chmod(new URL("lib/cli.js", packageRoot), 0o755);
