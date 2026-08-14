package nrfepd

import (
	"context"
	"fmt"
	"time"
)

// DefaultResponseTimeout is how long the panel has to say what it is after
// being initialised. The answer comes straight out of the firmware's own flash
// rather than off the panel, so it is quick or it is not coming.
const DefaultResponseTimeout = 5 * time.Second

// DefaultSettle is how long to stay connected after asking for a refresh.
//
// The refresh is the one command that does not finish when it is acknowledged.
// The soft device answers a write itself and hands the request to the firmware
// afterwards, so the acknowledgement says the panel heard, not that it drew:
// the firmware then sits in WaitBusy for as long as the panel takes, which on a
// three colour part is the better part of half a minute.
//
// Leaving at that point is not merely early, it undoes the work. The firmware
// puts the panel to sleep the moment the connection drops, so a disconnect
// during a refresh is a disconnect that cancels it, and what that looks like is
// a page that sent perfectly and never appeared.
const DefaultSettle = 30 * time.Second

// Timings are the two waits a conversation needs: one for the panel to say what
// it is, and one for it to finish drawing.
type Timings struct {
	Response time.Duration
	Settle   time.Duration
}

func (t Timings) response() time.Duration {
	if t.Response <= 0 {
		return DefaultResponseTimeout
	}
	return t.Response
}

func (t Timings) settle() time.Duration {
	if t.Settle < 0 {
		return 0
	}
	if t.Settle == 0 {
		return DefaultSettle
	}
	return t.Settle
}

// fallbackMTU is what one write can carry before the firmware says otherwise.
// Twenty-three bytes is the ATT default and three of them are the header, so a
// firmware too old to negotiate still gets a page, slowly.
const fallbackMTU = 20

// PageFor is asked for the two planes once the panel has said what it is.
//
// The page is requested in this direction, rather than handed over with the
// request, because this family does not advertise. A caller that had to name
// the panel before connecting would be guessing, and a page built for the
// wrong size is not a page that comes out wrong: it is a panel filled with
// bytes that mean something else.
type PageFor func(Model) (black, colour []byte, err error)

// transport is the panel at the other end of a connection: somewhere to write
// frames and somewhere the panel's answers arrive.
type transport interface {
	Write(frame []byte) error
	Notifications() <-chan []byte
}

// Session runs one conversation: initialise, learn what the panel is, ask for
// a page of that shape, and send it.
//
// It takes a transport rather than a radio so that the whole exchange can be
// exercised without one, which matters more here than usual: the radio cannot
// be reached at all from some of the places this is developed.
func Session(ctx context.Context, link transport, page PageFor, timings Timings, logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := link.Write([]byte{cmdInit}); err != nil {
		return fmt.Errorf("initialise: %w", err)
	}

	config, capability, err := listen(ctx, link, timings.response(), logf)
	if err != nil {
		return err
	}
	model, known := config.Model()
	if !known {
		// Guessing the geometry is the one thing this must never do: a wrong
		// size still writes, and what it writes is a ruined page.
		return fmt.Errorf("the panel reports model 0x%02x, which is not in this build's table", config.ModelID)
	}
	logf("panel is %s, link carries %d bytes%s", model, capability.MTU, rleNote(capability.RLE))

	black, colour, err := page(model)
	if err != nil {
		return err
	}
	plan, err := BuildPlan(model, black, colour, capability)
	if err != nil {
		return err
	}
	logf("sending %d frames%s", len(plan.Frames), compressionNote(plan.Compressed))
	for index, frame := range plan.Frames {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := link.Write(frame); err != nil {
			return fmt.Errorf("frame %d of %d: %w", index+1, len(plan.Frames), err)
		}
	}
	return waitForRefresh(ctx, timings.settle(), logf)
}

// waitForRefresh holds the connection open while the panel draws.
//
// There is nothing to wait for: the firmware sends no notification when a
// refresh completes, and the acknowledgement of the refresh command arrived
// before the firmware had even seen it. So this is a wait rather than a
// handshake, and it is a wait for the panel rather than for the link.
func waitForRefresh(ctx context.Context, settle time.Duration, logf func(string, ...any)) error {
	if settle == 0 {
		return nil
	}
	logf("staying connected %s while the panel refreshes; disconnecting now would cancel it", settle)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(settle):
		return nil
	}
}

// settleDelay is how long to keep listening once the panel has said what it
// is, in case it has more to say.
//
// It is needed because the two things worth hearing do not arrive together and
// the useful one comes second. A panel answers with its configuration, then its
// slot count, then a session id, then how much the link will carry. Stopping at
// the configuration reads as working: the page still goes, at twenty bytes a
// write and without compression, which turned a six frame page into sixteen
// hundred.
const settleDelay = 500 * time.Millisecond

// listen collects what the panel volunteers after an init.
//
// The configuration is required, because it is the only statement of what the
// panel is. The link description is not: a firmware that never sends one still
// works, just at the size every BLE link is guaranteed to carry. Anything else
// arriving here is somebody else's feature and is logged rather than refused.
func listen(ctx context.Context, link transport, timeout time.Duration, logf func(string, ...any)) (Config, Link, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// Nil until the panel has identified itself, because there is nothing to
	// wait a little longer for until then.
	var settle <-chan time.Time

	capability := Link{MTU: fallbackMTU}
	var config Config
	var haveConfig, haveLink bool
	for !haveConfig || !haveLink {
		select {
		case <-ctx.Done():
			return Config{}, Link{}, ctx.Err()
		case <-deadline.C:
			return Config{}, Link{}, fmt.Errorf("the panel did not say what it is within %s", timeout)
		case <-settle:
			// It said what it is and has gone quiet, so whatever it was going
			// to say about the link, it is not going to.
			return config, capability, nil
		case message := <-link.Notifications():
			switch {
			case !IsText(message):
				parsed, err := ParseConfig(message)
				if err != nil {
					logf("ignoring a %d byte message that is not a configuration", len(message))
					continue
				}
				config, haveConfig = parsed, true
				settle = time.After(settleDelay)
			default:
				if described, ok := ParseNotification(string(message)); ok {
					capability, haveLink = described, true
					continue
				}
				// slots, session ids and whatever the next firmware adds.
				logf("panel says %q, which this build has no use for", message)
			}
		}
	}
	return config, capability, nil
}

func rleNote(rle bool) string {
	if rle {
		return " and can decompress"
	}
	return ""
}

func compressionNote(compressed bool) string {
	if compressed {
		return " compressed"
	}
	return ""
}
