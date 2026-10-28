package display

import "unicode"

// BreakBetween reports whether a line wrapping words may end between a and b,
// where before is the character ahead of a (or zero at the start of the text).
//
// These are the places a browser breaks ordinary text, cut down to what a
// panel's text has in it:
//
//   - after a space, and never before one, so a space at the end of a line
//     stays with the word ahead of it and hangs past the edge;
//   - after a hyphen or dash inside a word, so "e-ink" may end a line at
//     "e-", though not one ahead of a digit, so "2026-10-26" holds together;
//   - between characters of Chinese, Japanese and Korean, which are not
//     written with spaces, except where that would start a line with closing
//     punctuation or end one with an opening bracket.
//
// Everywhere else — inside a word, and between a word and the punctuation
// after it — the line holds together.
func BreakBetween(before, a, b rune) bool {
	switch {
	case isSpace(b):
		return false
	case isSpace(a):
		return true
	case isHyphen(a):
		// Not before a digit, so a date or a range of numbers holds together.
		return isWordRune(before) && unicode.IsLetter(b)
	case a == '—':
		return true
	case isCJK(a) || isCJK(b):
		return !noBreakBefore(b) && !noBreakAfter(a)
	}
	return false
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '　' }

func isHyphen(r rune) bool { return r == '-' || r == '‐' || r == '–' }

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// isCJK reports whether a character belongs to the scripts written without
// spaces between words, or is their punctuation.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK symbols and punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // fullwidth and halfwidth forms
}

// noBreakBefore is the punctuation a line may not start with: what closes, and
// the marks that belong to the character ahead of them.
func noBreakBefore(r rune) bool {
	switch r {
	case '，', '。', '、', '．', '！', '？', '；', '：', '）', '」', '』', '】', '〕', '〉', '》', '”', '’',
		'…', '‥', 'ー', '・', '々', '〻', 'ぁ', 'ぃ', 'ぅ', 'ぇ', 'ぉ', 'っ', 'ゃ', 'ゅ', 'ょ', 'ゎ',
		'ァ', 'ィ', 'ゥ', 'ェ', 'ォ', 'ッ', 'ャ', 'ュ', 'ョ', 'ヮ', 'ヵ', 'ヶ',
		',', '.', ';', ':', '!', '?', ')', ']', '}', '%':
		return true
	}
	return false
}

// noBreakAfter is the punctuation a line may not end with: what opens.
func noBreakAfter(r rune) bool {
	switch r {
	case '（', '「', '『', '【', '〔', '〈', '《', '“', '‘', '(', '[', '{':
		return true
	}
	return false
}
