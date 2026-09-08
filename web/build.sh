#!/bin/sh
# Builds the renderer into the wasm module the page loads.
#
# Two toolchains can compile it and only one of them works. Go is what the CLI
# is built with, so what it emits is the renderer exactly as tested. TinyGo
# emits a quarter of the bytes, links without complaint, and then panics inside
# douceur's declaration parser on the first page it is given — regexp under
# TinyGo is not the regexp this depends on. It is left selectable so the next
# person to hope for 4 MB can find out in one command instead of an afternoon.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
out="$root/web/static"
toolchain=${1:-go}

# The panel catalogue lives in the driver packages, which reach the radio and
# cannot be compiled for a browser. It is generated rather than copied so that
# a model added to the drivers reaches the editor by rebuilding, not by anyone
# remembering to keep a second list in step.
go run "$root/web/tools/panels" -o "$out/panels.json"

# The completion vocabulary is read out of MARKUP.md for the same reason:
# a stock CSS list would propose most of a browser's properties, and this
# renderer implements 89 of them. web/verify/completions.mjs puts every
# entry it emits through the renderer.
go run "$root/web/tools/completions" -o "$out/completions.json"

case "$toolchain" in
go)
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -trimpath \
		-o "$out/inkwire.wasm" "$root/web/wasm"
	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/wasm_exec.js"
	;;
tinygo)
	echo "warning: the TinyGo build panics at runtime; see the comment above" >&2
	tinygo build -target wasm -opt=z -no-debug \
		-o "$out/inkwire.wasm" "$root/web/wasm"
	cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" "$out/wasm_exec.js"
	;;
*)
	echo "usage: $0 [go|tinygo]" >&2
	exit 2
	;;
esac

wc -c <"$out/inkwire.wasm" | awk -v t="$toolchain" '{printf "%s: %d bytes (%.2f MB)\n", t, $1, $1/1000000}'
