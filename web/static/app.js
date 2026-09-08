// Inkwire Studio.
//
// The preview is not an approximation of the panel: the same Go that the CLI
// renders with is compiled to wasm and run here, so what appears on screen is
// the frame that would be written to the tag. web/verify/parity.mjs holds that
// claim to the letter by comparing the two byte for byte.

import {
  autocompletion, bracketMatching, closeBrackets, closeBracketsKeymap,
  completionKeymap, crosshairCursor, css, defaultKeymap, drawSelection,
  dropCursor, EditorState, EditorView, Facet, HighlightStyle,
  highlightActiveLine, highlightActiveLineGutter, highlightSelectionMatches,
  highlightSpecialChars, history, historyKeymap, html, indentOnInput,
  indentUnit, indentWithTab, keymap, lineNumbers, rectangularSelection,
  searchKeymap, syntaxHighlighting, tags,
} from "./vendor/codemirror.js";

const $ = (selector) => document.querySelector(selector);

/* ------------------------------------------------------------------ *
 * Completion
 *
 * A stock CSS completion offers every property a browser has. This renderer
 * implements 89 of them, so a stock list would spend most of its suggestions
 * proposing declarations that compile to a warning — teaching the wrong
 * vocabulary at the moment someone is learning it.
 *
 * completions.json is generated from the manual's property table, which a Go
 * test already holds to naming every implemented property, and every entry in
 * it is put through the renderer by web/verify/completions.mjs. Nothing is
 * offered here that does not draw.
 * ------------------------------------------------------------------ */

let vocabulary = { properties: [], svgElements: [] };
let propertyOptions = [];
let svgElementOptions = [];
const propertiesByName = new Map();

async function loadVocabulary() {
  try {
    vocabulary = await (await fetch("completions.json")).json();
  } catch {
    return;
  }
  propertyOptions = vocabulary.properties.map((property) => ({
    label: property.name,
    type: "property",
    detail: property.category.toLowerCase(),
    info: [property.syntax, property.notes].filter(Boolean).join(" — "),
    // A property is nearly always followed by its value, so the completion
    // leaves the caret where the value goes.
    apply: `${property.name}: `,
  }));
  svgElementOptions = vocabulary.svgElements.map((name) => ({
    label: name,
    type: "type",
    detail: "svg",
  }));
  for (const property of vocabulary.properties) propertiesByName.set(property.name, property);
}

// Where the caret is, in the only two terms that change what may be typed
// next. This reads the text rather than the syntax tree so that one function
// serves both the CSS pane and the CSS inside a <style> element, which are the
// same language reached two different ways.
function cssPosition(state, pos) {
  const back = state.doc.sliceString(Math.max(0, pos - 400), pos);
  const open = back.lastIndexOf("{");
  const close = back.lastIndexOf("}");
  // Outside any block — before the first brace, or after the last one closed —
  // what comes next is a selector, and this vocabulary says nothing about those.
  if (open === -1 || close > open) return { where: "selector" };

  // Inside a block, the current declaration starts after whichever came last:
  // the brace that opened the block, or the semicolon that ended the one before.
  const declaration = back.slice(Math.max(open, back.lastIndexOf(";")) + 1);
  const colon = declaration.indexOf(":");
  if (colon === -1) return { where: "property" };
  return { where: "value", property: declaration.slice(0, colon).trim() };
}

// Whether the offset is inside a <style> element, and so inside CSS even
// though the document is HTML.
function insideStyle(text) {
  const open = text.lastIndexOf("<style");
  return open !== -1 && text.lastIndexOf("</style") < open;
}

// Whether the offset is inside an <svg> element, where the elements that draw
// are a closed set the manual names.
function insideSVG(text) {
  const open = text.lastIndexOf("<svg");
  return open !== -1 && text.lastIndexOf("</svg") < open;
}

