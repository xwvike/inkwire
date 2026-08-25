package nrfepd

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// panelStub answers the way the tag in hand answers, in the order it answered:
// the configuration first, then the link description, with the messages this
// build has never heard of mixed in among them.
type panelStub struct {
	t             *testing.T
	notifications chan []byte
	written       [][]byte
	// replies are sent the first time anything is written, which is what the
	// panel does: everything it volunteers follows the init.
	replies  [][]byte
	failOn   int
	failWith error
}

func newPanelStub(t *testing.T, replies ...[]byte) *panelStub {
	t.Helper()
	return &panelStub{t: t, notifications: make(chan []byte, 16), replies: replies, failOn: -1}
}

func (p *panelStub) Write(frame []byte) error {
	if len(p.written) == p.failOn {
		return p.failWith
	}
	p.written = append(p.written, append([]byte(nil), frame...))
	if len(p.written) == 1 {
		for _, reply := range p.replies {
			p.notifications <- reply
		}
	}
	return nil
}

func (p *panelStub) Notifications() <-chan []byte { return p.notifications }

func observedReplies(t *testing.T) [][]byte {
	t.Helper()
	config, err := hex.DecodeString(observedConfig)
	if err != nil {
		t.Fatal(err)
	}
	return [][]byte{
		config,
		[]byte("slots=15 1 0"),
		[]byte("sid=D332EAD65D9F83D56B923234BEFB9917177BEF9B32DE60AAB0914EAC60A47FF4"),
		[]byte("mtu=244 rle=1"),
		[]byte("t=1786737722"),
	}
}

// The whole exchange, against the answers a real panel gave.
func TestASessionLearnsThePanelAndSendsAPageBuiltForIt(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)

	var asked Model
	page := func(model Model) ([]byte, []byte, error) {
		asked = model
		return bytes.Repeat([]byte{0xff}, model.PlaneSize()),
			bytes.Repeat([]byte{0xff}, model.PlaneSize()), nil
	}
	if err := Session(context.Background(), panel, page, Timings{Response: time.Second, Settle: 0}, nil); err != nil {
		t.Fatal(err)
	}
	// The page was asked for after the panel said what it was, and asked for at
	// that panel's size rather than at one chosen in advance.
	if asked.Name != "UC8176_420_BWR" || asked.Width != 400 {
		t.Errorf("the page was built for %s", asked)
	}
	if got := panel.written[0]; !bytes.Equal(got, []byte{cmdInit}) {
		t.Errorf("first write = %x, want an init", got)
	}
	if got := panel.written[len(panel.written)-1]; !bytes.Equal(got, []byte{cmdRefresh}) {
		t.Errorf("last write = %x, want a refresh", got)
	}
	// Two blank planes compress to almost nothing, which is the point of
	// negotiating compression at all.
	if len(panel.written) > 12 {
		t.Errorf("a blank page took %d writes, which suggests it went uncompressed", len(panel.written))
	}
}

// A firmware that says nothing about the link still has to work. The size every
// BLE connection is guaranteed to carry is small, so the page goes slowly, and
// slowly is a great deal better than not at all.
func TestAPanelThatNeverDescribesItsLinkStillGetsThePage(t *testing.T) {
	config, err := hex.DecodeString(observedConfig)
	if err != nil {
		t.Fatal(err)
	}
	panel := newPanelStub(t, config)
	page := func(model Model) ([]byte, []byte, error) {
		return bytes.Repeat([]byte{0xff}, model.PlaneSize()),
			bytes.Repeat([]byte{0xff}, model.PlaneSize()), nil
	}
	if err := Session(context.Background(), panel, page, Timings{Response: time.Second, Settle: 0}, nil); err != nil {
		t.Fatal(err)
	}
	for index, frame := range panel.written {
		if len(frame) > fallbackMTU {
			t.Fatalf("write %d is %d bytes, over the %d the link is guaranteed to carry",
				index, len(frame), fallbackMTU)
		}
	}
}

// A panel this build does not know is the case where carrying on would do real
// damage: the size would have to be guessed, and a guess still writes.
func TestAnUnknownPanelIsRefusedRatherThanGuessedAt(t *testing.T) {
	config, err := hex.DecodeString(observedConfig)
	if err != nil {
		t.Fatal(err)
	}
	config[7] = 0x7e
	panel := newPanelStub(t, config)

	asked := false
	page := func(Model) ([]byte, []byte, error) {
		asked = true
		return nil, nil, nil
	}
	err = Session(context.Background(), panel, page, Timings{Response: time.Second, Settle: 0}, nil)
	if err == nil {
		t.Fatal("an unknown panel was driven anyway")
	}
	if !strings.Contains(err.Error(), "0x7e") {
		t.Errorf("the error does not name the model that was refused: %v", err)
	}
	if asked {
		t.Error("a page was built for a panel whose size is unknown")
	}
}

