// The editor surface Inkwire Studio uses, bundled into one file.
// Regenerate with web/vendor/build-codemirror.sh.
export { EditorState, Compartment, StateEffect, Facet } from "@codemirror/state";
export {
  EditorView, keymap, lineNumbers, highlightActiveLine,
  highlightActiveLineGutter, drawSelection, rectangularSelection,
  crosshairCursor, highlightSpecialChars, dropCursor,
} from "@codemirror/view";
export {
  defaultKeymap, history, historyKeymap, indentWithTab,
} from "@codemirror/commands";
export {
  syntaxHighlighting, HighlightStyle, indentUnit, bracketMatching,
  foldGutter, indentOnInput, syntaxTree, ensureSyntaxTree,
} from "@codemirror/language";
export {
  autocompletion, completionKeymap, closeBrackets, closeBracketsKeymap,
} from "@codemirror/autocomplete";
export { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
export { tags } from "@lezer/highlight";
export { html } from "@codemirror/lang-html";
export { css } from "@codemirror/lang-css";