function cssCompletions(context) {
  const at = cssPosition(context.state, context.pos);
  if (at.where === "selector") return null;

  const word = context.matchBefore(/[-\w]*/);
  if (!word || (word.from === word.to && !context.explicit)) return null;

  if (at.where === "property") {
    return { from: word.from, options: propertyOptions, validFor: /^[-\w]*$/ };
  }
  const property = propertiesByName.get(at.property);
  if (!property?.values?.length) return null;
  return {
    from: word.from,
    options: property.values.map((value) => ({
      label: value,
      type: "enum",
      detail: property.name,
    })),
    validFor: /^[-\w]*$/,
  };
}

// Inside an <svg>, after a "<", the elements this renderer draws. An element
// it does not implement is a silent blank rather than a warning, so offering
// the wrong one is worse here than in HTML.
function svgCompletions(context) {
  const before = context.state.doc.sliceString(Math.max(0, context.pos - 4000), context.pos);
  if (!insideSVG(before)) return null;
  const word = context.matchBefore(/<[-\w]*/);
  if (!word) return null;
  return {
    from: word.from + 1,
    options: svgElementOptions,
    validFor: /^[-\w]*$/,
  };
}

// The single source both editors use. It replaces every built-in source, so
// the CSS grammar's own thousand-property list never reaches the popup; where
// this has nothing to say about HTML, it hands back to the HTML language's own
// completion, which is about elements and attributes rather than about CSS.
function completionSource(context) {
  const before = context.state.doc.sliceString(Math.max(0, context.pos - 4000), context.pos);
  const isCSSDocument = context.state.facet(CSS_DOCUMENT);

  if (isCSSDocument || insideStyle(before)) return cssCompletions(context);

  const svg = svgCompletions(context);
  if (svg) return svg;

  for (const source of context.state.languageDataAt("autocomplete", context.pos)) {
    if (typeof source === "function") {
      const result = source(context);
      if (result) return result;
    }
  }
  return null;
}

/* ------------------------------------------------------------------ *
 * Editors
 * ------------------------------------------------------------------ */

// Marks a state as being a stylesheet in its own right rather than a document
// that may contain one, which is the whole difference between the two panes.
const CSS_DOCUMENT = Facet.define({ combine: (values) => values.length > 0 });

const inkHighlight = HighlightStyle.define([
  { tag: tags.comment, color: "#98a1ae", fontStyle: "italic" },
  { tag: [tags.tagName, tags.standard(tags.tagName)], color: "#1a4bb8" },
  { tag: [tags.attributeName, tags.propertyName], color: "#7a4bb8" },
  { tag: [tags.string, tags.attributeValue], color: "#146b3a" },
  { tag: [tags.number, tags.unit], color: "#8a3d12" },
  { tag: [tags.keyword, tags.atom, tags.constant(tags.name)], color: "#8a3d12" },
  { tag: tags.definitionKeyword, color: "#1a4bb8" },
  { tag: [tags.className, tags.labelName], color: "#8a3d12" },
  { tag: tags.angleBracket, color: "#5b6472" },
  { tag: tags.punctuation, color: "#5b6472" },
  { tag: tags.invalid, color: "#c0392b" },
]);

