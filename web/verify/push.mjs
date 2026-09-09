// Drives a complete upload into a tag that is not there.
//
// The push path had no check at all until its first bug, which was that the
// resource map was read from the wrong argument: the transport object was
// handed over as the page's files, so every picture was missing and every page
// went out blank. From outside it looked like a successful upload — the right
// number of bytes, every part acknowledged, the tag refreshing — and the only
// way to see it was to walk over and look at the tag.
//
// So: a stub that answers the way the firmware answers, and an assertion that
// what it received is exactly what payload() says should have been sent.
//
// Usage: node web/verify/push.mjs
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..", "..");
const staticDir = path.join(root, "web", "static");

const require = createRequire(import.meta.url);
require(path.join(staticDir, "wasm_exec.js"));

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

// The tag's side of internal/gicisky/protocol.go.
//
// Stage 1 asks how big a message it will take. Stage 2 states the length.
// Stage 3 starts the transfer, after which the tag names the part it wants and
// is given it, until there are none left and it says so.
function stubTag({ messageSize = 244 } = {}) {
  const blockSize = messageSize - 4;
  const parts = [];
  let expected = 0;
  let total = 0;
  let notify = null;

  // The firmware answers over a radio, never on the caller's stack. Deferring
  // keeps the module's goroutine parked on its channel the way a real one is,
  // which is the part of this worth simulating.
  const answer = (bytes) => queueMicrotask(() => notify?.(new Uint8Array(bytes)));
  const le32 = (n) => [n & 0xff, (n >> 8) & 0xff, (n >> 16) & 0xff, (n >> 24) & 0xff];

  return {
    attach(fn) {
      notify = fn;
    },
    writeControl(bytes) {
      const command = new Uint8Array(bytes);
      switch (command[0]) {
        case 0x01:
          answer([0x01, messageSize & 0xff, (messageSize >> 8) & 0xff]);
          break;
        case 0x02:
          total = command[1] | (command[2] << 8) | (command[3] << 16) | (command[4] << 24);
          answer([0x02]);
          break;
        case 0x03:
          answer([0x05, 0x00, ...le32(0)]);
          break;
        default:
          throw new Error(`the tag was sent an unknown control byte 0x${command[0].toString(16)}`);
      }
      return Promise.resolve();
    },
    writeData(bytes) {
      const frame = new Uint8Array(bytes);
      const part = frame[0] | (frame[1] << 8) | (frame[2] << 16) | (frame[3] << 24);
      if (part !== expected) throw new Error(`the tag was sent part ${part}, wanting ${expected}`);
      parts[part] = frame.slice(4);
      expected = part + 1;
      if (expected * blockSize >= total) answer([0x05, 0x08]);
      else answer([0x05, 0x00, ...le32(expected)]);
      return Promise.resolve();
    },
    received() {
      return Buffer.concat(parts.map((p) => Buffer.from(p)));
    },
  };
}

// The tag's side of internal/nrfepd/session.go.
//
// This family says nothing until it is asked. An init draws two answers out of
// it — a binary configuration blob naming the model, and a line of text about
// the link — and only then does the session know what shape of page to ask
// for. Everything after that is frames on the same characteristic.
function stubNRFEPD({ modelID = 0x03, mtu = 244 } = {}) {
  let notify = null;
  let frames = 0;
  let refreshed = false;
  const answer = (bytes) => queueMicrotask(() => notify?.(new Uint8Array(bytes)));

  // epd_config_t: pins, then the model at byte 7, then more pins and modes.
  const config = [2, 3, 4, 5, 6, 7, 8, modelID, 9, 10, 11, 0, 0];

  return {
    attach(fn) {
      notify = fn;
    },
    write(bytes) {
      const frame = new Uint8Array(bytes);
      if (frame[0] === 0x01) {
        answer(config);
        answer([...`mtu=${mtu} rle=1`].map((c) => c.charCodeAt(0)));
      } else if (frame[0] === 0x05) {
        refreshed = true;
      } else {
        frames++;
      }
      return Promise.resolve();
    },
    saw() {
      return { frames, refreshed };
    },
  };
}

