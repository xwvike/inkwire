// The renderer, off the thread the editor types on.
//
// Rendering is one synchronous call into Go: parse the page, cascade the CSS,
// lay it out, draw it, encode a PNG. On the page's own thread that is a freeze,
// and it is not a small one — a 296x128 starter costs about 9 ms, the same page
// with a picture in it about 45, and a 960x640 panel about 100. Debouncing only
// changes how often the freeze happens; it still lands in the middle of typing.
//
// So the module lives here and the page asks it questions. Everything it
// answers is data, and every call is one message with an id, so a reply that
// arrives after a newer one has been asked for can be dropped rather than
// painted over the newer answer.
//
// Pushing is the exception it cannot be, because a GATT characteristic only
// exists on the page's thread. The uploader still runs here — it is the tested
// Go, and splitting the protocol across two threads to avoid one message type
// would be a poor trade — and its writes are proxied back: writeControl returns
// a promise that settles when the page says the characteristic took the bytes.

importScripts("wasm_exec.js");

let api = null;

// Writes the uploader has sent to the page and is waiting to hear about.
const pendingWrites = new Map();
let nextWrite = 0;

// The upload in progress, if any. Notifications arrive from the page as their
// own messages rather than as replies, because the tag sends them when it likes
// rather than in answer to anything.
let session = null;

const ready = new Promise((resolve) => {
  self.inkwireReady = resolve;
});

const started = (async () => {
  const go = new Go();
  const module = await WebAssembly.instantiateStreaming(
    fetch("inkwire.wasm"),
    go.importObject,
  ).catch(async () => {
    const bytes = await (await fetch("inkwire.wasm")).arrayBuffer();
    return WebAssembly.instantiate(bytes, go.importObject);
  });
  // go.run never resolves: main blocks so the exported functions stay alive.
  go.run(module.instance);
  await ready;
  api = self.inkwire;
})();

// proxyWrite hands a characteristic write to the page and waits for it. The
// uploader's goroutine parks on this the same way it parks on a real radio.
function proxyWrite(which, bytes) {
  const id = ++nextWrite;
  return new Promise((resolve, reject) => {
    pendingWrites.set(id, { resolve, reject });
    // The bytes are copied rather than transferred: Go still owns the buffer it
    // handed over, and detaching it would pull the ground from under the
    // goroutine that is about to be resumed.
    self.postMessage({ kind: "write", id, which, bytes: bytes.slice() });
  });
}

self.onmessage = async (event) => {
  const message = event.data;

  // A write the page has finished with. Not a call, so it is answered here
  // rather than falling through to the dispatch below.
  if (message.kind === "wrote") {
    const waiting = pendingWrites.get(message.id);
    pendingWrites.delete(message.id);
    if (!waiting) return;
    if (message.error) waiting.reject(new Error(message.error));
    else waiting.resolve();
    return;
  }

  // A value the tag reported. It belongs to whatever upload is running.
  if (message.kind === "notify") {
    session?.notify(message.bytes);
    return;
  }

  await started;
  const { id, op, request } = message;
  try {
    switch (op) {
      case "render":
      case "compile":
      case "measure":
      case "payload": {
        self.postMessage({ id, ok: true, result: api[op](request) });
        return;
      }
      case "identify": {
        self.postMessage({ id, ok: true, result: api.identify(request.bytes) });
        return;
      }
      case "upload": {
        session = api.upload({
          ...request,
          transport: {
            writeControl: (bytes) => proxyWrite("control", bytes),
            writeData: (bytes) => proxyWrite("data", bytes),
            log: (text) => self.postMessage({ kind: "log", text }),
          },
        });
        if (!session.ok) {
          self.postMessage({ id, ok: true, result: session });
          session = null;
          return;
        }
        // The answer to "did it arrive" is the one worth waiting for, so the
        // reply is held until the upload settles rather than sent when it
        // starts. What the page needs meanwhile comes as log messages.
        self.postMessage({ kind: "sending", payloadBytes: session.payloadBytes, panel: session.panel });
        try {
          await session.done;
          self.postMessage({ id, ok: true, result: { ok: true, payloadBytes: session.payloadBytes } });
        } finally {
          session = null;
          // Anything still waiting cannot be answered now.
          for (const [, waiting] of pendingWrites) waiting.reject(new Error("the upload ended"));
          pendingWrites.clear();
        }
        return;
      }
      default:
        self.postMessage({ id, ok: false, error: `the worker has no ${op}` });
    }
  } catch (error) {
    self.postMessage({ id, ok: false, error: String(error?.message ?? error) });
  }
};