// The chrome is set from the same custom properties the rest of the page uses,
// so the editor stays part of the design rather than a panel dropped into it.
const inkTheme = EditorView.theme({
  "&": {
    height: "100%",
    fontSize: "12px",
    backgroundColor: "var(--surface)",
    color: "var(--ink)",
  },
  ".cm-scroller": {
    fontFamily: "var(--mono)",
    lineHeight: "20px",
    overflow: "auto",
  },
  ".cm-content": { padding: "10px 0", caretColor: "var(--ink)" },
  ".cm-gutters": {
    backgroundColor: "var(--chrome)",
    color: "var(--ink-faint)",
    border: "0",
    borderRight: "1px solid var(--line-soft)",
    paddingRight: "2px",
  },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 6px 0 12px" },
  ".cm-activeLineGutter": { backgroundColor: "var(--line-soft)", color: "var(--ink-soft)" },
  ".cm-activeLine": { backgroundColor: "rgba(47, 109, 246, 0.045)" },
  // Selection is drawn by CodeMirror rather than the browser, so unlike the
  // overlay this replaced there is no transparent text under a solid block.
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": {
    backgroundColor: "#cadcff",
  },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--ink)" },
  "&.cm-focused": { outline: "0" },
  ".cm-matchingBracket, &.cm-focused .cm-matchingBracket": {
    backgroundColor: "#dfe7f6",
    outline: "1px solid #b7c7e6",
  },
  ".cm-selectionMatch": { backgroundColor: "#eef2f8" },
  ".cm-tooltip": {
    border: "1px solid var(--line)",
    borderRadius: "6px",
    backgroundColor: "var(--surface)",
    boxShadow: "0 6px 20px rgba(20, 24, 32, 0.14)",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul": {
    fontFamily: "var(--mono)",
    fontSize: "12px",
    maxHeight: "16em",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li": { padding: "3px 8px" },
  ".cm-tooltip-autocomplete ul li[aria-selected]": {
    backgroundColor: "var(--accent)",
    color: "#fff",
  },
  ".cm-completionDetail": { color: "var(--ink-faint)", fontStyle: "normal", marginLeft: "10px" },
  ".cm-tooltip-autocomplete ul li[aria-selected] .cm-completionDetail": { color: "#dbe6ff" },
  ".cm-completionInfo": {
    border: "1px solid var(--line)",
    borderRadius: "6px",
    backgroundColor: "var(--surface)",
    fontFamily: "var(--sans)",
    fontSize: "12px",
    lineHeight: "1.5",
    maxWidth: "28em",
    padding: "7px 9px",
  },
  ".cm-panels": { backgroundColor: "var(--chrome)", color: "var(--ink)" },
  ".cm-panel.cm-search input, .cm-panel.cm-search button": {
    fontFamily: "var(--sans)",
    fontSize: "12px",
  },
  ".cm-searchMatch": { backgroundColor: "#fdf0c8" },
  ".cm-searchMatch.cm-searchMatch-selected": { backgroundColor: "#ffd97a" },
});

function makeEditor(mount, language, isCSS, onChange) {
  const extensions = [
    lineNumbers(),
    highlightActiveLineGutter(),
    highlightSpecialChars(),
    history(),
    drawSelection(),
    dropCursor(),
    indentOnInput(),
    indentUnit.of("  "),
    bracketMatching(),
    closeBrackets(),
    rectangularSelection(),
    crosshairCursor(),
    highlightActiveLine(),
    highlightSelectionMatches(),
    // override replaces every built-in source, which is the point: the CSS
    // grammar ships a completion for all of CSS and none of it applies here.
    autocompletion({ override: [completionSource], icons: false }),
    syntaxHighlighting(inkHighlight),
    inkTheme,
    language,
    keymap.of([
      ...closeBracketsKeymap,
      ...defaultKeymap,
      ...searchKeymap,
      ...historyKeymap,
      ...completionKeymap,
      indentWithTab,
    ]),
    EditorView.updateListener.of((update) => {
      if (update.docChanged) onChange();
    }),
  ];
  if (isCSS) extensions.push(CSS_DOCUMENT.of(true));

  const view = new EditorView({ parent: mount, state: EditorState.create({ extensions }) });
  return {
    view,
    get value() {
      return view.state.doc.toString();
    },
    set value(text) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
        selection: { anchor: 0 },
      });
    },
    focus() {
      view.focus();
    },
  };
}

/* ------------------------------------------------------------------ *
 * Renderer
 * ------------------------------------------------------------------ */

const state = {
  api: null,
  panels: [],
  size: { width: 296, height: 128 },
  view: "preview",
  png: null,
  pending: 0,
};

