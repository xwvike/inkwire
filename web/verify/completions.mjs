// Checks that every completion the editor offers actually renders.
//
// This is the whole reason the vocabulary is generated from MARKUP.md instead
// of taken from a stock CSS list: a completion that proposes a declaration the
// renderer does not implement teaches the wrong vocabulary at exactly the
// moment someone is learning it. So every property, and every value offered
// for it, is put through the renderer here — and none of them may come back
// with a warning.
//
// Usage: node web/verify/completions.mjs
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const staticDir = path.join(root, "web", "static");

const require = createRequire(import.meta.url);
require(path.join(staticDir, "wasm_exec.js"));

const vocabulary = JSON.parse(fs.readFileSync(path.join(staticDir, "completions.json"), "utf8"));

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

// A declaration is only meaningful where it applies: a flex item property on a
// block box is a warning about the box, not about the property. Each category
// is therefore tried in a context that gives it something to do, which is the
// same context the editor's user would have written it in.
const CONTEXTS = {
  Flex: (rule) => [`<div class="host"><div class="probe">x</div></div>`, `.host { display: flex } ${rule}`],
  Grid: (rule) => [
    `<div class="host"><div class="probe">x</div></div>`,
    `.host { display: grid; grid-template-columns: 1fr 1fr; grid-template-rows: 1fr 1fr } ${rule}`,
  ],
  Alignment: (rule) => [`<div class="host"><div class="probe">x</div></div>`, `.host { display: flex } ${rule}`],
  Gap: (rule) => [
    `<div class="probe"><div>a</div><div>b</div></div>`,
    `.probe { display: flex } ${rule}`,
  ],
  // Both paints are set as attributes so that overriding either one from CSS
  // still leaves the shape something to draw with. A rect with neither is
  // reported, correctly, and that report is about the rect, not the property.
  "SVG paint": (rule) => [
    `<svg width="20" height="20"><rect class="probe" x="1" y="1" width="10" height="10"` +
      ` fill="black" stroke="black"/></svg>`,
    rule,
  ],
  Text: (rule, property) =>
    property === "vertical-align"
      ? [`<div>text <span class="probe">x</span></div>`, rule]
      : [`<div class="probe">text</div>`, rule],
  Position: (rule) => [
    `<div class="host"><div class="probe">x</div></div>`,
    `.host { position: relative; width: 60px; height: 40px } ${rule}`,
  ],
  Image: (rule) => [
    `<div class="host"><svg class="probe" width="20" height="20"><rect width="9" height="9"/></svg></div>`,
    `.host { width: 40px; height: 40px } ${rule}`,
  ],
};

// Some declarations are meant to remove the box they are on — display: none,
// visibility: hidden — and a page whose only box is gone is reported as having
// nothing to draw. The anchor is there so the page always has something else,
// leaving the report to be about the declaration.
const ANCHOR = `<div style="width:2px;height:2px;background:black"></div>`;

// The page's first element is its root, so the probe is never allowed to be
// that element: display: none on the root leaves the page with nothing to
// draw, which is a fact about roots rather than about the declaration.
const wrap = (property, declaration) => {
  const rule = `.probe { ${declaration} }`;
  const context = CONTEXTS[categoryOf(property)];
  const [markup, css] = context
    ? context(rule, property)
    : [`<div class="probe">x</div>`, rule];
  return [`<div>${markup}${ANCHOR}</div>`, css];
};

const byName = new Map(vocabulary.properties.map((p) => [p.name, p]));
const categoryOf = (name) => byName.get(name)?.category ?? "";

// Properties with no keyword values still have to be shown to work, and each
// needs a value of its own kind — a category is too coarse, since "Border"
// covers both `border: 1px solid black` and `border-left-width: 1px`. First
// pattern to match wins.
const PROBE_VALUE = [
  [/^(border|border-(top|right|bottom|left))$/, "1px solid black"],
  [/-(width)$|^border-width$/, "1px"],
  [/-(style)$|^border-style$/, "solid"],
  [/-(color)$|^(color|background|background-color|border-color|fill|stroke)$/, "black"],
  [/^border-radius$/, "2px"],
  [/^(width|height|min-|max-|flex-basis)/, "10px"],
  [/^(padding|margin|gap|row-gap|column-gap|inset|top|right|bottom|left)/, "4px"],
  [/^z-index$/, "1"],
  [/^aspect-ratio$/, "2"],
  [/^font$/, "16px ui"],
  [/^font-size$/, "16px"],
  [/^font-family$/, "ui"],
  [/^line-height$/, "16px"],
  [/^rotate$/, "37deg"],
  [/^scale$/, "2"],
  [/^transform$/, "rotate(90deg)"],
  [/^transform-origin$/, "left top"],
  [/^stroke-width$/, "2px"],
  [/^stroke-dasharray$/, "2 2"],
  [/^stroke-dashoffset$/, "1"],
  [/^clip-path$/, "inset(1px)"],
  [/^grid-template-(columns|rows)$/, "1fr 1fr"],
  [/^grid-(column|row)$/, "span 1"],
  [/^flex-(grow|shrink)$/, "1"],
];

