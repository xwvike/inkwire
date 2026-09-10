// Checks that the editor can tell a half-written page from a finished one.
//
// The preview is not sent source that does not parse. A page is typed into,
// and on the way to being written it passes through states that are not yet
// anything — a tag with no closing bracket, a rule with no closing brace — and
// laying those out costs a full render to be told what the editor already
// knows, and answers with warnings about a page nobody has finished.
//
// That gate is only as good as Lezer's error nodes, so this pins what they do.
// Two of these look like failures and are not: a bare "<" is text in HTML, and
// a declaration with no value parses and is the renderer's to complain about.
// Both are cases where the page should go through, warnings and all.
//
// Usage: node web/verify/parsing.mjs
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const bundle = path.join(here, "..", "static", "vendor", "codemirror.js");
const { EditorState, syntaxTree, html, css } = await import(bundle);

// The same question app.js asks, asked the same way.
function unfinished(doc, language) {
  const state = EditorState.create({ doc, extensions: [language] });
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
  return broken;
}

const CASES = [
  ["a finished element", '<div class="a">hi</div>', "html", false],
  ["a tag still being typed", '<div class="a"', "html", true],
  ["an attribute whose quote is open", '<div class="a>hi</div>', "html", true],
  ["a lone bracket, which is text", "<", "html", false],
  // An empty HTML document reports an error node, which is a surprise and the
  // reason app.js does not put a blank document through this gate: clearing
  // the pane would otherwise leave the preview waiting for a page that was
  // never coming.
  ["nothing at all", "", "html", true],

  ["a finished rule", ".a { color: red; }", "css", false],
  ["a block still open", ".a { color: red;", "css", true],
  ["a property half typed", ".a { colo", "css", true],
  ["a value not yet given, which the renderer reports", ".a { color: }", "css", false],
  ["nothing at all", "", "css", false],
];

const LANGUAGES = { html: html(), css: css() };
let wrong = 0;
for (const [label, doc, kind, want] of CASES) {
  const got = unfinished(doc, LANGUAGES[kind]);
  if (got === want) continue;
  wrong++;
  console.log(`  FAIL  ${kind}: ${label} — reported ${got ? "unfinished" : "finished"}`);
}
console.log(`\n${CASES.length} documents, ${wrong} judged wrongly`);
process.exit(wrong === 0 ? 0 : 1);