async function boot() {
  const go = new Go();
  const ready = new Promise((resolve) => {
    globalThis.inkwireReady = resolve;
  });
  // Streaming needs the server to say application/wasm. Plenty of static file
  // servers do not, and falling back is cheaper than requiring one.
  let module;
  try {
    module = await WebAssembly.instantiateStreaming(fetch("inkwire.wasm"), go.importObject);
  } catch {
    const bytes = await (await fetch("inkwire.wasm")).arrayBuffer();
    module = await WebAssembly.instantiate(bytes, go.importObject);
  }
  go.run(module.instance);
  await ready;
  return globalThis.inkwire;
}

function setStatus(text, bad) {
  const status = $("#status");
  status.textContent = text;
  status.classList.toggle("is-bad", Boolean(bad));
}

// render runs the whole pipeline and puts every part of the answer somewhere a
// person can see it: the picture, the boxes it was laid out in, the scene it
// compiled to, and everything the renderer could not honour.
function render() {
  if (!state.api) return;
  const markup = editors.markup.value;
  const css = editors.css.value;
  const { width, height } = state.size;
  const started = performance.now();

  const result = state.api.render(markup, css, width, height, {});
  const elapsed = performance.now() - started;
  $("#timing").textContent = `${elapsed.toFixed(1)} ms`;

  if (result.png) {
    state.png = result.png;
    const image = $("#preview");
    image.src = `data:image/png;base64,${result.png}`;
    image.width = result.width;
    image.height = result.height;
    applyZoom();
    $("#download").disabled = false;
  } else {
    state.png = null;
    $("#preview").removeAttribute("src");
    $("#download").disabled = true;
  }

  setStatus(result.ok ? `${width}×${height}` : result.error ?? "render failed", !result.ok);
  report(result);

  if (state.view === "scene") refreshScene(markup, css);
  if (state.view === "measure") refreshMeasure(markup, css, width, height);
}

function report(result) {
  const warnings = result.warnings ?? [];
  const list = $("#warnings");
  list.innerHTML = "";
  for (const warning of warnings) {
    const item = document.createElement("li");
    item.innerHTML =
      `<code>${escapeHTML(warning.code)}</code>${escapeHTML(warning.message)}` +
      `<span class="path">${escapeHTML(warning.path)}</span>`;
    list.append(item);
  }
  if (!result.ok && result.error) {
    const item = document.createElement("li");
    item.className = "is-bad";
    item.innerHTML = `<code>error</code>${escapeHTML(result.error)}`;
    list.prepend(item);
  }

  const count = $("#report-count");
  const total = warnings.length + (result.ok ? 0 : 1);
  count.textContent =
    total === 0 ? "no warnings" : `${total} ${total === 1 ? "warning" : "warnings"}`;
  count.classList.toggle("is-warn", total > 0 && result.ok);
  count.classList.toggle("is-bad", !result.ok);

  const missing = result.missingRunes ?? [];
  $("#missing-runes").textContent = missing.length
    ? `no glyph for ${missing.map((c) => JSON.stringify(c)).join(" ")}`
    : "";
}

function refreshScene(markup, css) {
  const compiled = state.api.compile(markup, css, 0, 0, {});
  $("#scene").textContent = compiled.ok
    ? JSON.stringify(JSON.parse(compiled.json), null, 2)
    : compiled.error ?? "";
}

function refreshMeasure(markup, css, width, height) {
  const measured = state.api.measure(markup, css, width, height, {});
  const body = $("#nodes");
  body.innerHTML = "";
  for (const node of measured.nodes ?? []) {
    // Indent by path depth so the output is the tree it describes.
    const depth = (node.path.match(/\./g) ?? []).length;
    const row = document.createElement("tr");
    row.innerHTML =
      `<td>${"  ".repeat(depth)}${escapeHTML(node.path)}</td>` +
      `<td class="kind">${escapeHTML(node.type)}</td>` +
      `<td class="num">${node.width}×${node.height}</td>` +
      `<td class="num">@${node.x},${node.y}</td>`;
    body.append(row);
  }
}

