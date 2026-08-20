// Package tag names the two families of e-paper tag this program drives, and
// decides which one a target belongs to.
//
// The rule has nowhere else to live. Neither family package can own it,
// because each knows only itself; the shared Bluetooth layer deliberately
// knows about neither. So it sat inline in the command line and again in the
// service, one copy each, until the two drifted over what an unstated family
// looks like — a flag left at its default says "auto", a query parameter that
// was never sent says nothing at all, and only one copy accepted both.
package tag

import (
	"fmt"

	"github.com/xwvike/inkwire/internal/nrfepd"
)

// The two families a target can belong to. They are strings rather than a type
// because they leave this program as JSON and arrive as a query parameter.
const (
	Gicisky = "gicisky"
	NRFEPD  = "nrfepd"
)

// Resolve decides which family a target belongs to.
//
// An unstated request and "auto" are the same question. The answer comes from
// the name, because that is the only part of a target that carries a family: an
// address does not say who made the tag it belongs to.
//
// Gicisky is the fallback rather than a third answer. There is no name shape
// that identifies it — a Gicisky tag is called NEMR<mac> or PICKSMART, and
// neither is something a person types when they mean a specific tag — so a
// target that does not announce itself as the other family is assumed to be
// this one, and is told plainly when it turns out not to be there.
func Resolve(requested, target string) (string, error) {
	switch requested {
	case Gicisky, NRFEPD:
		return requested, nil
	case "", "auto":
		if nrfepd.LooksLikeName(target) {
			return NRFEPD, nil
		}
		return Gicisky, nil
	}
	return "", fmt.Errorf("unknown family %q: use auto, %s or %s", requested, Gicisky, NRFEPD)
}
