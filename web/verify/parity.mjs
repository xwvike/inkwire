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

// The panels whose size a page was written for. Naming one asks for its
// palette as well as its size, so that is where inks get flattened — a
// different code path from a bare viewport, and worth holding to the same
// standard.
const panels = JSON.parse(fs.readFileSync(path.join(staticDir, "panels.json"), "utf8"));
const panelsSized = (size) =>
	panels.filter((p) => p.width === size.width && p.height === size.height);

// A page the panel cannot take is still drawn: the command writes the
// picture and then exits non-zero, because what it looks like is what says
// which part of it has to change. The picture is the thing being compared,
// so a refusal is read from the file rather than from the exit status —
// and the module is held to refusing the same pages, below.
function cliRender(page, argv) {
	const out = path.join(os.tmpdir(), `parity-${process.pid}.png`);
	let refused = false;
	try {
		execFileSync(
			path.join(root, "web", "verify", "inkwire"),
			["render", ...argv, "-o", out, page],
			{ stdio: ["ignore", "ignore", "pipe"] },
		);
	} catch {
		refused = true;
		if (!fs.existsSync(out)) return { refused, bytes: null };
	}
	const bytes = fs.readFileSync(out);
	fs.unlinkSync(out);
	return { refused, bytes };
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

	// Once as a bare viewport, then once for every panel of that size.
	const targets = [
		{ label: `${size.width}x${size.height}`, key: "", argv: ["-size", `${size.width}x${size.height}`] },
		...panelsSized(size).map((p) => ({
			label: `${p.key} ${p.palette}`,
			key: p.key,
			argv: ["-panel", p.key],
		})),
	];

	for (const target of targets) {
		const rendered = api.render(
			source.markup, source.css, size.width, size.height, source.resources, target.key,
		);
		if (!rendered.png) {
			failed++;
			console.log(`  FAIL  ${relative}  ${target.label}  wasm drew nothing: ${rendered.error ?? "?"}`);
			continue;
		}

		const fromWasm = Buffer.from(rendered.png, "base64");
		const cli = cliRender(page, target.argv);
		checked++;
		const note = cli.refused ? " (both refused)" : "";
		if (cli.bytes && fromWasm.equals(cli.bytes) && rendered.ok !== cli.refused) {
			console.log(`  ok    ${relative}  ${target.label}  ${fromWasm.length}B${note}`);
		} else if (cli.bytes && fromWasm.equals(cli.bytes)) {
			failed++;
			console.log(
				`  FAIL  ${relative}  ${target.label}  same pixels but ` +
					`wasm ${rendered.ok ? "accepted" : "refused"} and cli ` +
					`${cli.refused ? "refused" : "accepted"}`,
			);
		} else {
			failed++;
			console.log(
				`  FAIL  ${relative}  ${target.label}  ` +
					`wasm ${fromWasm.length}B vs cli ${cli.bytes?.length ?? "none"}B`,
			);
		}
	}
}

console.log(`\n${toolchain}: ${checked} renders compared, ${failed} differing`);
process.exit(failed === 0 ? 0 : 1);