const probeValue = (name) => PROBE_VALUE.find(([pattern]) => pattern.test(name))?.[1];

// Geometry, so an element is asked to draw something. Without it the renderer
// reports a shape that draws nothing, which is true and not what is under test.
const SVG_GEOMETRY = {
  rect: ' x="1" y="1" width="8" height="8"',
  circle: ' cx="6" cy="6" r="4"',
  ellipse: ' cx="6" cy="6" rx="5" ry="3"',
  line: ' x1="1" y1="1" x2="9" y2="9" stroke="black"',
  polyline: ' points="1,1 5,8 9,2" fill="none" stroke="black"',
  polygon: ' points="1,1 9,1 5,8"',
  path: ' d="M1 1 L9 9"  fill="none" stroke="black"',
  use: ' href="#anchor"',
};

let checks = 0;
const failures = [];

const run = (property, declaration, label) => {
  const [markup, css] = wrap(property, declaration);
  const result = globalThis.inkwire.render(markup, css, 120, 60, {});
  checks++;
  const warnings = (result.warnings ?? []).filter(
    (w) => w.code === "unsupported-declaration" || w.code === "unsupported-selector",
  );
  if (!result.ok || warnings.length > 0) {
    failures.push(
      `${label}  ${warnings.map((w) => `[${w.code}] ${w.message}`).join("; ") || result.error}`,
    );
  }
};

for (const property of vocabulary.properties) {
  // Every keyword the editor would offer for this property.
  for (const value of property.values ?? []) {
    run(property.name, `${property.name}: ${value}`, `${property.name}: ${value}`);
  }
  // And, for the properties that take no keywords, the property itself with a
  // value of its own kind — otherwise completing the name would go unchecked.
  if (!property.values?.length) {
    const sample = probeValue(property.name);
    if (sample) {
      run(property.name, `${property.name}: ${sample}`, `${property.name}: ${sample}`);
    } else {
      failures.push(`${property.name}  no probe value; add one to PROBE_VALUE`);
    }
  }
}

// The SVG elements the editor completes have to draw too. An unsupported one
// is a silent blank rather than a warning, so each is given enough geometry to
// be asked for something and checked for a complaint.
const ANCHOR_SHAPE = '<rect id="anchor" x="0" y="0" width="4" height="4" fill="black"/>';
for (const element of vocabulary.svgElements) {
  const geometry = SVG_GEOMETRY[element] ?? "";
  const body =
    element === "g"
      ? `<g>${ANCHOR_SHAPE}</g>`
      : geometry
        ? `${element === "use" ? `<defs>${ANCHOR_SHAPE}</defs>` : ""}<${element}${geometry}/>`
        : // Containers and metadata draw nothing themselves, so they are put
          // beside a shape that does.
          `<${element}></${element}>${ANCHOR_SHAPE}`;

  const result = globalThis.inkwire.render(`<svg width="20" height="20">${body}</svg>`, "", 40, 40, {});
  checks++;
  const warnings = (result.warnings ?? []).filter((w) => w.code.startsWith("unsupported"));
  if (!result.ok || warnings.length > 0) {
    failures.push(`<${element}>  ${warnings.map((w) => w.message).join("; ") || result.error}`);
  }
}

for (const failure of failures) console.log(`  FAIL  ${failure}`);
console.log(
  `\n${vocabulary.properties.length} properties, ` +
    `${vocabulary.properties.reduce((n, p) => n + (p.values?.length ?? 0), 0)} values, ` +
    `${vocabulary.svgElements.length} svg elements — ` +
    `${checks} renders, ${failures.length} rejected`,
);
process.exit(failures.length === 0 ? 0 : 1);
