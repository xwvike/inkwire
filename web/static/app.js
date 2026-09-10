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
  searchKeymap, syntaxHighlighting, syntaxTree, tags,
} from "./vendor/codemirror.js";

const $ = (selector) => document.querySelector(selector);

// Anything that reaches innerHTML goes through this first. Warning text and
// node paths are the renderer's words, not the page author's, but they quote
// the page — a selector, a declaration, an element name — and a page is free
// to contain angle brackets.
const escapeHTML = (text) =>
  String(text).replace(/[&<>]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"));

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

// Whether an editor's document currently parses.
//
// A page is typed into, and on the way to being written it passes through
// states that are not yet anything: a tag with no closing bracket, a rule with
// no closing brace, a property with no value. Sending those to the renderer
// costs a full layout to be told what the editor already knows, and answers
// with warnings about a page nobody has finished writing.
//
// CodeMirror has already parsed the document — that is where the colours come
// from — so this asks its tree rather than parsing again. An error node is
// exactly the "not yet" this is looking for.
function parses(editor) {
  const state = editor.view.state;
  // A blank document reports an error node in the HTML grammar, so clearing a
  // pane would leave the preview waiting for a page that is not coming. There
  // is nothing half-written about nothing.
  if (state.doc.length === 0 || !state.doc.toString().trim()) return true;
  let broken = false;
  syntaxTree(state).iterate({
    enter(node) {
      if (broken) return false;
      if (node.type.isError) {
        broken = true;
        return false;
      }
      return undefined;
    },
  });
  return !broken;
}

/* ------------------------------------------------------------------ *
 * Renderer
 * ------------------------------------------------------------------ */

const state = {
  api: null,
  panels: [],
  panel: null,
  // What a tag said about itself, once one has been asked. Null until then,
  // and the whole reason the chosen panel can be checked rather than trusted.
  detected: null,
  // Which protocol the chosen tag speaks, learned by what it serves.
  family: null,
  // The tag granted in this page's lifetime. Web Bluetooth grants are per
  // page, so this is as long as it can be remembered without getDevices,
  // which Chrome also keeps behind a flag.
  device: null,
  size: { width: 296, height: 128 },
  view: "preview",
  png: null,
  pending: 0,
};

// The renderer runs in a worker, so the only thing on this thread is the page.
// Every call is a message with an id and every answer is a promise; the worker
// also speaks on its own initiative, for the two things an upload needs — a
// characteristic write it cannot perform, and the uploader's own log.
function startWorker() {
  const worker = new Worker("worker.js");
  const waiting = new Map();
  let next = 0;
  const handlers = { write: null, log: null, sending: null };

  worker.onmessage = (event) => {
    const message = event.data;
    if (message.kind) {
      handlers[message.kind]?.(message);
      return;
    }
    const pending = waiting.get(message.id);
    waiting.delete(message.id);
    if (!pending) return;
    if (message.ok) pending.resolve(message.result);
    else pending.reject(new Error(message.error));
  };

  worker.onerror = (event) => {
    for (const [, pending] of waiting) pending.reject(new Error(event.message ?? "the worker failed"));
    waiting.clear();
    setStatus(`renderer failed: ${event.message ?? "worker error"}`, true);
  };

  const call = (op, request) =>
    new Promise((resolve, reject) => {
      const id = ++next;
      waiting.set(id, { resolve, reject });
      worker.postMessage({ id, op, request });
    });

  return {
    render: (request) => call("render", request),
    compile: (request) => call("compile", request),
    measure: (request) => call("measure", request),
    payload: (request) => call("payload", request),
    identify: (bytes) => call("identify", { bytes }),
    identifyNRFEPD: (bytes) => call("identifyNRFEPD", { bytes }),
    upload: (request) => call("upload", request),
    on(kind, handler) {
      handlers[kind] = handler;
    },
    post: (message) => worker.postMessage(message),
  };
}

function setStatus(text, bad) {
  const status = $("#status");
  status.textContent = text;
  status.classList.toggle("is-bad", Boolean(bad));
}

// render runs the whole pipeline and puts every part of the answer somewhere a
// person can see it: the picture, the boxes it was laid out in, the scene it
// compiled to, and everything the renderer could not honour.
// Answers can arrive out of order, and an older one painted over a newer one
// is worse than no answer at all: it shows a page that is not the one on
// screen. Every render carries the number it was started with, and only the
// latest is allowed to land.
let renderCount = 0;

async function render() {
  if (!state.api) return;

  // Half-written source is not a page. The last good picture stays on screen
  // rather than being replaced by whatever a broken document lays out to, and
  // the status line says why — a preview that flickers between a page and its
  // wreckage as a tag is typed is worse than one that waits.
  const unfinished = ["markup", "css"].filter((which) => !parses(editors[which]));
  if (unfinished.length > 0) {
    setStatus(`waiting for the ${unfinished.join(" and ")} to finish`);
    return;
  }

  const markup = editors.markup.value;
  const css = editors.css.value;
  const { width, height } = state.size;
  const started = performance.now();
  const mine = ++renderCount;

  // Naming the panel is what makes this the panel's own picture: inks it
  // cannot show are flattened the way the tag would flatten them, and what was
  // flattened comes back to be reported. A custom size has no palette to check
  // against, so it renders as drawn.
  let result;
  try {
    result = await state.api.render({
      markup,
      css,
      width,
      height,
      files: files(),
      panel: state.panel ?? "",
    });
  } catch (error) {
    if (mine === renderCount) setStatus(String(error?.message ?? error), true);
    return;
  }
  if (mine !== renderCount) return;
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
    $("#push").disabled = !state.panel || !navigator.bluetooth;
  } else {
    state.png = null;
    $("#preview").removeAttribute("src");
    $("#download").disabled = true;
    $("#push").disabled = true;
  }

  // What the panel is, not just how big it is. Once a panel is chosen its
  // palette and whether the catalogue entry was ever checked against hardware
  // disappear back into the dropdown, and both change what arrives on the tag.
  // The module already answers with all of it.
  const drawn = [result.panel ?? `${width}×${height}`]
    .concat(result.payloadBytes ? [`${result.payloadBytes} B on the wire`] : [])
    .join(" · ");
  setStatus(result.ok ? drawn : (result.error ?? "render failed"), !result.ok);
  report(result);

  if (state.view === "scene") void refreshScene(markup, css);
  if (state.view === "measure") void refreshMeasure(markup, css, width, height);
  if (resources.size > 0) drawFileList();
  refreshSteps();
}

