package tag

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xwvike/inkwire/internal/ble"
	"github.com/xwvike/inkwire/internal/gicisky"
	"github.com/xwvike/inkwire/internal/nrfepd"
	"tinygo.org/x/bluetooth"
)

// Found is the tag a request resolved to and the family it turned out to be.
// Exactly one of the two device fields is meaningful; Family says which.
type Found struct {
	Family  string
	Gicisky gicisky.FoundDevice
	NRFEPD  nrfepd.FoundDevice
}

func (f Found) Name() string {
	if f.Family == NRFEPD {
		return f.NRFEPD.Name
	}
	return f.Gicisky.Name
}

func (f Found) Address() string {
	if f.Family == NRFEPD {
		return f.NRFEPD.Address.String()
	}
	return f.Gicisky.Address.String()
}

func (f Found) String() string {
	if name := f.Name(); name != "" {
		return fmt.Sprintf("%s (%s, %s)", name, f.Address(), f.Family)
	}
	return fmt.Sprintf("%s (%s)", f.Address(), f.Family)
}

// Sighting is one tag a scan saw, for the errors that have to say what was
// there instead of what was asked for.
type Sighting struct {
	Family, Name, Address string
	RSSI                  int16
}

func (s Sighting) String() string {
	label := s.Name
	if label == "" {
		label = s.Address
	}
	return fmt.Sprintf("%s (%s, %s, %d dBm)", label, s.Address, s.Family, s.RSSI)
}

// Locate listens once and works out which tag to write to.
//
// The family is discovered rather than guessed. Both families put something in
// their advertisements that identifies them — one a company id, the other a
// service UUID — so one pass of a promiscuous scan answers "which tag" and
// "whose tag" together. Deciding from the shape of a name could not: an address
// carries no family at all, which is what the default target used to be.
//
// An asserted family is obeyed and then checked. Somebody who says -family has
// a reason, so the search happens in that family; but a target that turns out
// to belong to the other one is a mistake worth naming rather than a write to
// whatever answered.
//
// An empty target means the one tag in range. Several is not an invitation to
// pick.
func Locate(ctx context.Context, adapter *bluetooth.Adapter, timeout time.Duration,
	target, asserted string) (Found, error) {
	// Checked before the scan rather than after it: a misspelled family, or
	// no target at all, is not worth a listening window to find out about.
	if err := ValidateFamily(asserted); err != nil {
		return Found{}, err
	}
	if target == "" {
		return Found{}, ErrNoDevice
	}

	tags, others := gicisky.NewCollector(), nrfepd.NewCollector()

	satisfied := stopFor(target, asserted, tags.Devices, others.Devices)
	err := ble.Scan(ctx, adapter, timeout, func(result bluetooth.ScanResult) {
		tags.Observe(result)
		others.Observe(result)
	}, satisfied)
	if err != nil {
		return Found{}, err
	}
	return Choose(tags.Devices(), others.Devices(), target, asserted)
}

// stopFor decides when a scan has heard enough: the tag that was named has
// answered, and answered completely. Nothing waits out the window any more,
// because nothing is enumerating — the target is always known before the scan
// starts.
func stopFor(target, asserted string, tags func() []gicisky.FoundDevice, others func() []nrfepd.FoundDevice) func() bool {
	return func() bool { return usable(Choose(tags(), others(), target, asserted)) }
}

// usable reports whether a match is complete enough to stop listening for.
//
// A Gicisky tag is not, until it has sent its manufacturer data: the name
// arrives in one advertisement and the model in another, and stopping at the
// first would end the scan holding a tag whose panel is still unknown.
func usable(found Found, err error) bool {
	if err != nil {
		return false
	}
	return found.Family != Gicisky || found.Gicisky.HasAdvertised
}

// ValidateFamily reports whether a stated family is one this program drives,
// so a caller can refuse a misspelling before listening for anything.
func ValidateFamily(asserted string) error {
	_, err := assertedFamily(asserted)
	return err
}

func assertedFamily(asserted string) (string, error) {
	switch asserted {
	case "", "auto":
		return "", nil
	case Gicisky, NRFEPD:
		return asserted, nil
	}
	return "", fmt.Errorf("unknown family %q: use auto, %s or %s", asserted, Gicisky, NRFEPD)
}

