package nrfepd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The firmware keeps timestamp + timezone and throws the timezone away, so what
// it ends up holding is local wall clock time. Folding the offset in here means
// the half hour zones land on the right minute instead of thirty minutes out.
func TestTheClockIsSetToLocalTimeWhateverTheOffset(t *testing.T) {
	moment := time.Date(2026, 8, 14, 21, 30, 0, 0, time.UTC)
	tests := []struct {
		zone   string
		offset int
	}{
		{"UTC", 0},
		{"CST", 8 * 3600},
		{"IST", 5*3600 + 30*60},     // 印度，半小时
		{"NPT", 5*3600 + 45*60},     // 尼泊尔，三刻钟
		{"MART", -(9*3600 + 30*60)}, // 马克萨斯，负的半小时
	}
	for _, test := range tests {
		when := moment.In(time.FixedZone(test.zone, test.offset))
		frame := SetTimeFrame(when, ModeClock)

		if frame[0] != cmdSetTime {
			t.Fatalf("%s: frame starts with 0x%02x", test.zone, frame[0])
		}
		if got := frame[5]; got != 0 {
			t.Errorf("%s: timezone byte = %d, want 0 with the offset already folded in", test.zone, got)
		}
		if got := Mode(frame[6]); got != ModeClock {
			t.Errorf("%s: mode = %s", test.zone, got)
		}
		// What the firmware will hold is exactly the local wall clock reading.
		held := int64(binary.BigEndian.Uint32(frame[1:5]))
		wall := time.Unix(held, 0).UTC()
		if got, want := wall.Format("15:04:05"), when.Format("15:04:05"); got != want {
			t.Errorf("%s: the tag would read %s, the wall clock says %s", test.zone, got, want)
		}
	}
}

func TestModeNamesRoundTripAndRubbishIsRefused(t *testing.T) {
	for _, name := range []string{"picture", "calendar", "clock"} {
		mode, err := ParseMode(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if mode.String() != name {
			t.Errorf("%s parsed to %s", name, mode)
		}
	}
	if _, err := ParseMode("Clock"); err != nil {
		t.Errorf("a capitalised name was refused: %v", err)
	}
	if _, err := ParseMode("watch"); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

// Handing the tag back to its own clock is the whole point, so the exchange is
// checked end to end against the answers a real panel gives.
func TestHandingTheTagBackToItsOwnClock(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	monday := time.Monday
	when := time.Date(2026, 8, 14, 21, 30, 0, 0, time.FixedZone("CST", 8*3600))

	if err := ModeSession(context.Background(), panel, when, ModeCalendar, &monday,
		Timings{Response: time.Second, Settle: 0}, nil); err != nil {
		t.Fatal(err)
	}
	if len(panel.written) != 3 {
		t.Fatalf("wrote %d frames, want an init, a week start and a set time", len(panel.written))
	}
	if !bytes.Equal(panel.written[0], []byte{cmdInit}) {
		t.Errorf("first write = %x, want an init", panel.written[0])
	}
	if !bytes.Equal(panel.written[1], []byte{cmdSetWeekStart, 1}) {
		t.Errorf("second write = %x, want Monday as the first day", panel.written[1])
	}
	if got := panel.written[2]; got[0] != cmdSetTime || Mode(got[6]) != ModeCalendar {
		t.Errorf("third write = %x, want a set time carrying the calendar mode", got)
	}
}

// Nothing is sent about the first day of the week unless it was asked for.
// Writing a default would quietly change a setting the tag already holds.
func TestTheFirstDayOfTheWeekIsLeftAloneUnlessGiven(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	if err := ModeSession(context.Background(), panel, time.Now(), ModeClock, nil,
		Timings{Response: time.Second, Settle: 0}, nil); err != nil {
		t.Fatal(err)
	}
	for _, frame := range panel.written {
		if frame[0] == cmdSetWeekStart {
			t.Error("the first day of the week was set without being asked for")
		}
	}
}

// Setting the time redraws the panel whatever the mode, so this has the same
// hazard a page does: disconnecting during the redraw cancels it.
func TestSettingTheModeWaitsForTheRedraw(t *testing.T) {
	panel := newPanelStub(t, observedReplies(t)...)
	start := time.Now()
	if err := ModeSession(context.Background(), panel, time.Now(), ModeClock, nil,
		Timings{Response: time.Second, Settle: 150 * time.Millisecond}, nil); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond {
		t.Errorf("returned after %s, before the panel could have finished redrawing", waited)
	}
}

// An unfamiliar panel is refused for a page, because a page has to be built at
// its size. This screen is drawn by the firmware, which knows its own geometry,
// so refusing here would withhold a thing that would have worked.
func TestAnUnknownPanelStillGetsItsClockSet(t *testing.T) {
	config := append([]byte(nil), observedReplies(t)[0]...)
	config[7] = 0x7e
	panel := newPanelStub(t, config)

	var logged strings.Builder
	err := ModeSession(context.Background(), panel, time.Now(), ModeClock, nil,
		Timings{Response: time.Second, Settle: 0},
		func(format string, args ...any) { fmt.Fprintf(&logged, format+"\n", args...) })
	if err != nil {
		t.Fatalf("an unfamiliar panel was refused its own clock: %v", err)
	}
	if !strings.Contains(logged.String(), "0x7e") {
		t.Errorf("the log does not mention the unfamiliar model: %s", logged.String())
	}
}