function report(result) {
  const warnings = result.warnings ?? [];
  const list = $("#warnings");
  // Warnings are rebuilt whole and read from the top: the first one is usually
  // the one that explains the rest, so this does not follow the end.
  listMode(list, "warnings");
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

async function refreshScene(markup, css) {
  const compiled = await state.api.compile({ markup, css, files: files() });
  $("#scene").textContent = compiled.ok
    ? JSON.stringify(JSON.parse(compiled.json), null, 2)
    : compiled.error ?? "";
}

async function refreshMeasure(markup, css, width, height) {
  const measured = await state.api.measure({ markup, css, width, height, files: files() });
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
 * Files
 *
 * The module reads a page's stylesheets and pictures out of a name-to-bytes
 * map, because a page in a browser has no directory beside it. This is that
 * map, and the names in it are the strings the page writes — src="assets/x.png"
 * is called "assets/x.png" here and nothing else.
 * ------------------------------------------------------------------ */

const resources = new Map();

// A fresh object each render. The module copies the bytes out synchronously,
// so nothing is retained on the other side, and building it here keeps the
// store a Map — ordered, and able to hold a name that would collide with a
// property of Object.
function files() {
  const out = {};
  for (const [name, bytes] of resources) out[name] = bytes;
  return out;
}

const IMAGE_TYPES = /\.(png|jpe?g|gif|webp|svg)$/i;

// Whether the page as written actually asks for this name. A file nobody
// references is not an error — it may be about to be — but it is worth saying,
// because a name that does not match is the likeliest reason a picture is
// missing, and the two look identical from the page.
function isReferenced(name) {
  const source = editors.markup.value + editors.css.value;
  return source.includes(name);
}

const previews = new Map();

function previewURL(name, bytes) {
  if (previews.has(name)) return previews.get(name);
  const type = name.toLowerCase().endsWith(".svg") ? "image/svg+xml" : "image/*";
  const url = URL.createObjectURL(new Blob([bytes], { type }));
  previews.set(name, url);
  return url;
}

function forgetPreview(name) {
  const url = previews.get(name);
  if (url) URL.revokeObjectURL(url);
  previews.delete(name);
}

function drawFileList() {
  const list = $("#file-list");
  list.innerHTML = "";
  for (const [name, bytes] of resources) {
    const item = document.createElement("li");

    const thumb = document.createElement("span");
    thumb.className = "thumb";
    if (IMAGE_TYPES.test(name)) {
      const image = document.createElement("img");
      image.src = previewURL(name, bytes);
      image.alt = "";
      thumb.append(image);
    } else {
      thumb.textContent = (name.split(".").pop() ?? "?").slice(0, 4);
    }

    const label = document.createElement("span");
    label.className = "name";
    label.textContent = name;

    const used = document.createElement("span");
    const referenced = isReferenced(name);
    used.className = referenced ? "used" : "used is-unused";
    used.textContent = referenced ? "referenced" : "not referenced";

    const size = document.createElement("span");
    size.className = "size";
    size.textContent = `${bytes.length.toLocaleString()} B`;

    const remove = document.createElement("button");
    remove.className = "drop-file";
    remove.type = "button";
    remove.title = `remove ${name}`;
    remove.textContent = "\u00d7";
    remove.addEventListener("click", () => {
      resources.delete(name);
      forgetPreview(name);
      drawFileList();
      render();
    });

    item.append(thumb, label, used, size, remove);
    list.append(item);
  }

  $("#file-empty").hidden = resources.size > 0;
  $("#file-count").textContent = resources.size > 0 ? String(resources.size) : "";
}

async function addFiles(fileList) {
  for (const file of fileList) {
    // webkitRelativePath is set when a directory is dropped, and it carries
    // the path the page would write. A plain file has only its own name.
    const name = file.webkitRelativePath || file.name;
    forgetPreview(name);
    resources.set(name, new Uint8Array(await file.arrayBuffer()));
  }
  drawFileList();
  render();
}

// Dropping is accepted anywhere on the window. Aiming at a panel that may not
// be the visible tab is a rule nobody should have to learn, and the page has
// nothing else a file could mean.
let dragDepth = 0;
window.addEventListener("dragenter", (event) => {
  if (![...event.dataTransfer.types].includes("Files")) return;
  event.preventDefault();
  dragDepth++;
  document.body.classList.add("is-dropping");
});
window.addEventListener("dragover", (event) => {
  if ([...event.dataTransfer.types].includes("Files")) event.preventDefault();
});
window.addEventListener("dragleave", () => {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) document.body.classList.remove("is-dropping");
});
window.addEventListener("drop", (event) => {
  if (![...event.dataTransfer.types].includes("Files")) return;
  event.preventDefault();
  dragDepth = 0;
  document.body.classList.remove("is-dropping");
  addFiles(event.dataTransfer.files);
});

/* ------------------------------------------------------------------ *
 * Detection
 *
 * The pages these tags usually ship with make the panel a dropdown, and
 * picking the wrong entry draws a page for hardware that is not there with
 * nothing to say so. A Gicisky tag does not need to be asked: the model is in
 * its advertisement, under company 0x5053, and the module resolves it against
 * the same table the command does.
 *
 * What the browser will not do is scan. requestLEScan is still behind a flag,
 * so the only way to an advertisement is to be granted a device first — the
 * user picks it from Chrome's own dialog — and then watch it. That is one more
 * step than the command needs and it is the whole of the difference.
 * ------------------------------------------------------------------ */

// The service the firmware serves, and the two names a factory tag answers to.
// The service cannot be a filter on its own: these tags do not put it in the
// advertisement, so the name is what the dialog matches on and the service is
// only declared so it may be used after.
// The two families serve different things. A Gicisky tag has the 16-bit FEF0
// with a control and a data characteristic; an EPD-nRF5 tag has a vendor
// 128-bit service and does everything over one. Which one answers is how the
// page tells them apart — the name will not, and neither will the advertisement
// in a browser that cannot read one.
const TAG_SERVICE = 0xfef0;
const NRF_SERVICE = "62750001-d828-918d-fb46-b6c11c675aec";
const NRF_CHARACTERISTIC = "62750002-d828-918d-fb46-b6c11c675aec";
const GICISKY_COMPANY = 0x5053;

// How long to wait for one advertisement. A tag advertises every couple of
// seconds; longer than this and something else is wrong, and saying so beats
// a spinner that never stops.
const ADVERTISEMENT_TIMEOUT = 12000;

// Detection happens in a dialog the page does not control, against a radio it
// cannot see, in a browser feature that three of the four steps may not have.
// When it stops, the only useful question is which step it stopped on — and a
// line of small text in the corner cannot answer that, because the step before
// has already overwritten it. So every step is written into the report panel,
// which is large, scrolls, and keeps what came before.
let detectSteps = [];

function step(text, kind) {
  detectSteps.push({ text, kind, at: Math.round(performance.now()) });
  drawDetectSteps();
}

// Whether the list is showing steps or warnings. The two share the panel and
// are rebuilt on different terms, so switching between them starts over.
function listMode(list, mode) {
  if (list.dataset.mode === mode) return false;
  list.dataset.mode = mode;
  list.innerHTML = "";
  return true;
}

function drawDetectSteps() {
  const list = $("#warnings");
  // A step log is appended to rather than rebuilt. An upload writes forty of
  // them, and rebuilding the list each time is quadratic — and, worse, throws
  // away where the reader had scrolled to.
  if (listMode(list, "steps") || list.childElementCount > detectSteps.length) {
    list.innerHTML = "";
  }
  // Following the newest entry, unless the reader has scrolled up to look at
  // something. Yanking them back down would be the tool arguing with them.
  const following = list.scrollHeight - list.scrollTop - list.clientHeight < 24;

  const first = detectSteps[0]?.at ?? 0;
  for (let index = list.childElementCount; index < detectSteps.length; index++) {
    const entry = detectSteps[index];
    const item = document.createElement("li");
    if (entry.kind === "bad") item.className = "is-bad";
    item.innerHTML =
      `<code>${String(entry.at - first).padStart(5)} ms</code>${escapeHTML(entry.text)}`;
    list.append(item);
  }
  if (following) list.scrollTop = list.scrollHeight;
  const count = $("#report-count");
  const failed = detectSteps.some((entry) => entry.kind === "bad");
  count.textContent = failed ? "detection stopped" : "detecting…";
  count.classList.toggle("is-bad", failed);
  count.classList.toggle("is-warn", false);
  $("#missing-runes").textContent = "";
}

function showDetected() {
  const chip = $("#detected");
  const found = state.detected;
  if (!found) {
    chip.hidden = true;
    return;
  }
  chip.hidden = false;
  chip.classList.toggle("is-unknown", !found.identified);
  chip.classList.toggle("is-mismatch", found.identified && found.key !== state.panel);

  if (!found.identified) {
    chip.textContent = `tag advertises ${found.id}, which this build has no entry for`;
    chip.title =
      "The tag answered and said what it is, but no profile in the catalogue has that id. " +
      "Nothing can be drawn for it until one is added.";
    return;
  }
  const battery = `${found.voltage.toFixed(1)} V`;
  chip.textContent =
    found.key === state.panel
      ? `detected · ${found.panel} · ${battery}`
      : `tag says ${found.panel} — you have chosen another`;
  chip.title = `Advertised id ${found.id}, firmware ${found.firmware}, battery ${battery}.`;
}

// Reads one advertisement from a device the user has granted, and answers with
// what the module made of it.
async function readAdvertisement(device) {
  // Chrome has had this since 105 and nothing else has it at all. Saying so is
  // better than a promise that never settles.
  if (typeof device.watchAdvertisements !== "function") {
    // Chrome keeps this behind chrome://flags/#enable-experimental-web-platform-features.
    // A Gicisky tag puts its model in the advertisement and nowhere else — the
    // GATT handshake reports the tag's message size, not its panel — so without
    // this the model cannot be read at all, by anything, which is why every
    // other tool for these tags asks you to pick it.
    throw new Error(
      "this Chrome has watchAdvertisements switched off, and a Gicisky tag says which " +
        "panel it has only in its advertisement. Turn on " +
        "chrome://flags/#enable-experimental-web-platform-features and restart to read it, " +
        "or choose the panel by hand — every other tool for these tags makes you do that.",
    );
  }
  const stop = new AbortController();
  return new Promise((resolve, reject) => {
    // Two failures look the same from outside and need different answers: no
    // packet ever arrives, or packets arrive stripped of the manufacturer data
    // the model is in. Counting them apart is the whole point of this.
    let seen = 0;
    const timer = setTimeout(() => {
      stop.abort();
      reject(
        new Error(
          seen === 0
            ? "no advertisement arrived in 12s — the tag may be asleep, out of range, or " +
              "already connected elsewhere; choose the panel by hand to carry on"
            : `${seen} advertisements arrived and none carried manufacturer data 0x5053, ` +
              "so this platform is not passing it on; choose the panel by hand to carry on",
        ),
      );
    }, ADVERTISEMENT_TIMEOUT);

    device.addEventListener(
      "advertisementreceived",
      (event) => {
        const data = event.manufacturerData?.get(GICISKY_COMPANY);
        if (!data) {
          // Another packet from the same tag. Which companies it did carry is
          // the evidence for whether the platform strips them or the tag is
          // simply not saying yet.
          seen++;
          const companies = [...(event.manufacturerData?.keys() ?? [])];
          step(
            `advertisement ${seen}: rssi ${event.rssi ?? "?"}, manufacturer data from ` +
              (companies.length
                ? companies.map((id) => `0x${id.toString(16).padStart(4, "0")}`).join(", ")
                : "nobody"),
          );
          return;
        }
        clearTimeout(timer);
        stop.abort();
        resolve(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
      },
      { signal: stop.signal },
    );

    device
      .watchAdvertisements({ signal: stop.signal })
      .then(() => step("watching advertisements"))
      .catch((error) => {
        clearTimeout(timer);
        reject(error);
      });
  });
}

// Whether this browser will hand over an advertisement at all. Chrome keeps it
// behind chrome://flags/#enable-experimental-web-platform-features, and a
// Gicisky tag says which panel it has there and nowhere else.
const CAN_READ_ADVERTISEMENTS = "watchAdvertisements" in (globalThis.BluetoothDevice?.prototype ?? {});

// Marks a button as waiting, and says what for.
//
// It is deliberately not tied to the button being disabled: a button is
// disabled for the whole of an action, and part of that action is the browser
// showing its own chooser, where the page is waiting on a person rather than
// working. A spinner through that would be claiming to be busy while doing
// nothing at all.
function working(selector, label) {
  const button = $(selector);
  if (!label) {
    button.classList.remove("is-busy");
    // The stored label is what to go back to, and a step may have changed it
    // while the spinner was up — Choose becomes Change once a tag is granted.
    if (button.dataset.label) button.textContent = button.dataset.label;
    delete button.dataset.label;
    return;
  }
  if (!button.dataset.label) button.dataset.label = button.textContent;
  button.classList.add("is-busy");
  button.textContent = label;
}

// Which family a connected tag belongs to, by what it serves. Neither name nor
// advertisement can answer this in a browser, and getting it wrong means
// speaking the wrong protocol at a tag that will not answer.
async function familyOf(server) {
  try {
    const service = await server.getPrimaryService(TAG_SERVICE);
    await service.getCharacteristic(CONTROL_CHARACTERISTIC);
    await service.getCharacteristic(DATA_CHARACTERISTIC);
    return "gicisky";
  } catch {
    // Not that one, then.
  }
  try {
    const service = await server.getPrimaryService(NRF_SERVICE);
    await service.getCharacteristic(NRF_CHARACTERISTIC);
    return "nrfepd";
  } catch {
    return null;
  }
}

// Ask an EPD-nRF5 tag which panel it has, the only way there is to ask.
//
// This family keeps its model in the firmware's flash rather than in an
// advertisement, so nothing is known about it until it is connected to and
// sent an init — which is why the page used to make somebody pick a panel for
// a tag that knew perfectly well what it was. The reference tool does the same
// thing on connect, and the init only powers the panel up; the firmware puts
// it back to sleep when the connection ends.
//
// The tag answers with several notifications and only one of them is the
// configuration. The rest are lines of text — slots, a session id, the link's
// message size — so the binary one is the one that is not text, which is the
// same rule internal/nrfepd uses.
const NRF_CONFIG_WAIT = 6000;

function looksLikeText(bytes) {
  if (bytes.length === 0) return false;
  return bytes.every((b) => b >= 0x20 && b <= 0x7e);
}

async function askNRFEPDPanel(server) {
  const service = await server.getPrimaryService(NRF_SERVICE);
  const characteristic = await service.getCharacteristic(NRF_CHARACTERISTIC);

  let settle;
  const answered = new Promise((resolve) => {
    settle = resolve;
  });
  const listener = (event) => {
    const value = event.target.value;
    const bytes = new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
    if (!looksLikeText(bytes)) settle(bytes.slice());
  };

  characteristic.addEventListener("characteristicvaluechanged", listener);
  try {
    await characteristic.startNotifications();
    await characteristic.writeValue(Uint8Array.of(0x01));
    const config = await Promise.race([
      answered,
      new Promise((resolve) => setTimeout(() => resolve(null), NRF_CONFIG_WAIT)),
    ]);
    if (!config) return null;
    return state.api.identifyNRFEPD(config);
  } finally {
    characteristic.removeEventListener("characteristicvaluechanged", listener);
    // Leaving notifications on would keep delivering into a listener that is
    // gone; the connection is about to be dropped either way.
    try {
      await characteristic.stopNotifications();
    } catch {
      // The tag may already be going; nothing here depends on it.
    }
  }
}

// Hold the panel list to the one the tag reported.
//
// A tag that has said what it is leaves nothing to choose, and an editable
// list invites choosing wrong — the preview would then be drawn for a panel
// that is not the one being written to. The options stay visible rather than
// being removed, so it is clear what was decided and by whom.
function lockPanelTo(key) {
  const select = $("#panel");
  for (const option of select.options) option.disabled = option.value !== key;
  select.value = key;
  select.dispatchEvent(new Event("change"));
  $("#step-panel").classList.add("is-fixed");
}

function unlockPanels() {
  const select = $("#panel");
  for (const option of select.options) option.disabled = false;
  $("#step-panel").classList.remove("is-fixed");
}

// Step 2. Granting, identifying and checking are one action because they are
// one question — "which tag, and is it the one this page is drawn for?" — and
// because splitting them made the page ask for the same tag twice.
async function chooseTag() {
  const button = $("#connect");
  if (!navigator.bluetooth) {
    setStatus("this browser has no Web Bluetooth; Chrome and Edge have it", true);
    return;
  }

  button.disabled = true;
  detectSteps = [];
  // Choosing again is how a second tag is reached, and what the last one said
  // about itself is not true of this one. Clearing first means a chooser that
  // is then cancelled leaves nothing stale behind either.
  state.detected = null;
  showDetected();
  try {
    step("opening the chooser");
    const device = await navigator.bluetooth.requestDevice({
      acceptAllDevices: true,
      optionalServices: [TAG_SERVICE, NRF_SERVICE],
    });
    step(`granted "${device.name ?? "unnamed"}"`);
    // From here the page is the one doing the waiting.
    working("#connect", "Connecting");

    // Read the model first: connecting can stop a tag advertising, and this is
    // the only place the model is written. A failure here is not fatal — it is
    // the ordinary case in a Chrome without the flag — so it is reported and
    // the tag is kept.
    if (CAN_READ_ADVERTISEMENTS) {
      try {
        step("reading the advertisement for the model");
        const bytes = await readAdvertisement(device);
        const found = await state.api.identify(bytes);
        if (found.ok) {
          state.detected = found;
          if (found.identified) {
            step(`the tag says it is ${found.panel} — ${found.voltage.toFixed(1)} V`);
            $("#panel").value = found.key;
            $("#panel").dispatchEvent(new Event("change"));
          } else {
            step(`the tag advertises id ${found.id}, which this build has no entry for`, "bad");
          }
        } else {
          step(found.error, "bad");
        }
      } catch (error) {
        step(`could not read the model: ${error?.message ?? error}`);
      }
    } else {
      step("this Chrome cannot read advertisements, so the panel stays as chosen");
    }

    // Then prove the tag is the thing it looks like, and learn which family it
    // belongs to, before Push is offered. Finding out at the third step that
    // the second one granted a pair of headphones is a worse place to find out.
    const server = await device.gatt.connect();
    const family = await familyOf(server);
    // An EPD-nRF5 tag is asked what it is while the connection is still open,
    // because that answer is the whole reason this family needs no panel
    // picked for it.
    let reported = null;
    if (family === "nrfepd") {
      try {
        step("asking the tag which panel it has");
        reported = await askNRFEPDPanel(server);
      } catch (error) {
        step(`could not ask the tag: ${error?.message ?? error}`);
      }
    }
    device.gatt.disconnect();
    if (!family) {
      step("this device serves neither FEF0 nor the EPD-nRF5 service", "bad");
      setStatus("that is not a tag this can write to", true);
      return;
    }
    step(
      family === "gicisky"
        ? "it serves FEF0 with both characteristics — ready to push"
        : "it serves the EPD-nRF5 service — ready to push, and it will say which panel it has",
    );

    if (family === "nrfepd") {
      if (reported?.ok && reported.identified) {
        step(`the tag says it is ${reported.panel}`);
        lockPanelTo(reported.key);
      } else if (reported?.ok) {
        step(`the tag reports model ${reported.id}, which this build has no entry for`, "bad");
        unlockPanels();
      } else {
        step("the tag did not say which panel it has; pick one", "bad");
        unlockPanels();
      }
    } else {
      unlockPanels();
    }

    state.family = family;
    state.device = device;
    $("#tag-name").textContent = device.name ?? "unnamed";
    $("#tag-name").hidden = false;
    // The label the spinner will fall back to when it clears, rather than the
    // text itself: this is inside the try, and the restore happens after it.
    button.dataset.label = "Change";
    $("#clear").hidden = false;
    setStatus(`tag ready: ${device.name ?? "unnamed"}`);
  } catch (error) {
    if (error?.name === "NotFoundError") {
      detectSteps = [];
      $("#warnings").innerHTML = "";
      delete $("#warnings").dataset.mode;
      setStatus("no tag chosen");
    } else {
      step(`${error?.name ?? "Error"}: ${error?.message ?? error}`, "bad");
      setStatus(String(error?.message ?? error), true);
      state.device = null;
      state.family = null;
      $("#tag-name").hidden = true;
      // There is no tag any more, so the button is offering to choose one
      // rather than to change it.
      button.dataset.label = "Choose";
    }
  } finally {
    working("#connect", null);
    button.disabled = false;
    showDetected();
    refreshSteps();
  }
}

// Stop pointing at the chosen tag.
//
// Named for what it does. There is no connection to end here: this page
// disconnects after identifying a tag and again after every push, because a
// tag left connected refuses the next attempt — so by the time anyone reaches
// for this, the radio is already idle. Nor does it hand the browser's grant
// back; that would be device.forget(), which is not called, and the grant
// lasts as long as the page does either way.
//
// What is actually cleared is this page's idea of which tag it is writing to,
// and the panel that tag had locked. The disconnect is a safety net for the
// case where something did leave a connection open.
function clearTag() {
  try {
    state.device?.gatt?.disconnect();
  } catch {
    // Already gone, which is the state this is asking for.
  }
  state.device = null;
  state.family = null;
  state.detected = null;
  unlockPanels();
  $("#tag-name").hidden = true;
  $("#clear").hidden = true;
  const button = $("#connect");
  button.dataset.label = "Choose";
  button.textContent = "Choose";
  detectSteps = [];
  showDetected();
  setStatus("no tag chosen");
  refreshSteps();
}

// Which step is asking for attention, which are satisfied, and which cannot be
// reached yet. Called after anything that changes one of those.
function refreshSteps() {
  const haveTag = Boolean(state.device);
  const drew = Boolean(state.png);

  const set = (id, cls) => {
    const node = $(id);
    node.classList.toggle("is-ready", cls === "ready");
    node.classList.toggle("is-done", cls === "done");
    node.classList.toggle("is-blocked", cls === "blocked");
  };

  // A panel is always chosen — the page opens on one — so this step is done
  // from the start rather than pretending to be a gate.
  set("#step-panel", state.panel ? "done" : "ready");
  set("#step-tag", haveTag ? "done" : navigator.bluetooth ? "ready" : "blocked");
  set("#step-push", !haveTag || !drew ? "blocked" : "ready");

  const needsPanel = state.family === "gicisky" && !state.panel;
  $("#push").disabled = !haveTag || !drew || needsPanel;
  $("#push").title = !navigator.bluetooth
    ? "This browser has no Web Bluetooth. Chrome and Edge have it."
    : !haveTag
      ? "Choose a tag first."
      : needsPanel
        ? "A custom size is not a tag; choose a panel."
        : `Write this page to ${state.device?.name ?? "the tag"}.`;
  $("#connect").disabled = !navigator.bluetooth;
  if (!navigator.bluetooth) {
    $("#connect").title = "This browser has no Web Bluetooth. Chrome and Edge have it.";
  }
}

// The two characteristics the firmware serves under FEF0: one carries the
// conversation, the other carries the picture.
const CONTROL_CHARACTERISTIC = 0xfef1;
const DATA_CHARACTERISTIC = 0xfef2;

// Step 3. The tag was chosen and checked at step 2, so this asks nothing and
// decides nothing: it connects, hands the page to the uploader, and reports.
//
// Writes are acknowledged. Both the command and the reference upload page do
// it that way, and the tag's flow control is built on the acknowledgement.
// Step 3. The tag was chosen and checked at step 2, so this asks nothing and
// decides nothing: it connects, hands the page to the uploader in the worker,
// and carries bytes between the two.
//
// The uploader is in the worker and the characteristics are here, because a
// GATT characteristic exists only on this thread. So the worker asks for each
// write and this answers when it is done — which is the same shape the Go side
// already had, where a write is a call that returns when the radio says so.
async function push() {
  const button = $("#push");
  if (!state.device) {
    setStatus("choose a tag first", true);
    return;
  }
  // Gicisky needs a panel named because it advertises one and this build has
  // to draw for it. EPD-nRF5 does not: it reports its model partway through
  // the conversation, and asking beforehand would only be a chance to be wrong.
  if (state.family === "gicisky" && !state.panel) {
    setStatus("choose a panel first — a custom size is not a tag", true);
    return;
  }

  button.disabled = true;
  detectSteps = [];
  // Unlike choosing, every part of this is the page working: there is no
  // dialog in the middle of it.
  working("#push", "Sending");
  const device = state.device;
  try {
    step(`connecting to "${device.name ?? "unnamed"}"`);
    const server = await device.gatt.connect();
    const nrf = state.family === "nrfepd";
    const service = await server.getPrimaryService(nrf ? NRF_SERVICE : TAG_SERVICE);
    // Gicisky talks over two characteristics and EPD-nRF5 over one, which is
    // the only difference the page has to know: the worker asks for a write by
    // name and this is where a name becomes a characteristic.
    const channels = nrf
      ? { write: await service.getCharacteristic(NRF_CHARACTERISTIC) }
      : {
          control: await service.getCharacteristic(CONTROL_CHARACTERISTIC),
          data: await service.getCharacteristic(DATA_CHARACTERISTIC),
        };
    const listenOn = nrf ? channels.write : channels.control;

    state.api.on("write", async ({ id, which, bytes }) => {
      try {
        await channels[which].writeValue(bytes);
        state.api.post({ kind: "wrote", id });
      } catch (error) {
        state.api.post({ kind: "wrote", id, error: String(error?.message ?? error) });
      }
    });
    state.api.on("log", ({ text }) => step(text));
    state.api.on("sending", ({ payloadBytes, panel }) => step(`sending ${payloadBytes} bytes for ${panel}`));

    // Notifications start before the first write, because the tag answers the
    // first command and an answer nobody is listening for is a stall.
    listenOn.addEventListener("characteristicvaluechanged", (event) => {
      const value = event.target.value;
      state.api.post({
        kind: "notify",
        bytes: new Uint8Array(value.buffer, value.byteOffset, value.byteLength),
      });
    });
    await listenOn.startNotifications();
    step(nrf ? "listening on the EPD characteristic" : "listening on FEF1");

    const result = await state.api.upload({
      markup: editors.markup.value,
      css: editors.css.value,
      files: files(),
      panel: state.panel ?? "",
      family: state.family,
    });
    if (!result.ok) {
      step(result.error, "bad");
      setStatus(result.error, true);
      return;
    }
    step("the tag took the page and is refreshing");
    setStatus(
      result.payloadBytes
        ? `pushed ${result.payloadBytes} B to ${device.name ?? "the tag"}`
        : `pushed to ${device.name ?? "the tag"}`,
    );
  } catch (error) {
    step(`${error?.name ?? "Error"}: ${error?.message ?? error}`, "bad");
    setStatus(String(error?.message ?? error), true);
  } finally {
    state.api.on("write", null);
    state.api.on("log", null);
    state.api.on("sending", null);
    // A tag left connected refuses the next attempt, and the next attempt is
    // the one where whatever went wrong gets looked at again.
    try {
      device?.gatt?.disconnect();
    } catch {
      // Already gone.
    }
    working("#push", null);
    button.disabled = false;
    refreshSteps();
  }
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
  markup: makeEditor($('.editor[data-source="markup"]'), html(), false, schedule),
  css: makeEditor($('.editor[data-source="css"]'), css(), true, schedule),
};

for (const tab of document.querySelectorAll(".tab[data-source]")) {
  tab.addEventListener("click", () => {
    for (const other of document.querySelectorAll(".tab[data-source]")) {
      other.classList.toggle("is-active", other === tab);
    }
    for (const panel of document.querySelectorAll(".editor, .files")) {
      panel.hidden = panel.dataset.source !== tab.dataset.source;
    }
    // A hidden CodeMirror measures nothing, so it is remeasured when shown.
    const shown = editors[tab.dataset.source];
    if (shown) {
      shown.view.requestMeasure();
      shown.focus();
    }
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
    state.panel = null;
    readCustomSize();
  } else {
    const panel = state.panels.find((p) => p.key === value);
    if (panel) {
      state.panel = panel.key;
      state.size = { width: panel.width, height: panel.height };
      $("#size-w").value = panel.width;
      $("#size-h").value = panel.height;
    }
  }
  showDetected();
  refreshSteps();
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

$("#connect").addEventListener("click", chooseTag);
$("#clear").addEventListener("click", clearTag);
$("#push").addEventListener("click", push);

// Step 2 says up front what it will and will not be able to do, because the
// answer changes what step 1 is for: with advertisements the panel is read
// off the tag, without them it stays whatever was chosen.
$("#connect").title = !navigator.bluetooth
  ? "This browser has no Web Bluetooth. Chrome and Edge have it."
  : CAN_READ_ADVERTISEMENTS
    ? "Choose the tag, read which panel it has, and check it answers."
    : "Choose the tag and check it answers. This Chrome cannot read advertisements, " +
      "so the panel stays as chosen — turn on " +
      "chrome://flags/#enable-experimental-web-platform-features to have it read.";

$("#file-input").addEventListener("change", (event) => {
  addFiles(event.target.files);
  // Clearing lets the same file be chosen again after it was removed, which
  // otherwise looks like the picker silently doing nothing.
  event.target.value = "";
});

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
      option.value = panel.key;
      // The size and the model, and nothing else. Every model's name already
      // carries its palette, so repeating that was noise; and whether the
      // catalogue entry has been checked against hardware is a fact about this
      // project rather than about the tag — the firmware is the same either
      // way, so it does not help anyone choosing a panel.
      option.textContent = `${panel.width}×${panel.height} ${panel.name}`;
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
    select.value = preferred.key;
    state.panel = preferred.key;
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
  drawFileList();
  refreshSteps();
  await Promise.all([loadPanels(), loadVocabulary()]);
  const first = STARTERS["Hello panel"];
  editors.markup.value = first.markup;
  editors.css.value = first.css;
  try {
    state.api = startWorker();
    // The first call is also the proof it started: the worker instantiates the
    // module on its first message, so a failure surfaces here rather than at
    // the first keystroke.
    await state.api.compile({ markup: "<div></div>", css: "" });
    setStatus("ready");
    await render();
  } catch (error) {
    setStatus(`renderer failed to load: ${error?.message ?? error}`, true);
  }
})();
