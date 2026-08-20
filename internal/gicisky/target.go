package gicisky

import (
	"strings"
)

// MatchesTarget decides whether a device answers to the target given. An empty
// target takes any tag of this family, which is what a single-tag setup wants.
//
// It used to mean one particular address — the tag this was written against —
// so anybody else running this with no argument was addressing hardware they
// did not have.
func MatchesTarget(target, name, address string) bool {
	if target == "" {
		return looksLikeTag(name)
	}
	if strings.EqualFold(name, target) || strings.EqualFold(address, target) {
		return true
	}
	// A MAC never equals the address on a host that does not expose one:
	// CoreBluetooth substitutes a per-host UUID, so every MAC target would
	// otherwise be unreachable on macOS. The advertised name is derived from
	// the MAC, so match on that instead.
	//
	// TargetName is deliberately not accepted here. Every tag advertises it
	// while powering up, so honouring it for a MAC target would let a write
	// aimed at one tag land on whichever tag happened to be booting. Ask for
	// it by name if that is genuinely what you want.
	if derived, ok := advertisedName(target); ok {
		return strings.EqualFold(name, derived)
	}
	return false
}

// advertisedName derives the name a tag settles on from its MAC. Gicisky tags
// are addressed FF:FF:xx:yy:zz:kk and advertise NEMRxxyyzzkk, which makes the
// name the one identifier that is both unique per tag and identical on every
// host that sees it.
//
// It reports false for anything that is not a complete MAC, so names and
// CoreBluetooth UUIDs pass through untouched.
func advertisedName(target string) (string, bool) {
	cleaned := strings.NewReplacer(":", "", "-", "").Replace(target)
	if len(cleaned) != macHexDigits {
		return "", false
	}
	for _, digit := range cleaned {
		if !isHexDigit(digit) {
			return "", false
		}
	}
	return "NEMR" + strings.ToUpper(cleaned[macHexDigits-8:]), true
}

// every tag and so identifies none of them.
const macHexDigits = 12

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
