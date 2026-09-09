// Checks that the pages the editor opens with render cleanly.
//
// A starter is the first thing anyone sees the renderer do, and a starter that
// warns teaches the warning. Every one of them has to lay out with no lost
// declaration and no missing glyph — held to the same standard the examples
// are, because that is what they are: examples someone will edit first.
//
// Usage: node web/verify/starters.mjs
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const staticDir = path.join(root, "web", "static");

const require = createRequire(import.meta.url);
require(path.join(staticDir, "wasm_exec.js"));

// Lifted from the shipped script for the same reason parity lifts nothing: a
// second copy of the starters here could pass while the ones on the page fail.
const source = fs.readFileSync(path.join(staticDir, "app.js"), "utf8");
const STARTERS = new Function(
  `${source.slice(source.indexOf("const STARTERS"), source.indexOf("function loadStarters"))}
   return STARTERS;`,
)();

const go = new Go();
const module = await WebAssembly.instantiate(
  fs.readFileSync(path.join(staticDir, "inkwire.wasm")),
  go.importObject,
);
const ready = new Promise((resolve) => {
  globalThis.inkwireReady = resolve;
});
go.run(module.instance);
await ready;

// The size the starters are written for. They declare it in their own CSS too;
// this is the panel they are meant to be read on.
const WIDTH = 296;
const HEIGHT = 128;

let problems = 0;
for (const [name, starter] of Object.entries(STARTERS)) {
  const result = globalThis.inkwire.render({
    markup: starter.markup,
    css: starter.css,
    width: WIDTH,
    height: HEIGHT,
  });
  const warnings = result.warnings ?? [];
  const missing = result.missingRunes ?? [];
  const clean = result.ok && warnings.length === 0 && missing.length === 0;
  if (!clean) problems++;

  const size = result.png ? `${Buffer.from(result.png, "base64").length}B` : "no picture";
  console.log(`  ${clean ? "ok  " : "FAIL"}  ${name.padEnd(16)} ${result.ok ? size : result.error}`);
  for (const warning of warnings) console.log(`          [${warning.code}] ${warning.message}`);
  for (const rune of missing) console.log(`          no glyph for ${JSON.stringify(rune)}`);
}

const total = Object.keys(STARTERS).length;
console.log(`\n${total} starters, ${problems} not clean`);
process.exit(problems === 0 ? 0 : 1);