function applyZoom() {
  const zoom = Number($("#zoom").value);
  const image = $("#preview");
  if (!image.getAttribute("src")) return;
  image.style.width = `${state.size.width * zoom}px`;
  image.style.height = `${state.size.height * zoom}px`;
}

/* ------------------------------------------------------------------ *
 * Wiring
 * ------------------------------------------------------------------ */

let queued = null;
const schedule = () => {
  clearTimeout(queued);
  queued = setTimeout(render, 120);
};

const editors = {
  markup: makeEditor($('[data-editor="markup"]'), html(), false, schedule),
  css: makeEditor($('[data-editor="css"]'), css(), true, schedule),
};

for (const tab of document.querySelectorAll(".tab[data-source]")) {
  tab.addEventListener("click", () => {
    for (const other of document.querySelectorAll(".tab[data-source]")) {
      other.classList.toggle("is-active", other === tab);
    }
    for (const editor of document.querySelectorAll(".editor")) {
      editor.hidden = editor.dataset.editor !== tab.dataset.source;
    }
    // A hidden CodeMirror measures nothing, so it is remeasured when shown.
    const shown = editors[tab.dataset.source];
    shown.view.requestMeasure();
    shown.focus();
  });
}

for (const tab of document.querySelectorAll(".tab[data-view]")) {
  tab.addEventListener("click", () => {
    state.view = tab.dataset.view;
    for (const other of document.querySelectorAll(".tab[data-view]")) {
      other.classList.toggle("is-active", other === tab);
    }
    for (const view of document.querySelectorAll(".view")) {
      view.classList.toggle("is-active", view.dataset.view === state.view);
    }
    render();
  });
}

$("#zoom").addEventListener("change", applyZoom);

$("#panel").addEventListener("change", () => {
  const value = $("#panel").value;
  const custom = value === "custom";
  $("#custom-size").hidden = !custom;
  if (custom) {
    readCustomSize();
  } else {
    const panel = state.panels.find((p) => p.key + p.family === value);
    if (panel) {
      state.size = { width: panel.width, height: panel.height };
      $("#size-w").value = panel.width;
      $("#size-h").value = panel.height;
    }
  }
  render();
});

function readCustomSize() {
  const width = Math.max(1, Math.min(4000, Number($("#size-w").value) || 1));
  const height = Math.max(1, Math.min(4000, Number($("#size-h").value) || 1));
  state.size = { width, height };
}

for (const input of [$("#size-w"), $("#size-h")]) {
  input.addEventListener("input", () => {
    readCustomSize();
    schedule();
  });
}

