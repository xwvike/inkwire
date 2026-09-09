// Checks that the page never calls a function that does not exist.
//
// This exists because a browser is the only thing that runs app.js, and a
// ReferenceError there is silent to everything else: the module keeps working,
// the preview keeps drawing, and one tab quietly stops filling in. That is
// exactly what happened when the editor moved to CodeMirror and escapeHTML —
// declared in the highlighting code that was deleted, still called five times —
// went with it. Scene kept working because it writes textContent; Layout, which
// builds rows, threw on the first one and showed nothing.
//
// It is deliberately narrow. It does not try to be a linter: it collects the
// names the file calls, subtracts the ones it declares or imports, subtracts
// the standard library and the browser, and complains about the rest. A false
// positive is a name to add to KNOWN below, which is cheap; the alternative was
// finding out from someone using the page.
//
// Usage: node web/verify/calls.mjs
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
// Both files run only in a browser, and neither is reached by any other check
// here: app.js on the page's thread, worker.js on the renderer's.
const FILES = ["app.js", "worker.js"];

// Comments and string bodies are not code, and both are full of prose that
// reads like a call. Template literals keep their ${...} parts, which are.
function stripNonCode(text) {
  let out = "";
  let i = 0;
  const skipTo = (open, close) => {
    let j = text.indexOf(close, i + open.length);
    return j === -1 ? text.length : j + close.length;
  };
  while (i < text.length) {
    const rest = text.slice(i);
    if (rest.startsWith("//")) {
      i = skipTo("//", "\n");
      out += "\n";
      continue;
    }
    if (rest.startsWith("/*")) {
      i = skipTo("/*", "*/");
      out += " ";
      continue;
    }
    const quote = text[i];
    if (quote === '"' || quote === "'") {
      i += 1;
      while (i < text.length && text[i] !== quote) i += text[i] === "\\" ? 2 : 1;
      i += 1;
      out += '""';
      continue;
    }
    if (quote === "`") {
      i += 1;
      while (i < text.length && text[i] !== "`") {
        if (text[i] === "\\") {
          i += 2;
          continue;
        }
        // ${...} is code and is kept, braces and all.
        if (text[i] === "$" && text[i + 1] === "{") {
          let depth = 0;
          const start = i + 2;
          i += 2;
          while (i < text.length && (depth > 0 || text[i] !== "}")) {
            if (text[i] === "{") depth++;
            if (text[i] === "}") depth--;
            i += 1;
          }
          out += ` ${text.slice(start, i)} `;
          i += 1;
          continue;
        }
        i += 1;
      }
      i += 1;
      continue;
    }
    out += text[i];
    i += 1;
  }
  return out;
}

// The object the worker hands the module, against the keys the module reads
// out of it.
//
// This is a third contract, and the one that had nobody watching it. push.mjs
// drives the module with a transport it builds itself, so it proves the Go
// side and says nothing about the worker; the message contract above is
// between the page and the worker, which is a different pair. In between sat
// the object worker.js passes into api.upload, and when EPD-nRF5 was added to
// the module and to the page, the worker was left supplying only the two keys
// Gicisky needs. The tag connected, the notifications arrived, and the first
// write failed with "the page did not supply a write".
//
// The keys come out of main.go rather than a list here, because a list here
// would be one more copy to forget.
function checkTransportWiring() {
  const module = fs.readFileSync(path.join(root, "web", "wasm", "main.go"), "utf8");
  const worker = stripNonCode(fs.readFileSync(path.join(root, "web", "static", "worker.js"), "utf8"));

  const wanted = new Set();
  for (const [, key] of module.matchAll(/wiring\.Get\("(\w+)"\)/g)) wanted.add(key);
  if (wanted.size < 3) {
    console.log(`  FAIL  wiring: found ${wanted.size} keys in main.go, which cannot be right`);
    return 1;
  }

  // The transport literal, so a key named anywhere else in the worker does not
  // count as supplying one.
  const literal = worker.match(/transport:\s*\{([\s\S]*?)\n\s*\},/);
  if (!literal) {
    console.log("  FAIL  wiring: worker.js has no transport object");
    return 1;
  }
  const supplied = new Set();
  for (const [, key] of literal[1].matchAll(/(\w+):/g)) supplied.add(key);

  let wrong = 0;
  for (const key of wanted) {
    if (supplied.has(key)) continue;
    wrong++;
    console.log(`  FAIL  wiring: the module reads transport.${key} and worker.js never supplies it`);
  }
  console.log(`  wiring: ${wanted.size} keys read, ${wrong} unsupplied`);
  return wrong;
}

let failures = 0;

for (const name of FILES) {
  failures += check(name, fs.readFileSync(path.join(root, "web", "static", name), "utf8"));
}
failures += checkMessageContract();
failures += checkTransportWiring();
process.exit(failures === 0 ? 0 : 1);

