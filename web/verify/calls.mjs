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
const file = path.join(root, "web", "static", "app.js");
const source = fs.readFileSync(file, "utf8");

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
  "window",
  // Keywords and operators that a naive scan reads as calls.
  "if", "for", "while", "switch", "catch", "return", "typeof", "function",
  "await", "new", "delete", "void", "in", "of", "do", "else", "case", "yield",
  "async", "get", "set", "try", "throw",
]);

const missing = new Map();
for (const match of code.matchAll(/(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(/g)) {
  const name = match[1];
  if (declared.has(name) || KNOWN.has(name)) continue;
  const line = source.slice(0, match.index).split("\n").length;
  if (!missing.has(name)) missing.set(name, line);
}

for (const [name, line] of missing) {
  console.log(`  FAIL  app.js:${line}  ${name}() is called and never declared, imported or standard`);
}
console.log(
  `\n${declared.size} names in scope, ${missing.size} called but undefined`,
);
process.exit(missing.size === 0 ? 0 : 1);