// A panel that never answers must not hang the caller, because the caller is a
// command someone is waiting on.
func TestAPanelThatNeverAnswersTimesOut(t *testing.T) {
	panel := newPanelStub(t)
	err := Session(context.Background(), panel, nil, Timings{Response: 50 * time.Millisecond, Settle: 0}, nil)
	if err == nil {
		t.Fatal("a silent panel was waited on forever")
	}
	if !strings.Contains(err.Error(), "did not say what it is") {
		t.Errorf("error = %v", err)
	}
}

// A write that fails partway has to say where, because "it stopped" and "it
// stopped after the black plane" are different problems on a panel that will
// be showing whatever it had before.
func TestAFailedWriteNamesWhereItStopped(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	// The init is written first and is not part of the plan, so failing on the
	// second write means failing on the plan's second frame.
	panel.failOn, panel.failWith = 2, errWrite
	page := func(model Model) ([]byte, []byte, error) {
		return bytes.Repeat([]byte{0xff}, model.PlaneSize()),
			bytes.Repeat([]byte{0xff}, model.PlaneSize()), nil
	}
	err := Session(context.Background(), panel, page, Timings{Response: time.Second, Settle: 0}, nil)
	if err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if !strings.Contains(err.Error(), "frame 2 of 3") {
		t.Errorf("the error does not say which frame failed: %v", err)
	}
}

var errWrite = writeError("the link dropped")

type writeError string

func (e writeError) Error() string { return string(e) }

// The refresh is acknowledged before the firmware has seen it and takes tens of
// seconds after that, and the firmware sleeps the panel the instant the link
// drops. Leaving early therefore cancels the drawing, and the way that failed
// was the worst way: every write succeeded, the log said so, and the panel
// showed the page it had before.
func TestTheConnectionIsHeldOpenWhileThePanelDraws(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	page := func(model Model) ([]byte, []byte, error) {
		return bytes.Repeat([]byte{0xff}, model.PlaneSize()),
			bytes.Repeat([]byte{0xff}, model.PlaneSize()), nil
	}
	start := time.Now()
	if err := Session(context.Background(), panel, page,
		Timings{Response: time.Second, Settle: 150 * time.Millisecond}, nil); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond {
		t.Errorf("returned after %s, before the panel could have finished drawing", waited)
	}
	// The wait comes after the refresh rather than instead of it.
	if got := panel.written[len(panel.written)-1]; !bytes.Equal(got, []byte{cmdRefresh}) {
		t.Errorf("last write = %x, want a refresh", got)
	}
}

// Waiting must still answer to the caller: someone who gives up on a thirty
// second wait should get their terminal back.
func TestTheRefreshWaitIsInterruptible(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	page := func(model Model) ([]byte, []byte, error) {
		return bytes.Repeat([]byte{0xff}, model.PlaneSize()),
			bytes.Repeat([]byte{0xff}, model.PlaneSize()), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := Session(ctx, panel, page, Timings{Response: time.Second, Settle: time.Minute}, nil); err == nil {
		t.Fatal("an interrupted wait was reported as a completed one")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("the wait ignored the cancellation for %s", waited)
	}
}

// A settle is the number that was asked for, zero included.
//
// Zero used to mean DefaultSettle, on the reasoning that an unset field wants
// the default. That reasoning is right about a struct and wrong about a flag:
// `-settle 0` is somebody stating a number, and thirty seconds is the one
// answer they cannot have meant. The default now lives in NewDriver, which is
// the only thing that should be deciding defaults.
func TestSettleIsTheNumberAsked(t *testing.T) {
	for _, test := range []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{"a stated zero stays zero", 0, 0},
		{"a stated wait is kept", 5 * time.Second, 5 * time.Second},
		{"the default is a number like any other", DefaultSettle, DefaultSettle},
		{"negative cannot be waited for", -time.Second, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := (Timings{Settle: test.set}).settle(); got != test.want {
				t.Fatalf("Timings{Settle: %s}.settle() = %s, want %s", test.set, got, test.want)
			}
		})
	}
}