function check(file, source) {
const code = stripNonCode(source);

// Names the file brings into scope. Parameters are included because a callback
// taking a function and calling it is ordinary.
const declared = new Set();
const add = (name) => name && declared.add(name);
for (const [, name] of code.matchAll(/\b(?:function|class)\s+([A-Za-z_$][\w$]*)/g)) add(name);
for (const [, name] of code.matchAll(/\b(?:const|let|var)\s+([A-Za-z_$][\w$]*)/g)) add(name);
for (const [, names] of code.matchAll(/\b(?:const|let|var)\s*\{([^}]*)\}/g)) {
  for (const part of names.split(",")) add(part.split(":").pop().trim().split("=")[0].trim());
}
for (const [, names] of code.matchAll(/\(([^()]*)\)\s*=>/g)) {
  for (const part of names.split(",")) add(part.trim().split(/[=:\s]/)[0].replace(/[{}[\].]/g, ""));
}
for (const [, names] of code.matchAll(/\bfunction\s*[\w$]*\s*\(([^()]*)\)/g)) {
  for (const part of names.split(",")) add(part.trim().split(/[=:\s]/)[0].replace(/[{}[\]]/g, ""));
}
for (const [, name] of code.matchAll(/\bcatch\s*\(\s*([A-Za-z_$][\w$]*)/g)) add(name);
// Shorthand methods in an object literal — `focus() { … }` — read as calls
// but are the definition of the thing being called.
for (const [, name] of code.matchAll(/(?:^|[,{;\s])([A-Za-z_$][\w$]*)\s*\([^()]*\)\s*\{/gm)) add(name);
for (const [, names] of code.matchAll(/^import\s*\{([\s\S]*?)\}\s*from/gm)) {
  for (const part of names.split(",")) add(part.trim());
}
for (const [, name] of code.matchAll(/^import\s+([A-Za-z_$][\w$]*)\s+from/gm)) add(name);

// The standard library, the browser, and the one global the Go runtime sets.
const KNOWN = new Set([
  "Array", "Blob", "Boolean", "Date", "Error", "File", "FileReader", "Function",
  "Go", "Image", "Intl", "JSON", "Map", "Math", "Number", "Object", "Promise",
  "Proxy", "Reflect", "RegExp", "Set", "String", "Symbol", "TextDecoder",
  "TextEncoder", "URL", "Uint8Array", "WeakMap", "WeakSet", "WebAssembly",
  "atob", "btoa", "clearInterval", "clearTimeout", "console", "decodeURI",
  "decodeURIComponent", "document", "encodeURI", "encodeURIComponent", "fetch",
  "globalThis", "isFinite", "isNaN", "localStorage", "location", "navigator",
  "parseFloat", "parseInt", "performance", "queueMicrotask",
  "requestAnimationFrame", "setInterval", "setTimeout", "structuredClone",
  "window", "AbortController", "AbortSignal", "Event", "CustomEvent",
  "Worker", "MessageChannel", "Blob", "importScripts", "self", "postMessage",
  "DOMException", "BluetoothUUID", "DataView", "ArrayBuffer",
  // Keywords and operators that a naive scan reads as calls.
  "if", "for", "while", "switch", "catch", "return", "typeof", "function",
  "await", "new", "delete", "void", "in", "of", "do", "else", "case", "yield",
  "async", "get", "set", "try", "throw",
]);

const missing = new Map();
const note = (name, index, how) => {
  if (declared.has(name) || KNOWN.has(name) || missing.has(name)) return;
  missing.set(name, { line: source.slice(0, index).split("\n").length, how });
};

// Called: escapeHTML(x).
for (const match of code.matchAll(/(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(/g)) {
  note(match[1], match.index, "called");
}

// Handed over to be called later: addEventListener("click", push). This is the
// one that got away — push was deleted by an edit that took the function above
// it too, and nothing noticed, because a callback is a reference and not a
// call. It reads as fine right up until the button is pressed.
for (const match of code.matchAll(
  /\.(?:addEventListener|removeEventListener)\s*\(\s*[^,]+,\s*([A-Za-z_$][\w$]*)\s*[,)]/g,
)) {
  note(match[1], match.index, "used as a listener");
}
for (const match of code.matchAll(/\.(?:then|catch|finally)\s*\(\s*([A-Za-z_$][\w$]*)\s*[,)]/g)) {
  note(match[1], match.index, "used as a promise callback");
}
for (const match of code.matchAll(/\.(?:map|filter|forEach|find|some|every|sort)\s*\(\s*([A-Za-z_$][\w$]*)\s*\)/g)) {
  note(match[1], match.index, "used as an array callback");
}

for (const [name, where] of missing) {
  console.log(
    `  FAIL  ${file}:${where.line}  ${name} is ${where.how} and never declared, imported or standard`,
  );
}
console.log(`  ${file}: ${declared.size} names in scope, ${missing.size} missing`);
return missing.size;
}

// The page and the worker only ever meet through postMessage, so nothing
// checks that they agree — a renamed op is a call that silently never answers,
// and a renamed kind is a message quietly dropped. Both sides are read for the
// names they use and the two sets are compared.
function checkMessageContract() {
  const app = fs.readFileSync(path.join(root, "web", "static", "app.js"), "utf8");
  const worker = fs.readFileSync(path.join(root, "web", "static", "worker.js"), "utf8");

  const names = (text, pattern) => new Set([...text.matchAll(pattern)].map((m) => m[1]));

  // Ops: the page calls them, the worker's switch answers them.
  const sent = names(app, /\bcall\("(\w+)"/g);
  const handled = names(worker, /^\s*case "(\w+)":/gm);
  // Kinds: the worker announces them, the page registers handlers for them.
  const announced = names(worker, /postMessage\(\{\s*kind:\s*"(\w+)"/g);
  const listened = names(app, /\.on\("(\w+)"/g);
  // And the reverse: kinds the page posts, which the worker reads by name.
  const posted = names(app, /post\(\{\s*kind:\s*"(\w+)"/g);
  const read = names(worker, /message\.kind === "(\w+)"/g);

  let wrong = 0;
  const compare = (label, from, to, fromName, toName) => {
    for (const name of from) {
      if (to.has(name)) continue;
      wrong++;
      console.log(`  FAIL  ${label}: ${fromName} uses "${name}" and ${toName} never handles it`);
    }
  };
  compare("op", sent, handled, "app.js", "worker.js");
  compare("kind", announced, listened, "worker.js", "app.js");
  compare("kind", posted, read, "app.js", "worker.js");

  console.log(
    `  contract: ${sent.size} ops, ${announced.size + posted.size} message kinds, ${wrong} unmatched`,
  );
  return wrong;
}
