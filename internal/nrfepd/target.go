package nrfepd

import "strings"

// Naming, which is what a tag can be picked out by before anything connects.
// It is kept apart from the code that connects so that a browser build, which
// has no radio, still has the vocabulary for talking about which tag is meant.

// NamePrefix is what this firmware advertises: the name is DEVICE_NAME
// with the last two bytes of the address after it, so every tag of this
// family starts the same way and none of them share a whole name.
//
// Unlike a Gicisky tag, the name is all there is. Nothing in the
// advertisement says what panel is attached; that is kept in the
// firmware's own flash and only comes out once it is asked.
const NamePrefix = "NRF_EPD"

// LooksLikeName reports whether an advertised name follows this firmware's
// convention. It answers a weaker question than Advertises and is kept for the
// places that have a name and nothing else, such as a target somebody typed.
func LooksLikeName(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), NamePrefix)
}

// MatchesTarget decides whether a device answers to the target given. An empty
// target takes any tag of this family, which is what a single-tag setup wants;
// anything else is matched by name or by address.
//
// Unlike the other family there is nothing to derive: this firmware's name
// carries the last two bytes of the address rather than the whole of it, so a
// full address cannot be turned into the name it implies.
func MatchesTarget(target, name, address string) bool {
	if target == "" {
		return LooksLikeName(name)
	}
	return strings.EqualFold(name, target) || strings.EqualFold(address, target)
}
