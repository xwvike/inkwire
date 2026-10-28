// Package inkwire carries the manuals into the binary, so that whoever holds
// only the executable — someone who downloaded a release, or a model asked to
// write a page for a tag — can read how a page is written without the
// repository at hand. `inkwire docs` prints them.
//
// It lives at the root because that is where the manuals are, and embed reads
// only from a package's own directory down.
package inkwire

import "embed"

//go:embed README.md README.zh-CN.md MARKUP.md MARKUP.zh-CN.md
var docs embed.FS

// Doc is a manual as it stands in the repository: topic is "markup" or
// "readme", and lang is "en" or "zh-CN". It is false for any other.
func Doc(topic, lang string) ([]byte, bool) {
	name, known := map[string]string{"markup": "MARKUP", "readme": "README"}[topic]
	if !known {
		return nil, false
	}
	switch lang {
	case "en":
	case "zh-CN":
		name += ".zh-CN"
	default:
		return nil, false
	}
	content, err := docs.ReadFile(name + ".md")
	return content, err == nil
}