$("#download").addEventListener("click", () => {
  if (!state.png) return;
  const bytes = Uint8Array.from(atob(state.png), (c) => c.charCodeAt(0));
  const url = URL.createObjectURL(new Blob([bytes], { type: "image/png" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = `inkwire-${state.size.width}x${state.size.height}.png`;
  link.click();
  URL.revokeObjectURL(url);
});

async function loadPanels() {
  const select = $("#panel");
  try {
    state.panels = await (await fetch("panels.json")).json();
  } catch {
    state.panels = [];
  }
  const groups = { gicisky: "Gicisky", nrfepd: "EPD-nRF5" };
  for (const [family, label] of Object.entries(groups)) {
    const panels = state.panels.filter((p) => p.family === family);
    if (panels.length === 0) continue;
    const group = document.createElement("optgroup");
    group.label = label;
    for (const panel of panels) {
      const option = document.createElement("option");
      option.value = panel.key + panel.family;
      // The catalogue carries entries read off firmware tables as well as
      // ones confirmed against a tag. Which is which is worth saying.
      option.textContent =
        `${panel.width}×${panel.height}  ${panel.name} · ${panel.palette}` +
        (panel.verified ? "  ✓" : "");
      group.append(option);
    }
    select.append(group);
  }
  const custom = document.createElement("option");
  custom.value = "custom";
  custom.textContent = "Custom size…";
  select.append(custom);

  // The 2.9" BWR is the one this project has on a desk, so it opens on that.
  const preferred = state.panels.find((p) => p.verified) ?? state.panels[0];
  if (preferred) {
    select.value = preferred.key + preferred.family;
    state.size = { width: preferred.width, height: preferred.height };
    $("#size-w").value = preferred.width;
    $("#size-h").value = preferred.height;
  }
}

const STARTERS = {
  "Hello panel": {
    markup: `<div class="card">
  <h1>Inkwire</h1>
  <p class="note">HTML and CSS, drawn for e-paper.</p>
  <div class="row">
    <span class="chip">296 × 128</span>
    <span class="chip">black · white · red</span>
  </div>
</div>
`,
    css: `* { box-sizing: border-box; }

.card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 296px;
  height: 128px;
  padding: 12px 14px;
  background: white;
  border: 2px solid black;
}

h1 {
  margin: 0;
  font-size: 32px;
  line-height: 34px;
}

.note {
  margin: 0;
  font-size: 14px;
  color: red;
}

.row {
  display: flex;
  gap: 6px;
  margin-top: auto;
}

.chip {
  padding: 3px 7px;
  font-size: 12px;
  background: black;
  color: white;
}
`,
  },
  "Flex and grid": {
    markup: `<div class="sheet">
  <div class="head">Layout</div>
  <div class="grid">
    <div class="cell">one</div>
    <div class="cell">two</div>
    <div class="cell">three</div>
    <div class="cell wide">spans two columns</div>
  </div>
</div>
`,
    css: `* { box-sizing: border-box; }

.sheet {
  display: flex;
  flex-direction: column;
  width: 296px;
  height: 128px;
  background: white;
}

.head {
  padding: 4px 8px;
  font-size: 14px;
  background: black;
  color: white;
}

.grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  grid-template-rows: 1fr 1fr;
  gap: 4px;
  padding: 6px;
}

.cell {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  border: 1px solid black;
}

.wide {
  grid-column: span 3;
  background: red;
  color: white;
  border-color: red;
}
`,
  },
  "SVG drawing": {
    markup: `<div class="plate">
  <svg width="120" height="80" viewBox="0 0 120 80">
    <polyline points="4,72 28,44 52,58 76,20 116,34"
              fill="none" stroke="red" stroke-width="3"
              stroke-linecap="round" stroke-linejoin="round" />
    <line x1="4" y1="76" x2="116" y2="76" stroke="black" stroke-width="2" />
  </svg>
  <div class="legend">
    <strong>1,284</strong>
    <span>steps today</span>
  </div>
</div>
`,
    css: `* { box-sizing: border-box; }

.plate {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 296px;
  height: 128px;
  padding: 12px;
  background: white;
}

.legend {
  display: flex;
  flex-direction: column;
}

strong { font-size: 32px; line-height: 34px; }
span { font-size: 12px; }
`,
  },
};

function loadStarters() {
  const select = $("#starter");
  for (const name of Object.keys(STARTERS)) {
    const option = document.createElement("option");
    option.value = name;
    option.textContent = name;
    select.append(option);
  }
  select.addEventListener("change", () => {
    const starter = STARTERS[select.value];
    if (!starter) return;
    editors.markup.value = starter.markup;
    editors.css.value = starter.css;
    select.value = "";
    render();
  });
}

(async () => {
  loadStarters();
  await Promise.all([loadPanels(), loadVocabulary()]);
  const first = STARTERS["Hello panel"];
  editors.markup.value = first.markup;
  editors.css.value = first.css;
  try {
    state.api = await boot();
    setStatus("ready");
    render();
  } catch (error) {
    setStatus(`renderer failed to load: ${error}`, true);
  }
})();