// A page whose whole content is a picture, so that a resource going missing is
// the difference between a page and a blank one — which is the bug this exists
// for.
const DRAWING = Buffer.from(
  `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80" viewBox="0 0 120 80">
     <rect x="4" y="4" width="112" height="72" fill="none" stroke="black" stroke-width="4"/>
     <polyline points="16,64 44,24 72,48 104,16" fill="none" stroke="red" stroke-width="5"/>
   </svg>`,
  "utf8",
);

const CASES = [
  {
    name: "a page of its own",
    markup: `<div class="p"><h1>Inkwire</h1><p>pushed from a browser</p></div>`,
    css: `.p{width:296px;height:128px;background:white;padding:12px}
          h1{margin:0;font-size:32px}p{margin:0;font-size:14px;color:red}`,
    files: {},
  },
  {
    name: "a page that is a picture",
    markup: `<div class="p"><img src="chart.svg"></div>`,
    css: `.p{width:296px;height:128px;background:white;display:flex;
             align-items:center;justify-content:center}
          img{width:240px;height:100px}`,
    files: { "chart.svg": new Uint8Array(DRAWING) },
  },
];

const PANEL = "gicisky:0x0033";
let failed = 0;

for (const testCase of CASES) {
  const expected = globalThis.inkwire.payload({
    markup: testCase.markup,
    css: testCase.css,
    files: testCase.files,
    panel: PANEL,
  });
  if (!expected.ok) {
    failed++;
    console.log(`  FAIL  ${testCase.name}: payload refused: ${expected.error}`);
    continue;
  }
  const wanted = Buffer.from(expected.bytes, "base64");

  const tag = stubTag();
  const session = globalThis.inkwire.upload({
    markup: testCase.markup,
    css: testCase.css,
    files: testCase.files,
    panel: PANEL,
    transport: {
      writeControl: (bytes) => tag.writeControl(bytes),
      writeData: (bytes) => tag.writeData(bytes),
    },
  });
  if (!session.ok) {
    failed++;
    console.log(`  FAIL  ${testCase.name}: upload refused: ${session.error}`);
    continue;
  }
  tag.attach(session.notify);

  try {
    await session.done;
  } catch (error) {
    failed++;
    console.log(`  FAIL  ${testCase.name}: upload failed: ${error?.message ?? error}`);
    continue;
  }

  const arrived = tag.received();
  // A page that packs to nothing but paper would pass a comparison against
  // itself, so blankness is refused on its own terms as well.
  const blank = wanted.every((byte) => byte === wanted[0]);
  if (arrived.equals(wanted) && !blank) {
    console.log(`  ok    ${testCase.name}  ${arrived.length}B, ${expected.panel}`);
  } else {
    failed++;
    console.log(
      `  FAIL  ${testCase.name}  ` +
        (blank
          ? "the page packed to a single repeated byte, so it drew nothing"
          : `tag received ${arrived.length}B, payload says ${wanted.length}B`),
    );
  }
}

// EPD-nRF5, where the panel is not chosen but reported. Nothing is named here:
// the stub says it is model 0x03 and the page is drawn for whatever that is,
// which is the whole difference between the two families.
{
  const tag = stubNRFEPD({ modelID: 0x03 });
  const session = globalThis.inkwire.upload({
    markup: CASES[0].markup,
    css: CASES[0].css,
    files: {},
    family: "nrfepd",
    // The real wait is thirty seconds of the panel drawing, which is a fact
    // about e-paper rather than anything under test here.
    settleMs: 50,
    transport: { write: (bytes) => tag.write(bytes) },
  });
  if (!session.ok) {
    failed++;
    console.log(`  FAIL  EPD-nRF5: upload refused: ${session.error}`);
  } else {
    tag.attach(session.notify);
    try {
      await session.done;
      const { frames, refreshed } = tag.saw();
      // 400x300 black and colour planes at 240 bytes a frame is well over a
      // hundred; the number that matters is that it is not none.
      if (frames > 0 && refreshed) {
        console.log(`  ok    EPD-nRF5 model 0x03  ${frames} frames, then refresh`);
      } else {
        failed++;
        console.log(`  FAIL  EPD-nRF5: ${frames} frames, refresh ${refreshed}`);
      }
    } catch (error) {
      failed++;
      console.log(`  FAIL  EPD-nRF5: ${error?.message ?? error}`);
    }
  }
}

console.log(`\n${CASES.length + 1} uploads driven into a stub tag, ${failed} wrong`);
process.exit(failed === 0 ? 0 : 1);
