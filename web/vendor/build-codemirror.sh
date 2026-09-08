#!/bin/sh
# Rebuilds web/static/vendor/codemirror.js.
#
# The bundle is committed rather than fetched at load time, so the editor works
# offline and no page load reaches a third-party host. That means this script
# runs on somebody's machine now and then rather than in CI, and everything it
# needs — npm, the packages, esbuild — is installed into a temporary directory
# and thrown away. Nothing it touches stays in the repository except the one
# file it writes.
#
# Usage: web/vendor/build-codemirror.sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
out="$root/web/static/vendor/codemirror.js"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cp "$root/web/vendor/entry.js" "$work/entry.js"
cd "$work"

npm init -y >/dev/null 2>&1
npm install --silent --no-audit --no-fund \
	codemirror@^6 \
	@codemirror/lang-html@^6 \
	@codemirror/lang-css@^6 \
	@codemirror/language@^6 \
	@codemirror/autocomplete@^6 \
	@codemirror/state@^6 \
	@codemirror/view@^6 \
	@codemirror/commands@^6 \
	@codemirror/search@^6 \
	@lezer/highlight@^1 \
	esbuild

./node_modules/.bin/esbuild entry.js \
	--bundle --format=esm --minify --legal-comments=none --target=es2020 \
	--outfile=bundle.js

{
	echo '// CodeMirror 6, bundled for Inkwire Studio. MIT licensed; see LICENSE.md'
	echo '// beside this file for the notice and the exact package versions.'
	echo '// Regenerate with web/vendor/build-codemirror.sh — do not edit by hand.'
	cat bundle.js
} >"$out"

# The versions that went in are part of what the bundle is, so they are written
# back into the notice rather than left to whoever remembers to look.
node -e '
const fs = require("fs");
const deps = Object.keys(require("./package.json").dependencies).filter((n) => n !== "esbuild");
const rows = deps.sort().map((name) => {
  const version = require(`./node_modules/${name}/package.json`).version;
  return `| \`${name}\` | ${version} |`;
}).join("\n");
const file = process.argv[1];
const text = fs.readFileSync(file, "utf8");
fs.writeFileSync(file, text.replace(
  /\| Package \| Version \|\n\|---\|---\|\n(?:\|.*\|\n)+/,
  `| Package | Version |\n|---|---|\n${rows}\n`,
));
' "$root/web/vendor/LICENSE.md"

wc -c <"$out" | awk '{printf "codemirror.js: %d bytes (%.0f KB)\n", $1, $1/1024}'
