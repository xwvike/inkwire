// Checks that the wasm module draws what the CLI draws.
//
// This is the whole basis of the editor: a preview is only worth having if it
// is the panel's own picture rather than a browser's guess at it. Both sides
// run the same Go, so the PNG bytes should be identical — not close, identical
// — and anything less means the module and the command have diverged.
//
// Usage: node web/verify/parity.mjs [go|tinygo]
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const staticDir = path.join(root, "web", "static");

const require = createRequire(import.meta.url);
require(path.join(staticDir, "wasm_exec.js"));

// Files a page can reach. A browser has no directory beside the page, so
// everything it links or draws has to be handed over by name.
const RESOURCE_TYPES = new Set([".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp"]);

async function boot() {
	const go = new Go();
	const module = await WebAssembly.instantiate(
		fs.readFileSync(path.join(staticDir, "inkwire.wasm")),
		go.importObject,
	);
	const ready = new Promise((resolve) => {
		globalThis.inkwireReady = resolve;
	});
	// go.run never resolves: main blocks so the exported functions stay alive.
	go.run(module.instance);
	await ready;
	return globalThis.inkwire;
}

function pages() {
	const found = [];
	const walk = (dir) => {
		for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
			const full = path.join(dir, entry.name);
			if (entry.isDirectory()) walk(full);
			else if (entry.name.endsWith(".html")) found.push(full);
		}
	};
	walk(path.join(root, "examples"));
	return found.sort();
}

// inputs gathers a page the way the CLI would see it: the markup, the
// stylesheet named after it, and every picture in its directory.
function inputs(page) {
	const dir = path.dirname(page);
	const stylesheet = page.replace(/\.html$/, ".css");
	// Keyed by the path the page writes, subdirectories included: a page says
	// src="assets/photo.png" and that string is the whole of the name it has.
	const resources = {};
	const collect = (from) => {
		for (const entry of fs.readdirSync(from, { withFileTypes: true })) {
			const full = path.join(from, entry.name);
			if (entry.isDirectory()) collect(full);
			else if (RESOURCE_TYPES.has(path.extname(entry.name).toLowerCase())) {
				resources[path.relative(dir, full)] = new Uint8Array(fs.readFileSync(full));
			}
		}
	};
	collect(dir);
	return {
		markup: fs.readFileSync(page, "utf8"),
		css: fs.existsSync(stylesheet) ? fs.readFileSync(stylesheet, "utf8") : "",
		resources,
	};
}

// declaredSize reads the viewport the page asks for, so both sides are given
// the same one. A render needs a target named to it; the page's own size is
// the target its examples were written against.
function declaredSize(api, { markup, css, resources }) {
	const compiled = api.compile(markup, css, 0, 0, resources);
	if (!compiled.ok) return null;
	const size = JSON.parse(compiled.json).size;
	return size && size.width > 0 && size.height > 0 ? size : null;
}

function cliRender(page, size) {
	const out = path.join(os.tmpdir(), `parity-${process.pid}.png`);
	execFileSync(
		path.join(root, "web", "verify", "inkwire"),
		["render", "-size", `${size.width}x${size.height}`, "-o", out, page],
		{ stdio: ["ignore", "ignore", "pipe"] },
	);
	const bytes = fs.readFileSync(out);
	fs.unlinkSync(out);
	return bytes;
}

const api = await boot();
const toolchain = process.argv[2] ?? "go";
let checked = 0;
let failed = 0;

for (const page of pages()) {
	const relative = path.relative(root, page);
	const source = inputs(page);
	const size = declaredSize(api, source);
	if (!size) {
		console.log(`  skip  ${relative}  (no declared size)`);
		continue;
	}

	const rendered = api.render(source.markup, source.css, size.width, size.height, source.resources);
	if (!rendered.png) {
		failed++;
		console.log(`  FAIL  ${relative}  wasm produced no picture: ${rendered.error ?? "?"}`);
		continue;
	}

	const fromWasm = Buffer.from(rendered.png, "base64");
	const fromCLI = cliRender(page, size);
	checked++;
	if (fromWasm.equals(fromCLI)) {
		console.log(`  ok    ${relative}  ${size.width}x${size.height}  ${fromWasm.length}B`);
	} else {
		failed++;
		console.log(
			`  FAIL  ${relative}  ${size.width}x${size.height}  ` +
				`wasm ${fromWasm.length}B vs cli ${fromCLI.length}B`,
		);
	}
}

console.log(`\n${toolchain}: ${checked} pages compared, ${failed} differing`);
process.exit(failed === 0 ? 0 : 1);