// Choose is the decision Locate makes once a scan is in.
//
// It is separate from the scan for two callers. The service supplies its own
// results, because it has hooks where a radio would be and can then be tested
// without one. And every way this can go is reachable from a test, which is
// not true of anything holding a radio.
func Choose(tags []gicisky.FoundDevice, others []nrfepd.FoundDevice, target, asserted string) (Found, error) {
	family, err := assertedFamily(asserted)
	if err != nil {
		return Found{}, err
	}
	// Which tag is not something to work out. A scan that found one tag today
	// finds two the day somebody brings another into the room, and the write
	// that used to go to the right one goes wherever. Naming it is the whole
	// of the guarantee.
	if target == "" {
		return Found{}, ErrNoDevice
	}
	var matches []Found
	if family == "" || family == Gicisky {
		for _, device := range tags {
			if gicisky.MatchesTarget(target, device.Name, device.Address.String()) {
				matches = append(matches, Found{Family: Gicisky, Gicisky: device})
			}
		}
	}
	if family == "" || family == NRFEPD {
		for _, device := range others {
			if nrfepd.MatchesTarget(target, device.Name, device.Address.String()) {
				matches = append(matches, Found{Family: NRFEPD, NRFEPD: device})
			}
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Found{}, nothingMatched(tags, others, target, family)
	}
	return Found{}, fmt.Errorf("%q matches more than one tag: %s", target, list(matches))
}

// ErrNoneInRange marks the one failure worth looking again for. A tag between
// advertising windows is indistinguishable from a tag that is not there, and
// on hardware here that gap runs to fifteen seconds. Every other failure —
// several tags, the wrong family, a name nothing answers to — describes the
// room, and asking again does not change the room.
var ErrNoneInRange = errors.New("no tags are in range")

// ErrNoDevice is returned when nothing said which tag to write to. It is not a
// question this can answer: `inkwire scan` lists what is there, and the answer
// belongs in the command rather than in a guess.
var ErrNoDevice = errors.New("no device named: say which tag with -device, and `inkwire scan` lists them")

// LocateWithRetry looks again when nothing at all answered, and only then.
//
// It does not go through ble.Retry.Do because the condition is different:
// that one retries whatever failed, and here all but one failure describes the
// room rather than a missed moment. Reporting "several tags are in range"
// three times, fifteen seconds apart, would be three scans to say what the
// first already said.
func LocateWithRetry(ctx context.Context, adapter *bluetooth.Adapter, timeout time.Duration,
	target, asserted string, retry ble.Retry) (Found, error) {
	attempts := retry.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	var last error
	for number := 1; number <= attempts; number++ {
		found, err := Locate(ctx, adapter, timeout, target, asserted)
		if err == nil {
			return found, nil
		}
		if !errors.Is(err, ErrNoneInRange) {
			return Found{}, err
		}
		last = err
		if retry.Logf != nil {
			retry.Logf("locate attempt %d/%d found nothing", number, attempts)
		}
		if number < attempts {
			if err := ble.Wait(ctx, retry.Delay); err != nil {
				return Found{}, err
			}
		}
	}
	return Found{}, last
}

func nothingMatched(tags []gicisky.FoundDevice, others []nrfepd.FoundDevice, target, family string) error {
	// Asked for one family and found the target in the other: that is a
	// mistake with an answer, so say which family it really is.
	if family != "" {
		if elsewhere, ok := Choose(tags, others, target, ""); ok == nil && elsewhere.Family != family {
			return fmt.Errorf("%s is a %s tag, but the family asked for is %s", elsewhere, elsewhere.Family, family)
		}
	}
	seen := sightings(tags, others)
	if len(seen) == 0 {
		return ErrNoneInRange
	}
	return fmt.Errorf("no tag matching %q is in range; these are: %s", target, joinSightings(seen))
}

func sightings(tags []gicisky.FoundDevice, others []nrfepd.FoundDevice) []Sighting {
	seen := make([]Sighting, 0, len(tags)+len(others))
	for _, d := range tags {
		seen = append(seen, Sighting{Family: Gicisky, Name: d.Name, Address: d.Address.String(), RSSI: d.RSSI})
	}
	for _, d := range others {
		seen = append(seen, Sighting{Family: NRFEPD, Name: d.Name, Address: d.Address.String(), RSSI: d.RSSI})
	}
	sort.SliceStable(seen, func(i, j int) bool { return seen[i].RSSI > seen[j].RSSI })
	return seen
}

func joinSightings(seen []Sighting) string {
	parts := make([]string, len(seen))
	for i, s := range seen {
		parts[i] = s.String()
	}
	return strings.Join(parts, ", ")
}

func list(matches []Found) string {
	parts := make([]string, len(matches))
	for i, m := range matches {
		parts[i] = m.String()
	}
	return strings.Join(parts, ", ")
}
